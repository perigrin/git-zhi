// ABOUTME: Tests for the Jira bidirectional sync logic.
// ABOUTME: Uses httptest.NewServer for real HTTP and real temp files for snapshots.
package sync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	jclient "github.com/perigrin/git-zhi/internal/jira/client"
	"github.com/perigrin/git-zhi/internal/jira/sync"
	"github.com/perigrin/git-zhi/internal/issue"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeIssue creates a minimal issue with the given id string, state, and
// tracker key (e.g. "jira:LOPS-142"). Labels and urgency use safe defaults.
func makeIssue(t *testing.T, idStr, state, trackerID string) *issue.Issue {
	t.Helper()
	id, err := uuid.FromString(idStr)
	if err != nil {
		t.Fatalf("makeIssue: invalid uuid %q: %v", idStr, err)
	}
	return &issue.Issue{
		ID:        id,
		Title:     "test issue",
		State:     issue.State(state),
		Urgency:   issue.UrgencyNormal,
		TrackerID: trackerID,
		Labels:    []string{},
	}
}

// jiraIssueFixture builds the JSON shape returned by GET /rest/api/3/issue/{key}.
func jiraIssueFixture(key, status, priority, assignee string, labels []string) map[string]interface{} {
	return map[string]interface{}{
		"key": key,
		"fields": map[string]interface{}{
			"summary":     "some summary",
			"description": nil,
			"status":      map[string]string{"name": status},
			"priority":    map[string]string{"name": priority},
			"assignee":    map[string]string{"displayName": assignee},
			"labels":      labels,
			"created":     "2026-03-01T10:00:00.000+0000",
			"updated":     "2026-03-15T14:30:00.000+0000",
		},
	}
}

// transitionsFixture returns the standard three-step workflow.
func transitionsFixture() map[string]interface{} {
	return map[string]interface{}{
		"transitions": []map[string]string{
			{"id": "11", "name": "To Do"},
			{"id": "21", "name": "In Progress"},
			{"id": "31", "name": "Done"},
		},
	}
}

// capturedTransitionID records the last transition id posted to the fake server.
type capturedTransitionID struct {
	ID string
}

