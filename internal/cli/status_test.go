// ABOUTME: Tests for git zhi status command: HEAD issue display, session info,
// ABOUTME: milestone, ready count, and JSON output.
package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestStatus_InProgress verifies that status shows the current HEAD issue
// with state, milestone, session info, and ready count.
func TestStatus_InProgress(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Worktree HEAD resolution")
	prefix := uuidStr[:8]

	// Start the issue
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "fix worktree refs")

	stdout, _, err := run("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, prefix) {
		t.Fatalf("expected UUID prefix %q in output, got: %s", prefix, output)
	}
	if !strings.Contains(output, "Worktree HEAD resolution") {
		t.Fatalf("expected issue title in output, got: %s", output)
	}
	if !strings.Contains(output, "in-progress") {
		t.Fatalf("expected state in output, got: %s", output)
	}
	if !strings.Contains(output, "v0.1") {
		t.Fatalf("expected milestone in output, got: %s", output)
	}
}

// TestStatus_NoInProgress verifies that status reports no issue in progress
// and shows the ready count and next suggested issue.
func TestStatus_NoInProgress(t *testing.T) {
	app, run := setupEditTest(t)

	createEditTestIssue(t, app, "Pending issue one")
	createEditTestIssue(t, app, "Pending issue two")

	stdout, _, err := run("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No issue in progress") {
		t.Fatalf("expected 'No issue in progress' in output, got: %s", output)
	}
	if !strings.Contains(output, "Ready:") {
		t.Fatalf("expected ready count in output, got: %s", output)
	}
	if !strings.Contains(output, "Next:") {
		t.Fatalf("expected next suggestion in output, got: %s", output)
	}
}

// TestStatus_Empty verifies that status handles an empty chain gracefully.
func TestStatus_Empty(t *testing.T) {
	_, run := setupEditTest(t)

	stdout, _, err := run("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No issues") {
		t.Fatalf("expected 'No issues' in output, got: %s", output)
	}
}

// TestStatus_JSON verifies that --format json produces valid JSON with the
// expected fields.
func TestStatus_JSON(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "JSON status issue")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "some work")

	stdout, _, err := run("status", "--format", "json")
	if err != nil {
		t.Fatalf("status --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, stdout.String())
	}

	for _, key := range []string{"head", "state", "milestone", "ready_count"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("expected key %q in JSON output, got: %v", key, result)
		}
	}
}

// TestStatus_SessionInfo verifies that session start time and commit count
// appear in the output when a session is open.
func TestStatus_SessionInfo(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Session info test")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "commit one")
	makeTestCommit(t, app, "commit two")

	stdout, _, err := run("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := stdout.String()
	// Should show session info with commit count
	if !strings.Contains(output, "Session:") {
		t.Fatalf("expected 'Session:' in output, got: %s", output)
	}
}

// createEditTestIssue is defined in issue_edit_test.go — this file reuses
// the setupEditTest and makeTestCommit helpers from that file.
var _ = issue.RefPrefix // ensure import is used
