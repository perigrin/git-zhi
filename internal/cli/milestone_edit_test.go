// ABOUTME: Tests for the milestone edit command: setting due date, clearing
// ABOUTME: due date with "none", renaming a milestone, running --resolve, and --state complete gate.
package cli_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// createMilestoneWithResolution writes a milestone with a resolution command set.
func createMilestoneWithResolution(t *testing.T, app *cli.App, name, resolution string) {
	t.Helper()
	ms := &milestone.Milestone{
		Name:       name,
		Created:    time.Now(),
		Resolution: resolution,
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	refPath := milestone.RefPrefix + name
	if err := app.Store.WriteEntity(refPath, "milestone.yaml", data, "Create milestone: "+name); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
}

// createTestIssueInMilestoneWithState writes an issue to the store and returns its UUID.
func createTestIssueInMilestoneWithState(t *testing.T, app *cli.App, title string, state issue.State, ms string) uuid.UUID {
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
		Created:   now,
		Updated:   now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := fmt.Sprintf("refs/zhi/_/issues/%s", id.String())
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}

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

	tagRef := "refs/zhi/_/tags/current-sprint"
	if !app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref %s to exist after --tag", tagRef)
	}

	// The tag should point to the milestone ref path.
	data, err := app.Store.ReadEntity(tagRef, "tag.txt")
	if err != nil {
		t.Fatalf("ReadEntity tag.txt: %v", err)
	}
	content := strings.TrimSpace(string(data))
	expectedTarget := "refs/zhi/_/milestones/v0.1"
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

	tagRef := "refs/zhi/_/tags/release"
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

