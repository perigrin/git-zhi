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

	"github.com/perigrin/git-chain/internal/cli"
	"github.com/perigrin/git-chain/internal/issue"
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
	if iss.Sessions[0].Commits <= 0 {
		t.Fatalf("expected Commits > 0 after making commits, got %d", iss.Sessions[0].Commits)
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
	if iss.Sessions[0].Commits <= 0 {
		t.Fatalf("expected Commits > 0, got %d", iss.Sessions[0].Commits)
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
