// ABOUTME: Tests for label index management: BuildLabelIndexes and LoadLabelIndex.
// ABOUTME: Verifies correct ref creation, UUID extraction, idempotency, and edge cases.
package issue_test

import (
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/storage"
)

func initIndexTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func makeIssueWithLabels(t *testing.T, store *storage.Store, title string, labels []string) *issue.Issue {
	t.Helper()
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
		Labels:    labels,
		Created:   now,
		Updated:   now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := issue.RefPrefix + id.String()
	if err := store.WriteEntity(refPath, "issue.md", data, "add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return iss
}

// TestBuildLabelIndexes_CreatesRefs verifies that BuildLabelIndexes creates
// a ref under refs/zhi/<label>/ for each issue-label pair.
func TestBuildLabelIndexes_CreatesRefs(t *testing.T) {
	store := initIndexTestStore(t)

	iss := makeIssueWithLabels(t, store, "Frontend task", []string{"frontend", "ui"})

	issues := []*issue.Issue{iss}
	if err := issue.BuildLabelIndexes(store, issues); err != nil {
		t.Fatalf("BuildLabelIndexes: %v", err)
	}

	// Verify refs exist for both labels.
	for _, label := range []string{"frontend", "ui"} {
		ids, err := issue.LoadLabelIndex(store, label)
		if err != nil {
			t.Fatalf("LoadLabelIndex(%q): %v", label, err)
		}
		if len(ids) != 1 {
			t.Fatalf("expected 1 issue for label %q, got %d", label, len(ids))
		}
		if ids[0] != iss.ID {
			t.Errorf("expected issue ID %s for label %q, got %s", iss.ID, label, ids[0])
		}
	}
}

// TestBuildLabelIndexes_MultipleIssues verifies that multiple issues with the
// same label all appear in that label's index.
func TestBuildLabelIndexes_MultipleIssues(t *testing.T) {
	store := initIndexTestStore(t)

	iss1 := makeIssueWithLabels(t, store, "Task one", []string{"backend"})
	iss2 := makeIssueWithLabels(t, store, "Task two", []string{"backend"})
	iss3 := makeIssueWithLabels(t, store, "Task three", []string{"frontend"})

	issues := []*issue.Issue{iss1, iss2, iss3}
	if err := issue.BuildLabelIndexes(store, issues); err != nil {
		t.Fatalf("BuildLabelIndexes: %v", err)
	}

	backendIDs, err := issue.LoadLabelIndex(store, "backend")
	if err != nil {
		t.Fatalf("LoadLabelIndex(backend): %v", err)
	}
	if len(backendIDs) != 2 {
		t.Fatalf("expected 2 issues for label 'backend', got %d", len(backendIDs))
	}

	frontendIDs, err := issue.LoadLabelIndex(store, "frontend")
	if err != nil {
		t.Fatalf("LoadLabelIndex(frontend): %v", err)
	}
	if len(frontendIDs) != 1 {
		t.Fatalf("expected 1 issue for label 'frontend', got %d", len(frontendIDs))
	}
	if frontendIDs[0] != iss3.ID {
		t.Errorf("expected issue ID %s for 'frontend', got %s", iss3.ID, frontendIDs[0])
	}
}

// TestBuildLabelIndexes_NoLabels verifies that issues with no labels create
// no index entries.
func TestBuildLabelIndexes_NoLabels(t *testing.T) {
	store := initIndexTestStore(t)

	iss := makeIssueWithLabels(t, store, "Unlabeled task", nil)

	issues := []*issue.Issue{iss}
	if err := issue.BuildLabelIndexes(store, issues); err != nil {
		t.Fatalf("BuildLabelIndexes: %v", err)
	}

	// Load all refs under refs/zhi/ — index refs would live there.
	refs, err := store.ListRefs("refs/zhi/")
	if err != nil {
		t.Fatalf("ListRefs: %v", err)
	}
	// Only the issue ref should exist; no label index refs.
	for _, ref := range refs {
		if ref != issue.RefPrefix+iss.ID.String() {
			t.Errorf("unexpected ref created for unlabeled issue: %s", ref)
		}
	}
}

// TestBuildLabelIndexes_Idempotent verifies that calling BuildLabelIndexes
// twice produces the same result as calling it once.
func TestBuildLabelIndexes_Idempotent(t *testing.T) {
	store := initIndexTestStore(t)

	iss := makeIssueWithLabels(t, store, "Idempotent task", []string{"ops"})
	issues := []*issue.Issue{iss}

	if err := issue.BuildLabelIndexes(store, issues); err != nil {
		t.Fatalf("first BuildLabelIndexes: %v", err)
	}
	if err := issue.BuildLabelIndexes(store, issues); err != nil {
		t.Fatalf("second BuildLabelIndexes: %v", err)
	}

	ids, err := issue.LoadLabelIndex(store, "ops")
	if err != nil {
		t.Fatalf("LoadLabelIndex(ops): %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected exactly 1 entry after idempotent build, got %d", len(ids))
	}
}

// TestBuildLabelIndexes_RebuildClearsOldEntries verifies that if an issue's
// labels change between builds, stale index entries are replaced.
func TestBuildLabelIndexes_RebuildClearsOldEntries(t *testing.T) {
	store := initIndexTestStore(t)

	iss1 := makeIssueWithLabels(t, store, "Task alpha", []string{"old-label"})
	iss2 := makeIssueWithLabels(t, store, "Task beta", []string{"new-label"})

	// Build initial indexes with iss1 having "old-label".
	if err := issue.BuildLabelIndexes(store, []*issue.Issue{iss1}); err != nil {
		t.Fatalf("first BuildLabelIndexes: %v", err)
	}

	// Rebuild with a new issue set — iss1 is gone, iss2 has "new-label".
	if err := issue.BuildLabelIndexes(store, []*issue.Issue{iss2}); err != nil {
		t.Fatalf("second BuildLabelIndexes: %v", err)
	}

	// "old-label" index should be empty (cleared during rebuild for that label).
	oldIDs, err := issue.LoadLabelIndex(store, "old-label")
	if err != nil {
		t.Fatalf("LoadLabelIndex(old-label): %v", err)
	}
	if len(oldIDs) != 0 {
		t.Fatalf("expected old-label index to be empty after rebuild, got %d entries", len(oldIDs))
	}

	// "new-label" index should have iss2.
	newIDs, err := issue.LoadLabelIndex(store, "new-label")
	if err != nil {
		t.Fatalf("LoadLabelIndex(new-label): %v", err)
	}
	if len(newIDs) != 1 || newIDs[0] != iss2.ID {
		t.Fatalf("expected new-label index to contain iss2, got %v", newIDs)
	}
}

// TestLoadLabelIndex_EmptyLabel verifies that LoadLabelIndex returns an empty
// slice (not an error) when no issues are indexed for a given label.
func TestLoadLabelIndex_EmptyLabel(t *testing.T) {
	store := initIndexTestStore(t)

	ids, err := issue.LoadLabelIndex(store, "nonexistent")
	if err != nil {
		t.Fatalf("LoadLabelIndex on nonexistent label should not error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(ids))
	}
}

// TestBuildLabelIndexes_RejectsUnderscoreLabel verifies that the reserved
// label "_" is rejected to prevent collision with the refs/zhi/_/ namespace.
func TestBuildLabelIndexes_RejectsUnderscoreLabel(t *testing.T) {
	store := initIndexTestStore(t)

	iss := makeIssueWithLabels(t, store, "Reserved label issue", []string{"_"})
	err := issue.BuildLabelIndexes(store, []*issue.Issue{iss})
	if err == nil {
		t.Fatal("expected error for label '_', got nil")
	}
	if !strings.Contains(err.Error(), "_") {
		t.Errorf("expected error to mention the invalid label, got: %v", err)
	}
}

// TestBuildLabelIndexes_RejectsSlashInLabel verifies that labels containing
// "/" are rejected to prevent malformed ref paths.
func TestBuildLabelIndexes_RejectsSlashInLabel(t *testing.T) {
	store := initIndexTestStore(t)

	iss := makeIssueWithLabels(t, store, "Slash label issue", []string{"team/subteam"})
	err := issue.BuildLabelIndexes(store, []*issue.Issue{iss})
	if err == nil {
		t.Fatal("expected error for label containing '/', got nil")
	}
	if !strings.Contains(err.Error(), "/") {
		t.Errorf("expected error to mention the invalid label, got: %v", err)
	}
}

// TestBuildLabelIndexes_MultipleLabelsPerIssue verifies that an issue with
// multiple labels appears in all of those labels' indexes.
func TestBuildLabelIndexes_MultipleLabelsPerIssue(t *testing.T) {
	store := initIndexTestStore(t)

	iss := makeIssueWithLabels(t, store, "Multi-label task", []string{"alpha", "beta", "gamma"})
	issues := []*issue.Issue{iss}

	if err := issue.BuildLabelIndexes(store, issues); err != nil {
		t.Fatalf("BuildLabelIndexes: %v", err)
	}

	for _, label := range []string{"alpha", "beta", "gamma"} {
		ids, err := issue.LoadLabelIndex(store, label)
		if err != nil {
			t.Fatalf("LoadLabelIndex(%q): %v", label, err)
		}
		if len(ids) != 1 || ids[0] != iss.ID {
			t.Errorf("expected issue %s in label %q index, got %v", iss.ID, label, ids)
		}
	}
}
