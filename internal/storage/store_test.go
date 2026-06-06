// ABOUTME: Tests for the storage Store: write/read entities on git refs,
// ABOUTME: list refs by prefix, check ref existence, and repo HEAD/commit counting.
package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-zhi/internal/storage"
)

func initTestRepo(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return store
}

// initTestRepoWithGit returns both the underlying *git.Repository and the Store,
// needed for tests that create real worktree commits (e.g., RepoHEAD, CountCommits).
func initTestRepoWithGit(t *testing.T) (*git.Repository, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return repo, store
}

// makeCommit creates a real worktree commit in the repo with the given file content.
// Returns the commit SHA.
func makeCommit(t *testing.T, repo *git.Repository, dir, filename, content, message string) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("git add: %v", err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return hash.String()
}

func TestWriteEntity_CreatesRef(t *testing.T) {
	store := initTestRepo(t)
	err := store.WriteEntity("refs/zhi/_/issues/test-id", "issue.md", []byte("hello"), "create issue")
	if err != nil {
		t.Fatalf("WriteEntity failed: %v", err)
	}
	if !store.RefExists("refs/zhi/_/issues/test-id") {
		t.Fatal("expected ref to exist after WriteEntity")
	}
}

func TestWriteEntity_AppendsCommit(t *testing.T) {
	store := initTestRepo(t)
	ref := "refs/zhi/_/issues/test-id"
	err := store.WriteEntity(ref, "issue.md", []byte("version 1"), "first")
	if err != nil {
		t.Fatalf("first WriteEntity failed: %v", err)
	}
	err = store.WriteEntity(ref, "issue.md", []byte("version 2"), "second")
	if err != nil {
		t.Fatalf("second WriteEntity failed: %v", err)
	}
	content, err := store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity failed: %v", err)
	}
	if string(content) != "version 2" {
		t.Fatalf("expected 'version 2', got %q", string(content))
	}
}

func TestReadEntity_RoundTrip(t *testing.T) {
	store := initTestRepo(t)
	ref := "refs/zhi/_/issues/test-id"
	original := []byte("---\ntitle: Test\n---\nBody content")
	err := store.WriteEntity(ref, "issue.md", original, "create")
	if err != nil {
		t.Fatalf("WriteEntity failed: %v", err)
	}
	content, err := store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity failed: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("content mismatch:\ngot:  %q\nwant: %q", string(content), string(original))
	}
}

func TestReadEntity_NotFound(t *testing.T) {
	store := initTestRepo(t)
	_, err := store.ReadEntity("refs/zhi/_/issues/nonexistent", "issue.md")
	if err == nil {
		t.Fatal("expected error reading nonexistent ref")
	}
}

func TestListRefs(t *testing.T) {
	store := initTestRepo(t)
	for _, id := range []string{"aaa", "bbb", "ccc"} {
		err := store.WriteEntity("refs/zhi/_/issues/"+id, "issue.md", []byte("test"), "create")
		if err != nil {
			t.Fatalf("WriteEntity failed for %s: %v", id, err)
		}
	}
	err := store.WriteEntity("refs/zhi/_/milestones/v0.1", "milestone.yaml", []byte("test"), "create")
	if err != nil {
		t.Fatalf("WriteEntity failed for milestone: %v", err)
	}
	refs, err := store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %v", len(refs), refs)
	}
}

func TestRefExists(t *testing.T) {
	store := initTestRepo(t)
	if store.RefExists("refs/zhi/_/issues/nonexistent") {
		t.Fatal("expected RefExists to return false for nonexistent ref")
	}
	err := store.WriteEntity("refs/zhi/_/issues/test-id", "issue.md", []byte("test"), "create")
	if err != nil {
		t.Fatalf("WriteEntity failed: %v", err)
	}
	if !store.RefExists("refs/zhi/_/issues/test-id") {
		t.Fatal("expected RefExists to return true after WriteEntity")
	}
}

func TestRepoHEAD(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	dir := t.TempDir()
	// Reinitialize into the same dir so makeCommit has the right path.
	// Actually, get the repo's worktree path directly.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir = wt.Filesystem.Root()

	sha := makeCommit(t, repo, dir, "test.txt", "v1", "first commit")
	if sha == "" {
		t.Fatal("expected non-empty SHA from makeCommit")
	}

	head, err := store.RepoHEAD()
	if err != nil {
		t.Fatalf("RepoHEAD failed: %v", err)
	}
	if head == "" {
		t.Fatal("expected non-empty SHA from RepoHEAD")
	}
	if head != sha {
		t.Fatalf("RepoHEAD returned %q, expected %q", head, sha)
	}
}

func TestCountCommits(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir := wt.Filesystem.Root()

	sha1 := makeCommit(t, repo, dir, "a.txt", "v1", "first")
	makeCommit(t, repo, dir, "b.txt", "v2", "second")
	sha3 := makeCommit(t, repo, dir, "c.txt", "v3", "third")

	count, err := store.CountCommits(sha1, sha3)
	if err != nil {
		t.Fatalf("CountCommits failed: %v", err)
	}
	// sha1 is exclusive, sha3 is inclusive: counts sha2 and sha3 = 2
	if count != 2 {
		t.Fatalf("expected 2 commits, got %d", count)
	}
}

func TestDeleteRef(t *testing.T) {
	store := initTestRepo(t)
	ref := "refs/zhi/_/tags/test-tag"
	if err := store.WriteEntity(ref, "tag.txt", []byte("target"), "create tag"); err != nil {
		t.Fatalf("WriteEntity failed: %v", err)
	}
	if !store.RefExists(ref) {
		t.Fatal("expected ref to exist")
	}
	if err := store.DeleteRef(ref); err != nil {
		t.Fatalf("DeleteRef failed: %v", err)
	}
	if store.RefExists(ref) {
		t.Fatal("expected ref to be deleted")
	}
}

