// ABOUTME: Tests for ref resolution helpers: IsHead recognizes "HEAD"
// ABOUTME: and empty string; ResolveRef dispatches HEAD and UUID prefix scans.
package resolve_test

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/resolve"
	"github.com/perigrin/git-chain/internal/storage"
)

// initTestStore creates a temp git repo and returns a Store backed by it.
func initTestStore(t *testing.T) *storage.Store {
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

// writeTestIssue writes an issue to the store and returns its UUID string.
func writeTestIssue(t *testing.T, store *storage.Store, id uuid.UUID, state issue.State, title string) string {
	t.Helper()
	iss := &issue.Issue{
		ID:      id,
		Title:   title,
		State:   state,
		Created: time.Now(),
		Updated: time.Now(),
	}
	raw, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("failed to marshal issue: %v", err)
	}
	refPath := "refs/chain/_/issues/" + id.String()
	err = store.WriteEntity(refPath, "issue.md", raw, "create issue")
	if err != nil {
		t.Fatalf("failed to write issue: %v", err)
	}
	return id.String()
}

func TestIsHead(t *testing.T) {
	if !resolve.IsHead("HEAD") {
		t.Fatal("expected 'HEAD' to be recognized as HEAD")
	}
	if !resolve.IsHead("") {
		t.Fatal("expected empty string to be recognized as HEAD (no explicit target defaults to HEAD)")
	}
	if resolve.IsHead("019444a1") {
		t.Fatal("expected UUID prefix not to be recognized as HEAD")
	}
}

func TestResolveRef_UUIDPrefix(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()

	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	id3, _ := gen.NewV7()

	writeTestIssue(t, store, id1, issue.StatePending, "Issue One")
	writeTestIssue(t, store, id2, issue.StatePending, "Issue Two")
	writeTestIssue(t, store, id3, issue.StatePending, "Issue Three")

	// Resolve by the first 18 characters of id1's UUID string (past the
	// shared prefix that rapid UUIDv7 generation produces).
	prefix := id1.String()[:18]
	resolved, err := resolve.ResolveRef(store, prefix)
	if err != nil {
		t.Fatalf("ResolveRef(%q) unexpected error: %v", prefix, err)
	}
	expected := "refs/chain/_/issues/" + id1.String()
	if resolved != expected {
		t.Fatalf("ResolveRef(%q) = %q, want %q", prefix, resolved, expected)
	}
}

func TestResolveRef_NotFound(t *testing.T) {
	store := initTestStore(t)

	_, err := resolve.ResolveRef(store, "00000000")
	if err == nil {
		t.Fatal("expected error for nonexistent prefix, got nil")
	}
}

func TestResolveRef_HEAD_InProgress(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()

	id1, _ := gen.NewV7()
	time.Sleep(time.Millisecond)
	id2, _ := gen.NewV7()

	writeTestIssue(t, store, id1, issue.StatePending, "Pending Issue")
	writeTestIssue(t, store, id2, issue.StateInProgress, "In-Progress Issue")

	resolved, err := resolve.ResolveRef(store, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef(HEAD) unexpected error: %v", err)
	}
	expected := "refs/chain/_/issues/" + id2.String()
	if resolved != expected {
		t.Fatalf("ResolveRef(HEAD) = %q, want %q (in-progress issue)", resolved, expected)
	}
}

func TestResolveRef_HEAD_Pending(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()

	id1, _ := gen.NewV7()
	time.Sleep(time.Millisecond)
	id2, _ := gen.NewV7()

	writeTestIssue(t, store, id1, issue.StatePending, "First Pending")
	writeTestIssue(t, store, id2, issue.StatePending, "Second Pending")

	resolved, err := resolve.ResolveRef(store, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef(HEAD) unexpected error: %v", err)
	}
	// UUIDv7 sorts by creation time; id1 was created first, so it sorts first.
	expected := "refs/chain/_/issues/" + id1.String()
	if resolved != expected {
		t.Fatalf("ResolveRef(HEAD) = %q, want %q (first pending by UUID sort)", resolved, expected)
	}
}

func TestResolveRef_HEAD_Empty(t *testing.T) {
	store := initTestStore(t)

	_, err := resolve.ResolveRef(store, "HEAD")
	if err == nil {
		t.Fatal("expected error when no issues exist, got nil")
	}
}
