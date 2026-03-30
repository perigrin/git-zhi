// ABOUTME: Tests for the issue edit --state command: state transitions,
// ABOUTME: measurement session bookmarks, and JSON output.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/storage"
)

// setupEditTest creates a temporary git repo, makes an initial commit so
// RepoHEAD() works, and returns an App plus a run function.
func setupEditTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, *bytes.Buffer, error)) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	// Make an initial commit so RepoHEAD() resolves
	makeTestCommit(t, app, "initial commit")

	run := func(args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, stderr, err
	}
	return app, run
}

// makeTestCommit creates a real commit in the repo using go-git's worktree API.
func makeTestCommit(t *testing.T, app *cli.App, message string) {
	t.Helper()
	wt, err := app.Repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	filename := fmt.Sprintf("file-%d.txt", time.Now().UnixNano())
	filepath := wt.Filesystem.Root() + "/" + filename
	if err := os.WriteFile(filepath, []byte(message), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("stage file: %v", err)
	}
	_, err = wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// createEditTestIssue writes a pending issue directly to the store and returns its UUID string.
func createEditTestIssue(t *testing.T, app *cli.App, title string) string {
	t.Helper()
	id := createTestIssue(t, app, title, issue.StatePending, "")
	return id.String()
}

func TestIssueEdit_Start(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Parse basic sub declarations")
	prefix := uuidStr[:8]

	stdout, _, err := run("issue", "edit", "--state", "start", prefix)
	if err != nil {
		t.Fatalf("issue edit --state start failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Started") {
		t.Fatalf("expected 'Started' in output, got: %s", output)
	}
	if !strings.Contains(output, prefix) {
		t.Fatalf("expected UUID prefix in output, got: %s", output)
	}
	if !strings.Contains(output, "Parse basic sub declarations") {
		t.Fatalf("expected title in output, got: %s", output)
	}

	// Verify persisted state
	ref := issue.RefPrefix + uuidStr
	data, err2 := app.Store.ReadEntity(ref, "issue.md")
	if err2 != nil {
		t.Fatalf("ReadEntity: %v", err2)
	}
	iss, err2 := issue.Parse(data)
	if err2 != nil {
		t.Fatalf("Parse: %v", err2)
	}

	if iss.State != issue.StateInProgress {
		t.Fatalf("expected state in-progress, got %s", iss.State)
	}
	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session after start, got %d", len(iss.Sessions))
	}
	if iss.Sessions[0].StartSHA == "" {
		t.Fatal("expected StartSHA to be set")
	}
	if iss.Sessions[0].EndSHA != "" {
		t.Fatalf("expected EndSHA to be empty (open session), got %q", iss.Sessions[0].EndSHA)
	}
	if iss.Sessions[0].Commits != 0 {
		t.Fatalf("expected Commits=0 on open session, got %d", iss.Sessions[0].Commits)
	}
}

func TestIssueEdit_PauseAndResume(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Implement parser")
	prefix := uuidStr[:8]

	// Start the issue
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Make some commits so CountCommits returns > 0
	makeTestCommit(t, app, "commit A")
	makeTestCommit(t, app, "commit B")
	makeTestCommit(t, app, "commit C")

	// Pause the issue
	stdout, _, err := run("issue", "edit", "--state", "pause", prefix)
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Paused") {
		t.Fatalf("expected 'Paused' in output, got: %s", output)
	}

	// Verify first session is closed with commit count
	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if iss.State != issue.StateInProgress {
		t.Fatalf("expected state still in-progress after pause, got %s", iss.State)
	}
	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session after pause, got %d", len(iss.Sessions))
	}
	if iss.Sessions[0].EndSHA == "" {
		t.Fatal("expected EndSHA to be set after pause")
	}
	if iss.Sessions[0].Commits != 3 {
		t.Fatalf("expected Commits=3 after making 3 commits, got %d", iss.Sessions[0].Commits)
	}

	// Make more commits before resume
	makeTestCommit(t, app, "commit D")

	// Resume the issue
	stdout2, _, err2 := run("issue", "edit", "--state", "resume", prefix)
	if err2 != nil {
		t.Fatalf("resume failed: %v", err2)
	}

	output2 := stdout2.String()
	if !strings.Contains(output2, "Resumed") {
		t.Fatalf("expected 'Resumed' in output, got: %s", output2)
	}

	// Verify a second open session is added
	data2, _ := app.Store.ReadEntity(ref, "issue.md")
	iss2, _ := issue.Parse(data2)

	if iss2.State != issue.StateInProgress {
		t.Fatalf("expected state in-progress after resume, got %s", iss2.State)
	}
	if len(iss2.Sessions) != 2 {
		t.Fatalf("expected 2 sessions after resume, got %d", len(iss2.Sessions))
	}
	// First session still closed
	if iss2.Sessions[0].EndSHA == "" {
		t.Fatal("expected first session to remain closed")
	}
	// Second session open
	if iss2.Sessions[1].StartSHA == "" {
		t.Fatal("expected second session StartSHA to be set")
	}
	if iss2.Sessions[1].EndSHA != "" {
		t.Fatalf("expected second session EndSHA to be empty, got %q", iss2.Sessions[1].EndSHA)
	}
}