func TestCountCommits_UnreachableSHA(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir := wt.Filesystem.Root()

	sha1 := makeCommit(t, repo, dir, "a.txt", "v1", "first")
	_ = sha1

	sha2 := makeCommit(t, repo, dir, "b.txt", "v2", "second")

	// Use a bogus SHA that is not in the ancestry of sha2.
	bogusSHA := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	_, err = store.CountCommits(bogusSHA, sha2)
	if err == nil {
		t.Fatal("expected error when startSHA is not reachable from endSHA, got nil")
	}
	if !strings.Contains(err.Error(), "not found in ancestry") {
		t.Fatalf("expected 'not found in ancestry' in error, got: %v", err)
	}
}

func TestCountCommits_SameSHA(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir := wt.Filesystem.Root()

	sha := makeCommit(t, repo, dir, "a.txt", "v1", "first")

	count, err := store.CountCommits(sha, sha)
	if err != nil {
		t.Fatalf("CountCommits same SHA failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 commits when start==end, got %d", count)
	}
}

func TestMigrateStrandedWorktreeRefs_MainWorktreeNoOp(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir := wt.Filesystem.Root()
	gitDir := filepath.Join(dir, ".git")

	// In the main worktree, the per-worktree git dir IS the common dir, so
	// migration must short-circuit to a no-op.
	migrated, err := store.MigrateStrandedWorktreeRefs(gitDir, gitDir, "refs/zhi")
	if err != nil {
		t.Fatalf("MigrateStrandedWorktreeRefs (main worktree): %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 in main worktree", migrated)
	}
}

func TestMigrateStrandedWorktreeRefs_EmptyDirsNoOp(t *testing.T) {
	_, store := initTestRepoWithGit(t)
	migrated, err := store.MigrateStrandedWorktreeRefs("", "", "refs/zhi")
	if err != nil {
		t.Fatalf("MigrateStrandedWorktreeRefs (empty dirs): %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 for empty dirs", migrated)
	}
}

func TestMigrateStrandedWorktreeRefs_LooseAndPacked(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir := wt.Filesystem.Root()
	commonGitDir := filepath.Join(dir, ".git")

	// A real commit so the migrated refs point at an existing object.
	sha := makeCommit(t, repo, dir, "a.txt", "v1", "first")

	// Simulate a linked worktree's own git dir, distinct from the common dir,
	// holding one loose stranded ref and one packed stranded ref.
	wtGitDir := t.TempDir()

	loosePath := filepath.Join(wtGitDir, "refs", "zhi", "_", "issues", "loose-one")
	if err := os.MkdirAll(filepath.Dir(loosePath), 0o755); err != nil {
		t.Fatalf("mkdir loose: %v", err)
	}
	if err := os.WriteFile(loosePath, []byte(sha+"\n"), 0o644); err != nil {
		t.Fatalf("write loose ref: %v", err)
	}

	packed := "# pack-refs with: peeled fully-peeled sorted\n" +
		sha + " refs/zhi/_/issues/packed-one\n"
	if err := os.WriteFile(filepath.Join(wtGitDir, "packed-refs"), []byte(packed), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	migrated, err := store.MigrateStrandedWorktreeRefs(wtGitDir, commonGitDir, "refs/zhi")
	if err != nil {
		t.Fatalf("MigrateStrandedWorktreeRefs: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated = %d, want 2 (one loose + one packed)", migrated)
	}
	if !store.RefExists("refs/zhi/_/issues/loose-one") {
		t.Error("loose stranded ref not migrated into shared namespace")
	}
	if !store.RefExists("refs/zhi/_/issues/packed-one") {
		t.Error("packed stranded ref not migrated into shared namespace")
	}
}

func TestMigrateStrandedWorktreeRefs_SameNameLoosePackedDedup(t *testing.T) {
	repo, store := initTestRepoWithGit(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	dir := wt.Filesystem.Root()
	commonGitDir := filepath.Join(dir, ".git")

	looseSHA := makeCommit(t, repo, dir, "a.txt", "v1", "first")
	packedSHA := makeCommit(t, repo, dir, "a.txt", "v2", "second")
	if looseSHA == packedSHA {
		t.Fatal("expected distinct SHAs for loose and packed copies")
	}

	wtGitDir := t.TempDir()

	// Same ref name appears both loose and packed, pointing at different SHAs.
	const refName = "refs/zhi/_/issues/dup"
	loosePath := filepath.Join(wtGitDir, "refs", "zhi", "_", "issues", "dup")
	if err := os.MkdirAll(filepath.Dir(loosePath), 0o755); err != nil {
		t.Fatalf("mkdir loose: %v", err)
	}
	if err := os.WriteFile(loosePath, []byte(looseSHA+"\n"), 0o644); err != nil {
		t.Fatalf("write loose ref: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "packed-refs"),
		[]byte(packedSHA+" "+refName+"\n"), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	migrated, err := store.MigrateStrandedWorktreeRefs(wtGitDir, commonGitDir, "refs/zhi")
	if err != nil {
		t.Fatalf("MigrateStrandedWorktreeRefs: %v", err)
	}
	// Loose migrates first; the packed duplicate is skipped as already-present.
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1 (loose wins, packed duplicate skipped)", migrated)
	}
	ref, err := repo.Reference(plumbing.ReferenceName(refName), false)
	if err != nil {
		t.Fatalf("resolve migrated ref: %v", err)
	}
	if got := ref.Hash().String(); got != looseSHA {
		t.Errorf("migrated ref points at %q, want loose SHA %q", got, looseSHA)
	}
}
