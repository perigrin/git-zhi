// ABOUTME: Tests for the storage Store: write/read entities on git refs,
// ABOUTME: list refs by prefix, and check ref existence. Uses real on-disk repos.
package storage_test

import (
	"testing"

	git "github.com/go-git/go-git/v5"

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