func TestIssueEdit_Done(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Complete the implementation")
	prefix := uuidStr[:8]

	// Start first
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Make a couple of commits
	makeTestCommit(t, app, "work commit 1")
	makeTestCommit(t, app, "work commit 2")

	// Mark done
	stdout, _, err := run("issue", "edit", "--state", "done", prefix)
	if err != nil {
		t.Fatalf("done failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Completed") {
		t.Fatalf("expected 'Completed' in output, got: %s", output)
	}
	if !strings.Contains(output, "Total:") {
		t.Fatalf("expected 'Total:' commit summary in output, got: %s", output)
	}

	// Verify persisted state
	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if iss.State != issue.StateDone {
		t.Fatalf("expected state done, got %s", iss.State)
	}
	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(iss.Sessions))
	}
	if iss.Sessions[0].EndSHA == "" {
		t.Fatal("expected EndSHA set on done")
	}
	if iss.Sessions[0].Commits != 2 {
		t.Fatalf("expected Commits=2 after making 2 commits, got %d", iss.Sessions[0].Commits)
	}
}

func TestIssueEdit_Cancel(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Cancelled feature")
	prefix := uuidStr[:8]

	stdout, _, err := run("issue", "edit", "--state", "cancel", prefix)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Cancelled") {
		t.Fatalf("expected 'Cancelled' in output, got: %s", output)
	}
	// Cancel should not mention commit counts or windows
	if strings.Contains(output, "Window:") {
		t.Fatalf("cancel output should not mention Window, got: %s", output)
	}
	if strings.Contains(output, "Total:") {
		t.Fatalf("cancel output should not mention Total, got: %s", output)
	}

	// Verify persisted state
	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if iss.State != issue.StateCancelled {
		t.Fatalf("expected state cancelled, got %s", iss.State)
	}
	// No sessions on cancel
	if len(iss.Sessions) != 0 {
		t.Fatalf("expected no sessions on cancel, got %d", len(iss.Sessions))
	}
}

func TestIssueEdit_InvalidTransition(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Already started")
	prefix := uuidStr[:8]

	// Start it first
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Try to start it again — invalid transition
	_, _, err := run("issue", "edit", "--state", "start", prefix)
	if err == nil {
		t.Fatal("expected error when starting an in-progress issue, got nil")
	}
	if !strings.Contains(err.Error(), "cannot start") {
		t.Fatalf("expected 'cannot start' in error, got: %v", err)
	}
}

func TestIssueEdit_JsonOutput(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "JSON state test")
	prefix := uuidStr[:8]

	stdout, _, err := run("issue", "edit", "--format", "json", "--state", "start", prefix)
	if err != nil {
		t.Fatalf("issue edit --format json --state start failed: %v", err)
	}

	var result issue.Issue
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}
	if result.State != issue.StateInProgress {
		t.Fatalf("expected state in-progress in JSON, got %s", result.State)
	}
	if result.Title != "JSON state test" {
		t.Fatalf("expected title 'JSON state test' in JSON, got %q", result.Title)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session in JSON, got %d", len(result.Sessions))
	}
}

