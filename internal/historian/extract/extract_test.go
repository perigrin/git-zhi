// ABOUTME: Tests for the historian extract stage: git log parsing and diff fingerprinting.
// ABOUTME: Uses real on-disk git repos to verify commit extraction, ticket refs, and fingerprints.
package extract_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-zhi/internal/historian/extract"
)

// makeTestRepo initialises a bare git repo in a temp dir and returns the
// go-git Repository and the worktree root path.
func makeTestRepo(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return repo, dir
}

// addCommit creates a file and commits it with the given message, author, and time.
// Returns the commit SHA string.
func addCommit(t *testing.T, repo *git.Repository, dir, filename, content, message, authorName, authorEmail string, when time.Time) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("wt.Add: %v", err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  when,
		},
	})
	if err != nil {
		t.Fatalf("wt.Commit: %v", err)
	}
	return hash.String()
}

func TestExtractCommits_BasicFields(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	addCommit(t, repo, dir, "alpha.txt", "hello", "first commit", "Alice", "alice@example.com", t0)

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	c := commits[0]
	if c.Author != "Alice" {
		t.Errorf("Author = %q, want %q", c.Author, "Alice")
	}
	if c.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", c.Email, "alice@example.com")
	}
	if c.Message != "first commit" {
		t.Errorf("Message = %q, want %q", c.Message, "first commit")
	}
	if c.SHA == "" {
		t.Error("SHA must not be empty")
	}
}

func TestExtractCommits_ChronologicalOrder(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	addCommit(t, repo, dir, "a.txt", "a", "first", "Alice", "a@a.com", t0)
	addCommit(t, repo, dir, "b.txt", "b", "second", "Alice", "a@a.com", t1)
	addCommit(t, repo, dir, "c.txt", "c", "third", "Alice", "a@a.com", t2)

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}
	// Oldest first.
	if commits[0].Message != "first" {
		t.Errorf("commits[0].Message = %q, want %q", commits[0].Message, "first")
	}
	if commits[1].Message != "second" {
		t.Errorf("commits[1].Message = %q, want %q", commits[1].Message, "second")
	}
	if commits[2].Message != "third" {
		t.Errorf("commits[2].Message = %q, want %q", commits[2].Message, "third")
	}
}

func TestExtractCommits_SinceFilter(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)
	t2 := t0.Add(48 * time.Hour)

	addCommit(t, repo, dir, "a.txt", "a", "old commit", "Alice", "a@a.com", t0)
	addCommit(t, repo, dir, "b.txt", "b", "recent one", "Alice", "a@a.com", t1)
	addCommit(t, repo, dir, "c.txt", "c", "newest", "Alice", "a@a.com", t2)

	// Only commits at or after t1.
	commits, err := extract.ExtractCommits(repo, &t1)
	if err != nil {
		t.Fatalf("ExtractCommits with since: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits after since filter, got %d", len(commits))
	}
	if commits[0].Message != "recent one" {
		t.Errorf("commits[0].Message = %q, want %q", commits[0].Message, "recent one")
	}
	if commits[1].Message != "newest" {
		t.Errorf("commits[1].Message = %q, want %q", commits[1].Message, "newest")
	}
}

func TestExtractTicketRefs_JiraStyle(t *testing.T) {
	cases := []struct {
		message string
		want    []string
	}{
		{"LOPS-142: fix the thing", []string{"LOPS-142"}},
		{"[LOPS-142] fix the thing", []string{"LOPS-142"}},
		{"fix the thing LOPS-142", []string{"LOPS-142"}},
		{"fix LOPS-142 and LOPS-200", []string{"LOPS-142", "LOPS-200"}},
		{"no ticket here", []string{}},
	}
	for _, tc := range cases {
		refs := extract.ExtractTicketRefs(tc.message)
		if len(refs) != len(tc.want) {
			t.Errorf("message=%q: got %v, want %v", tc.message, refs, tc.want)
			continue
		}
		for i := range refs {
			if refs[i] != tc.want[i] {
				t.Errorf("message=%q: refs[%d]=%q, want %q", tc.message, i, refs[i], tc.want[i])
			}
		}
	}
}

func TestExtractTicketRefs_GitHubStyle(t *testing.T) {
	refs := extract.ExtractTicketRefs("Fixes #42 and #100")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %v", refs)
	}
	// Result must be sorted.
	want := []string{"#100", "#42"}
	for i := range refs {
		if refs[i] != want[i] {
			t.Errorf("refs[%d]=%q, want %q", i, refs[i], want[i])
		}
	}
}

func TestExtractTicketRefs_Deduplication(t *testing.T) {
	refs := extract.ExtractTicketRefs("LOPS-142 LOPS-142 LOPS-142")
	if len(refs) != 1 {
		t.Fatalf("expected 1 unique ref, got %v", refs)
	}
	if refs[0] != "LOPS-142" {
		t.Errorf("got %q, want LOPS-142", refs[0])
	}
}

