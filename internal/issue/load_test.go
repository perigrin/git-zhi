// ABOUTME: Tests for LoadAllIssues: loading all issues from storage refs,
// ABOUTME: verifying ID assignment from ref path, and empty-store behavior.
package issue_test

import (
	"fmt"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/storage"
)

func initLoadTestStore(t *testing.T) *storage.Store {
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

func TestLoadAllIssues_ReturnsIssuesWithIDs(t *testing.T) {
	store := initLoadTestStore(t)

	// Write two issues as git refs.
	issueIDs := []string{
		"019444a1-b2c3-7000-8000-000000000001",
		"019444a1-b2c3-7000-8000-000000000002",
	}
	for i, id := range issueIDs {
		content := fmt.Sprintf("---\ntitle: \"Issue %d\"\nstate: pending\nmilestone: v0.1\n---\n", i+1)
		ref := issue.RefPrefix + id
		if err := store.WriteEntity(ref, "issue.md", []byte(content), "create"); err != nil {
			t.Fatalf("WriteEntity for %s: %v", id, err)
		}
	}

	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues failed: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	for _, iss := range issues {
		if iss.ID.IsNil() {
			t.Errorf("issue %q has nil ID", iss.Title)
		}
	}
}

func TestLoadAllIssues_EmptyStore(t *testing.T) {
	store := initLoadTestStore(t)

	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues on empty store failed: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues, got %d", len(issues))
	}
}
