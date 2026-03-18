// ABOUTME: Integration tests for the git-zhi-jira Cobra command tree.
// ABOUTME: Uses httptest.NewServer for Jira API calls and real on-disk git repos.
package jira_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	jira "github.com/perigrin/git-zhi/internal/jira"
	"github.com/perigrin/git-zhi/internal/storage"
)

// ---------------------------------------------------------------------------
// Test repo helpers
// ---------------------------------------------------------------------------

// makeTestRepo creates a minimal git repo with one commit and returns the App
// and snapshot directory path.
func makeTestRepo(t *testing.T) (*cli.App, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}
	// Make an initial commit so the repo is non-empty.
	wt, _ := repo.Worktree()
	_ = os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0644)
	_, _ = wt.Add("README")
	_, _ = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	snapshotDir := filepath.Join(dir, ".git", "zhi-sync")
	return app, snapshotDir
}

// writeIssue persists a minimal issue with the given tracker ID and state to
// the test store, returning the written issue.
func writeIssue(t *testing.T, store *storage.Store, trackerID, state, title string) *issue.Issue {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     issue.State(state),
		Urgency:   issue.UrgencyNormal,
		Milestone: "v0.1",
		TrackerID: trackerID,
		Labels:    []string{},
		Transitions: []issue.Transition{},
		ObservedPaths: []string{},
		Created:   time.Now(),
		Updated:   time.Now(),
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	ref := issue.RefPrefix + id.String()
	if err := store.WriteEntity(ref, "issue.md", data, "add test issue"); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return iss
}

// runJira executes the jira command tree with the given args against the
// provided App and returns stdout, stderr, and the execution error.
func runJira(t *testing.T, app *cli.App, snapshotDir string, jiraURL string, args ...string) (string, string, error) {
	t.Helper()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := jira.NewJiraCommand()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	ctx := cli.WithApp(context.Background(), app)
	cmd.SetContext(ctx)
	// Inject snapshot dir and jira URL via command-level flags.
	allArgs := append([]string{"--snapshot-dir", snapshotDir, "--jira-url", jiraURL}, args...)
	cmd.SetArgs(allArgs)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// ---------------------------------------------------------------------------
// Fake Jira server helpers
// ---------------------------------------------------------------------------

// jiraIssueFixture returns the minimal JSON shape for a Jira issue.
func jiraIssueFixture(key, summary, status, priority string) map[string]interface{} {
	return map[string]interface{}{
		"key": key,
		"fields": map[string]interface{}{
			"summary":     summary,
			"description": nil,
			"status":      map[string]string{"name": status},
			"priority":    map[string]string{"name": priority},
			"assignee":    nil,
			"labels":      []string{},
			"created":     "2026-03-01T10:00:00.000+0000",
			"updated":     "2026-03-15T14:30:00.000+0000",
		},
	}
}

// transitionsFixture returns a standard transitions response with target
// status names that differ from transition action names (matching real Jira
// workflow behavior).
func transitionsFixture() map[string]interface{} {
	return map[string]interface{}{
		"transitions": []map[string]interface{}{
			{"id": "11", "name": "Backlog", "to": map[string]string{"name": "To Do"}},
			{"id": "21", "name": "Start Progress", "to": map[string]string{"name": "In Progress"}},
			{"id": "31", "name": "Resolve Issue", "to": map[string]string{"name": "Done"}},
		},
	}
}

// buildFakeJiraServer starts an httptest server that serves issue GET requests
// from issuesByKey, transitions, and records any transition POST in cap.
type capturedTransition struct {
	Key string
	ID  string
}

func buildFakeJiraServer(t *testing.T, issuesByKey map[string]map[string]interface{}, cap *capturedTransition) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Require basic auth.
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
				// Extract the key from the path: /rest/api/3/issue/{key}/transitions
				parts := strings.Split(path, "/")
				if len(parts) >= 6 {
					cap.Key = parts[5]
				}
				cap.ID = body.Transition.ID
			}
			w.WriteHeader(http.StatusNoContent)

		case strings.HasPrefix(path, "/rest/api/3/issue/") && r.Method == http.MethodGet:
			remainder := strings.TrimPrefix(path, "/rest/api/3/issue/")
			// Strip any trailing /transitions already handled above.
			key := strings.Split(remainder, "/")[0]
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
// Tests: jira <ticket-key>
// ---------------------------------------------------------------------------

