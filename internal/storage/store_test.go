// ABOUTME: Tests for the storage Store: write/read entities on git refs,
// ABOUTME: list refs by prefix, check ref existence, and repo HEAD/commit counting.
package storage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-chain/internal/storage"
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
	err := store.WriteEntity("refs/chain/_/issues/test-id", "issue.md", []byte("hello"), "create issue")
	if err != nil {
		t.Fatalf("WriteEntity failed: %v", err)
	}
	if !store.RefExists("refs/chain/_/issues/test-id") {
		t.Fatal("expected ref to exist after WriteEntity")
	}
}

func TestWriteEntity_AppendsCommit(t *testing.T) {
	store := initTestRepo(t)
	ref := "refs/chain/_/issues/test-id"
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
	ref := "refs/chain/_/issues/test-id"
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
	_, err := store.ReadEntity("refs/chain/_/issues/nonexistent", "issue.md")
	if err == nil {
		t.Fatal("expected error reading nonexistent ref")
	}
}

func TestListRefs(t *testing.T) {
	store := initTestRepo(t)
	for _, id := range []string{"aaa", "bbb", "ccc"} {
		err := store.WriteEntity("refs/chain/_/issues/"+id, "issue.md", []byte("test"), "create")
		if err != nil {
			t.Fatalf("WriteEntity failed for %s: %v", id, err)
		}
	}
	err := store.WriteEntity("refs/chain/_/milestones/v0.1", "milestone.yaml", []byte("test"), "create")
	if err != nil {
		t.Fatalf("WriteEntity failed for milestone: %v", err)
	}
	refs, err := store.ListRefs("refs/chain/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %v", len(refs), refs)
	}
}

func TestRefExists(t *testing.T) {
	store := initTestRepo(t)
	if store.RefExists("refs/chain/_/issues/nonexistent") {
		t.Fatal("expected RefExists to return false for nonexistent ref")
	}
	err := store.WriteEntity("refs/chain/_/issues/test-id", "issue.md", []byte("test"), "create")
	if err != nil {
		t.Fatalf("WriteEntity failed: %v", err)
	}
	if !store.RefExists("refs/chain/_/issues/test-id") {
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
	ref := "refs/chain/_/tags/test-tag"
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
