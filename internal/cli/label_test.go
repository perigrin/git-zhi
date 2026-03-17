// ABOUTME: Tests for label CLI flags: issue edit --label/--unlabel, list --label filter,
// ABOUTME: next --label filter, and milestone show --label scoping.
package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// createTestIssueWithLabels writes an issue with specific labels to the store.
func createTestIssueWithLabels(t *testing.T, app *cli.App, title string, state issue.State, labels []string) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     state,
		Milestone: "v0.1",
		Labels:    labels,
		Created:   now,
		Updated:   now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := "refs/zhi/_/issues/" + id.String()
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}

// TestIssueEdit_Label verifies that --label appends the label to the issue's
// Labels slice without duplicating an already-present label.
func TestIssueEdit_Label(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Label target issue")

	_, _, err := run("issue", "edit", id, "--label", "LOPS")
	if err != nil {
		t.Fatalf("issue edit --label failed: %v", err)
	}

	ref := issue.RefPrefix + id
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	found := false
	for _, l := range iss.Labels {
		if l == "LOPS" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Labels to contain 'LOPS', got: %v", iss.Labels)
	}
}

// TestIssueEdit_Label_Deduplication verifies that adding the same label twice
// results in it appearing only once.
func TestIssueEdit_Label_Deduplication(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Dedup label issue")

	if _, _, err := run("issue", "edit", id, "--label", "LOPS"); err != nil {
		t.Fatalf("first --label failed: %v", err)
	}
	if _, _, err := run("issue", "edit", id, "--label", "LOPS"); err != nil {
		t.Fatalf("second --label failed: %v", err)
	}

	ref := issue.RefPrefix + id
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	count := 0
	for _, l := range iss.Labels {
		if l == "LOPS" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected label 'LOPS' to appear exactly once, got %d: %v", count, iss.Labels)
	}
}

// TestIssueEdit_Unlabel verifies that --unlabel removes a label from the
// issue's Labels slice.
func TestIssueEdit_Unlabel(t *testing.T) {
	app, run := setupEditTest(t)

	// Pre-populate with labels.
	id := createTestIssueWithLabels(t, app, "Unlabel target", issue.StatePending, []string{"LOPS", "platform"})

	_, _, err := run("issue", "edit", id.String(), "--unlabel", "LOPS")
	if err != nil {
		t.Fatalf("issue edit --unlabel failed: %v", err)
	}

	ref := issue.RefPrefix + id.String()
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for _, l := range iss.Labels {
		if l == "LOPS" {
			t.Fatalf("expected 'LOPS' to be removed from Labels, but it is still present: %v", iss.Labels)
		}
	}

	// "platform" label should still be present.
	found := false
	for _, l := range iss.Labels {
		if l == "platform" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'platform' to remain in Labels after --unlabel LOPS, got: %v", iss.Labels)
	}
}

// TestIssueEdit_Unlabel_NonExistent verifies that --unlabel on a label that
// does not exist is a no-op and does not error.
func TestIssueEdit_Unlabel_NonExistent(t *testing.T) {
	app, run := setupEditTest(t)

	id := createTestIssueWithLabels(t, app, "Unlabel no-op", issue.StatePending, []string{"LOPS"})

	_, _, err := run("issue", "edit", id.String(), "--unlabel", "missing-label")
	if err != nil {
		t.Fatalf("--unlabel on absent label should succeed, got: %v", err)
	}

	ref := issue.RefPrefix + id.String()
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	found := false
	for _, l := range iss.Labels {
		if l == "LOPS" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'LOPS' to remain unchanged after --unlabel missing-label, got: %v", iss.Labels)
	}
}

// TestIssueList_FilterLabel verifies that 'issue list --label' shows only
// issues carrying that label.
func TestIssueList_FilterLabel(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssueWithLabels(t, app, "Frontend issue", issue.StatePending, []string{"frontend"})
	createTestIssueWithLabels(t, app, "Backend issue", issue.StatePending, []string{"backend"})
	createTestIssueWithLabels(t, app, "No label issue", issue.StatePending, nil)

	stdout, err := run("issue", "list", "--label", "frontend")
	if err != nil {
		t.Fatalf("issue list --label frontend failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Frontend issue") {
		t.Errorf("expected 'Frontend issue' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Backend issue") {
		t.Errorf("expected 'Backend issue' to be excluded, got:\n%s", output)
	}
	if strings.Contains(output, "No label issue") {
		t.Errorf("expected 'No label issue' to be excluded, got:\n%s", output)
	}
}

// TestChainList_FilterLabel verifies that 'list --label' (top-level) shows
// only issues carrying that label.
func TestChainList_FilterLabel(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssueWithLabels(t, app, "Platform task", issue.StatePending, []string{"platform"})
	createTestIssueWithLabels(t, app, "Infra task", issue.StatePending, []string{"infra"})

	stdout, err := run("list", "--label", "platform")
	if err != nil {
		t.Fatalf("list --label platform failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Platform task") {
		t.Errorf("expected 'Platform task' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Infra task") {
		t.Errorf("expected 'Infra task' to be excluded, got:\n%s", output)
	}
}

// TestChainNext_FilterLabel verifies that 'next --label' picks from the
// filtered ready set when --label is given.
func TestChainNext_FilterLabel(t *testing.T) {
	app, run := setupListTest(t)

	// Create two independent (unblocked) issues with different labels.
	// The LOPS-labeled issue should be the one reported by next --label LOPS.
	createTestIssueWithLabels(t, app, "LOPS ready task", issue.StatePending, []string{"LOPS"})
	createTestIssueWithLabels(t, app, "Other ready task", issue.StatePending, []string{"other"})

	stdout, err := run("next", "--label", "LOPS")
	if err != nil {
		t.Fatalf("next --label LOPS failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "LOPS ready task") {
		t.Errorf("expected 'LOPS ready task' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Other ready task") {
		t.Errorf("expected 'Other ready task' to be excluded, got:\n%s", output)
	}
}

// TestMilestoneShow_FilterLabel verifies that 'milestone show --label' scopes
// the issue list in the output to only issues with that label.
func TestMilestoneShow_FilterLabel(t *testing.T) {
	app, run := setupListTest(t)

	// Create a milestone first.
	if _, err := run("milestone", "add", "sprint-1"); err != nil {
		t.Fatalf("milestone add failed: %v", err)
	}

	// Issue A: milestone sprint-1, label LOPS
	createTestIssueWithLabelsAndMilestone(t, app, "LOPS issue", issue.StatePending, "sprint-1", []string{"LOPS"})
	// Issue B: milestone sprint-1, no label
	createTestIssueWithLabelsAndMilestone(t, app, "Unlabeled issue", issue.StatePending, "sprint-1", nil)

	stdout, err := run("milestone", "show", "sprint-1", "--label", "LOPS")
	if err != nil {
		t.Fatalf("milestone show --label failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "LOPS issue") {
		t.Errorf("expected 'LOPS issue' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Unlabeled issue") {
		t.Errorf("expected 'Unlabeled issue' to be excluded, got:\n%s", output)
	}
}

// createTestIssueWithLabelsAndMilestone writes an issue with labels and a
// specific milestone to the store.
func createTestIssueWithLabelsAndMilestone(t *testing.T, app *cli.App, title string, state issue.State, ms string, labels []string) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     state,
		Milestone: ms,
		Labels:    labels,
		Created:   now,
		Updated:   now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := "refs/zhi/_/issues/" + id.String()
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}
