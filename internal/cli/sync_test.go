// ABOUTME: Tests for git zhi sync: push/pull of refs/zhi/* plus branch fast-forward.
// ABOUTME: Uses real on-disk bare remote + clones, mirroring the C&C round-trip spike.

package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// newBareRemote creates a bare repo to act as the shared remote.
func newBareRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, "", "init", "--bare", dir)
	return dir
}

// cloneRepo clones remote into a fresh temp dir and configures identity.
func cloneRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	runGit(t, "", "clone", remote, dir)
	runGit(t, dir, "config", "user.email", "t@t")
	runGit(t, dir, "config", "user.name", "t")
	return dir
}

// addIssue writes an issue into the chain at dir via OpenRepo + the store.
func addIssue(t *testing.T, dir, title string) {
	t.Helper()
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo(%s): %v", dir, err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   now,
		Updated:   now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	if err := app.Store.WriteEntity(issue.RefPrefix+id.String(), "issue.md", data, "Add issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
}

func TestSyncPush_PushesBranchAndZhiRefs(t *testing.T) {
	remote := newBareRemote(t)
	a := cloneRepo(t, remote)

	// Seed an initial commit and push it so the branch exists on the remote.
	if err := os.WriteFile(filepath.Join(a, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "f.txt")
	runGitCommit(t, a, "initial")
	runGit(t, a, "push", "-u", "origin", "HEAD")

	// Create chain state and a new commit, then sync --push.
	addIssue(t, a, "Sync me")
	if err := os.WriteFile(filepath.Join(a, "g.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "g.txt")
	runGitCommit(t, a, "second")

	var out strings.Builder
	if err := cli.SyncPush(a, "origin", &out); err != nil {
		t.Fatalf("SyncPush: %v\noutput: %s", err, out.String())
	}

	// The bare remote must now have a refs/zhi/* ref.
	refs := runGit(t, "", "--git-dir", remote, "for-each-ref", "--format=%(refname)", "refs/zhi/")
	if !strings.Contains(refs, "refs/zhi/_/issues/") {
		t.Errorf("remote missing zhi issue refs after push; got:\n%s", refs)
	}

	// And the branch tip must match the local second commit.
	localTip := strings.TrimSpace(runGit(t, a, "rev-parse", "HEAD"))
	remoteTip := strings.TrimSpace(runGit(t, "", "--git-dir", remote, "rev-parse", "HEAD"))
	if localTip != remoteTip {
		t.Errorf("remote branch tip %s != local %s", remoteTip, localTip)
	}
}

func TestSyncPull_FetchesRefsAndFastForwardsBranch(t *testing.T) {
	remote := newBareRemote(t)

	// Establish an initial commit on the remote first, so clone B can branch
	// from it before A advances the remote with the issue + second commit.
	seed := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(seed, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, seed, "add", "f.txt")
	runGitCommit(t, seed, "initial")
	runGit(t, seed, "push", "-u", "origin", "HEAD")

	// Clone B now, at the initial commit.
	b := cloneRepo(t, remote)
	beforeTip := strings.TrimSpace(runGit(t, b, "rev-parse", "HEAD"))

	// A adds an issue + a second commit and pushes both via sync.
	addIssue(t, seed, "Shared issue")
	if err := os.WriteFile(filepath.Join(seed, "g.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, seed, "add", "g.txt")
	runGitCommit(t, seed, "second")
	var pushOut strings.Builder
	if err := cli.SyncPush(seed, "origin", &pushOut); err != nil {
		t.Fatalf("seed SyncPush: %v\n%s", err, pushOut.String())
	}
	pushedTip := strings.TrimSpace(runGit(t, seed, "rev-parse", "HEAD"))
	if beforeTip == pushedTip {
		t.Fatal("clone B unexpectedly already at pushed tip")
	}

	var out strings.Builder
	if err := cli.SyncPull(b, "origin", &out); err != nil {
		t.Fatalf("SyncPull: %v\n%s", err, out.String())
	}

	// B must now see the chain issue.
	app, err := cli.OpenRepo(b)
	if err != nil {
		t.Fatalf("OpenRepo(b): %v", err)
	}
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) != 1 || issues[0].Title != "Shared issue" {
		t.Errorf("expected 1 issue 'Shared issue', got %d: %+v", len(issues), issues)
	}

	// B's branch (working tree) must have fast-forwarded to the pushed tip.
	afterTip := strings.TrimSpace(runGit(t, b, "rev-parse", "HEAD"))
	if afterTip != pushedTip {
		t.Errorf("branch not fast-forwarded: at %s, want %s", afterTip, pushedTip)
	}
	if _, err := os.Stat(filepath.Join(b, "g.txt")); err != nil {
		t.Errorf("working tree missing g.txt after pull (tree not consistent): %v", err)
	}
}

func TestSyncPull_RefusesOnDivergence(t *testing.T) {
	remote := newBareRemote(t)

	seed := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(seed, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, seed, "add", "f.txt")
	runGitCommit(t, seed, "initial")
	runGit(t, seed, "push", "-u", "origin", "HEAD")

	b := cloneRepo(t, remote)

	// Remote advances.
	if err := os.WriteFile(filepath.Join(seed, "g.txt"), []byte("remote"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, seed, "add", "g.txt")
	runGitCommit(t, seed, "remote work")
	runGit(t, seed, "push", "origin", "HEAD")

	// B makes its own divergent commit.
	if err := os.WriteFile(filepath.Join(b, "h.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, b, "add", "h.txt")
	runGitCommit(t, b, "local work")
	localTipBefore := strings.TrimSpace(runGit(t, b, "rev-parse", "HEAD"))

	var out strings.Builder
	err := cli.SyncPull(b, "origin", &out)
	if err == nil {
		t.Fatal("expected SyncPull to refuse on divergence, got nil")
	}
	if !strings.Contains(err.Error(), "diverged") {
		t.Errorf("error should mention divergence, got: %v", err)
	}

	// The working tree must be untouched: HEAD unchanged, no merge happened.
	if got := strings.TrimSpace(runGit(t, b, "rev-parse", "HEAD")); got != localTipBefore {
		t.Errorf("HEAD moved on refused pull: %s != %s", got, localTipBefore)
	}
}

func TestSync_PushAndPullMutuallyExclusive(t *testing.T) {
	remote := newBareRemote(t)
	dir := cloneRepo(t, remote)
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	root := cli.NewRootCommand()
	root.SetContext(cli.WithApp(context.Background(), app))
	root.SetArgs([]string{"sync", "--push", "--pull"})
	var buf strings.Builder
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for --push --pull together, got nil")
	} else if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should say mutually exclusive, got: %v", err)
	}
}

func TestSyncPull_DetachedHEADFetchesRefsSkipsFastForward(t *testing.T) {
	remote := newBareRemote(t)

	seed := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(seed, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, seed, "add", "f.txt")
	runGitCommit(t, seed, "initial")
	runGit(t, seed, "push", "-u", "origin", "HEAD")

	b := cloneRepo(t, remote)

	// A publishes an issue.
	addIssue(t, seed, "Detached-safe issue")
	var pushOut strings.Builder
	if err := cli.SyncPush(seed, "origin", &pushOut); err != nil {
		t.Fatalf("seed SyncPush: %v\n%s", err, pushOut.String())
	}

	// B detaches HEAD.
	runGit(t, b, "checkout", "--detach")

	var out strings.Builder
	if err := cli.SyncPull(b, "origin", &out); err != nil {
		t.Fatalf("SyncPull on detached HEAD should not error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "detached HEAD") {
		t.Errorf("expected a detached-HEAD notice, got: %s", out.String())
	}

	// The chain refs must still have arrived.
	app, err := cli.OpenRepo(b)
	if err != nil {
		t.Fatalf("OpenRepo(b): %v", err)
	}
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue fetched on detached HEAD, got %d", len(issues))
	}
}

// TestSyncPull_EmptyRemoteDoesNotDeleteLocalZhiRefs guards against a wildcard
// fetch pruning local refs/zhi/* when the remote has none yet — which would
// destroy unpushed local chain state.
func TestSyncPull_EmptyRemoteDoesNotDeleteLocalZhiRefs(t *testing.T) {
	remote := newBareRemote(t)
	a := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(a, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "f.txt")
	runGitCommit(t, a, "initial")
	runGit(t, a, "push", "-u", "origin", "HEAD")

	// Local chain state exists; remote has NO refs/zhi/* yet.
	addIssue(t, a, "Unpushed issue")
	before := strings.TrimSpace(runGit(t, a, "for-each-ref", "--format=%(refname)", "refs/zhi/"))
	if !strings.Contains(before, "refs/zhi/_/issues/") {
		t.Fatalf("precondition failed: expected local zhi issue refs, got:\n%s", before)
	}

	var out strings.Builder
	if err := cli.SyncPull(a, "origin", &out); err != nil {
		t.Fatalf("SyncPull: %v\n%s", err, out.String())
	}

	after := strings.TrimSpace(runGit(t, a, "for-each-ref", "--format=%(refname)", "refs/zhi/"))
	if after != before {
		t.Errorf("pull against empty remote changed local zhi refs.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	app, err := cli.OpenRepo(a)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("local issue lost after pull against empty remote: got %d", len(issues))
	}
}

// TestSyncPull_PartialOverlapKeepsLocalOnlyRefs guards the common multi-worker
// case: the remote has SOME zhi refs but the local has additional unpushed
// ones. A wildcard fetch would prune the local-only refs (source absent); the
// pull must preserve them.
func TestSyncPull_PartialOverlapKeepsLocalOnlyRefs(t *testing.T) {
	remote := newBareRemote(t)

	a := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(a, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "f.txt")
	runGitCommit(t, a, "initial")
	runGit(t, a, "push", "-u", "origin", "HEAD")

	// A publishes one issue to the remote.
	addIssue(t, a, "Shared A issue")
	var pushOut strings.Builder
	if err := cli.SyncPush(a, "origin", &pushOut); err != nil {
		t.Fatalf("A SyncPush: %v\n%s", err, pushOut.String())
	}

	// B clones (gets the shared issue) then creates its own local-only issue.
	b := cloneRepo(t, remote)
	if err := cli.SyncPull(b, "origin", new(strings.Builder)); err != nil {
		t.Fatalf("B initial pull: %v", err)
	}
	addIssue(t, b, "Local-only B issue")

	beforeCount := strings.Count(runGit(t, b, "for-each-ref", "--format=%(refname)", "refs/zhi/_/issues/"), "refs/zhi/_/issues/")
	if beforeCount != 2 {
		t.Fatalf("precondition: expected 2 local issue refs on B, got %d", beforeCount)
	}

	// Pulling again (remote still has only the shared issue) must NOT delete
	// B's local-only issue.
	var out strings.Builder
	if err := cli.SyncPull(b, "origin", &out); err != nil {
		t.Fatalf("B second pull: %v\n%s", err, out.String())
	}

	app, err := cli.OpenRepo(b)
	if err != nil {
		t.Fatalf("OpenRepo(b): %v", err)
	}
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) != 2 {
		titles := make([]string, len(issues))
		for i, is := range issues {
			titles[i] = is.Title
		}
		t.Errorf("expected 2 issues after partial-overlap pull, got %d: %v", len(issues), titles)
	}
}

// TestSyncPull_NoUpstreamBranchSkipsFastForward verifies a local branch with no
// remote counterpart is a notice + skip, not an error, while refs/zhi/* still
// fetches.
func TestSyncPull_NoUpstreamBranchSkipsFastForward(t *testing.T) {
	remote := newBareRemote(t)

	a := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(a, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "f.txt")
	runGitCommit(t, a, "initial")
	runGit(t, a, "push", "-u", "origin", "HEAD")
	addIssue(t, a, "Upstreamless-safe issue")
	if err := cli.SyncPush(a, "origin", new(strings.Builder)); err != nil {
		t.Fatalf("A SyncPush: %v", err)
	}

	b := cloneRepo(t, remote)
	// Create a local branch that does not exist on the remote.
	runGit(t, b, "checkout", "-b", "local-feature")

	var out strings.Builder
	if err := cli.SyncPull(b, "origin", &out); err != nil {
		t.Fatalf("SyncPull on no-upstream branch should not error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "upstream") && !strings.Contains(out.String(), "skipped") {
		t.Errorf("expected a no-upstream/skip notice, got: %s", out.String())
	}

	app, err := cli.OpenRepo(b)
	if err != nil {
		t.Fatalf("OpenRepo(b): %v", err)
	}
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected refs/zhi/* fetched on no-upstream branch, got %d issues", len(issues))
	}
}

// TestSyncPull_BranchNameTrailingComponentCollision guards against
// remoteHasBranch false-matching: a local branch "bar" must not be considered
// present on the remote just because the remote has "foo/bar".
func TestSyncPull_BranchNameTrailingComponentCollision(t *testing.T) {
	remote := newBareRemote(t)

	seed := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(seed, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, seed, "add", "f.txt")
	runGitCommit(t, seed, "initial")
	runGit(t, seed, "push", "-u", "origin", "HEAD")
	// Remote gets a branch foo/bar but NOT a top-level bar.
	runGit(t, seed, "checkout", "-b", "foo/bar")
	runGit(t, seed, "push", "-u", "origin", "foo/bar")
	addIssue(t, seed, "Collision-safe issue")
	if err := cli.SyncPush(seed, "origin", new(strings.Builder)); err != nil {
		t.Fatalf("seed SyncPush: %v", err)
	}

	b := cloneRepo(t, remote)
	runGit(t, b, "checkout", "-b", "bar") // local "bar", no remote "bar"

	var out strings.Builder
	if err := cli.SyncPull(b, "origin", &out); err != nil {
		t.Fatalf("SyncPull with colliding branch name should not error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "not on origin") && !strings.Contains(out.String(), "skipped") {
		t.Errorf("expected a no-upstream/skip notice for branch bar, got: %s", out.String())
	}
}

func TestSync_BareRoundTrip(t *testing.T) {
	remote := newBareRemote(t)

	a := cloneRepo(t, remote)
	if err := os.WriteFile(filepath.Join(a, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "f.txt")
	runGitCommit(t, a, "initial")
	runGit(t, a, "push", "-u", "origin", "HEAD")

	b := cloneRepo(t, remote)

	// A creates an issue and a commit, then bare-syncs (pull then push).
	addIssue(t, a, "Round-trip issue")
	if err := os.WriteFile(filepath.Join(a, "g.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, a, "add", "g.txt")
	runGitCommit(t, a, "second")

	appA, err := cli.OpenRepo(a)
	if err != nil {
		t.Fatalf("OpenRepo(a): %v", err)
	}
	rootA := cli.NewRootCommand()
	rootA.SetContext(cli.WithApp(context.Background(), appA))
	rootA.SetArgs([]string{"sync"})
	var outA strings.Builder
	rootA.SetOut(&outA)
	rootA.SetErr(&outA)
	if err := rootA.Execute(); err != nil {
		t.Fatalf("bare sync on A: %v\n%s", err, outA.String())
	}

	// B bare-syncs and must receive both the issue and the commit.
	appB, err := cli.OpenRepo(b)
	if err != nil {
		t.Fatalf("OpenRepo(b): %v", err)
	}
	rootB := cli.NewRootCommand()
	rootB.SetContext(cli.WithApp(context.Background(), appB))
	rootB.SetArgs([]string{"sync"})
	var outB strings.Builder
	rootB.SetOut(&outB)
	rootB.SetErr(&outB)
	if err := rootB.Execute(); err != nil {
		t.Fatalf("bare sync on B: %v\n%s", err, outB.String())
	}

	appB2, err := cli.OpenRepo(b)
	if err != nil {
		t.Fatalf("re-open B: %v", err)
	}
	issues, err := issue.LoadAllIssues(appB2.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues(b): %v", err)
	}
	if len(issues) != 1 || issues[0].Title != "Round-trip issue" {
		t.Errorf("B did not receive the issue via bare sync: %+v", issues)
	}
	if _, err := os.Stat(filepath.Join(b, "g.txt")); err != nil {
		t.Errorf("B working tree missing g.txt after bare sync: %v", err)
	}
}

func TestSyncPush_NoRemote(t *testing.T) {
	dir := t.TempDir()
	runGit(t, "", "init", dir)
	runGit(t, dir, "config", "user.email", "t@t")
	runGit(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "f.txt")
	runGitCommit(t, dir, "initial")

	var out strings.Builder
	err := cli.SyncPush(dir, "origin", &out)
	if err == nil {
		t.Fatal("expected error when remote is absent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention missing remote, got: %v", err)
	}
}
