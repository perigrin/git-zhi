// ABOUTME: Tests for 'issue edit --batch': bulk updates piped from stdin.
// ABOUTME: Covers two-op success, invalid ID continuation, state transitions, empty stdin.
package cli_test

import (
	"context"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// runWithStdin is like the run function returned by setupEditTest, but accepts
// explicit stdin content instead of an empty reader.
func runWithStdin(app *cli.App, stdin string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := cli.NewRootCommand()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	cmd.SetContext(cli.WithApp(context.Background(), app))
	err := cmd.Execute()
	return stdout, stderr, err
}

// TestIssueEditBatch_TwoOps verifies that two valid batch operations both
// succeed and each issue is correctly updated.
func TestIssueEditBatch_TwoOps(t *testing.T) {
	app, _ := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "Batch issue one")
	id2 := createEditTestIssue(t, app, "Batch issue two")

	// Use full UUIDs in batch input to avoid prefix-ambiguity when both IDs
	// are generated within the same millisecond timestamp.
	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"assigned": "agent:sync-bot", "urgency": "high"}}`+"\n"+
			`{"issue_id": "%s", "fields": {"urgency": "low"}}`,
		id1, id2,
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch failed: %v\nstdout:\n%s", err, stdout.String())
	}

	out := stdout.String()
	lines := nonEmptyLines(out)
	if len(lines) != 2 {
		t.Fatalf("expected 2 result lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "ok") {
		t.Errorf("expected 'ok' in first result line, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ok") {
		t.Errorf("expected 'ok' in second result line, got: %s", lines[1])
	}

	// Verify issue 1 was updated.
	ref1 := issue.RefPrefix + id1
	data1, err := app.Store.ReadEntity(ref1, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id1: %v", err)
	}
	iss1, err := issue.Parse(data1)
	if err != nil {
		t.Fatalf("Parse id1: %v", err)
	}
	if iss1.Assigned != "agent:sync-bot" {
		t.Errorf("expected assigned='agent:sync-bot' on issue 1, got %q", iss1.Assigned)
	}
	if iss1.Urgency != issue.UrgencyHigh {
		t.Errorf("expected urgency=high on issue 1, got %q", iss1.Urgency)
	}

	// Verify issue 2 was updated.
	ref2 := issue.RefPrefix + id2
	data2, err := app.Store.ReadEntity(ref2, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id2: %v", err)
	}
	iss2, err := issue.Parse(data2)
	if err != nil {
		t.Fatalf("Parse id2: %v", err)
	}
	if iss2.Urgency != issue.UrgencyLow {
		t.Errorf("expected urgency=low on issue 2, got %q", iss2.Urgency)
	}
}

// TestIssueEditBatch_InvalidIDContinues verifies that an invalid issue_id
// produces a per-line error but the batch continues processing the next op.
func TestIssueEditBatch_InvalidIDContinues(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "Valid issue")

	batchInput := fmt.Sprintf(
		`{"issue_id": "deadbeef", "fields": {"urgency": "high"}}`+"\n"+
			`{"issue_id": "%s", "fields": {"urgency": "low"}}`,
		id[:8],
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch returned error: %v", err)
	}

	out := stdout.String()
	lines := nonEmptyLines(out)
	if len(lines) != 2 {
		t.Fatalf("expected 2 result lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "error") {
		t.Errorf("expected 'error' in first result line, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ok") {
		t.Errorf("expected 'ok' in second result line, got: %s", lines[1])
	}

	// The valid issue should have been updated regardless of the first error.
	ref := issue.RefPrefix + id
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if iss.Urgency != issue.UrgencyLow {
		t.Errorf("expected urgency=low after batch, got %q", iss.Urgency)
	}
}

// TestIssueEditBatch_StateChange verifies that a state transition in a batch
// operation uses ValidateTransition and records a Transition entry.
func TestIssueEditBatch_StateChange(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "State batch issue")

	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"state": "cancel"}}`,
		id[:8],
	)

	before := time.Now().Add(-time.Second)
	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	after := time.Now().Add(time.Second)
	if err != nil {
		t.Fatalf("issue edit --batch failed: %v", err)
	}

	out := stdout.String()
	lines := nonEmptyLines(out)
	if len(lines) != 1 || !strings.Contains(lines[0], "ok") {
		t.Fatalf("expected 1 'ok' line, got:\n%s", out)
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
	if iss.State != issue.StateCancelled {
		t.Errorf("expected state cancelled, got %q", iss.State)
	}
	if len(iss.Transitions) == 0 {
		t.Fatal("expected at least one transition recorded")
	}
	tr := iss.Transitions[len(iss.Transitions)-1]
	if tr.State != string(issue.StateCancelled) {
		t.Errorf("expected last transition state=cancelled, got %q", tr.State)
	}
	if tr.Timestamp.Before(before) || tr.Timestamp.After(after) {
		t.Errorf("transition timestamp %v outside expected range", tr.Timestamp)
	}
}

