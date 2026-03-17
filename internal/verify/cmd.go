// ABOUTME: Cobra command definition and main logic for the git-zhi-verify binary.
// ABOUTME: Loads a milestone, runs AC commands for done issues, and reports pass/fail results.
package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// JSONResultEntry is a single command execution result for JSON output.
type JSONResultEntry struct {
	IssueID    string `json:"issue_id"`
	IssueTitle string `json:"issue_title"`
	Subsection string `json:"subsection"`
	Command    string `json:"command"`
	Passed     bool   `json:"passed"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
}

// JSONReport is the top-level JSON output structure for --format json.
type JSONReport struct {
	Milestone string            `json:"milestone"`
	Total     int               `json:"total"`
	Passed    int               `json:"passed"`
	Failed    int               `json:"failed"`
	Results   []JSONResultEntry `json:"results"`
}

// NewVerifyCommand creates and returns the top-level verify Cobra command.
// It requires a milestone name as the sole positional argument.
func NewVerifyCommand() *cobra.Command {
	var failFast bool
	var dryRun bool
	var format string
	var timeout int

	cmd := &cobra.Command{
		Use:   "git-zhi-verify <milestone>",
		Short: "Run acceptance-criteria commands for all done issues in a milestone",
		Long: `git-zhi-verify loads the named milestone, finds all done issues,
extracts backtick-delimited commands from their Acceptance Criteria sections,
and executes each command in priority order (recently-changed paths first).

Exit code 0 means all commands passed. Exit code 1 means at least one regression.`,
		Args: cobra.ExactArgs(1),
		// SilenceUsage prevents Cobra from printing usage on every error.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			milestoneName := args[0]

			// Resolve per-command timeout.
			cmdTimeout := time.Duration(timeout) * time.Second

			// Retrieve the repo App from context (injected by main or test harness).
			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			// Load milestone to validate it exists.
			_, err := milestone.LoadMilestone(app.Store, milestoneName)
			if err != nil {
				return fmt.Errorf("load milestone %q: %w", milestoneName, err)
			}

			// Load all issues and filter to done issues in this milestone.
			allIssues, err := issue.LoadAllIssues(app.Store)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			var doneIssues []*issue.Issue
			for _, iss := range allIssues {
				if iss.Milestone == milestoneName && iss.State == issue.StateDone {
					doneIssues = append(doneIssues, iss)
				}
			}

			// Compute recent changes: union of all files changed since the earliest
			// session's start SHA across done issues.
			recentChanges := computeRecentChanges(app, doneIssues)

			// Build a simple topological order by issue UUID for tier-2 ordering.
			// Since done issues are not in the active graph, we use UUID sort as
			// a stable proxy for creation order.
			topoOrder := topoOrderByUUID(doneIssues)

			// Prioritize issues: tier-1 (path overlap), tier-2 (lineage, nil = not
			// yet integrated), tier-3 (topo order). Phase 5 integration will pass a
			// computed lineage set here; for now nil disables tier-2.
			prioritized := PrioritizeIssues(doneIssues, recentChanges, topoOrder, nil)

			// Execute each issue's commands in priority order, collecting results.
			var jsonResults []JSONResultEntry
			totalCount := 0
			passedCount := 0
			failedCount := 0
			earlyStop := false

			for _, iss := range prioritized {
				sections := issue.ParseSections(iss.Body)
				cmds := ExtractCommands(sections, iss.ID, iss.Title)
				if len(cmds) == 0 {
					continue
				}

				// Group commands by subsection for output.
				var positives, negatives []Command
				for _, c := range cmds {
					if c.Subsection == "negative" {
						negatives = append(negatives, c)
					} else {
						positives = append(positives, c)
					}
				}

				if !dryRun {
					// Print issue header in human format.
					if format != "json" {
						shortID := iss.ID.String()
						if len(shortID) >= 8 {
							shortID = shortID[:8]
						}
						fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", shortID, iss.Title)
					}
				}

				// Execute positive commands.
				if len(positives) > 0 && !dryRun && format != "json" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Positive:\n")
				}
				for _, c := range positives {
					totalCount++
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s  %s  %s\n", iss.Title, c.Subsection, c.Text)
						continue
					}

					if format != "json" {
						fmt.Fprintf(cmd.OutOrStdout(), "    ")
					}

					repoRoot := repoRootFromApp(app)
					result := Execute(c.Text, repoRoot, cmdTimeout)
					passed := result.ExitCode == 0 && !result.TimedOut

					if format != "json" {
						if passed {
							fmt.Fprintf(cmd.OutOrStdout(), "✓ %s\n", c.Text)
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), "✗ %s          ← REGRESSION\n", c.Text)
						}
					}

					if passed {
						passedCount++
					} else {
						failedCount++
					}

					jsonResults = append(jsonResults, JSONResultEntry{
						IssueID:    iss.ID.String(),
						IssueTitle: iss.Title,
						Subsection: c.Subsection,
						Command:    c.Text,
						Passed:     passed,
						ExitCode:   result.ExitCode,
						Stdout:     result.Stdout,
						Stderr:     result.Stderr,
					})

					if failFast && !passed {
						earlyStop = true
						break
					}
				}
				if earlyStop {
					break
				}

				// Execute negative commands.
				if len(negatives) > 0 && !dryRun && format != "json" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Negative:\n")
				}
				for _, c := range negatives {
					totalCount++
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s  %s  %s\n", iss.Title, c.Subsection, c.Text)
						continue
					}

					if format != "json" {
						fmt.Fprintf(cmd.OutOrStdout(), "    ")
					}

					repoRoot := repoRootFromApp(app)
					result := Execute(c.Text, repoRoot, cmdTimeout)
					passed := result.ExitCode == 0 && !result.TimedOut

					if format != "json" {
						if passed {
							fmt.Fprintf(cmd.OutOrStdout(), "✓ %s\n", c.Text)
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), "✗ %s          ← REGRESSION\n", c.Text)
						}
					}

					if passed {
						passedCount++
					} else {
						failedCount++
					}

					jsonResults = append(jsonResults, JSONResultEntry{
						IssueID:    iss.ID.String(),
						IssueTitle: iss.Title,
						Subsection: c.Subsection,
						Command:    c.Text,
						Passed:     passed,
						ExitCode:   result.ExitCode,
						Stdout:     result.Stdout,
						Stderr:     result.Stderr,
					})

					if failFast && !passed {
						earlyStop = true
						break
					}
				}
				if earlyStop {
					break
				}
			}

			// If dry-run, nothing to summarize.
			if dryRun {
				return nil
			}

			if format == "json" {
				if jsonResults == nil {
					jsonResults = []JSONResultEntry{}
				}
				report := JSONReport{
					Milestone: milestoneName,
					Total:     totalCount,
					Passed:    passedCount,
					Failed:    failedCount,
					Results:   jsonResults,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}
			} else {
				// Human-readable summary.
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s: %d/%d acceptance criteria passing\n",
					milestoneName, passedCount, totalCount)
				if failedCount > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "      %d regression(s) detected\n", failedCount)
				}
			}

			if failedCount > 0 {
				return fmt.Errorf("%d regression(s) detected in milestone %s", failedCount, milestoneName)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "stop on first failing command")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list commands without executing them")
	cmd.Flags().StringVar(&format, "format", "", "output format (json)")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "per-command timeout in seconds")

	return cmd
}

// computeRecentChanges returns the union of files changed since the earliest
// done issue's session start SHA. Issues without sessions are skipped for
// this computation. Returns empty slice if nothing can be determined.
func computeRecentChanges(app *cli.App, doneIssues []*issue.Issue) []string {
	if len(doneIssues) == 0 {
		return nil
	}

	// Find the earliest session start SHA across all done issues.
	var earliestSHA string
	for _, iss := range doneIssues {
		for _, sess := range iss.Sessions {
			if sess.StartSHA != "" {
				if earliestSHA == "" {
					earliestSHA = sess.StartSHA
				}
				// We take the first one found; true earliest would require
				// comparing commit timestamps, but UUID sort already provides
				// a reasonable proxy for creation order.
			}
		}
	}

	if earliestSHA == "" {
		return nil
	}

	headSHA, err := app.Store.RepoHEAD()
	if err != nil {
		// HEAD unavailable (e.g. empty repo) — skip path computation.
		return nil
	}

	paths, err := app.Store.DiffNameOnly(earliestSHA, headSHA)
	if err != nil {
		return nil
	}
	return paths
}

// topoOrderByUUID returns UUIDs sorted by UUID string (creation-time proxy)
// for done issues. This provides a stable tier-2 ordering when no topological
// data from the active graph is available.
func topoOrderByUUID(issues []*issue.Issue) []uuid.UUID {
	sorted := make([]*issue.Issue, len(issues))
	copy(sorted, issues)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID.String() < sorted[j].ID.String()
	})
	ids := make([]uuid.UUID, len(sorted))
	for i, iss := range sorted {
		ids[i] = iss.ID
	}
	return ids
}

// repoRootFromApp attempts to extract the working directory of the repo from
// the App. Falls back to "." if the repo's worktree path cannot be resolved.
func repoRootFromApp(app *cli.App) string {
	if app.Repo != nil {
		wt, err := app.Repo.Worktree()
		if err == nil {
			return wt.Filesystem.Root()
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