func TestIssueEdit_CancelFromInProgress(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Cancelled in-progress feature")
	prefix := uuidStr[:8]

	// Start the issue so a session is open.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Make some commits while in progress.
	makeTestCommit(t, app, "work commit 1")
	makeTestCommit(t, app, "work commit 2")

	// Cancel the issue.
	stdout, _, err := run("issue", "edit", "--state", "cancel", prefix)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Cancelled") {
		t.Fatalf("expected 'Cancelled' in output, got: %s", output)
	}

	// Verify persisted state.
	ref := issue.RefPrefix + uuidStr
	data, err2 := app.Store.ReadEntity(ref, "issue.md")
	if err2 != nil {
		t.Fatalf("ReadEntity: %v", err2)
	}
	iss, err2 := issue.Parse(data)
	if err2 != nil {
		t.Fatalf("Parse: %v", err2)
	}

	if iss.State != issue.StateCancelled {
		t.Fatalf("expected state cancelled, got %s", iss.State)
	}
	// Session should be closed (EndSHA non-empty).
	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session after cancel-from-in-progress, got %d", len(iss.Sessions))
	}
	if iss.Sessions[0].EndSHA == "" {
		t.Fatal("expected session EndSHA to be set after cancel from in-progress")
	}
}

func TestIssueEdit_Start_SetsStartedAt(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Timestamp start test")
	prefix := uuidStr[:8]

	before := time.Now().Add(-time.Second)
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	after := time.Now().Add(time.Second)

	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(iss.Sessions))
	}
	sess := iss.Sessions[0]
	if sess.StartedAt == nil {
		t.Fatal("expected StartedAt to be set after start")
	}
	if sess.StartedAt.Before(before) || sess.StartedAt.After(after) {
		t.Fatalf("StartedAt %v outside expected range [%v, %v]", sess.StartedAt, before, after)
	}
	if sess.EndedAt != nil {
		t.Fatalf("expected EndedAt to be nil on open session, got %v", sess.EndedAt)
	}
}

func TestIssueEdit_Pause_SetsEndedAt(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Timestamp pause test")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	makeTestCommit(t, app, "work commit")

	before := time.Now().Add(-time.Second)
	if _, _, err := run("issue", "edit", "--state", "pause", prefix); err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	after := time.Now().Add(time.Second)

	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	sess := iss.Sessions[0]
	if sess.EndedAt == nil {
		t.Fatal("expected EndedAt to be set after pause")
	}
	if sess.EndedAt.Before(before) || sess.EndedAt.After(after) {
		t.Fatalf("EndedAt %v outside expected range [%v, %v]", sess.EndedAt, before, after)
	}
}

func TestIssueEdit_Done_SetsEndedAt(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Timestamp done test")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	makeTestCommit(t, app, "work commit")

	before := time.Now().Add(-time.Second)
	if _, _, err := run("issue", "edit", "--state", "done", prefix); err != nil {
		t.Fatalf("done failed: %v", err)
	}
	after := time.Now().Add(time.Second)

	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	sess := iss.Sessions[0]
	if sess.EndedAt == nil {
		t.Fatal("expected EndedAt to be set after done")
	}
	if sess.EndedAt.Before(before) || sess.EndedAt.After(after) {
		t.Fatalf("EndedAt %v outside expected range [%v, %v]", sess.EndedAt, before, after)
	}
}

func TestIssueEdit_Cancel_SetsEndedAt(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Timestamp cancel test")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	makeTestCommit(t, app, "work commit")

	before := time.Now().Add(-time.Second)
	if _, _, err := run("issue", "edit", "--state", "cancel", prefix); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	after := time.Now().Add(time.Second)

	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(iss.Sessions))
	}
	sess := iss.Sessions[0]
	if sess.EndedAt == nil {
		t.Fatal("expected EndedAt to be set after cancel from in-progress")
	}
	if sess.EndedAt.Before(before) || sess.EndedAt.After(after) {
		t.Fatalf("EndedAt %v outside expected range [%v, %v]", sess.EndedAt, before, after)
	}
}

func TestIssueEdit_DoubleResume(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Double resume attempt")
	prefix := uuidStr[:8]

	// Start the issue — this opens a session.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Attempt to resume while a session is already open — should fail.
	_, _, err := run("issue", "edit", "--state", "resume", prefix)
	if err == nil {
		t.Fatal("expected error when resuming with a session already open, got nil")
	}
	if !strings.Contains(err.Error(), "measurement session is already open") {
		t.Fatalf("expected 'measurement session is already open' in error, got: %v", err)
	}
}

