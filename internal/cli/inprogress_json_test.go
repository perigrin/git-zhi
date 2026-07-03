// ABOUTME: Tests for in-progress state visibility in JSON output for issue list,
// ABOUTME: show, and status commands. Covers positive and negative scenarios.
package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestInProgressJSON_ShowAfterStart verifies that after --state start,
// issue show --format json returns state: "in-progress".
func TestInProgressJSON_ShowAfterStart(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Start then show JSON")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	stdout, _, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	state, ok := result["state"]
	if !ok {
		t.Fatalf("expected 'state' key in JSON, got: %v", result)
	}
	if state != string(issue.StateInProgress) {
		t.Fatalf("expected state %q, got %q", string(issue.StateInProgress), state)
	}
}

// TestInProgressJSON_ListAfterStart verifies that after --state start,
// issue list --format json includes the issue with state: "in-progress".
func TestInProgressJSON_ListAfterStart(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Start then list JSON")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	stdout, _, err := run("issue", "list", "--format", "json")
	if err != nil {
		t.Fatalf("issue list --format json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON array: %v\noutput: %s", err, stdout.String())
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 issue in list, got %d", len(results))
	}

	state, ok := results[0]["state"]
	if !ok {
		t.Fatalf("expected 'state' key in issue JSON, got: %v", results[0])
	}
	if state != string(issue.StateInProgress) {
		t.Fatalf("expected state %q, got %q", string(issue.StateInProgress), state)
	}
}

// TestInProgressJSON_StatusJSON verifies that git zhi status --format json
// returns state: "in-progress" for the HEAD issue after --state start.
func TestInProgressJSON_StatusJSON(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Start then status JSON")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	stdout, _, err := run("status", "--format", "json")
	if err != nil {
		t.Fatalf("status --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	state, ok := result["state"]
	if !ok {
		t.Fatalf("expected 'state' key in status JSON, got: %v", result)
	}
	if state != string(issue.StateInProgress) {
		t.Fatalf("expected state %q in status JSON, got %q", string(issue.StateInProgress), state)
	}
}

// TestInProgressJSON_StatusText verifies that git zhi status (human-readable)
// mentions the in-progress issue by title, not "No issue in progress".
func TestInProgressJSON_StatusText(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Status text in-progress check")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	stdout, _, err := run("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "No issue in progress") {
		t.Fatalf("expected in-progress issue in status, got 'No issue in progress'\noutput: %s", output)
	}
	if !strings.Contains(output, "Status text in-progress check") {
		t.Fatalf("expected issue title in status output, got:\n%s", output)
	}
	if !strings.Contains(output, string(issue.StateInProgress)) {
		t.Fatalf("expected %q in status output, got:\n%s", string(issue.StateInProgress), output)
	}
}

// TestInProgressJSON_PendingNeverLeaks verifies that a pending issue's JSON
// state field never reads "in-progress".
func TestInProgressJSON_PendingNeverLeaks(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Pending should not leak in-progress")
	prefix := uuidStr[:8]

	// Do NOT start the issue — it should stay pending.
	stdout, _, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	state, ok := result["state"]
	if !ok {
		t.Fatalf("expected 'state' key in JSON, got: %v", result)
	}
	if state == string(issue.StateInProgress) {
		t.Fatalf("pending issue must not have state %q; got %q", string(issue.StateInProgress), state)
	}
	if state != string(issue.StatePending) {
		t.Fatalf("expected state %q for pending issue, got %q", string(issue.StatePending), state)
	}
}

// TestInProgressJSON_StartPauseResumeRoundTrip verifies that the JSON state
// field accurately reflects each transition: pending → in-progress → in-progress (paused) → in-progress (resumed).
// Pause keeps the state as in-progress; this test tracks session bookmarks via show output.
func TestInProgressJSON_StartPauseResumeRoundTrip(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Start pause resume round trip")
	prefix := uuidStr[:8]

	// Step 1: start → in-progress.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "work")

	stdout, _, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("show after start failed: %v", err)
	}
	assertStateInJSON(t, stdout.Bytes(), string(issue.StateInProgress), "after start")

	// Step 2: pause → state stays in-progress.
	if _, _, err := run("issue", "edit", "--state", "pause", prefix); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	stdout, _, err = run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("show after pause failed: %v", err)
	}
	assertStateInJSON(t, stdout.Bytes(), string(issue.StateInProgress), "after pause")

	// Step 3: resume → state stays in-progress.
	if _, _, err := run("issue", "edit", "--state", "resume", prefix); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	stdout, _, err = run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("show after resume failed: %v", err)
	}
	assertStateInJSON(t, stdout.Bytes(), string(issue.StateInProgress), "after resume")
}

// TestInProgressJSON_NeverStartedNeverInProgress verifies that an issue that
// was never started does not appear with state in-progress in JSON.
func TestInProgressJSON_NeverStartedNeverInProgress(t *testing.T) {
	app, run := setupEditTest(t)

	// Create two issues; start only the second.
	// Use full UUID string (not 8-char prefix) to avoid ambiguity when two
	// UUIDs are created within the same millisecond in test environments.
	uuidStr1 := createEditTestIssue(t, app, "Never started issue")
	uuidStr2 := createEditTestIssue(t, app, "Actually started issue")

	if _, _, err := run("issue", "edit", "--state", "start", uuidStr2); err != nil {
		t.Fatalf("start second issue failed: %v", err)
	}

	stdout, _, err := run("issue", "show", "--format", "json", uuidStr1)
	if err != nil {
		t.Fatalf("show never-started issue failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if result["state"] == string(issue.StateInProgress) {
		t.Fatalf("never-started issue must not have state in-progress, got: %v", result["state"])
	}
}

// TestInProgressJSON_ListContainsInProgress verifies that issue list --format json
// includes the in-progress issue alongside pending issues in the default view.
func TestInProgressJSON_ListContainsInProgress(t *testing.T) {
	app, run := setupEditTest(t)

	// Use full UUID string (not 8-char prefix) to avoid ambiguity when two
	// UUIDs are created within the same millisecond in test environments.
	uuidStr1 := createEditTestIssue(t, app, "Pending issue in list")
	_ = uuidStr1
	uuidStr2 := createEditTestIssue(t, app, "In-progress issue in list")

	if _, _, err := run("issue", "edit", "--state", "start", uuidStr2); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	stdout, _, err := run("issue", "list", "--format", "json")
	if err != nil {
		t.Fatalf("issue list --format json failed: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 issues (1 pending + 1 in-progress), got %d\noutput: %s", len(results), stdout.String())
	}

	found := false
	for _, r := range results {
		if r["state"] == string(issue.StateInProgress) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected at least one in-progress issue in JSON list, got: %v", results)
	}
}

// assertStateInJSON is a helper that unmarshals JSON and asserts the state field.
func assertStateInJSON(t *testing.T, data []byte, wantState string, context string) {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON (%s): %v\ndata: %s", context, err, string(data))
	}
	state, ok := result["state"]
	if !ok {
		t.Fatalf("expected 'state' key in JSON (%s), got: %v", context, result)
	}
	if state != wantState {
		t.Fatalf("expected state %q (%s), got %q", wantState, context, state)
	}
}
