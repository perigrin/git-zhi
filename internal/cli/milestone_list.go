// ABOUTME: Implementation of the milestone list command: loads all milestones
// ABOUTME: with issue counts and marks the current (first with active issues).
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// milestoneListEntry bundles a milestone with its computed issue counts for output.
type milestoneListEntry struct {
	*milestone.Milestone
	TotalIssues int  `json:"total_issues"`
	DoneIssues  int  `json:"done_issues"`
	IsCurrent   bool `json:"is_current"`
}

// runMilestoneList loads all milestones and issues, computes counts, marks the
// current milestone, and renders in human or JSON format.
func runMilestoneList(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	milestones, err := milestone.LoadAllMilestones(app.Store)
	if err != nil {
		return fmt.Errorf("load milestones: %w", err)
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	entries := buildMilestoneEntries(milestones, issues)

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if entries == nil {
			entries = []*milestoneListEntry{}
		}
		return enc.Encode(entries)
	}

	return listMilestonesHuman(cmd, entries)
}

// buildMilestoneEntries associates each milestone with its issue counts and
// marks the first milestone (sorted by name) that has pending or in-progress issues
// as "current".
func buildMilestoneEntries(milestones []*milestone.Milestone, issues []*issue.Issue) []*milestoneListEntry {
	entries := make([]*milestoneListEntry, 0, len(milestones))
	currentMarked := false

	for _, ms := range milestones {
		entry := &milestoneListEntry{Milestone: ms}
		for _, iss := range issues {
			if iss.Milestone != ms.Name {
				continue
			}
			entry.TotalIssues++
			if iss.State == issue.StateDone || iss.State == issue.StateCancelled {
				entry.DoneIssues++
			}
		}
		// First milestone with active (pending/in-progress) issues is "current".
		if !currentMarked && entry.TotalIssues > entry.DoneIssues {
			entry.IsCurrent = true
			currentMarked = true
		}
		entries = append(entries, entry)
	}
	return entries
}

// listMilestonesHuman renders the milestone list as a human-readable table.
func listMilestonesHuman(cmd *cobra.Command, entries []*milestoneListEntry) error {
	w := cmd.OutOrStdout()
	for _, e := range entries {
		duePart := "(no date)"
		if e.Due != nil {
			duePart = fmt.Sprintf("due %s", e.Due.Format("Jan 02"))
		}
		countPart := fmt.Sprintf("%d/%d done", e.DoneIssues, e.TotalIssues)
		line := fmt.Sprintf("%-16s  %-14s  %s", e.Name, duePart, countPart)
		if e.IsCurrent {
			line += "    ← current"
		}
		fmt.Fprintln(w, line)
	}
	return nil
}
