// ABOUTME: Tests for ref resolution helpers: IsHead recognizes "HEAD"
// ABOUTME: and empty string; ResolveRef dispatches HEAD, tag, and UUID prefix scans.
package resolve_test

import (
	"strings"
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

func TestResolveRef_HEAD_SkipsDoneAndCancelled(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()

	idDone, _ := gen.NewV7()
	time.Sleep(time.Millisecond)
	idCancelled, _ := gen.NewV7()
	time.Sleep(time.Millisecond)
	idPending, _ := gen.NewV7()

	writeTestIssue(t, store, idDone, issue.StateDone, "Done Issue")
	writeTestIssue(t, store, idCancelled, issue.StateCancelled, "Cancelled Issue")
	writeTestIssue(t, store, idPending, issue.StatePending, "Pending Issue")

	resolved, err := resolve.ResolveRef(store, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef(HEAD) unexpected error: %v", err)
	}
	expected := "refs/chain/_/issues/" + idPending.String()
	if resolved != expected {
		t.Fatalf("ResolveRef(HEAD) = %q, want %q (pending issue, skipping done and cancelled)", resolved, expected)
	}
}

// writeTestIssueWithBlocks writes an issue with block relationships to the store.
func writeTestIssueWithBlocks(t *testing.T, store *storage.Store, id uuid.UUID, state issue.State, title string, blocks []uuid.UUID) {
	t.Helper()
	iss := &issue.Issue{
		ID:      id,
		Title:   title,
		State:   state,
		Blocks:  blocks,
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
}

// TestResolveRef_HEAD_CriticalChain verifies that HEAD resolves to the
// critical chain leader (the issue with the most downstream deps) when
// no issue is in-progress.
func TestResolveRef_HEAD_CriticalChain(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()

	// Create three issues. id1 blocks id2 which blocks id3.
	// The critical chain is id1 → id2 → id3; HEAD should be id1.
	// Create id3 first so it has the smallest UUID (would be "first" by
	// naive UUID sort), letting us verify the critical chain wins.
	id3, _ := gen.NewV7()
	time.Sleep(time.Millisecond)
	id2, _ := gen.NewV7()
	time.Sleep(time.Millisecond)
	id1, _ := gen.NewV7()

	writeTestIssueWithBlocks(t, store, id1, issue.StatePending, "Root blocker", []uuid.UUID{id2})
	writeTestIssueWithBlocks(t, store, id2, issue.StatePending, "Middle issue", []uuid.UUID{id3})
	writeTestIssue(t, store, id3, issue.StatePending, "Leaf issue")

	resolved, err := resolve.ResolveRef(store, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef(HEAD) unexpected error: %v", err)
	}
	// id1 is the critical chain leader — it has the most downstream deps.
	expected := "refs/chain/_/issues/" + id1.String()
	if resolved != expected {
		t.Fatalf("ResolveRef(HEAD) = %q, want %q (critical chain leader)", resolved, expected)
	}
}

func TestResolveRef_Tag(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	writeTestIssue(t, store, id, issue.StatePending, "Tagged Issue")

	// Write a tag pointing to the issue ref.
	issueRef := "refs/chain/_/issues/" + id.String()
	tagRef := "refs/chain/_/tags/my-feature"
	if err := store.WriteEntity(tagRef, "tag.txt", []byte(issueRef+"\n"), "create tag"); err != nil {
		t.Fatalf("failed to write tag: %v", err)
	}

	resolved, err := resolve.ResolveRef(store, "my-feature")
	if err != nil {
		t.Fatalf("ResolveRef(my-feature) unexpected error: %v", err)
	}
	if resolved != issueRef {
		t.Fatalf("ResolveRef(my-feature) = %q, want %q", resolved, issueRef)
	}
}

func TestResolveRef_TagNotFound(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	writeTestIssue(t, store, id, issue.StatePending, "Some Issue")

	// A nonexistent tag name should fall through to UUID prefix resolution,
	// then to title substring resolution, all of which fail for this input.
	_, err := resolve.ResolveRef(store, "nonexistent-tag")
	if err == nil {
		t.Fatal("expected error for nonexistent tag name, got nil")
	}
}

func TestResolveRef_TitleSubstring(t *testing.T) {
	store := initTestStore(t)

	gen := uuid.NewGen()

	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	id3, _ := gen.NewV7()

	writeTestIssue(t, store, id1, issue.StatePending, "Implement the parser module")
	writeTestIssue(t, store, id2, issue.StatePending, "Write integration tests")
	writeTestIssue(t, store, id3, issue.StatePending, "Deploy to production")

	// Substring match on a unique word.
	resolved, err := resolve.ResolveRef(store, "parser")
	if err != nil {
		t.Fatalf("ResolveRef(parser) unexpected error: %v", err)
	}
	expected := "refs/chain/_/issues/" + id1.String()
	if resolved != expected {
		t.Fatalf("ResolveRef(parser) = %q, want %q", resolved, expected)
	}

	// Case-insensitive match.
	resolved2, err2 := resolve.ResolveRef(store, "INTEGRATION")
	if err2 != nil {
		t.Fatalf("ResolveRef(INTEGRATION) unexpected error: %v", err2)
	}
	expected2 := "refs/chain/_/issues/" + id2.String()
	if resolved2 != expected2 {
		t.Fatalf("ResolveRef(INTEGRATION) = %q, want %q", resolved2, expected2)
	}

	// No match returns error.
	_, err3 := resolve.ResolveRef(store, "no-such-title-substring")
	if err3 == nil {
		t.Fatal("expected error for no-match title substring, got nil")
	}

	// Ambiguous match returns error: "ion" appears in "integration" and "production".
	_, err4 := resolve.ResolveRef(store, "ion")
	if err4 == nil {
		t.Fatal("expected error for ambiguous title substring, got nil")
	}
	if !strings.Contains(err4.Error(), "ambiguous") {
		t.Fatalf("expected 'ambiguous' in error, got: %v", err4)
	}
}