// buildSyncServer returns a test server. The server tracks transition POSTs in
// cap (which may be nil if the caller does not need transition capture) and
// serves the given issue responses keyed by issue key.
func buildSyncServer(t *testing.T, issuesByKey map[string]map[string]interface{}, cap *capturedTransitionID) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		path := r.URL.Path

		switch {
		case strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(transitionsFixture())

		case strings.HasSuffix(path, "/transitions") && r.Method == http.MethodPost:
			if cap != nil {
				var body struct {
					Transition struct {
						ID string `json:"id"`
					} `json:"transition"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				cap.ID = body.Transition.ID
			}
			w.WriteHeader(http.StatusNoContent)

		case strings.HasPrefix(path, "/rest/api/3/issue/") && r.Method == http.MethodGet:
			key := strings.TrimPrefix(path, "/rest/api/3/issue/")
			fixture, found := issuesByKey[key]
			if !found {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(fixture)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// ---------------------------------------------------------------------------
// Snapshot tests
// ---------------------------------------------------------------------------

func TestSnapshotRoundtrip(t *testing.T) {
	dir := t.TempDir()

	fields := map[string]string{
		"state":   "in-progress",
		"urgency": "high",
		"labels":  "backend,infra",
	}

	issueID := "01900000-0000-7000-0000-000000000001"
	if err := sync.SaveSnapshot(dir, issueID, fields); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := sync.LoadSnapshot(dir, issueID)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	for k, v := range fields {
		if got := loaded[k]; got != v {
			t.Errorf("field %q: got %q, want %q", k, got, v)
		}
	}
}

func TestLoadSnapshotMissing(t *testing.T) {
	dir := t.TempDir()
	loaded, err := sync.LoadSnapshot(dir, "no-such-id")
	if err != nil {
		t.Fatalf("LoadSnapshot of missing file should return empty map, got error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty map for missing snapshot, got %v", loaded)
	}
}

func TestSnapshotDirectory(t *testing.T) {
	dir := t.TempDir()
	issueID := "01900000-0000-7000-0000-000000000099"
	fields := map[string]string{"state": "done"}

	if err := sync.SaveSnapshot(dir, issueID, fields); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// File must be under <snapshotDir>/jira/<issueID>.yaml
	expected := filepath.Join(dir, "jira", issueID+".yaml")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("snapshot file not found at %q: %v", expected, err)
	}
}

// ---------------------------------------------------------------------------
// FormatBatchEdits tests
// ---------------------------------------------------------------------------

func TestFormatBatchEditsEmpty(t *testing.T) {
	out := sync.FormatBatchEdits(nil)
	if len(out) != 0 {
		t.Errorf("expected empty output for nil updates, got %q", string(out))
	}
}

func TestFormatBatchEditsProducesJSONLines(t *testing.T) {
	updates := []sync.PullUpdate{
		{IssueID: "aaa", Field: "state", OldValue: "pending", NewValue: "in-progress"},
		{IssueID: "bbb", Field: "urgency", OldValue: "normal", NewValue: "high"},
	}
	out := sync.FormatBatchEdits(updates)
	if len(out) == 0 {
		t.Fatal("FormatBatchEdits returned empty output")
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSON lines, got %d: %q", len(lines), string(out))
	}

	for i, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v — %q", i, err, line)
		}
	}

	// First line must reference issue aaa with a nested fields map containing state.
	var first map[string]interface{}
	_ = json.Unmarshal([]byte(lines[0]), &first)
	if first["issue_id"] != "aaa" {
		t.Errorf("line 0 issue_id = %v, want aaa", first["issue_id"])
	}
	fields, ok := first["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("line 0 fields is not a map: %T %v", first["fields"], first["fields"])
	}
	// FormatBatchEdits converts state names to transition actions, so
	// "in-progress" becomes "start".
	if fields["state"] != "start" {
		t.Errorf("line 0 fields[state] = %v, want start", fields["state"])
	}
	// Flat field/value keys must not appear at the top level.
	if _, has := first["field"]; has {
		t.Error("line 0 must not have top-level 'field' key")
	}
	if _, has := first["value"]; has {
		t.Error("line 0 must not have top-level 'value' key")
	}
}

func TestFormatBatchEditsLabelsEmitsJSONArray(t *testing.T) {
	updates := []sync.PullUpdate{
		{IssueID: "aaa", Field: "labels", OldValue: "", NewValue: "backend,infra"},
	}
	out := sync.FormatBatchEdits(updates)
	if len(out) == 0 {
		t.Fatal("FormatBatchEdits returned empty output")
	}

	// The batch consumer (applyBatchOp) deserializes labels as []string.
	// FormatBatchEdits must emit labels as a JSON array, not a comma-separated
	// string, so the consumer can unmarshal it without error.
	var obj struct {
		IssueID string                     `json:"issue_id"`
		Fields  map[string]json.RawMessage `json:"fields"`
	}
	line := strings.TrimSpace(string(out))
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("FormatBatchEdits produced invalid JSON: %v — %q", err, line)
	}

	raw, ok := obj.Fields["labels"]
	if !ok {
		t.Fatal("FormatBatchEdits output missing 'labels' field")
	}

	var labels []string
	if err := json.Unmarshal(raw, &labels); err != nil {
		t.Fatalf("labels field is not a JSON array: %v — raw: %s", err, string(raw))
	}
	if len(labels) != 2 || labels[0] != "backend" || labels[1] != "infra" {
		t.Errorf("expected [backend, infra], got %v", labels)
	}
}

func TestFormatBatchEditsStateEmitsTransitionAction(t *testing.T) {
	// When Jira reports a status that maps to zhi state "in-progress",
	// FormatBatchEdits must emit the transition action "start" (not the state
	// name "in-progress"), because the batch consumer passes the value to
	// ValidateTransition which expects action names.
	updates := []sync.PullUpdate{
		{IssueID: "aaa", Field: "state", OldValue: "pending", NewValue: "in-progress"},
	}
	out := sync.FormatBatchEdits(updates)
	if len(out) == 0 {
		t.Fatal("FormatBatchEdits returned empty output")
	}

	var obj struct {
		Fields map[string]string `json:"fields"`
	}
	line := strings.TrimSpace(string(out))
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	stateVal := obj.Fields["state"]
	// The value must be a valid transition action, not a state name.
	validActions := map[string]bool{"start": true, "pause": true, "resume": true, "done": true, "cancel": true, "reopen": true}
	if !validActions[stateVal] {
		t.Errorf("state field value %q is not a valid transition action (got state name instead of action)", stateVal)
	}
}

// ---------------------------------------------------------------------------
// Pull tests
// ---------------------------------------------------------------------------

// TestPullDetectsJiraPriorityChange verifies that when Jira has a different
// priority than the snapshot (and zhi is unchanged), Pull emits a PullUpdate.
func TestPullDetectsJiraPriorityChange(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000002"
	trackerKey := "LOPS-100"

	iss := makeIssue(t, issueID, "in-progress", "jira:"+trackerKey)
	iss.Urgency = issue.UrgencyNormal // zhi has normal

	// Snapshot says priority was "normal" (i.e. Medium in Jira)
	_ = sync.SaveSnapshot(dir, issueID, map[string]string{
		"state":   "in-progress",
		"urgency": "normal",
		"labels":  "",
		"assigned": "",
	})

	// Jira now shows "High"
	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "In Progress", "High", "", []string{}),
	}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	result, err := sync.Pull(c, []*issue.Issue{iss}, dir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if len(result.Conflicts) != 0 {
		t.Errorf("unexpected conflicts: %v", result.Conflicts)
	}

	if len(result.Pulled) == 0 {
		t.Fatal("expected at least one PullUpdate for priority change, got none")
	}

	found := false
	for _, u := range result.Pulled {
		if u.IssueID == issueID && u.Field == "urgency" {
			found = true
			if u.NewValue != "high" {
				t.Errorf("PullUpdate.NewValue = %q, want %q", u.NewValue, "high")
			}
		}
	}
	if !found {
		t.Errorf("no PullUpdate for field urgency on issue %s; got %v", issueID, result.Pulled)
	}
}

// TestPullDetectsConflict verifies that when both zhi and Jira changed the
// same field since the snapshot, Pull emits a Conflict rather than a PullUpdate.
func TestPullDetectsConflict(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000003"
	trackerKey := "LOPS-200"

	iss := makeIssue(t, issueID, "done", "jira:"+trackerKey) // zhi says done

	// Snapshot says state was "in-progress" for both sides at last sync
	_ = sync.SaveSnapshot(dir, issueID, map[string]string{
		"state":   "in-progress",
		"urgency": "normal",
		"labels":  "",
		"assigned": "",
	})

	// Jira also shows a different state now: "Done" (mapped from zhi "done")
	// but let's simulate Jira moved to "Closed" while zhi moved to "done"
	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "Closed", "Medium", "", []string{}),
	}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")

	// We need a state mapping so Pull knows "Closed" is not the expected "Done"
	// This test verifies the conflict detection path: zhi changed state (in-progress → done)
	// AND Jira changed state (in-progress → Closed) — both changed the same field.
	result, err := sync.Pull(c, []*issue.Issue{iss}, dir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if len(result.Conflicts) == 0 {
		t.Fatal("expected at least one Conflict, got none")
	}

	found := false
	for _, c := range result.Conflicts {
		if c.IssueID == issueID && c.Field == "state" {
			found = true
			if c.ZhiValue != "done" {
				t.Errorf("Conflict.ZhiValue = %q, want %q", c.ZhiValue, "done")
			}
		}
	}
	if !found {
		t.Errorf("no Conflict for field state on issue %s; got %v", issueID, result.Conflicts)
	}
}

// TestPullSkipsIssuesWithoutTrackerID verifies that issues lacking a TrackerID
// are silently skipped — no error, no updates.
func TestPullSkipsIssuesWithoutTrackerID(t *testing.T) {
	dir := t.TempDir()

	iss := makeIssue(t, "01900000-0000-7000-0000-000000000004", "pending", "")

	srv := buildSyncServer(t, map[string]map[string]interface{}{}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	result, err := sync.Pull(c, []*issue.Issue{iss}, dir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Pulled) != 0 || len(result.Conflicts) != 0 {
		t.Errorf("expected no updates for issue without tracker id; got pulled=%v conflicts=%v", result.Pulled, result.Conflicts)
	}
}

// TestPullNoChanges verifies that when Jira and zhi match the snapshot
// exactly, no updates or conflicts are emitted.
func TestPullNoChanges(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000005"
	trackerKey := "LOPS-300"

	iss := makeIssue(t, issueID, "in-progress", "jira:"+trackerKey)

	_ = sync.SaveSnapshot(dir, issueID, map[string]string{
		"state":   "in-progress",
		"urgency": "normal",
		"labels":  "",
		"assigned": "",
	})

	// Jira matches the snapshot exactly: In Progress / Medium
	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "In Progress", "Medium", "", []string{}),
	}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	result, err := sync.Pull(c, []*issue.Issue{iss}, dir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Pulled) != 0 {
		t.Errorf("expected no PullUpdates, got %v", result.Pulled)
	}
	if len(result.Conflicts) != 0 {
		t.Errorf("expected no Conflicts, got %v", result.Conflicts)
	}
}

// TestPullSavesSnapshot verifies that Pull updates the snapshot after each
// comparison so a second pull with unchanged Jira state produces no updates.
func TestPullSavesSnapshot(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000010"
	trackerKey := "LOPS-600"

	iss := makeIssue(t, issueID, "in-progress", "jira:"+trackerKey)

	// Write an initial snapshot showing urgency=normal so that the first pull
	// can detect Jira's "high" as a change.
	_ = sync.SaveSnapshot(dir, issueID, map[string]string{
		"state":    "in-progress",
		"urgency":  "normal",
		"labels":   "",
		"assigned": "",
	})

	// Jira reports urgency=high — Jira side changed since snapshot.
	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "In Progress", "High", "", []string{}),
	}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")

	// First pull: detects high urgency (snapshot said normal, Jira now high).
	result1, err := sync.Pull(c, []*issue.Issue{iss}, dir)
	if err != nil {
		t.Fatalf("first Pull: %v", err)
	}
	if len(result1.Pulled) == 0 {
		t.Fatal("expected at least one PullUpdate on first pull")
	}

	// Second pull: snapshot now matches Jira — no updates expected.
	result2, err := sync.Pull(c, []*issue.Issue{iss}, dir)
	if err != nil {
		t.Fatalf("second Pull: %v", err)
	}
	if len(result2.Pulled) != 0 {
		t.Errorf("expected no PullUpdates on second pull (snapshot converged), got %v", result2.Pulled)
	}
	if len(result2.Conflicts) != 0 {
		t.Errorf("expected no Conflicts on second pull, got %v", result2.Conflicts)
	}
}

// ---------------------------------------------------------------------------
// Push tests
// ---------------------------------------------------------------------------

// TestPushMapsStateToJiraTransition verifies that Push calls DoTransition
// with the correct transition id for the mapped zhi state.
func TestPushMapsStateToJiraTransition(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000006"
	trackerKey := "LOPS-400"

	iss := makeIssue(t, issueID, "done", "jira:"+trackerKey)
	// Record a LastSyncedAt in the past so the issue is "changed since last sync"
	past := time.Now().Add(-time.Hour)
	iss.LastSyncedAt = &past

	cap := &capturedTransitionID{}
	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "In Progress", "Medium", "", []string{}),
	}, cap)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")

	mapping := sync.StateMapping{
		"done":        "Done",
		"in-progress": "In Progress",
		"pending":     "To Do",
	}

	result, err := sync.Push(c, []*issue.Issue{iss}, mapping, dir)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if len(result.Pushed) == 0 {
		t.Fatal("expected at least one PushUpdate, got none")
	}

	// Verify the transition id posted was "31" (Done)
	if cap.ID != "31" {
		t.Errorf("posted transition id = %q, want %q", cap.ID, "31")
	}

	found := false
	for _, p := range result.Pushed {
		if p.TrackerKey == trackerKey && p.Field == "state" {
			found = true
			if p.Value != "Done" {
				t.Errorf("PushUpdate.Value = %q, want %q", p.Value, "Done")
			}
		}
	}
	if !found {
		t.Errorf("no PushUpdate for tracker key %s field state; got %v", trackerKey, result.Pushed)
	}
}

// TestPushSkipsIssuesWithoutTrackerID verifies that issues without TrackerID
// are silently skipped.
func TestPushSkipsIssuesWithoutTrackerID(t *testing.T) {
	dir := t.TempDir()

	iss := makeIssue(t, "01900000-0000-7000-0000-000000000007", "done", "")
	past := time.Now().Add(-time.Hour)
	iss.LastSyncedAt = &past

	srv := buildSyncServer(t, map[string]map[string]interface{}{}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	result, err := sync.Push(c, []*issue.Issue{iss}, sync.StateMapping{"done": "Done"}, dir)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(result.Pushed) != 0 {
		t.Errorf("expected no PushUpdates for issue without tracker id, got %v", result.Pushed)
	}
}

// TestPushSavesSnapshot verifies that a successful Push writes a snapshot so
// subsequent pulls do not echo the pushed state back as an inbound change.
func TestPushSavesSnapshot(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000011"
	trackerKey := "LOPS-700"

	iss := makeIssue(t, issueID, "done", "jira:"+trackerKey)

	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "In Progress", "Medium", "", []string{}),
	}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	mapping := sync.StateMapping{"done": "Done", "in-progress": "In Progress"}

	_, err := sync.Push(c, []*issue.Issue{iss}, mapping, dir)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Snapshot must now record state=done so Pull won't echo it back.
	snap, err := sync.LoadSnapshot(dir, issueID)
	if err != nil {
		t.Fatalf("LoadSnapshot after Push: %v", err)
	}
	if snap["state"] != "done" {
		t.Errorf("snapshot state = %q, want %q", snap["state"], "done")
	}
}

// TestPushSkipsAlreadySyncedState verifies that Push skips an issue whose state
// matches the snapshot, preventing redundant Jira API calls.
func TestPushSkipsAlreadySyncedState(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000012"
	trackerKey := "LOPS-800"

	iss := makeIssue(t, issueID, "done", "jira:"+trackerKey)

	// Write a snapshot that already records state=done.
	_ = sync.SaveSnapshot(dir, issueID, map[string]string{
		"state":    "done",
		"urgency":  "normal",
		"labels":   "",
		"assigned": "",
	})

	cap := &capturedTransitionID{}
	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "Done", "Medium", "", []string{}),
	}, cap)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	mapping := sync.StateMapping{"done": "Done"}

	result, err := sync.Push(c, []*issue.Issue{iss}, mapping, dir)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(result.Pushed) != 0 {
		t.Errorf("expected no pushes when state already synced, got %v", result.Pushed)
	}
	if cap.ID != "" {
		t.Errorf("expected no DoTransition call when state matches snapshot, got transition id=%q", cap.ID)
	}
}

// TestPushSkipsUnmappedState verifies that a zhi state with no mapping in
// StateMapping is skipped without error.
func TestPushSkipsUnmappedState(t *testing.T) {
	dir := t.TempDir()

	issueID := "01900000-0000-7000-0000-000000000008"
	trackerKey := "LOPS-500"

	iss := makeIssue(t, issueID, "reopened", "jira:"+trackerKey)
	past := time.Now().Add(-time.Hour)
	iss.LastSyncedAt = &past

	srv := buildSyncServer(t, map[string]map[string]interface{}{
		trackerKey: jiraIssueFixture(trackerKey, "In Progress", "Medium", "", []string{}),
	}, nil)
	defer srv.Close()

	c := jclient.NewClient(srv.URL, "user@example.com", "token")
	// "reopened" is not in the mapping
	result, err := sync.Push(c, []*issue.Issue{iss}, sync.StateMapping{"done": "Done"}, dir)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(result.Pushed) != 0 {
		t.Errorf("expected no PushUpdates for unmapped state, got %v", result.Pushed)
	}
}
