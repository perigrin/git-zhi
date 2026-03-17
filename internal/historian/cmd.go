// ABOUTME: Cobra command definitions for the git-zhi-historian binary.
// ABOUTME: Wires the extract→cluster→enrich pipeline with progress tracking, dry-run, and subcommands.
package historian

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/historian/enrich"
	"github.com/perigrin/git-zhi/internal/historian/extract"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/storage"
)

// lastSHARef is the ref path where the historian stores the last processed HEAD SHA.
const lastSHARef = "refs/zhi/_/historian/last_processed_sha"

// NewHistorianCommand creates and returns the top-level historian Cobra command
// with all subcommands (status, reindex) registered.
func NewHistorianCommand() *cobra.Command {
	var titleMatch string
	var label string
	var since string
	var dryRun bool
	var incremental bool

	cmd := &cobra.Command{
		Use:   "git-zhi-historian",
		Short: "Reconstruct issue history from git commit log",
		Long: `git-zhi-historian walks the git log, clusters related commits into
issue candidates, enriches them with confidence scores and actor identity,
and writes them as retrospective done issues into refs/zhi/_/issues/.

Use --dry-run to preview without writing.
Use --incremental to process only commits since the last run.
Use --label to assign a label to all created issues and build the index.
Use --title-match to filter commits by ticket ref pattern before clustering.
Use --since to restrict extraction to commits after a given date.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			// Resolve since: either --since flag, --incremental SHA, or nil.
			var sinceTime *time.Time

			if incremental {
				sha, err := readLastProcessedSHA(app.Store)
				if err != nil {
					return fmt.Errorf("read last_processed_sha: %w", err)
				}
				if sha != "" {
					// Verify the SHA is an ancestor of HEAD before trusting it.
					if err := verifyAncestor(app, sha); err != nil {
						return fmt.Errorf("history rewrite detected: %w", err)
					}
					// Resolve the SHA to a commit timestamp to use as the since cutoff.
					ts, err := commitTimestamp(app, sha)
					if err != nil {
						return fmt.Errorf("resolve last_processed_sha timestamp: %w", err)
					}
					// Add 1 nanosecond so the boundary commit itself is excluded.
					next := ts.Add(time.Nanosecond)
					sinceTime = &next
				}
			} else if since != "" {
				t, err := time.Parse("2006-01-02", since)
				if err != nil {
					return fmt.Errorf("parse --since date %q: %w (expected YYYY-MM-DD)", since, err)
				}
				sinceTime = &t
			}

			// Extract commits from git log.
			commits, err := extract.ExtractCommits(app.Repo, sinceTime)
			if err != nil {
				return fmt.Errorf("extract commits: %w", err)
			}

			// Apply --title-match filter.
			if titleMatch != "" {
				commits = filterByTicketPattern(commits, titleMatch)
			}

			if len(commits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "historian: no commits to process")
				return nil
			}

			// Cluster commits into issue candidates.
			clusters := cluster.ClusterCommits(commits, cluster.DefaultConfig())

			// Enrich clusters into Issue structs.
			issues := enrich.EnrichClusters(clusters)

			// Assign label if provided.
			if label != "" {
				for _, iss := range issues {
					iss.Labels = appendIfMissing(iss.Labels, label)
				}
			}

			if dryRun {
				return printDryRun(cmd, issues)
			}

			// Write issues to refs.
			for _, iss := range issues {
				content, err := issue.Marshal(iss)
				if err != nil {
					return fmt.Errorf("marshal issue %s: %w", iss.ID, err)
				}
				refPath := issue.RefPrefix + iss.ID.String()
				commitMsg := "historian: create " + iss.Title
				if err := app.Store.WriteEntity(refPath, "issue.md", content, commitMsg); err != nil {
					return fmt.Errorf("write issue %s: %w", iss.ID, err)
				}
			}

			// Build label indexes when --label is set.
			if label != "" {
				allIssues, err := issue.LoadAllIssues(app.Store)
				if err != nil {
					return fmt.Errorf("load all issues for reindex: %w", err)
				}
				if err := issue.BuildLabelIndexes(app.Store, allIssues); err != nil {
					return fmt.Errorf("build label indexes: %w", err)
				}
			}

			// Record the current HEAD SHA as last_processed_sha.
			headSHA, err := app.Store.RepoHEAD()
			if err != nil {
				return fmt.Errorf("get HEAD SHA: %w", err)
			}
			if err := writeLastProcessedSHA(app.Store, headSHA); err != nil {
				return fmt.Errorf("write last_processed_sha: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "historian: created %d issue(s) from %d commit(s)\n",
				len(issues), len(commits))
			return nil
		},
	}

	cmd.Flags().StringVar(&titleMatch, "title-match", "", "filter commits by ticket ref pattern (e.g. LOPS-*)")
	cmd.Flags().StringVar(&label, "label", "", "assign this label to all created issues and build the index")
	cmd.Flags().StringVar(&since, "since", "", "only extract commits after this date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be created without writing to refs")
	cmd.Flags().BoolVar(&incremental, "incremental", false, "only process commits since the last run")

	// Register subcommands.
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newReindexCommand())

	return cmd
}

// newStatusCommand returns the "status" subcommand that prints a coverage report.
func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "Show historian coverage: mapped vs unmapped commits",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			// Extract all commits to count total.
			allCommits, err := extract.ExtractCommits(app.Repo, nil)
			if err != nil {
				return fmt.Errorf("extract commits: %w", err)
			}

			// Load all issues to count mapped commits.
			issues, err := issue.LoadAllIssues(app.Store)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			// Count commits that appear in sessions.
			mappedCommits := 0
			for _, iss := range issues {
				for _, sess := range iss.Sessions {
					mappedCommits += sess.Commits
				}
			}

			total := len(allCommits)
			unmapped := total - mappedCommits
			if unmapped < 0 {
				unmapped = 0
			}

			// Count low-coherence issues (confidence < 0.55 = single-commit or ticket-ref only).
			lowCoherence := 0
			for _, iss := range issues {
				if iss.Confidence < 0.55 {
					lowCoherence++
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "historian status\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  total commits:     %d\n", total)
			fmt.Fprintf(cmd.OutOrStdout(), "  mapped commits:    %d\n", mappedCommits)
			fmt.Fprintf(cmd.OutOrStdout(), "  unmapped commits:  %d\n", unmapped)
			fmt.Fprintf(cmd.OutOrStdout(), "  issues created:    %d\n", len(issues))
			fmt.Fprintf(cmd.OutOrStdout(), "  low-coherence:     %d\n", lowCoherence)

			return nil
		},
	}
}

// newReindexCommand returns the "reindex" subcommand that rebuilds label indexes.
func newReindexCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "reindex",
		Short:         "Rebuild label indexes for all existing issues",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			issues, err := issue.LoadAllIssues(app.Store)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			if err := issue.BuildLabelIndexes(app.Store, issues); err != nil {
				return fmt.Errorf("build label indexes: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "historian reindex: rebuilt label indexes for %d issue(s)\n", len(issues))
			return nil
		},
	}
}

// readLastProcessedSHA reads the last recorded HEAD SHA from the historian ref.
// Returns empty string if the ref does not exist (first run).
func readLastProcessedSHA(store *storage.Store) (string, error) {
	if !store.RefExists(lastSHARef) {
		return "", nil
	}
	data, err := store.ReadEntity(lastSHARef, "sha")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// writeLastProcessedSHA writes the given SHA to the historian progress ref.
func writeLastProcessedSHA(store *storage.Store, sha string) error {
	return store.WriteEntity(lastSHARef, "sha", []byte(sha), "historian: record last processed SHA")
}

// verifyAncestor checks that sha is an ancestor of HEAD in app's repository.
// Returns an error if the SHA cannot be found in HEAD's ancestry (history rewrite).
func verifyAncestor(app *cli.App, sha string) error {
	headSHA, err := app.Store.RepoHEAD()
	if err != nil {
		return fmt.Errorf("get HEAD: %w", err)
	}
	if headSHA == sha {
		return nil
	}
	// Use CountCommits to walk from sha to HEAD. If sha is not in HEAD's
	// ancestry, CountCommits returns an error.
	_, err = app.Store.CountCommits(sha, headSHA)
	return err
}

// commitTimestamp resolves a commit SHA to its author timestamp using go-git.
func commitTimestamp(app *cli.App, sha string) (time.Time, error) {
	repo := app.Repo
	rev := plumbing.Revision(sha + "^{commit}")
	hash, err := repo.ResolveRevision(rev)
	if err != nil {
		return time.Time{}, fmt.Errorf("resolve commit %s: %w", sha, err)
	}
	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return time.Time{}, fmt.Errorf("get commit object %s: %w", sha, err)
	}
	return commit.Author.When, nil
}

// filterByTicketPattern filters commits to only those whose TicketRefs contain
// at least one ref matching the glob pattern (e.g. "LOPS-*").
func filterByTicketPattern(commits []extract.CommitData, pattern string) []extract.CommitData {
	var result []extract.CommitData
	for _, c := range commits {
		for _, ref := range c.TicketRefs {
			matched, err := filepath.Match(pattern, ref)
			if err == nil && matched {
				result = append(result, c)
				break
			}
		}
	}
	return result
}

// appendIfMissing adds label to labels only if it's not already present.
func appendIfMissing(labels []string, label string) []string {
	for _, l := range labels {
		if l == label {
			return labels
		}
	}
	return append(labels, label)
}

// printDryRun prints a preview of what would be created to cmd's output.
func printDryRun(cmd *cobra.Command, issues []*issue.Issue) error {
	fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] would create %d issue(s):\n", len(issues))
	for _, iss := range issues {
		shortID := iss.ID.String()
		if len(shortID) >= 8 {
			shortID = shortID[:8]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  (confidence=%.2f source=%s)\n",
			shortID, iss.Title, iss.Confidence, iss.Source)
	}
	return nil
}