func TestIssueEdit_Done_RecordsObservedPaths(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Feature with observed paths")
	prefix := uuidStr[:8]

	// Start the issue.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Make commits that touch specific files — makeTestCommit creates unique
	// files so these will appear in the diff between start SHA and done SHA.
	makeTestCommit(t, app, "work commit 1")
	makeTestCommit(t, app, "work commit 2")

	// Mark done — should populate ObservedPaths with files touched.
	if _, _, err := run("issue", "edit", "--state", "done", prefix); err != nil {
		t.Fatalf("done failed: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if iss.State != issue.StateDone {
		t.Fatalf("expected state done, got %s", iss.State)
	}
	// ObservedPaths should be non-empty because we made commits that touched files.
	if len(iss.ObservedPaths) == 0 {
		t.Fatal("expected ObservedPaths to be populated after done, got empty slice")
	}
}

func TestIssueEdit_Done_MultiSession_ObservedPaths(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Multi-session observed paths")
	prefix := uuidStr[:8]

	// Session 1: start → make commits → pause.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "session 1 commit")

	if _, _, err := run("issue", "edit", "--state", "pause", prefix); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	// Commit between sessions (not attributed to this issue).
	makeTestCommit(t, app, "between sessions commit")

	// Session 2: resume → make commits → done.
	if _, _, err := run("issue", "edit", "--state", "resume", prefix); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	makeTestCommit(t, app, "session 2 commit")

	if _, _, err := run("issue", "edit", "--state", "done", prefix); err != nil {
		t.Fatalf("done failed: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// ObservedPaths should span the full range (first session start → current HEAD).
	if len(iss.ObservedPaths) == 0 {
		t.Fatal("expected ObservedPaths to be populated after multi-session done")
	}
	// Should have at least 3 files (one per commit in the diff range, including
	// the between-sessions commit which is within the first-SHA..HEAD range).
	if len(iss.ObservedPaths) < 3 {
		t.Fatalf("expected at least 3 observed paths (commits span first session to HEAD), got %d: %v",
			len(iss.ObservedPaths), iss.ObservedPaths)
	}
}

func TestIssueEdit_Done_NoSession_ObservedPathsEmpty(t *testing.T) {
	app, run := setupEditTest(t)

	// Create an issue and transition it directly to done without starting
	// (simulate a direct pending→done via reopen→done path or force via store).
	// Since the state machine requires start before done, we test that when
	// there are no sessions the ObservedPaths remains empty rather than panicking.
	uuidStr := createEditTestIssue(t, app, "Direct done no sessions")

	// Manually write the issue in in-progress state with no sessions so we
	// can transition to done without going through start.
	ref := issue.RefPrefix + uuidStr
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	iss.State = issue.StateInProgress
	iss.Sessions = nil // explicitly no sessions
	out, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := app.Store.WriteEntity(ref, "issue.md", out, "force in-progress no sessions"); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}

	prefix := uuidStr[:8]
	// Use --force because the zero-commit guard would otherwise reject this
	// transition (no sessions means no commits recorded).
	if _, _, err := run("issue", "edit", "--state", "done", "--force", prefix); err != nil {
		t.Fatalf("done failed: %v", err)
	}

	data2, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss2, err := issue.Parse(data2)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if iss2.State != issue.StateDone {
		t.Fatalf("expected state done, got %s", iss2.State)
	}
	// No sessions means no first SHA to diff from; ObservedPaths must be empty.
	if len(iss2.ObservedPaths) != 0 {
		t.Fatalf("expected empty ObservedPaths when no sessions, got %v", iss2.ObservedPaths)
	}
}

// setupEditTestWithAuthor creates a temporary git repo with a specific git
// user.name and user.email configured, so tests can assert on actor values.
func setupEditTestWithAuthor(t *testing.T, name, email string) (*cli.App, func(args ...string) (*bytes.Buffer, *bytes.Buffer, error)) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Set git user config so the Store picks up the desired author.
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("get repo config: %v", err)
	}
	cfg.User.Name = name
	cfg.User.Email = email
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("set repo config: %v", err)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	// Make an initial commit so RepoHEAD() resolves.
	makeTestCommit(t, app, "initial commit")

	run := func(args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, stderr, err
	}
	return app, run
}