// TestIssueEditBatch_InvalidStateTransition verifies that an invalid state
// transition in a batch op reports an error per-line but does not abort.
func TestIssueEditBatch_InvalidStateTransition(t *testing.T) {
	app, _ := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "Already done issue")
	id2 := createEditTestIssue(t, app, "Normal issue")

	// Manually put id1 into done state.
	ref1 := issue.RefPrefix + id1
	data1, _ := app.Store.ReadEntity(ref1, "issue.md")
	iss1, _ := issue.Parse(data1)
	iss1.State = issue.StateDone
	out1, _ := issue.Marshal(iss1)
	_ = app.Store.WriteEntity(ref1, "issue.md", out1, "force done")

	// Use full UUIDs to avoid prefix-ambiguity between two closely-generated IDs.
	// Try to cancel the done issue (invalid transition) and assign the normal one.
	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"state": "cancel"}}`+"\n"+
			`{"issue_id": "%s", "fields": {"assigned": "agent:worker"}}`,
		id1, id2,
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch returned error: %v", err)
	}

	out := stdout.String()
	lines := nonEmptyLines(out)
	if len(lines) != 2 {
		t.Fatalf("expected 2 result lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "error") {
		t.Errorf("expected 'error' in first line, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ok") {
		t.Errorf("expected 'ok' in second line, got: %s", lines[1])
	}

	// Issue 2 should be assigned.
	ref2 := issue.RefPrefix + id2
	data2, err := app.Store.ReadEntity(ref2, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id2: %v", err)
	}
	iss2, err := issue.Parse(data2)
	if err != nil {
		t.Fatalf("Parse id2: %v", err)
	}
	if iss2.Assigned != "agent:worker" {
		t.Errorf("expected assigned='agent:worker' on issue 2, got %q", iss2.Assigned)
	}
}

// TestIssueEditBatch_EmptyStdin verifies that an empty stdin is a no-op with
// no output and no error.
func TestIssueEditBatch_EmptyStdin(t *testing.T) {
	app, _ := setupEditTest(t)

	stdout, _, err := runWithStdin(app, "", "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch with empty stdin failed: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no output for empty stdin, got: %q", stdout.String())
	}
}

// TestIssueEditBatch_Labels verifies that a "labels" field replaces the
// entire labels array on the issue.
func TestIssueEditBatch_Labels(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "Labels batch issue")

	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"labels": ["LOPS", "infra"]}}`,
		id[:8],
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch failed: %v", err)
	}
	lines := nonEmptyLines(stdout.String())
	if len(lines) != 1 || !strings.Contains(lines[0], "ok") {
		t.Fatalf("expected 1 'ok' line, got:\n%s", stdout.String())
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
	if len(iss.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(iss.Labels), iss.Labels)
	}
	if iss.Labels[0] != "LOPS" || iss.Labels[1] != "infra" {
		t.Errorf("expected labels [LOPS infra], got %v", iss.Labels)
	}
}