// TestJiraTicket_emitsValidIssueYAML verifies that 'jira <ticket-key>' fetches
// the Jira issue and emits valid YAML frontmatter parseable by issue.Parse.
func TestJiraTicket_emitsValidIssueYAML(t *testing.T) {
	fixtures := map[string]map[string]interface{}{
		"LOPS-42": jiraIssueFixture("LOPS-42", "Deploy logging stack", "In Progress", "High"),
	}
	srv := buildFakeJiraServer(t, fixtures, nil)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	app, snapDir := makeTestRepo(t)
	stdout, _, err := runJira(t, app, snapDir, srv.URL, "LOPS-42")
	if err != nil {
		t.Fatalf("jira LOPS-42: unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "title:") {
		t.Errorf("output missing 'title:' field:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Deploy logging stack") {
		t.Errorf("output missing issue summary:\n%s", stdout)
	}
	if !strings.Contains(stdout, "LOPS-42") {
		t.Errorf("output missing tracker_id:\n%s", stdout)
	}

	// Parse the emitted YAML to confirm it's valid.
	_, parseErr := issue.Parse([]byte(stdout))
	if parseErr != nil {
		t.Errorf("issue.Parse on output failed: %v\noutput:\n%s", parseErr, stdout)
	}
}

// TestJiraTicket_notFound verifies that a 404 from Jira returns a clear error.
func TestJiraTicket_notFound(t *testing.T) {
	srv := buildFakeJiraServer(t, map[string]map[string]interface{}{}, nil)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	app, snapDir := makeTestRepo(t)
	_, _, err := runJira(t, app, snapDir, srv.URL, "NOPE-999")
	if err == nil {
		t.Fatal("expected error for not-found ticket, got nil")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404 or not-found: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: jira sync pull
// ---------------------------------------------------------------------------

// TestJiraSyncPull_emitsBatchJSON verifies that 'jira sync pull' emits
// newline-delimited JSON lines consumable by 'git zhi issue edit --batch'.
// The test writes a snapshot showing the issue was previously "pending" in Jira,
// then Jira reports "In Progress" — so Pull detects the Jira-side change.
func TestJiraSyncPull_emitsBatchJSON(t *testing.T) {
	app, snapDir := makeTestRepo(t)

	// Write an issue with a tracker ID pointing to LOPS-10.
	iss := writeIssue(t, app.Store, "jira:LOPS-10", "pending", "deploy stack")

	// Write a prior snapshot so Pull can detect that Jira changed state from
	// "pending" to "in-progress" (only-Jira-changed → emit PullUpdate).
	_ = os.MkdirAll(filepath.Join(snapDir, "jira"), 0o755)
	snapContent := "state: pending\nurgency: normal\nassigned: \"\"\nlabels: \"\"\n"
	_ = os.WriteFile(filepath.Join(snapDir, "jira", iss.ID.String()+".yaml"), []byte(snapContent), 0o644)

	fixtures := map[string]map[string]interface{}{
		"LOPS-10": jiraIssueFixture("LOPS-10", "deploy stack", "In Progress", "High"),
	}
	srv := buildFakeJiraServer(t, fixtures, nil)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, srv.URL, "sync", "pull")
	if err != nil {
		t.Fatalf("jira sync pull: unexpected error: %v", err)
	}

	lines := nonEmptyLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("expected at least one batch edit line, got none; issue_id=%s", iss.ID)
	}
	for _, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line is not valid JSON: %q: %v", line, err)
		}
		if _, ok := obj["issue_id"]; !ok {
			t.Errorf("batch line missing 'issue_id': %q", line)
		}
		// Batch lines must use the nested fields map (not flat field/value keys).
		fields, ok := obj["fields"].(map[string]interface{})
		if !ok || len(fields) == 0 {
			t.Errorf("batch line missing non-empty 'fields' map: %q", line)
		}
		if _, has := obj["field"]; has {
			t.Errorf("batch line must not have top-level 'field' key: %q", line)
		}
		if _, has := obj["value"]; has {
			t.Errorf("batch line must not have top-level 'value' key: %q", line)
		}
	}
}

