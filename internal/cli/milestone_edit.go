// ABOUTME: Implementation of the milestone edit command: set or clear due date,
// ABOUTME: rename milestones, tag/untag, and run the milestone's resolution command.
package cli

import (
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// knownMilestoneEditFlags lists all flags that constitute a valid edit operation.
var knownMilestoneEditFlags = []string{"due", "name", "tag", "untag", "resolve"}

// runMilestoneEdit loads a milestone, applies --due, --name, --tag, and/or
// --untag changes. On rename, the old ref is deleted and all issues'
// Milestone field is updated to the new name. --tag and --untag create and
// delete named tag refs pointing to the milestone ref.
func runMilestoneEdit(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("milestone edit requires exactly one argument: <name>")
	}
	name := args[0]

	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	// Require at least one known flag.
	anyFlagSet := false
	for _, f := range knownMilestoneEditFlags {
		if cmd.Flags().Changed(f) {
			anyFlagSet = true
			break
		}
	}
	if !anyFlagSet {
		return fmt.Errorf("no changes specified: use --due, --name, --tag, --untag, or --resolve")
	}

	ms, err := milestone.LoadMilestone(app.Store, name)
	if err != nil {
		return fmt.Errorf("milestone %q not found: %w", name, err)
	}

	w := cmd.OutOrStdout()

	// Apply --resolve: execute the milestone's resolution command and report results.
	// This is a standalone operation; it does not persist any changes to the milestone.
	if cmd.Flags().Changed("resolve") {
		return runMilestoneResolve(app, ms, w)
	}

	// Apply --due change.
	if cmd.Flags().Changed("due") {
		dueStr, _ := cmd.Flags().GetString("due")
		if dueStr == "none" {
			fmt.Fprintf(w, "%s: due → (cleared)\n", ms.Name)
			ms.Due = nil
		} else {
			due, err := time.Parse("2006-01-02", dueStr)
			if err != nil {
				return fmt.Errorf("invalid --due date %q: use YYYY-MM-DD format", dueStr)
			}
			fmt.Fprintf(w, "%s: due → %s\n", ms.Name, due.Format("Jan 02 2006"))
			ms.Due = &due
		}
	}

	// Apply --name change (rename).
	newName, _ := cmd.Flags().GetString("name")
	if cmd.Flags().Changed("name") {
		if newName == name {
			return fmt.Errorf("new name is the same as the current name")
		}
		newRef := milestone.RefPrefix + newName
		if app.Store.RefExists(newRef) {
			return fmt.Errorf("milestone %q already exists", newName)
		}

		fmt.Fprintf(w, "%s: name → %s\n", name, newName)
		ms.Name = newName

		// Write new ref with updated name.
		data, err := milestone.MarshalMilestone(ms)
		if err != nil {
			return fmt.Errorf("marshal milestone: %w", err)
		}
		if err := app.Store.WriteEntity(newRef, "milestone.yaml", data, "Rename milestone: "+name+" → "+newName); err != nil {
			return fmt.Errorf("write new milestone ref: %w", err)
		}

		// Delete old ref.
		oldRef := milestone.RefPrefix + name
		if err := app.Store.DeleteRef(oldRef); err != nil {
			return fmt.Errorf("delete old milestone ref: %w", err)
		}

		// Update all issues that belong to the old milestone.
		if err := updateIssuesMilestone(app, name, newName); err != nil {
			return fmt.Errorf("update issue milestones: %w", err)
		}

		// Tag/untag operations after rename use the new milestone name.
		name = newName
	} else if cmd.Flags().Changed("due") {
		// Persist due-date change (no rename, so write to the existing ref).
		data, err := milestone.MarshalMilestone(ms)
		if err != nil {
			return fmt.Errorf("marshal milestone: %w", err)
		}
		refPath := milestone.RefPrefix + name
		if err := app.Store.WriteEntity(refPath, "milestone.yaml", data, "Edit milestone: "+name); err != nil {
			return fmt.Errorf("write milestone: %w", err)
		}
	}

	// Apply --tag: create a tag ref pointing to this milestone.
	if cmd.Flags().Changed("tag") {
		tagName, _ := cmd.Flags().GetString("tag")
		tagRef := "refs/zhi/_/tags/" + tagName
		refPath := milestone.RefPrefix + name
		if err := app.Store.WriteEntity(tagRef, "tag.txt", []byte(refPath), "Tag milestone: "+tagName); err != nil {
			return fmt.Errorf("write tag ref: %w", err)
		}
	}

	// Apply --untag: delete the named tag ref.
	if cmd.Flags().Changed("untag") {
		tagName, _ := cmd.Flags().GetString("untag")
		tagRef := "refs/zhi/_/tags/" + tagName
		if err := app.Store.DeleteRef(tagRef); err != nil {
			return fmt.Errorf("delete tag ref: %w", err)
		}
	}

	return nil
}

// runMilestoneResolve executes the milestone's resolution command via `sh -c`
// in the repository working directory. Stdout and stderr from the command are
// written to w. Returns an error wrapping the command's exit status when the
// command exits non-zero, or an error if no resolution command is configured.
func runMilestoneResolve(app *App, ms *milestone.Milestone, w io.Writer) error {
	if ms.Resolution == "" {
		return fmt.Errorf("no resolution command configured for milestone %s", ms.Name)
	}

	// Determine the repository working directory for subprocess execution.
	wt, err := app.Repo.Worktree()
	if err != nil {
		return fmt.Errorf("get repo worktree: %w", err)
	}
	repoRoot := wt.Filesystem.Root()

	cmd := exec.Command("sh", "-c", ms.Resolution)
	cmd.Dir = repoRoot
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resolution command failed: %w", err)
	}
	return nil
}

// updateIssuesMilestone loads all issues, updates the Milestone field for any
// issue that belonged to oldName, and persists each updated issue.
func updateIssuesMilestone(app *App, oldName, newName string) error {
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}
	for _, iss := range issues {
		if iss.Milestone != oldName {
			continue
		}
		iss.Milestone = newName
		data, err := issue.Marshal(iss)
		if err != nil {
			return fmt.Errorf("marshal issue %s: %w", iss.ID, err)
		}
		refPath := issue.RefPrefix + iss.ID.String()
		if err := app.Store.WriteEntity(refPath, "issue.md", data, "Update milestone reference: "+oldName+" → "+newName); err != nil {
			return fmt.Errorf("write issue %s: %w", iss.ID, err)
		}
	}
	return nil
}
