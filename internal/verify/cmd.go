// ABOUTME: Cobra command definition and main logic for the verify subcommand.
// ABOUTME: Loads a milestone, runs AC commands for done issues, and reports pass/fail results.
package verify

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/storage"
)

// JSONResultEntry is a single command execution result for JSON output.
type JSONResultEntry struct {
	IssueID    string `json:"issue_id"`
	IssueTitle string `json:"issue_title"`
	Subsection string `json:"subsection"`
	Command    string `json:"command"`
	Passed     bool   `json:"passed"`
	NoTestsRan bool   `json:"no_tests_ran,omitempty"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
}

// JSONUnverifiableEntry is a single AC item that carried backtick text but
// produced no parenthesized verification command.
type JSONUnverifiableEntry struct {
	IssueID    string `json:"issue_id"`
	IssueTitle string `json:"issue_title"`
	Subsection string `json:"subsection"`
	Item       string `json:"item"`
}

// JSONReport is the top-level JSON output structure for --format json.
type JSONReport struct {
	Milestone         string                  `json:"milestone"`
	Total             int                     `json:"total"`
	Passed            int                     `json:"passed"`
	NoTestsRan        int                     `json:"no_tests_ran"`
	Failed            int                     `json:"failed"`
	Unverifiable      int                     `json:"unverifiable"`
	Results           []JSONResultEntry       `json:"results"`
	UnverifiableItems []JSONUnverifiableEntry `json:"unverifiable_items"`
}

// classify decides how a completed criterion counts. A run that exited clean
// without executing any test verified nothing, so it is neither a pass nor a
// regression — the distinction matters when reading a red gate, and stating
// the rule once keeps the positive and negative loops from drifting apart.
func classify(r Result) (passed, vacuous bool) {
	return r.ExitCode == 0 && !r.TimedOut && !r.NoTestsRan, r.NoTestsRan
}

// milestoneHint explains a failed milestone lookup by naming what is actually
// available, and calls out HEAD specifically because it resolves elsewhere in
// the CLI and reads as though it should work here too.
func milestoneHint(store *storage.Store, name string) string {
	var hint string
	if name == "HEAD" {
		hint = "HEAD resolves an issue, not a milestone; pass a milestone name. "
	}

	all, err := milestone.LoadAllMilestones(store)
	if err != nil {
		return hint + fmt.Sprintf("could not list milestones: %v", err)
	}
	if len(all) == 0 {
		return hint + "no milestones exist in this chain"
	}
	names := make([]string, len(all))
	for i, ms := range all {
		names[i] = ms.Name
	}
	sort.Strings(names)
	return hint + "known milestones: " + strings.Join(names, ", ")
}

// NewVerifyCommand creates and returns the top-level verify Cobra command.
// It requires a milestone name as the sole positional argument.
func NewVerifyCommand() *cobra.Command {
	var failFast bool
	var dryRun bool
	var format string
	var timeout int

	cmd := &cobra.Command{
		Use:   "verify <milestone>",
		Short: "Run acceptance-criteria commands for all done issues in a milestone",
		Long: `verify loads the named milestone, finds all done issues,
extracts backtick-delimited commands from their Acceptance Criteria sections,
and executes each command in priority order (recently-changed paths first).
Criteria in the milestone's own body are extracted and run alongside them.

--dry-run lists rather than executes, and reads every issue in the milestone
regardless of state, including pending ones. That is deliberate: a review that
runs before execution starts sees a chain where nothing is done yet, and a dry
run limited to done issues would find nothing to report.

Exit code 0 means every acceptance criterion was verified and passed. Exit code
1 means at least one regression, unverifiable criterion, criterion that ran no
tests, or no acceptance criteria extracted at all.`,
		Args: cobra.ExactArgs(1),
		// SilenceUsage prevents Cobra from printing usage on every error.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			milestoneName := args[0]

			// Guard the timeout here rather than at each caller: verify is
			// invoked directly and by the milestone completion gate, and both
			// need the same bounds. Above ~9.2e9 seconds the multiplication
			// overflows int64 and wraps, which makes AfterFunc fire at once and
			// reports every criterion as a regression.
			const maxTimeoutSeconds = int(math.MaxInt64 / int64(time.Second))
			if timeout <= 0 || timeout > maxTimeoutSeconds {
				return fmt.Errorf("invalid --timeout %d: must be between 1 and %d seconds", timeout, maxTimeoutSeconds)
			}
			cmdTimeout := time.Duration(timeout) * time.Second

			// Retrieve the repo App from context (injected by main or test harness).
			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			// Load milestone to validate it exists. verify takes a milestone
			// name, not the issue refs (HEAD, a UUID prefix, a title) that most
			// other commands accept, so a wrong argument here needs to say what
			// kind of name it wanted.
			ms, err := milestone.LoadMilestone(app.Store, milestoneName)
			if err != nil {
				// Keep the underlying error: LoadMilestone also fails on a
				// milestone that exists but will not parse, and reporting that
				// as "no milestone" sends the user looking for something they
				// never deleted.
				return fmt.Errorf("%w (%s)", err, milestoneHint(app.Store, milestoneName))
			}

			// Load all issues and filter to this milestone. A real run only
			// executes done issues — running a pending issue's criteria would
			// report failures for work not yet done. --dry-run lists rather
			// than executes, so it selects by milestone alone (minus
			// cancelled issues, which are not pending verification): a
			// pre-execution review runs while every issue is still pending,
			// and a dry-run limited to done issues would extract nothing.
			allIssues, err := issue.LoadAllIssues(app.Store)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			var doneIssues []*issue.Issue
			for _, iss := range allIssues {
				if iss.Milestone != milestoneName {
					continue
				}
				if dryRun {
					if iss.State != issue.StateCancelled {
						doneIssues = append(doneIssues, iss)
					}
					continue
				}
				if iss.State == issue.StateDone {
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
			var jsonUnverifiable []JSONUnverifiableEntry
			totalCount := 0
			passedCount := 0
			failedCount := 0
			vacuousCount := 0
			unverifiableCount := 0
			earlyStop := false

			// runBodyGroup executes one subsection (positive or negative) of
			// milestone-body commands. It closes over the shared counters
			// and jsonResults above so a milestone-body criterion folds into
			// the same tally an issue's would, including the zero-extraction
			// check below: a milestone with no done issues but real body
			// criteria must count as non-empty.
			runBodyGroup := func(cmds []Command, label string) (stop bool) {
				if len(cmds) == 0 {
					return false
				}
				if !dryRun && format != "json" {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s:\n", label)
				}
				for _, c := range cmds {
					totalCount++
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] milestone body  %s  %s\n", c.Subsection, c.Text)
						continue
					}
					if format != "json" {
						fmt.Fprintf(cmd.OutOrStdout(), "    %s ", c.Text)
					}
					repoRoot := repoRootFromApp(app)
					result := Execute(c.Text, repoRoot, cmdTimeout)
					passed, vacuous := classify(result)
					if format != "json" {
						switch {
						case passed:
							fmt.Fprintln(cmd.OutOrStdout(), "✓")
						case vacuous:
							fmt.Fprintln(cmd.OutOrStdout(), "⚠  ← NO TESTS RAN")
						default:
							fmt.Fprintln(cmd.OutOrStdout(), "✗  ← REGRESSION")
						}
					}
					switch {
					case passed:
						passedCount++
					case vacuous:
						vacuousCount++
					default:
						failedCount++
					}
					jsonResults = append(jsonResults, JSONResultEntry{
						IssueTitle: milestoneName,
						Subsection: c.Subsection,
						Command:    c.Text,
						Passed:     passed,
						NoTestsRan: result.NoTestsRan,
						ExitCode:   result.ExitCode,
						Stdout:     result.Stdout,
						Stderr:     result.Stderr,
					})
					if failFast && !passed {
						return true
					}
				}
				return false
			}

			// Milestone-body acceptance criteria are extracted with the same
			// parser used for an issue's body, just pointed at the milestone's
			// own Body instead. A body criterion has no issue to name, so it
			// is reported in its own section above the issue rows rather than
			// as a pseudo-titled issue row. The header is conditional on the
			// body actually yielding criteria: every fixture in this package
			// builds a milestone with an empty body, and an unconditional
			// header would break all of them at once.
			bodySections := issue.ParseSections(ms.Body)
			bodyCmds, bodyDropped := ExtractCommands(bodySections, uuid.Nil, milestoneName)
			if len(bodyCmds) > 0 || len(bodyDropped) > 0 {
				var bodyPositives, bodyNegatives []Command
				for _, c := range bodyCmds {
					if c.Subsection == "negative" {
						bodyNegatives = append(bodyNegatives, c)
					} else {
						bodyPositives = append(bodyPositives, c)
					}
				}

				if !dryRun && format != "json" {
					fmt.Fprintf(cmd.OutOrStdout(), "Milestone Acceptance Criteria:\n")
				}

				if runBodyGroup(bodyPositives, "Positive") {
					earlyStop = true
				}
				if !earlyStop && runBodyGroup(bodyNegatives, "Negative") {
					earlyStop = true
				}

				if len(bodyDropped) > 0 && !dryRun && format != "json" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Unverifiable:\n")
				}
				for _, d := range bodyDropped {
					unverifiableCount++
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] milestone body  %s  unverifiable: %s\n", d.Subsection, d.Text)
						continue
					}
					if format != "json" {
						fmt.Fprintf(cmd.OutOrStdout(), "    ⚠ %s          ← no (`command`) form\n", d.Text)
					}
					jsonUnverifiable = append(jsonUnverifiable, JSONUnverifiableEntry{
						IssueTitle: milestoneName,
						Subsection: d.Subsection,
						Item:       d.Text,
					})
				}
			}

			for _, iss := range prioritized {
				if earlyStop {
					break
				}
				sections := issue.ParseSections(iss.Body)
				cmds, dropped := ExtractCommands(sections, iss.ID, iss.Title)
				if len(cmds) == 0 && len(dropped) == 0 {
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
						// Name the criterion before running it, not after. A
						// suite-length command otherwise emits four spaces and
						// nothing else for minutes, which reads as a hang and
						// does not say which criterion is responsible.
						fmt.Fprintf(cmd.OutOrStdout(), "    %s ", c.Text)
					}

					repoRoot := repoRootFromApp(app)
					result := Execute(c.Text, repoRoot, cmdTimeout)
					passed, vacuous := classify(result)

					if format != "json" {
						// The criterion text was printed before the run, so only
						// the mark goes here.
						switch {
						case passed:
							fmt.Fprintln(cmd.OutOrStdout(), "✓")
						case vacuous:
							fmt.Fprintln(cmd.OutOrStdout(), "⚠  ← NO TESTS RAN")
						default:
							fmt.Fprintln(cmd.OutOrStdout(), "✗  ← REGRESSION")
						}
					}

					switch {
					case passed:
						passedCount++
					case vacuous:
						vacuousCount++
					default:
						failedCount++
					}

					jsonResults = append(jsonResults, JSONResultEntry{
						IssueID:    iss.ID.String(),
						IssueTitle: iss.Title,
						Subsection: c.Subsection,
						Command:    c.Text,
						Passed:     passed,
						NoTestsRan: result.NoTestsRan,
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
						// Name the criterion before running it, not after. A
						// suite-length command otherwise emits four spaces and
						// nothing else for minutes, which reads as a hang and
						// does not say which criterion is responsible.
						fmt.Fprintf(cmd.OutOrStdout(), "    %s ", c.Text)
					}

					repoRoot := repoRootFromApp(app)
					result := Execute(c.Text, repoRoot, cmdTimeout)
					passed, vacuous := classify(result)

					if format != "json" {
						// The criterion text was printed before the run, so only
						// the mark goes here.
						switch {
						case passed:
							fmt.Fprintln(cmd.OutOrStdout(), "✓")
						case vacuous:
							fmt.Fprintln(cmd.OutOrStdout(), "⚠  ← NO TESTS RAN")
						default:
							fmt.Fprintln(cmd.OutOrStdout(), "✗  ← REGRESSION")
						}
					}

					switch {
					case passed:
						passedCount++
					case vacuous:
						vacuousCount++
					default:
						failedCount++
					}

					jsonResults = append(jsonResults, JSONResultEntry{
						IssueID:    iss.ID.String(),
						IssueTitle: iss.Title,
						Subsection: c.Subsection,
						Command:    c.Text,
						Passed:     passed,
						NoTestsRan: result.NoTestsRan,
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

				// Report dropped AC commands: backtick text that yielded no
				// parenthesized command. These run nothing, so they fail the gate
				// as a category distinct from regressions.
				if len(dropped) > 0 && !dryRun && format != "json" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Unverifiable:\n")
				}
				for _, d := range dropped {
					unverifiableCount++
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s  %s  unverifiable: %s\n", iss.Title, d.Subsection, d.Text)
						continue
					}
					if format != "json" {
						fmt.Fprintf(cmd.OutOrStdout(), "    ⚠ %s          ← no (`command`) form\n", d.Text)
					}
					jsonUnverifiable = append(jsonUnverifiable, JSONUnverifiableEntry{
						IssueID:    d.IssueID.String(),
						IssueTitle: d.IssueTitle,
						Subsection: d.Subsection,
						Item:       d.Text,
					})
				}
			}

			// Extracting nothing at all — not even a dropped, malformed
			// command — is a distinct failure from every criterion passing.
			// Without this, a milestone whose issues carry no runnable
			// criteria (or no issues at all) reports "0/0 passing" and exits
			// 0, indistinguishable from one whose criteria all passed. The
			// issue count in the message is what tells an empty milestone
			// apart from one whose issues simply extracted nothing.
			if totalCount == 0 && unverifiableCount == 0 {
				return fmt.Errorf("milestone %s: no acceptance criteria extracted from %d done issue(s)", milestoneName, len(doneIssues))
			}

			// If dry-run, nothing to summarize.
			if dryRun {
				return nil
			}

			if format == "json" {
				if jsonResults == nil {
					jsonResults = []JSONResultEntry{}
				}
				if jsonUnverifiable == nil {
					jsonUnverifiable = []JSONUnverifiableEntry{}
				}
				report := JSONReport{
					Milestone:         milestoneName,
					Total:             totalCount,
					Passed:            passedCount,
					Failed:            failedCount,
					NoTestsRan:        vacuousCount,
					Unverifiable:      unverifiableCount,
					Results:           jsonResults,
					UnverifiableItems: jsonUnverifiable,
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
				if unverifiableCount > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "      %d unverifiable acceptance criterion(s)\n", unverifiableCount)
				}
				if vacuousCount > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "      %d criterion(s) ran no tests\n", vacuousCount)
				}
				if failedCount > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "      %d regression(s) detected\n", failedCount)
				}
			}

			if failedCount > 0 || unverifiableCount > 0 || vacuousCount > 0 {
				return fmt.Errorf("milestone %s: %d regression(s), %d unverifiable acceptance criterion(s), %d that ran no tests",
					milestoneName, failedCount, unverifiableCount, vacuousCount)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "stop on first failing command")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list commands without executing them, including from pending issues")
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
