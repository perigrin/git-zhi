// ABOUTME: Implementation of the milestone show command: displays milestone
// ABOUTME: details with issue list, progress percentage, and telemetry signals.
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/resolve"
	"github.com/perigrin/git-zhi/internal/telemetry"
)

// milestoneShowJSON is the JSON presentation of a milestone with its issues
// and computed telemetry.
type milestoneShowJSON struct {
	*milestone.Milestone
	Issues      []*issue.Issue   `json:"issues"`
	TotalIssues int              `json:"total_issues"`
	DoneIssues  int              `json:"done_issues"`
	Progress    int              `json:"progress_pct"`
	Telemetry   *telemetry.Stats `json:"telemetry"`
}

// runMilestoneShow loads a milestone by name (or the current milestone if no
// arg is provided) and displays its details with the associated issue list
// and computed telemetry signals.
func runMilestoneShow(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	milestones, err := milestone.LoadAllMilestones(app.Store)
	if err != nil {
		return fmt.Errorf("load milestones: %w", err)
	}

	if len(milestones) == 0 {
		return fmt.Errorf("no milestones found")
	}

	// Load all issues once; used for both current-milestone detection and filtering.
	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	// If no name given, resolve via the HEAD issue's milestone. Fall back to
	// scanning for the first milestone with pending/in-progress issues, then
	// to the first milestone in the list.
	var ms *milestone.Milestone
	if name == "" {
		// Attempt to derive the milestone from the current HEAD issue.
		headRef, headErr := resolve.ResolveRef(app.Store, "")
		if headErr == nil {
			headData, _ := app.Store.ReadEntity(headRef, "issue.md")
			if headData != nil {
				headIss, _ := issue.Parse(headData)
				if headIss != nil && headIss.Milestone != "" {
					name = headIss.Milestone
				}
			}
		}
		// Fall back to first milestone with pending/in-progress issues.
		if name == "" {
			found := findCurrentMilestone(milestones, allIssues)
			if found != nil {
				ms = found
			} else {
				// Last resort: the first milestone in the list.
				ms = milestones[0]
			}
		}
	}
	// If name was resolved (either from HEAD or provided by the caller), load it.
	if ms == nil {
		loaded, err := milestone.LoadMilestone(app.Store, name)
		if err != nil {
			return fmt.Errorf("milestone %q not found: %w", name, err)
		}
		ms = loaded
	}

	var msIssues []*issue.Issue
	doneCount := 0
	for _, iss := range allIssues {
		if iss.Milestone == ms.Name {
			msIssues = append(msIssues, iss)
			if iss.State == issue.StateDone || iss.State == issue.StateCancelled {
				doneCount++
			}
		}
	}

	progressPct := 0
	if len(msIssues) > 0 {
		progressPct = (doneCount * 100) / len(msIssues)
	}

	// Compute telemetry from milestone issues.
	stats := telemetry.Compute(allIssues, ms)

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		out := &milestoneShowJSON{
			Milestone:   ms,
			Issues:      msIssues,
			TotalIssues: len(msIssues),
			DoneIssues:  doneCount,
			Progress:    progressPct,
			Telemetry:   stats,
		}
		if out.Issues == nil {
			out.Issues = []*issue.Issue{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	return showMilestoneHuman(cmd, ms, msIssues, doneCount, progressPct, stats)
}

// showMilestoneHuman renders the milestone detail view in human-readable format,
// including telemetry signals below the progress line.
func showMilestoneHuman(cmd *cobra.Command, ms *milestone.Milestone, issues []*issue.Issue, doneCount, progressPct int, stats *telemetry.Stats) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Milestone: %s\n", ms.Name)
	if ms.Due != nil {
		fmt.Fprintf(w, "Due:       %s\n", ms.Due.Format("Jan 02 2006"))
	}
	fmt.Fprintf(w, "Progress:  %d/%d done (%d%%)\n", doneCount, len(issues), progressPct)

	// Telemetry section.
	if stats != nil && stats.FeverStatus != "" {
		fmt.Fprintf(w, "MPG:       %.1f commits/issue\n", stats.MPG)
		fmt.Fprintf(w, "Speed:     %.2f issues/week\n", stats.Speed)
		fmt.Fprintf(w, "Buffer:    %.1f total, %.1f burned\n", stats.BufferTotal, stats.BufferBurned)
		fmt.Fprintf(w, "Fever:     %s\n", stats.FeverStatus)
		if stats.TimeInChain > 0 {
			fmt.Fprintf(w, "  Time-in-chain: %.0f%%\n", stats.TimeInChain*100)
			fmt.Fprintf(w, "  Shadow work:   %.0f%%\n", stats.ShadowWork*100)
		}
	}

	if len(issues) > 0 {
		fmt.Fprintln(w, "\nIssues:")
		for _, iss := range issues {
			icon := stateIcon[iss.State]
			if icon == "" {
				icon = "?"
			}
			fmt.Fprintf(w, "  %s  %-40s %s %s\n",
				iss.ID.String()[:8],
				iss.Title,
				icon,
				string(iss.State),
			)
		}
	}

	return nil
}

// findCurrentMilestone returns the first milestone (by sort order) that has at
// least one pending or in-progress issue.
func findCurrentMilestone(milestones []*milestone.Milestone, issues []*issue.Issue) *milestone.Milestone {
	for _, ms := range milestones {
		for _, iss := range issues {
			if iss.Milestone == ms.Name &&
				(iss.State == issue.StatePending || iss.State == issue.StateInProgress) {
				return ms
			}
		}
	}
	return nil
}