func TestExtractTicketRefs_Sorted(t *testing.T) {
	refs := extract.ExtractTicketRefs("LOPS-200 LOPS-10 LOPS-100")
	if !sort.StringsAreSorted(refs) {
		t.Errorf("refs not sorted: %v", refs)
	}
}

func TestComputeFingerprint_SingleFileCommit(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	// First commit: add alpha.txt with 3 lines.
	addCommit(t, repo, dir, "alpha.txt", "line1\nline2\nline3\n", "add alpha", "Alice", "a@a.com", t0)

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	fp := commits[0].Fingerprint
	if fp.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1", fp.FileCount)
	}
	if fp.Insertions != 3 {
		t.Errorf("Insertions = %d, want 3", fp.Insertions)
	}
	if fp.Deletions != 0 {
		t.Errorf("Deletions = %d, want 0", fp.Deletions)
	}
	if len(fp.Paths) != 1 || fp.Paths[0] != "alpha.txt" {
		t.Errorf("Paths = %v, want [alpha.txt]", fp.Paths)
	}
}

func TestComputeFingerprint_MultiFileCommit(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	// First commit: establish baseline.
	addCommit(t, repo, dir, "a.txt", "line1\nline2\n", "first", "Alice", "a@a.com", t0)

	// Second commit: add b.txt and c.txt via separate files — simulate by
	// writing two files and creating one commit.
	wt, _ := repo.Worktree()
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc\nddd\n"), 0644)
	wt.Add("b.txt")
	wt.Add("c.txt")
	wt.Commit("second: add b and c", &git.CommitOptions{
		Author: &object.Signature{Name: "Alice", Email: "a@a.com", When: t1},
	})

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	// Second commit (index 1 = chronologically last) touches 2 files.
	fp := commits[1].Fingerprint
	if fp.FileCount != 2 {
		t.Errorf("FileCount = %d, want 2", fp.FileCount)
	}
	if fp.Insertions != 3 {
		t.Errorf("Insertions = %d, want 3 (1 + 2 lines)", fp.Insertions)
	}
	if len(fp.Paths) != 2 {
		t.Errorf("Paths = %v, want 2 paths", fp.Paths)
	}
}

func TestComputeFingerprint_ChangeRatio(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	// First commit: 2 lines.
	addCommit(t, repo, dir, "f.txt", "line1\nline2\n", "initial", "Alice", "a@a.com", t0)
	// Second commit: replace with 4 lines (2 deletions + 4 insertions).
	addCommit(t, repo, dir, "f.txt", "new1\nnew2\nnew3\nnew4\n", "replace content", "Alice", "a@a.com", t1)

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	second := commits[1]
	fp := second.Fingerprint
	// ChangeRatio = insertions / (deletions + 1)
	// = 4 / (2 + 1) ≈ 1.333...
	if fp.Insertions == 0 {
		t.Error("expected non-zero insertions")
	}
	if fp.Deletions == 0 {
		t.Error("expected non-zero deletions")
	}
	expectedRatio := float64(fp.Insertions) / float64(fp.Deletions+1)
	if fp.ChangeRatio != expectedRatio {
		t.Errorf("ChangeRatio = %f, want %f", fp.ChangeRatio, expectedRatio)
	}
}

func TestExtractCommits_TicketRefsInMessage(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	addCommit(t, repo, dir, "x.txt", "x", "LOPS-42: implement the widget", "Dev", "dev@co.com", t0)

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	refs := commits[0].TicketRefs
	if len(refs) != 1 || refs[0] != "LOPS-42" {
		t.Errorf("TicketRefs = %v, want [LOPS-42]", refs)
	}
}

func TestExtractCommits_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits on empty repo: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(commits))
	}
}

func TestComputeFingerprint_PathsSorted(t *testing.T) {
	repo, dir := makeTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	addCommit(t, repo, dir, "z.txt", "z", "first", "Alice", "a@a.com", t0)

	wt, _ := repo.Worktree()
	os.WriteFile(filepath.Join(dir, "m.txt"), []byte("m\n"), 0644)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644)
	wt.Add("m.txt")
	wt.Add("a.txt")
	wt.Commit("add m and a", &git.CommitOptions{
		Author: &object.Signature{Name: "Alice", Email: "a@a.com", When: t1},
	})

	commits, err := extract.ExtractCommits(repo, nil)
	if err != nil {
		t.Fatalf("ExtractCommits: %v", err)
	}
	fp := commits[1].Fingerprint
	if !sort.StringsAreSorted(fp.Paths) {
		t.Errorf("Paths not sorted: %v", fp.Paths)
	}
}