// TestIssueEditBatch_LastSyncedAt verifies that the last_synced_at field is
// correctly set from a batch operation.
func TestIssueEditBatch_LastSyncedAt(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "LastSyncedAt batch issue")
	syncTime := "2026-03-17T10:00:00Z"

	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"last_synced_at": "%s"}}`,
		id[:8], syncTime,
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch failed: %v", err)
	}
	lines := nonEmptyLines(stdout.String())
	if len(lines) != 1 || !strings.Contains(lines[0], "ok") {
		t.Fatalf("expected 1 'ok' line, got:\n%s", stdout.String())
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
	if iss.LastSyncedAt == nil {
		t.Fatal("expected LastSyncedAt to be set")
	}
	if iss.LastSyncedAt.Format(time.RFC3339) != syncTime {
		t.Errorf("expected last_synced_at=%s, got %s", syncTime, iss.LastSyncedAt.Format(time.RFC3339))
	}
}

// TestIssueEditBatch_TrackerID verifies that the tracker_id field is set.
func TestIssueEditBatch_TrackerID(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "TrackerID batch issue")

	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"tracker_id": "jira:LOPS-142"}}`,
		id[:8],
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch failed: %v", err)
	}
	lines := nonEmptyLines(stdout.String())
	if len(lines) != 1 || !strings.Contains(lines[0], "ok") {
		t.Fatalf("expected 1 'ok' line, got:\n%s", stdout.String())
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
	if iss.TrackerID != "jira:LOPS-142" {
		t.Errorf("expected tracker_id='jira:LOPS-142', got %q", iss.TrackerID)
	}
}

// TestIssueEditBatch_EmptyIssueID verifies that an operation with no issue_id
// produces a per-line error and does not silently operate on HEAD.
func TestIssueEditBatch_EmptyIssueID(t *testing.T) {
	app, _ := setupEditTest(t)

	batchInput := `{"fields": {"urgency": "high"}}`

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch returned unexpected error: %v", err)
	}

	out := stdout.String()
	lines := nonEmptyLines(out)
	if len(lines) != 1 {
		t.Fatalf("expected 1 result line, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "error") {
		t.Errorf("expected 'error' for empty issue_id, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "issue_id is required") {
		t.Errorf("expected 'issue_id is required' in error, got: %s", lines[0])
	}
}

// TestIssueEditBatch_InvalidUrgency verifies that an unknown urgency value
// produces a per-line error and does not update the issue.
func TestIssueEditBatch_InvalidUrgency(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "Urgency validation issue")

	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"urgency": "extreme"}}`,
		id[:8],
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch returned unexpected error: %v", err)
	}

	out := stdout.String()
	lines := nonEmptyLines(out)
	if len(lines) != 1 {
		t.Fatalf("expected 1 result line, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "error") {
		t.Errorf("expected 'error' for invalid urgency, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "invalid urgency") {
		t.Errorf("expected 'invalid urgency' in error message, got: %s", lines[0])
	}

	// The issue should not have been modified.
	ref := issue.RefPrefix + id
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if iss.Urgency != issue.UrgencyNormal {
		t.Errorf("urgency should remain normal after invalid update, got %q", iss.Urgency)
	}
}

// TestIssueEditBatch_Title verifies that a "title" field updates the issue
// title and does not return an "unsupported field" error.
func TestIssueEditBatch_Title(t *testing.T) {
	app, _ := setupEditTest(t)

	id := createEditTestIssue(t, app, "Original Title")

	batchInput := fmt.Sprintf(
		`{"issue_id": "%s", "fields": {"title": "Renamed Title"}}`,
		id[:8],
	)

	stdout, _, err := runWithStdin(app, batchInput, "issue", "edit", "--batch")
	if err != nil {
		t.Fatalf("issue edit --batch failed: %v", err)
	}
	lines := nonEmptyLines(stdout.String())
	if len(lines) != 1 || !strings.Contains(lines[0], "ok") {
		t.Fatalf("expected 1 'ok' line, got:\n%s", stdout.String())
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
	if iss.Title != "Renamed Title" {
		t.Errorf("expected title=%q, got %q", "Renamed Title", iss.Title)
	}
}

// nonEmptyLines splits a string into lines, dropping blank ones.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
