// ABOUTME: Implementation of the milestone show command: displays milestone
// ABOUTME: details with issue list and progress percentage.
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/milestone"
)

// milestoneShowJSON is the JSON presentation of a milestone with its issues.
type milestoneShowJSON struct {
	*milestone.Milestone
	Issues     []*issue.Issue `json:"issues"`
	TotalIssues int           `json:"total_issues"`
	DoneIssues  int           `json:"done_issues"`
	Progress    int           `json:"progress_pct"`
}

// runMilestoneShow loads a milestone by name (or the current milestone if no
// arg is provided) and displays its details with the associated issue list.
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

	// If no name given, find the current milestone (first with pending/in-progress issues).
	var ms *milestone.Milestone
	if name == "" {
		allIssues, err := issue.LoadAllIssues(app.Store)
		if err != nil {
			return fmt.Errorf("load issues: %w", err)
		}
		ms = findCurrentMilestone(milestones, allIssues)
		if ms == nil {
			// Fall back to the first milestone if none have active issues.
			ms = milestones[0]
		}
	} else {
		loaded, err := milestone.LoadMilestone(app.Store, name)
		if err != nil {
			return fmt.Errorf("milestone %q not found: %w", name, err)
		}
		ms = loaded
	}

	// Load issues belonging to this milestone.
	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
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

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		out := &milestoneShowJSON{
			Milestone:   ms,
			Issues:      msIssues,
			TotalIssues: len(msIssues),
			DoneIssues:  doneCount,
			Progress:    progressPct,
		}
		if out.Issues == nil {
			out.Issues = []*issue.Issue{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	return showMilestoneHuman(cmd, ms, msIssues, doneCount, progressPct)
}

// showMilestoneHuman renders the milestone detail view in human-readable format.
func showMilestoneHuman(cmd *cobra.Command, ms *milestone.Milestone, issues []*issue.Issue, doneCount, progressPct int) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Milestone: %s\n", ms.Name)
	if ms.Due != nil {
		fmt.Fprintf(w, "Due:       %s\n", ms.Due.Format("Jan 02 2006"))
	}
	fmt.Fprintf(w, "Progress:  %d/%d done (%d%%)\n", doneCount, len(issues), progressPct)

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