// TestMilestoneResolve_Pass verifies that --resolve executes the resolution
// command and reports success when the command exits zero.
func TestMilestoneResolve_Pass(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create a milestone whose resolution command always succeeds.
	createMilestoneWithResolution(t, app, "release", "echo resolution-ok")

	stdout, err := run("milestone", "edit", "release", "--resolve")
	if err != nil {
		t.Fatalf("milestone edit --resolve failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "resolution-ok") {
		t.Errorf("expected resolution command stdout in output, got: %q", output)
	}
}

// TestMilestoneResolve_Fail verifies that --resolve reports failure and returns
// a non-zero exit when the resolution command exits non-zero.
func TestMilestoneResolve_Fail(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create a milestone whose resolution command always fails.
	createMilestoneWithResolution(t, app, "failing", "exit 1")

	_, err := run("milestone", "edit", "failing", "--resolve")
	if err == nil {
		t.Fatal("expected error from milestone edit --resolve with failing command, got nil")
	}
}

// TestMilestoneResolve_NoResolution verifies that --resolve on a milestone
// without a resolution field prints an error and does not execute anything.
func TestMilestoneResolve_NoResolution(t *testing.T) {
	_, run := setupMilestoneTest(t)

	// v0.1 has no resolution field (created by EnsureInitialized).
	_, err := run("milestone", "edit", "v0.1", "--resolve")
	if err == nil {
		t.Fatal("expected error when milestone has no resolution command, got nil")
	}
	if !strings.Contains(err.Error(), "no resolution command") {
		t.Errorf("expected 'no resolution command' in error, got: %v", err)
	}
}

// TestMilestoneComplete_AllIssuesDone verifies that --state complete succeeds
// when all issues in the milestone are done or cancelled, and that the
// milestone state is updated to "completed" with a non-nil Completed timestamp.
func TestMilestoneComplete_AllIssuesDone(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create a milestone with a passing resolution command.
	createMilestoneWithResolution(t, app, "release", "echo ok")

	// Add issues that are all done or cancelled.
	createTestIssueInMilestoneWithState(t, app, "Feature A", issue.StateDone, "release")
	createTestIssueInMilestoneWithState(t, app, "Feature B", issue.StateCancelled, "release")

	_, err := run("milestone", "edit", "release", "--state", "complete")
	if err != nil {
		t.Fatalf("milestone edit --state complete failed: %v", err)
	}

	ms, err := milestone.LoadMilestone(app.Store, "release")
	if err != nil {
		t.Fatalf("LoadMilestone after complete: %v", err)
	}
	if ms.State != "completed" {
		t.Errorf("expected State 'completed', got %q", ms.State)
	}
	if ms.Completed == nil {
		t.Error("expected Completed timestamp to be set")
	}
}

// TestMilestoneComplete_BlockedByPendingIssues verifies that --state complete
// refuses when one or more issues in the milestone are not done or cancelled,
// and the error lists the blocking issues.
func TestMilestoneComplete_BlockedByPendingIssues(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createMilestoneWithResolution(t, app, "release", "echo ok")
	createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")
	createTestIssueInMilestoneWithState(t, app, "Pending blocker", issue.StatePending, "release")

	_, err := run("milestone", "edit", "release", "--state", "complete")
	if err == nil {
		t.Fatal("expected error with pending issues blocking completion, got nil")
	}
	if !strings.Contains(err.Error(), "Pending blocker") {
		t.Errorf("expected blocking issue title in error, got: %v", err)
	}
}

// TestMilestoneComplete_ResolutionFails verifies that --state complete refuses
// when the milestone's resolution command exits non-zero.
func TestMilestoneComplete_ResolutionFails(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createMilestoneWithResolution(t, app, "release", "exit 1")
	createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")

	_, err := run("milestone", "edit", "release", "--state", "complete")
	if err == nil {
		t.Fatal("expected error when resolution command fails, got nil")
	}
	if !strings.Contains(err.Error(), "resolution command failed") {
		t.Errorf("expected 'resolution command failed' in error, got: %v", err)
	}
}

// TestMilestoneComplete_VerifySkippedWithWarning verifies that --state complete
// skips the verify gate (with a warning) when git-zhi-verify is not on PATH,
// and still succeeds when all other gates pass.
func TestMilestoneComplete_VerifySkippedWithWarning(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createMilestoneWithResolution(t, app, "release", "echo ok")
	createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")

	// Hide git-zhi-verify by restricting PATH to system directories only.
	// This ensures exec.LookPath("git-zhi-verify") fails, triggering
	// the warning path. We keep /usr/bin and /bin so the resolution
	// command ("echo ok") still works.
	t.Setenv("PATH", "/usr/bin:/bin")

	stdout, err := run("milestone", "edit", "release", "--state", "complete")
	if err != nil {
		t.Fatalf("milestone edit --state complete failed unexpectedly: %v", err)
	}

	// The warning about skipping the verify gate should appear in output.
	output := stdout.String()
	if !strings.Contains(output, "git-zhi-verify") {
		t.Errorf("expected warning about git-zhi-verify not found in output: %q", output)
	}
}

// TestIssueEdit_MilestoneLocked verifies that assigning an issue to a
// completed milestone returns an error.
func TestIssueEdit_MilestoneLocked(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create and complete the release milestone.
	createMilestoneWithResolution(t, app, "release", "echo ok")
	createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")
	if _, err := run("milestone", "edit", "release", "--state", "complete"); err != nil {
		t.Fatalf("complete milestone: %v", err)
	}

	// Create a new pending issue not in the milestone.
	newID := createTestIssueInMilestoneWithState(t, app, "New work", issue.StatePending, "")

	// Attempt to assign it to the completed milestone.
	_, err := run("issue", "edit", newID.String(), "--milestone", "release")
	if err == nil {
		t.Fatal("expected error assigning issue to completed milestone, got nil")
	}
	if !strings.Contains(err.Error(), "completed") {
		t.Errorf("expected 'completed' in error, got: %v", err)
	}
}

// TestMilestoneEdit_Reopen verifies that --state reopen transitions a completed
// milestone back to "open" and clears the Completed timestamp.
func TestMilestoneEdit_Reopen(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create and complete a milestone.
	createMilestoneWithResolution(t, app, "release", "echo ok")
	createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")

	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := run("milestone", "edit", "release", "--state", "complete"); err != nil {
		t.Fatalf("complete milestone: %v", err)
	}

	// Verify it's completed.
	ms, err := milestone.LoadMilestone(app.Store, "release")
	if err != nil {
		t.Fatalf("LoadMilestone after complete: %v", err)
	}
	if ms.State != "completed" {
		t.Fatalf("expected State 'completed', got %q", ms.State)
	}
	if ms.Completed == nil {
		t.Fatal("expected Completed timestamp to be set")
	}

	// Reopen it.
	stdout, err := run("milestone", "edit", "release", "--state", "reopen")
	if err != nil {
		t.Fatalf("milestone edit --state reopen failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "reopened") {
		t.Errorf("expected 'reopened' in output, got: %q", output)
	}

	// Verify state is "open" and Completed is nil.
	ms, err = milestone.LoadMilestone(app.Store, "release")
	if err != nil {
		t.Fatalf("LoadMilestone after reopen: %v", err)
	}
	if ms.State != "open" {
		t.Errorf("expected State 'open' after reopen, got %q", ms.State)
	}
	if ms.Completed != nil {
		t.Errorf("expected Completed to be nil after reopen, got: %v", ms.Completed)
	}
}

// TestMilestoneEdit_Reopen_AlreadyOpen verifies that reopening an already-open
// milestone returns an error.
func TestMilestoneEdit_Reopen_AlreadyOpen(t *testing.T) {
	_, run := setupMilestoneTest(t)

	// v0.1 is created by EnsureInitialized in "open" state.
	_, err := run("milestone", "edit", "v0.1", "--state", "reopen")
	if err == nil {
		t.Fatal("expected error when reopening an open milestone, got nil")
	}
	if !strings.Contains(err.Error(), "cannot reopen") {
		t.Errorf("expected 'cannot reopen' in error, got: %v", err)
	}
}

// TestMilestoneEdit_Reopen_IssuesUnchanged verifies that issues in the
// milestone retain their state after the milestone is reopened.
func TestMilestoneEdit_Reopen_IssuesUnchanged(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createMilestoneWithResolution(t, app, "release", "echo ok")
	doneID := createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")
	cancelledID := createTestIssueInMilestoneWithState(t, app, "Cancelled issue", issue.StateCancelled, "release")

	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := run("milestone", "edit", "release", "--state", "complete"); err != nil {
		t.Fatalf("complete milestone: %v", err)
	}

	// Reopen.
	if _, err := run("milestone", "edit", "release", "--state", "reopen"); err != nil {
		t.Fatalf("reopen milestone: %v", err)
	}

	// Check that issue states are unchanged.
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	for _, iss := range issues {
		switch iss.ID {
		case doneID:
			if iss.State != issue.StateDone {
				t.Errorf("Done issue state changed to %q after reopen", iss.State)
			}
		case cancelledID:
			if iss.State != issue.StateCancelled {
				t.Errorf("Cancelled issue state changed to %q after reopen", iss.State)
			}
		}
	}
}

// TestMilestoneEdit_Reopen_RoundTrip verifies the complete → reopen → complete
// cycle works and quality gates are re-enforced on the second completion.
func TestMilestoneEdit_Reopen_RoundTrip(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createMilestoneWithResolution(t, app, "release", "echo ok")
	createTestIssueInMilestoneWithState(t, app, "Done issue", issue.StateDone, "release")

	t.Setenv("PATH", "/usr/bin:/bin")

	// First complete.
	if _, err := run("milestone", "edit", "release", "--state", "complete"); err != nil {
		t.Fatalf("first complete: %v", err)
	}

	// Reopen.
	if _, err := run("milestone", "edit", "release", "--state", "reopen"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	// Add a pending issue — this should block second completion.
	createTestIssueInMilestoneWithState(t, app, "New pending", issue.StatePending, "release")

	// Second complete should fail (pending issue blocks).
	_, err := run("milestone", "edit", "release", "--state", "complete")
	if err == nil {
		t.Fatal("expected error on second complete with pending issue, got nil")
	}
	if !strings.Contains(err.Error(), "New pending") {
		t.Errorf("expected blocking issue title in error, got: %v", err)
	}
}