// TestStartRecordsTransition verifies that --state start appends a Transition
// record with the actor derived from the Store's git author config.
func TestStartRecordsTransition(t *testing.T) {
	app, run := setupEditTestWithAuthor(t, "alice", "alice@example.com")

	uuidStr := createEditTestIssue(t, app, "Transition start test")
	prefix := uuidStr[:8]

	before := time.Now().Add(-time.Second)
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	after := time.Now().Add(time.Second)

	ref := issue.RefPrefix + uuidStr
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(iss.Transitions) != 1 {
		t.Fatalf("expected 1 transition after start, got %d: %v", len(iss.Transitions), iss.Transitions)
	}
	tr := iss.Transitions[0]
	if string(tr.State) != string(issue.StateInProgress) {
		t.Fatalf("expected transition state %q, got %q", issue.StateInProgress, tr.State)
	}
	if tr.Actor != "human:alice" {
		t.Fatalf("expected actor 'human:alice', got %q", tr.Actor)
	}
	if tr.Timestamp.Before(before) || tr.Timestamp.After(after) {
		t.Fatalf("transition timestamp %v outside expected range [%v, %v]", tr.Timestamp, before, after)
	}
}

// TestDoneRecordsTransition verifies that --state done appends a Transition.
func TestDoneRecordsTransition(t *testing.T) {
	app, run := setupEditTestWithAuthor(t, "bob", "bob@example.com")

	uuidStr := createEditTestIssue(t, app, "Transition done test")
	prefix := uuidStr[:8]

	// Start it first (required by state machine).
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "work commit")

	if _, _, err := run("issue", "edit", "--state", "done", prefix); err != nil {
		t.Fatalf("done failed: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Expect two transitions: one for start, one for done.
	if len(iss.Transitions) != 2 {
		t.Fatalf("expected 2 transitions after start+done, got %d: %v", len(iss.Transitions), iss.Transitions)
	}
	doneTransition := iss.Transitions[1]
	if string(doneTransition.State) != string(issue.StateDone) {
		t.Fatalf("expected done transition state %q, got %q", issue.StateDone, doneTransition.State)
	}
	if doneTransition.Actor != "human:bob" {
		t.Fatalf("expected actor 'human:bob', got %q", doneTransition.Actor)
	}
}

// TestIssueEdit_Done_ZeroCommits_Rejected verifies that --state done is rejected
// when the session has zero commits (no work recorded).
func TestIssueEdit_Done_ZeroCommits_Rejected(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Zero commit done attempt")
	prefix := uuidStr[:8]

	// Start the issue but make no commits.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Attempt to mark done with zero commits — should be rejected.
	_, _, err := run("issue", "edit", "--state", "done", prefix)
	if err == nil {
		t.Fatal("expected error when marking done with zero commits, got nil")
	}
	if !strings.Contains(err.Error(), "0 commits") {
		t.Fatalf("expected '0 commits' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected '--force' in error, got: %v", err)
	}

	// Verify the issue is still in-progress (transition was rejected).
	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)
	if iss.State != issue.StateInProgress {
		t.Fatalf("expected issue to remain in-progress after rejected done, got %s", iss.State)
	}
}

// TestIssueEdit_Done_ZeroCommits_ForceOverride verifies that --force bypasses
// the zero-commit guard on --state done.
func TestIssueEdit_Done_ZeroCommits_ForceOverride(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Force done with zero commits")
	prefix := uuidStr[:8]

	// Start the issue but make no commits.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Mark done with --force — should succeed despite zero commits.
	stdout, _, err := run("issue", "edit", "--state", "done", "--force", prefix)
	if err != nil {
		t.Fatalf("done --force failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Completed") {
		t.Fatalf("expected 'Completed' in output, got: %s", output)
	}

	// Verify the issue is done.
	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)
	if iss.State != issue.StateDone {
		t.Fatalf("expected state done after --force, got %s", iss.State)
	}
}