// TestJiraSyncPull_noIssuesWithTracker verifies no output when no issues have a
// tracker_id.
func TestJiraSyncPull_noIssuesWithTracker(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	writeIssue(t, app.Store, "", "pending", "no tracker issue")

	srv := buildFakeJiraServer(t, map[string]map[string]interface{}{}, nil)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, srv.URL, "sync", "pull")
	if err != nil {
		t.Fatalf("jira sync pull (no tracker issues): unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected no output, got: %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Tests: jira sync push
// ---------------------------------------------------------------------------

// TestJiraSyncPush_callsTransition verifies that 'jira sync push' sends a
// transition request to Jira when the issue state has a configured mapping.
func TestJiraSyncPush_callsTransition(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	writeIssue(t, app.Store, "jira:LOPS-20", "done", "finished task")

	cap := &capturedTransition{}
	fixtures := map[string]map[string]interface{}{
		"LOPS-20": jiraIssueFixture("LOPS-20", "finished task", "Done", "Medium"),
	}
	srv := buildFakeJiraServer(t, fixtures, cap)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	_, _, err := runJira(t, app, snapDir, srv.URL, "sync", "push")
	if err != nil {
		t.Fatalf("jira sync push: unexpected error: %v", err)
	}
	if cap.ID == "" {
		t.Error("expected a transition POST to be made to Jira, none observed")
	}
}

// TestJiraSyncPush_reportsResults verifies that 'jira sync push' reports the
// key and value for each pushed update.
func TestJiraSyncPush_reportsResults(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	writeIssue(t, app.Store, "jira:LOPS-21", "done", "another done task")

	cap := &capturedTransition{}
	fixtures := map[string]map[string]interface{}{
		"LOPS-21": jiraIssueFixture("LOPS-21", "another done task", "Done", "Low"),
	}
	srv := buildFakeJiraServer(t, fixtures, cap)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, srv.URL, "sync", "push")
	if err != nil {
		t.Fatalf("jira sync push: unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "LOPS-21") {
		t.Errorf("push output should mention the key: %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Tests: jira sync --dry-run
// ---------------------------------------------------------------------------

// TestJiraSyncDryRun_noSideEffects verifies that --dry-run previews both
// directions without making any API calls or writing snapshots.
func TestJiraSyncDryRun_noSideEffects(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	writeIssue(t, app.Store, "jira:LOPS-30", "done", "dry run task")

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError) // should never be reached
	}))
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, srv.URL, "sync", "--dry-run")
	if err != nil {
		t.Fatalf("jira sync --dry-run: unexpected error: %v", err)
	}
	if called {
		t.Error("dry-run should not make any HTTP calls")
	}
	// Should print some preview output.
	if strings.TrimSpace(stdout) == "" {
		t.Error("dry-run should emit preview output, got none")
	}
}

// ---------------------------------------------------------------------------
// Tests: jira resolve
// ---------------------------------------------------------------------------

// TestJiraResolve_keepZhi verifies that --keep-zhi emits a batch edit clearing
// the conflict marker by keeping the zhi value.
func TestJiraResolve_keepZhi(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	iss := writeIssue(t, app.Store, "jira:LOPS-40", "done", "conflict issue")

	// Write a snapshot with a different value to create a conflict scenario.
	_ = os.MkdirAll(filepath.Join(snapDir, "jira"), 0o755)
	snapContent := "state: pending\nurgency: normal\nassigned: \nlabels: \n"
	_ = os.WriteFile(filepath.Join(snapDir, "jira", iss.ID.String()+".yaml"), []byte(snapContent), 0o644)

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, "http://unused", "resolve", "LOPS-40", "--keep-zhi")
	if err != nil {
		t.Fatalf("jira resolve --keep-zhi: unexpected error: %v", err)
	}
	// Should emit a batch edit (or at minimum a JSON line referencing the issue).
	if strings.TrimSpace(stdout) == "" {
		t.Error("resolve --keep-zhi should emit output, got none")
	}
}

