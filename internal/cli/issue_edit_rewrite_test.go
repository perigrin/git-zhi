// ABOUTME: Tests that an issue can leave in-progress after a history rewrite
// ABOUTME: has orphaned the SHA recorded when its session started.
package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// orphanRecordedHistory rewrites the branch onto a fresh root commit, so the
// SHA recorded when a session started is no longer reachable from HEAD. This
// is what a rebase or a filter-branch does to every SHA on a branch; a plain
// `git commit --amend` of the recorded commit is enough to produce it.
func orphanRecordedHistory(t *testing.T, app *cli.App) {
	t.Helper()
	st := app.Repo.Storer

	// A root commit over an empty tree, sharing no ancestry with the branch.
	treeObj := &plumbing.MemoryObject{}
	if err := (&object.Tree{}).Encode(treeObj); err != nil {
		t.Fatalf("encode tree: %v", err)
	}
	treeHash, err := st.SetEncodedObject(treeObj)
	if err != nil {
		t.Fatalf("store tree: %v", err)
	}

	sig := object.Signature{Name: "test", Email: "test@test", When: time.Now()}
	commitObj := &plumbing.MemoryObject{}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   "rewritten history",
		TreeHash:  treeHash,
	}
	if err := commit.Encode(commitObj); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	commitHash, err := st.SetEncodedObject(commitObj)
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}

	// Move the branch onto it, as a rebase or filter-branch would.
	head, err := app.Repo.Head()
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	if err := st.SetReference(plumbing.NewHashReference(head.Name(), commitHash)); err != nil {
		t.Fatalf("move branch: %v", err)
	}
}

// assertRecordedSHAUnreachable fails the test if the setup did not actually
// orphan the recorded SHA, so a passing test cannot be a vacuous one.
func assertRecordedSHAUnreachable(t *testing.T, app *cli.App, uuidStr string) {
	t.Helper()
	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	var startSHA string
	for _, iss := range all {
		if iss.ID.String() == uuidStr && len(iss.Sessions) > 0 {
			startSHA = iss.Sessions[0].StartSHA
		}
	}
	if startSHA == "" {
		t.Fatal("no session start SHA was recorded")
	}
	head, err := app.Store.RepoHEAD()
	if err != nil {
		t.Fatalf("RepoHEAD: %v", err)
	}
	if _, err := app.Store.CountCommits(startSHA, head); err == nil {
		t.Fatalf("setup did not orphan the recorded SHA %s; it is still reachable from %s", startSHA, head)
	}
}

// TestIssueEdit_DoneAfterHistoryRewrite — every exit from in-progress counted
// commits first and treated an unreachable start SHA as fatal, so a rebase
// left the issue with no way out. The count must not be the thing that blocks.
func TestIssueEdit_DoneAfterHistoryRewrite(t *testing.T) {
	app, run := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Work that outlived its SHA")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "real work")
	orphanRecordedHistory(t, app)
	assertRecordedSHAUnreachable(t, app, uuidStr)

	// Without --force the documented guard should speak, not the counter.
	_, _, err := run("issue", "edit", "--state", "done", prefix)
	if err == nil {
		t.Fatal("expected the zero-commit guard to refuse, got nil")
	}
	if strings.Contains(err.Error(), "count commits") {
		t.Errorf("expected the documented zero-commit guard, got a count failure: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected the error to offer --force, got: %v", err)
	}
}

// TestIssueEdit_DoneForceAfterHistoryRewrite — --force is documented as the
// override for exactly this refusal, and today never reaches its own check.
func TestIssueEdit_DoneForceAfterHistoryRewrite(t *testing.T) {
	app, run := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Work that outlived its SHA")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "real work")
	orphanRecordedHistory(t, app)
	assertRecordedSHAUnreachable(t, app, uuidStr)

	if _, _, err := run("issue", "edit", "--state", "done", "--force", prefix); err != nil {
		t.Fatalf("--state done --force should close the issue, got: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	for _, iss := range all {
		if iss.ID.String() == uuidStr && iss.State != issue.StateDone {
			t.Errorf("expected state done, got %s", iss.State)
		}
	}
}

// TestIssueEdit_PauseAfterHistoryRewrite — pause carries no commit guard at
// all, so a rewrite should never stop an issue being set down.
func TestIssueEdit_PauseAfterHistoryRewrite(t *testing.T) {
	app, run := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Work that outlived its SHA")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "real work")
	orphanRecordedHistory(t, app)
	assertRecordedSHAUnreachable(t, app, uuidStr)

	if _, _, err := run("issue", "edit", "--state", "pause", prefix); err != nil {
		t.Fatalf("--state pause should succeed after a rewrite, got: %v", err)
	}
}

// TestIssueEdit_RewriteRecordsZeroCommits — an uncountable window records zero
// rather than a guess, matching what cancel already did.
func TestIssueEdit_RewriteRecordsZeroCommits(t *testing.T) {
	app, run := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Work that outlived its SHA")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "real work")
	orphanRecordedHistory(t, app)

	if _, _, err := run("issue", "edit", "--state", "pause", prefix); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	for _, iss := range all {
		if iss.ID.String() != uuidStr {
			continue
		}
		if len(iss.Sessions) == 0 {
			t.Fatal("expected a session")
		}
		if got := iss.Sessions[0].Commits; got != 0 {
			t.Errorf("expected 0 commits for an uncountable window, got %d", got)
		}
		if iss.Sessions[0].EndSHA == "" {
			t.Error("expected the session to be closed with an end SHA")
		}
	}
}

// TestIssueEdit_DoneStillCountsNormally — the tolerant path must not swallow
// real counting; an intact history still records its commits and closes.
func TestIssueEdit_DoneStillCountsNormally(t *testing.T) {
	app, run := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Ordinary work")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	makeTestCommit(t, app, "real work")

	if _, _, err := run("issue", "edit", "--state", "done", prefix); err != nil {
		t.Fatalf("done with intact history failed: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	for _, iss := range all {
		if iss.ID.String() != uuidStr {
			continue
		}
		if iss.State != issue.StateDone {
			t.Errorf("expected state done, got %s", iss.State)
		}
		if got := iss.Sessions[0].Commits; got != 1 {
			t.Errorf("expected 1 counted commit with intact history, got %d", got)
		}
	}
}