// TestIssueEdit_Cancel_ZeroCommits_Unaffected verifies that --state cancel
// is not blocked by the zero-commit guard (only done is guarded).
func TestIssueEdit_Cancel_ZeroCommits_Unaffected(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Cancel with zero commits")
	prefix := uuidStr[:8]

	// Start the issue but make no commits.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Cancel should succeed even with zero commits.
	_, _, err := run("issue", "edit", "--state", "cancel", prefix)
	if err != nil {
		t.Fatalf("cancel with zero commits should succeed, got: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)
	if iss.State != issue.StateCancelled {
		t.Fatalf("expected state cancelled, got %s", iss.State)
	}
}

// TestIssueEdit_Done_AutoVerify_AllPass verifies that --state done runs
// auto-verify on acceptance criteria and reports passing results.
func TestIssueEdit_Done_AutoVerify_AllPass(t *testing.T) {
	app, run := setupEditTest(t)

	// Create an issue with a body containing AC commands that will pass.
	iss := &issue.Issue{
		Title:     "Issue with passing ACs",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   time.Now(),
		Updated:   time.Now(),
		Body: "## Acceptance Criteria\n- [ ] true always passes (`true`)\n- [ ] echo produces output (`echo hello`)\n",
	}
	out, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	uuidStr := createEditTestIssue(t, app, "placeholder")
	ref := issue.RefPrefix + uuidStr
	if err := app.Store.WriteEntity(ref, "issue.md", out, "create with AC"); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}

	prefix := uuidStr[:8]

	// Start and make a commit so the guard passes.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "work commit")

	// Mark done — should auto-verify and report passing ACs.
	stdout, _, err := run("issue", "edit", "--state", "done", prefix)
	if err != nil {
		t.Fatalf("done failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "acceptance criteria verified") {
		t.Fatalf("expected 'acceptance criteria verified' in output, got: %s", output)
	}
}

// TestIssueEdit_Done_AutoVerify_PartialFail verifies that --state done reports
// a warning when some acceptance criteria fail, but does not block the transition.
func TestIssueEdit_Done_AutoVerify_PartialFail(t *testing.T) {
	app, run := setupEditTest(t)

	// Create an issue with one passing and one failing AC.
	iss := &issue.Issue{
		Title:     "Issue with mixed ACs",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   time.Now(),
		Updated:   time.Now(),
		Body: "## Acceptance Criteria\n- [ ] this passes (`true`)\n- [ ] this fails (`false`)\n",
	}
	out, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	uuidStr := createEditTestIssue(t, app, "placeholder")
	ref := issue.RefPrefix + uuidStr
	if err := app.Store.WriteEntity(ref, "issue.md", out, "create with AC"); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}

	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "work commit")

	// Mark done — should succeed but warn about failed ACs.
	stdout, _, err := run("issue", "edit", "--state", "done", prefix)
	if err != nil {
		t.Fatalf("done should succeed even with failed ACs, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "could not be verified") {
		t.Fatalf("expected 'could not be verified' warning in output, got: %s", output)
	}

	// Verify the issue is actually done (not blocked by failed ACs).
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss2, _ := issue.Parse(data)
	if iss2.State != issue.StateDone {
		t.Fatalf("expected state done despite failed ACs, got %s", iss2.State)
	}
}

// TestIssueEdit_Done_AutoVerify_NoACs verifies that --state done skips
// auto-verify silently when there are no acceptance criteria.
func TestIssueEdit_Done_AutoVerify_NoACs(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Issue without ACs")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "work commit")

	stdout, _, err := run("issue", "edit", "--state", "done", prefix)
	if err != nil {
		t.Fatalf("done failed: %v", err)
	}

	output := stdout.String()
	// Should not mention verification at all when no ACs exist.
	if strings.Contains(output, "acceptance criteria") {
		t.Fatalf("expected no verification output when no ACs, got: %s", output)
	}
}

// TestReopenRecordsTransition verifies that --state reopen appends a Transition.
func TestReopenRecordsTransition(t *testing.T) {
	app, run := setupEditTestWithAuthor(t, "carol", "carol@example.com")

	uuidStr := createEditTestIssue(t, app, "Transition reopen test")
	prefix := uuidStr[:8]

	// Bring the issue to done before reopening.
	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "work commit")
	if _, _, err := run("issue", "edit", "--state", "done", prefix); err != nil {
		t.Fatalf("done failed: %v", err)
	}

	if _, _, err := run("issue", "edit", "--state", "reopen", prefix); err != nil {
		t.Fatalf("reopen failed: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Expect three transitions: start, done, reopen.
	if len(iss.Transitions) != 3 {
		t.Fatalf("expected 3 transitions after start+done+reopen, got %d: %v", len(iss.Transitions), iss.Transitions)
	}
	reopenTransition := iss.Transitions[2]
	if string(reopenTransition.State) != string(issue.StateReopened) {
		t.Fatalf("expected reopened transition state %q, got %q", issue.StateReopened, reopenTransition.State)
	}
	if reopenTransition.Actor != "human:carol" {
		t.Fatalf("expected actor 'human:carol', got %q", reopenTransition.Actor)
	}
}

