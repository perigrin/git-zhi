// ABOUTME: Interactive triage subcommand for the historian. Presents commit clusters
// ABOUTME: one at a time and reads single-character responses to assign, skip, merge, or quit.
package historian

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/historian/enrich"
	"github.com/perigrin/git-zhi/internal/historian/extract"
	"github.com/perigrin/git-zhi/internal/issue"
)

// newTriageCommand returns the "triage" subcommand that presents clusters
// interactively for human review.
func newTriageCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "triage",
		Short: "Interactively review commit clusters and decide which become issues",
		Long: `Triage walks each commit cluster and prompts for a decision:

  [a]ssign  — write this cluster as an issue
  [s]kip    — skip this cluster (do not create an issue)
  [m]erge   — merge this cluster with the previous one
  [q]uit    — stop processing remaining clusters

Merged clusters are written as a single combined issue when the next
non-merge action is taken or when all clusters have been processed.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			// Extract commits.
			commits, err := extract.ExtractCommits(app.Repo, nil)
			if err != nil {
				return fmt.Errorf("extract commits: %w", err)
			}
			if len(commits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "triage: no commits to process")
				return nil
			}

			// Load clustering config.
			cfg, err := LoadConfig(app.Store)
			if err != nil {
				return fmt.Errorf("load historian config: %w", err)
			}

			// Cluster commits.
			clusters := cluster.ClusterCommits(commits, cfg)
			if len(clusters) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "triage: no clusters formed")
				return nil
			}

			// Convert to pointers for merge operations.
			clusterPtrs := make([]*cluster.Cluster, len(clusters))
			for i := range clusters {
				clusterPtrs[i] = &clusters[i]
			}

			// Run interactive triage loop.
			return runTriage(cmd, app, clusterPtrs)
		},
	}
}

// runTriage presents each cluster to the user and processes their response.
// Clusters marked for assignment are held pending until a non-merge action
// flushes them, allowing subsequent merge responses to combine clusters before
// they are written.
func runTriage(cmd *cobra.Command, app *cli.App, clusters []*cluster.Cluster) error {
	out := cmd.OutOrStdout()
	scanner := bufio.NewScanner(cmd.InOrStdin())

	var pending *cluster.Cluster // accumulates assign + merge
	created := 0

	// flushPending writes the accumulated pending cluster as an issue.
	flushPending := func() error {
		if pending == nil {
			return nil
		}
		if err := writeTriageIssue(app, pending); err != nil {
			return err
		}
		created++
		pending = nil
		return nil
	}

	for i, c := range clusters {
		displayCluster(out, c, i+1, len(clusters))

		fmt.Fprintf(out, "\n  [a]ssign [s]kip [m]erge [q]uit > ")

		if !scanner.Scan() {
			break
		}
		response := strings.TrimSpace(strings.ToLower(scanner.Text()))

		switch response {
		case "a":
			// Flush any prior pending cluster, then hold this one pending.
			if err := flushPending(); err != nil {
				return err
			}
			pending = c
			fmt.Fprintf(out, "  -> assigned (%d commits)\n", len(c.Commits))

		case "s":
			// Flush any pending cluster, then skip this one.
			if err := flushPending(); err != nil {
				return err
			}
			fmt.Fprintf(out, "  -> skipped\n")

		case "m":
			// Merge this cluster into the pending one.
			if pending != nil {
				pending = mergeClusters(pending, c)
			} else {
				pending = c
			}
			fmt.Fprintf(out, "  -> merged (pending, %d commits total)\n", len(pending.Commits))

		case "q":
			if err := flushPending(); err != nil {
				return err
			}
			fmt.Fprintf(out, "triage: quit after %d of %d clusters, created %d issue(s)\n",
				i+1, len(clusters), created)
			return nil

		default:
			fmt.Fprintf(out, "  unknown response %q, try a/s/m/q\n", response)
		}
	}

	// Flush any remaining pending cluster.
	if err := flushPending(); err != nil {
		return err
	}

	fmt.Fprintf(out, "triage: reviewed %d cluster(s), created %d issue(s)\n",
		len(clusters), created)
	return nil
}

// displayCluster prints a summary of a cluster for triage review.
func displayCluster(w io.Writer, c *cluster.Cluster, idx, total int) {
	fmt.Fprintf(w, "\n--- Cluster %d/%d ", idx, total)
	if c.TicketRef != "" {
		fmt.Fprintf(w, "[%s] ", c.TicketRef)
	}
	fmt.Fprintf(w, "(%d commits) ---\n", len(c.Commits))

	for _, commit := range c.Commits {
		msg := strings.TrimSpace(commit.Message)
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx]
		}
		if len(msg) > 72 {
			msg = msg[:72]
		}
		fmt.Fprintf(w, "  %s %s <%s> %s\n",
			commit.SHA[:7], commit.Author, commit.Email, msg)
	}
}

// mergeClusters combines two clusters into one by appending commits.
func mergeClusters(a, b *cluster.Cluster) *cluster.Cluster {
	merged := &cluster.Cluster{
		ID:        a.ID + "+" + b.ID,
		Commits:   append(a.Commits, b.Commits...),
		TicketRef: a.TicketRef,
	}
	if merged.TicketRef == "" {
		merged.TicketRef = b.TicketRef
	}
	return merged
}

// writeTriageIssue enriches a cluster into an issue and writes it to storage.
func writeTriageIssue(app *cli.App, c *cluster.Cluster) error {
	iss := enrich.ClusterToIssue(c)
	content, err := issue.Marshal(iss)
	if err != nil {
		return fmt.Errorf("marshal issue %s: %w", iss.ID, err)
	}
	refPath := issue.RefPrefix + iss.ID.String()
	commitMsg := "historian triage: create " + iss.Title
	if err := app.Store.WriteEntity(refPath, "issue.md", content, commitMsg); err != nil {
		return fmt.Errorf("write issue %s: %w", iss.ID, err)
	}
	return nil
}