// TestJiraResolve_keepTracker verifies that --keep-tracker emits a batch edit
// applying the Jira value to the zhi issue.
func TestJiraResolve_keepTracker(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	iss := writeIssue(t, app.Store, "jira:LOPS-41", "pending", "conflict issue 2")

	// Write a snapshot showing the issue was previously "in-progress" so Jira
	// changed it to "done" and zhi changed it to "pending" → conflict.
	_ = os.MkdirAll(filepath.Join(snapDir, "jira"), 0o755)
	snapContent := "state: in-progress\nurgency: normal\nassigned: \nlabels: \n"
	_ = os.WriteFile(filepath.Join(snapDir, "jira", iss.ID.String()+".yaml"), []byte(snapContent), 0o644)

	fixtures := map[string]map[string]interface{}{
		"LOPS-41": jiraIssueFixture("LOPS-41", "conflict issue 2", "Done", "Medium"),
	}
	srv := buildFakeJiraServer(t, fixtures, nil)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, srv.URL, "resolve", "LOPS-41", "--keep-tracker")
	if err != nil {
		t.Fatalf("jira resolve --keep-tracker: unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("resolve --keep-tracker should emit output, got none")
	}
	// The emitted batch edit should reference the issue ID.
	if !strings.Contains(stdout, iss.ID.String()) {
		t.Errorf("output should reference issue ID %s:\n%s", iss.ID, stdout)
	}
}

// ---------------------------------------------------------------------------
// Tests: jira enrich
// ---------------------------------------------------------------------------

// TestJiraEnrich_updatesTitleFromJira verifies that 'jira enrich <milestone>'
// emits batch edits that update the title field from Jira metadata.
func TestJiraEnrich_updatesTitleFromJira(t *testing.T) {
	app, snapDir := makeTestRepo(t)
	iss := writeIssue(t, app.Store, "jira:LOPS-50", "pending", "old title")

	fixtures := map[string]map[string]interface{}{
		"LOPS-50": jiraIssueFixture("LOPS-50", "New Title From Jira", "To Do", "Medium"),
	}
	srv := buildFakeJiraServer(t, fixtures, nil)
	defer srv.Close()

	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@test.com")
	t.Setenv("ZHI_JIRA_URL", "")

	stdout, _, err := runJira(t, app, snapDir, srv.URL, "enrich", "v0.1")
	if err != nil {
		t.Fatalf("jira enrich: unexpected error: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("expected batch edit lines from enrich, got none; issue_id=%s", iss.ID)
	}
	foundTitle := false
	for _, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line not valid JSON: %q", line)
			continue
		}
		// Batch lines use the nested fields map format.
		fields, _ := obj["fields"].(map[string]interface{})
		if fields["title"] == "New Title From Jira" {
			foundTitle = true
		}
	}
	if !foundTitle {
		t.Errorf("expected a batch edit with fields.title='New Title From Jira'; got:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Tests: missing credentials
// ---------------------------------------------------------------------------

// TestJiraTicket_missingCredentials verifies a clear error when credentials
// are not set.
func TestJiraTicket_missingCredentials(t *testing.T) {
	t.Setenv("ZHI_JIRA_TOKEN", "")
	t.Setenv("ZHI_JIRA_EMAIL", "")
	t.Setenv("ZHI_JIRA_URL", "")

	app, snapDir := makeTestRepo(t)
	_, _, err := runJira(t, app, snapDir, "", "LOPS-99")
	if err == nil {
		t.Fatal("expected error when credentials are missing, got nil")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nonEmptyLines returns the non-empty lines from s.
func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
