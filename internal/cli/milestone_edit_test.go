// ABOUTME: Tests for the milestone edit command: setting due date, clearing
// ABOUTME: due date with "none", and renaming a milestone.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/milestone"
)

func TestMilestoneEdit_Due(t *testing.T) {
	app, run := setupMilestoneTest(t)

	_, err := run("milestone", "edit", "v0.1", "--due", "2026-06-30")
	if err != nil {
		t.Fatalf("milestone edit --due failed: %v", err)
	}

	ms, err := milestone.LoadMilestone(app.Store, "v0.1")
	if err != nil {
		t.Fatalf("LoadMilestone after edit: %v", err)
	}
	if ms.Due == nil {
		t.Fatal("expected Due to be set after edit --due")
	}
	if ms.Due.Month().String() != "June" || ms.Due.Day() != 30 {
		t.Errorf("expected due date Jun 30, got: %v", ms.Due)
	}
}

func TestMilestoneEdit_ClearDue(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Set due first.
	if _, err := run("milestone", "edit", "v0.1", "--due", "2026-06-30"); err != nil {
		t.Fatalf("set due: %v", err)
	}

	// Clear it.
	if _, err := run("milestone", "edit", "v0.1", "--due", "none"); err != nil {
		t.Fatalf("clear due: %v", err)
	}

	ms, err := milestone.LoadMilestone(app.Store, "v0.1")
	if err != nil {
		t.Fatalf("LoadMilestone after clear: %v", err)
	}
	if ms.Due != nil {
		t.Errorf("expected Due to be nil after --due none, got: %v", ms.Due)
	}
}

func TestMilestoneEdit_Name(t *testing.T) {
	app, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "edit", "v0.1", "--name", "parser-mvp")
	if err != nil {
		t.Fatalf("milestone edit --name failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "parser-mvp") {
		t.Errorf("expected new name in output, got: %s", output)
	}

	// New ref should exist.
	ms, err := milestone.LoadMilestone(app.Store, "parser-mvp")
	if err != nil {
		t.Fatalf("LoadMilestone for new name: %v", err)
	}
	if ms.Name != "parser-mvp" {
		t.Errorf("expected Name 'parser-mvp', got %q", ms.Name)
	}

	// Old ref should be gone.
	_, err = milestone.LoadMilestone(app.Store, "v0.1")
	if err == nil {
		t.Fatal("expected old ref 'v0.1' to be deleted after rename")
	}
}

func TestMilestoneEdit_Name_UpdatesIssues(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create an issue in v0.1.
	createTestIssueWithMilestone(t, app, "Owned issue", issue.StatePending, "v0.1", "")

	// Rename v0.1 to v1.0.
	if _, err := run("milestone", "edit", "v0.1", "--name", "v1.0"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// The issue should now belong to v1.0.
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected at least one issue")
	}
	for _, iss := range issues {
		if iss.Title == "Owned issue" && iss.Milestone != "v1.0" {
			t.Errorf("expected issue milestone to be updated to 'v1.0', got %q", iss.Milestone)
		}
	}
}

func TestMilestoneEdit_NotFound(t *testing.T) {
	_, run := setupMilestoneTest(t)

	_, err := run("milestone", "edit", "doesnotexist", "--due", "2026-01-01")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone, got nil")
	}
}

// TestMilestoneEdit_Tag verifies that --tag creates a tag ref pointing to the milestone.
func TestMilestoneEdit_Tag(t *testing.T) {
	app, run := setupMilestoneTest(t)

	_, err := run("milestone", "edit", "v0.1", "--tag", "current-sprint")
	if err != nil {
		t.Fatalf("milestone edit --tag failed: %v", err)
	}

	tagRef := "refs/chain/_/tags/current-sprint"
	if !app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref %s to exist after --tag", tagRef)
	}

	// The tag should point to the milestone ref path.
	data, err := app.Store.ReadEntity(tagRef, "tag.txt")
	if err != nil {
		t.Fatalf("ReadEntity tag.txt: %v", err)
	}
	content := strings.TrimSpace(string(data))
	expectedTarget := "refs/chain/_/milestones/v0.1"
	if content != expectedTarget {
		t.Fatalf("tag content = %q, want %q", content, expectedTarget)
	}
}

// TestMilestoneEdit_Untag verifies that --untag deletes the tag ref.
func TestMilestoneEdit_Untag(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Tag first.
	if _, err := run("milestone", "edit", "v0.1", "--tag", "release"); err != nil {
		t.Fatalf("milestone edit --tag failed: %v", err)
	}

	tagRef := "refs/chain/_/tags/release"
	if !app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref to exist after --tag")
	}

	// Then untag.
	if _, err := run("milestone", "edit", "v0.1", "--untag", "release"); err != nil {
		t.Fatalf("milestone edit --untag failed: %v", err)
	}

	if app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref %s to be deleted after --untag", tagRef)
	}
}

func TestMilestoneEdit_OutputFormat(t *testing.T) {
	_, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "edit", "v0.1", "--due", "2026-05-01")
	if err != nil {
		t.Fatalf("milestone edit failed: %v", err)
	}

	output := stdout.String()
	// Should show "<field> → <new value>" pattern.
	if !strings.Contains(output, "→") {
		t.Errorf("expected arrow '→' in output, got: %s", output)
	}
}
