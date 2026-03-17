// ABOUTME: Tests for the lineage package: BuildSessionIndex maps session SHA ranges
// ABOUTME: to issue IDs, and ComputeLineage runs git blame to count lines per issue.
package lineage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/lineage"
)

// initTestRepo creates a new temporary git repository and returns the
// repository and its working directory path.
func initTestRepo(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return repo, dir
}

// makeCommit writes filename with content, stages it, and commits. Returns the
// commit SHA string.
func makeCommit(t *testing.T, repo *git.Repository, dir, filename, content, message string) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", filename, err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("git add %s: %v", filename, err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return hash.String()
}

// newIssueID returns a deterministic UUID for testing purposes using a fixed
// namespace and the given name string.
func newIssueID(t *testing.T, name string) uuid.UUID {
	t.Helper()
	ns := uuid.Must(uuid.FromString("019444a1-0000-7000-8000-000000000000"))
	return uuid.NewV5(ns, name)
}

// TestBuildSessionIndex_MapsCommitSHAsToIssue verifies that BuildSessionIndex
// produces a map from each commit SHA in a session range to the issue's UUID.
func TestBuildSessionIndex_MapsCommitSHAsToIssue(t *testing.T) {
	repo, dir := initTestRepo(t)

	sha1 := makeCommit(t, repo, dir, "a.txt", "line1\n", "first")
	sha2 := makeCommit(t, repo, dir, "a.txt", "line1\nline2\n", "second")
	sha3 := makeCommit(t, repo, dir, "a.txt", "line1\nline2\nline3\n", "third")

	id := newIssueID(t, "issue-a")
	iss := &issue.Issue{
		ID:    id,
		Title: "Issue A",
		State: issue.StateDone,
		Sessions: []issue.Session{
			{StartSHA: sha1, EndSHA: sha3, Commits: 2},
		},
	}

	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})

	// sha2 and sha3 are between sha1 (exclusive) and sha3 (inclusive)
	if got, ok := idx[sha2]; !ok || got != id {
		t.Errorf("expected sha2 %q → issueID %v, got %v (ok=%v)", sha2, id, got, ok)
	}
	if got, ok := idx[sha3]; !ok || got != id {
		t.Errorf("expected sha3 %q → issueID %v, got %v (ok=%v)", sha3, id, got, ok)
	}
	// sha1 is the exclusive start — it should NOT be mapped to this issue
	if _, ok := idx[sha1]; ok {
		t.Errorf("sha1 (exclusive start) should not be in index, but it was mapped")
	}
}

// TestBuildSessionIndex_SkipsNonDoneIssues verifies that issues not in the
// done state are not indexed.
func TestBuildSessionIndex_SkipsNonDoneIssues(t *testing.T) {
	repo, dir := initTestRepo(t)

	sha1 := makeCommit(t, repo, dir, "b.txt", "v1\n", "first")
	sha2 := makeCommit(t, repo, dir, "b.txt", "v2\n", "second")

	id := newIssueID(t, "in-progress-issue")
	iss := &issue.Issue{
		ID:    id,
		Title: "In Progress",
		State: issue.StateInProgress,
		Sessions: []issue.Session{
			{StartSHA: sha1, EndSHA: sha2, Commits: 1},
		},
	}

	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})

	if _, ok := idx[sha2]; ok {
		t.Errorf("in-progress issue should not be indexed, but sha2 was found")
	}
}

// TestBuildSessionIndex_SkipsIssuesWithNoSessions verifies that done issues
// without any sessions produce no index entries.
func TestBuildSessionIndex_SkipsIssuesWithNoSessions(t *testing.T) {
	repo, dir := initTestRepo(t)
	_ = makeCommit(t, repo, dir, "c.txt", "v1\n", "first")

	id := newIssueID(t, "no-sessions")
	iss := &issue.Issue{
		ID:       id,
		Title:    "No Sessions",
		State:    issue.StateDone,
		Sessions: []issue.Session{},
	}

	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})
	if len(idx) != 0 {
		t.Errorf("expected empty index for issue with no sessions, got %d entries", len(idx))
	}
}

// TestComputeLineage_CountsLinesPerIssue verifies that ComputeLineage maps
// blame lines to issues and aggregates per-issue line counts.
func TestComputeLineage_CountsLinesPerIssue(t *testing.T) {
	repo, dir := initTestRepo(t)

	// sha1 is the start of the session (exclusive), sha2 writes 3 lines,
	// sha3 adds 2 more lines. Both sha2 and sha3 are within the session range.
	sha1 := makeCommit(t, repo, dir, "src.txt", "alpha\nbeta\ngamma\n", "initial")
	sha2 := makeCommit(t, repo, dir, "src.txt", "alpha\nbeta\ngamma\ndelta\nepsilon\n", "add two lines")

	id := newIssueID(t, "compute-lineage-issue")
	iss := &issue.Issue{
		ID:    id,
		Title: "Compute Lineage Issue",
		State: issue.StateDone,
		Sessions: []issue.Session{
			{StartSHA: sha1, EndSHA: sha2, Commits: 1},
		},
	}
	issueMap := map[uuid.UUID]*issue.Issue{id: iss}

	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})
	entries := lineage.ComputeLineage(repo, []string{"src.txt"}, idx, issueMap)

	if len(entries) == 0 {
		t.Fatal("expected at least one lineage entry, got none")
	}

	// sha2 added lines 4 and 5 (delta, epsilon). sha1's lines 1-3 are not
	// in the session index. So we expect 2 lines traced to this issue.
	var found *lineage.LineageEntry
	for i := range entries {
		if entries[i].UpstreamIssueID == id {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no lineage entry found for issue %v", id)
	}
	if found.FilePath != "src.txt" {
		t.Errorf("expected FilePath=src.txt, got %q", found.FilePath)
	}
	if found.LineCount != 2 {
		t.Errorf("expected LineCount=2 (lines added in sha2), got %d", found.LineCount)
	}
	if found.Title != iss.Title {
		t.Errorf("expected Title=%q, got %q", iss.Title, found.Title)
	}
}

// TestComputeLineage_EmptyFilesSlice verifies that passing no files returns
// an empty result.
func TestComputeLineage_EmptyFilesSlice(t *testing.T) {
	repo, dir := initTestRepo(t)
	sha1 := makeCommit(t, repo, dir, "x.txt", "hello\n", "init")
	sha2 := makeCommit(t, repo, dir, "x.txt", "hello\nworld\n", "add line")

	id := newIssueID(t, "empty-files-issue")
	iss := &issue.Issue{
		ID:       id,
		State:    issue.StateDone,
		Sessions: []issue.Session{{StartSHA: sha1, EndSHA: sha2, Commits: 1}},
	}
	issueMap := map[uuid.UUID]*issue.Issue{id: iss}
	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})

	entries := lineage.ComputeLineage(repo, []string{}, idx, issueMap)
	if len(entries) != 0 {
		t.Errorf("expected empty result for empty file list, got %d entries", len(entries))
	}
}

// TestComputeLineage_FileWithNoBlameMatches verifies that a file whose blame
// lines are not in the session index produces no lineage entries.
func TestComputeLineage_FileWithNoBlameMatches(t *testing.T) {
	repo, dir := initTestRepo(t)

	// Commit the file before any session starts
	sha0 := makeCommit(t, repo, dir, "pre.txt", "old line\n", "pre-session")
	_ = sha0

	// Issue session is sha0→sha0 (same SHA = no commits in range)
	id := newIssueID(t, "no-match-issue")
	iss := &issue.Issue{
		ID:    id,
		State: issue.StateDone,
		Sessions: []issue.Session{
			{StartSHA: sha0, EndSHA: sha0, Commits: 0},
		},
	}
	issueMap := map[uuid.UUID]*issue.Issue{id: iss}
	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})

	entries := lineage.ComputeLineage(repo, []string{"pre.txt"}, idx, issueMap)
	if len(entries) != 0 {
		t.Errorf("expected no lineage for file with no session matches, got %d entries", len(entries))
	}
}

// TestComputeLineage_NonexistentFileSkipped verifies that a file that does not
// exist in the repo is silently skipped.
func TestComputeLineage_NonexistentFileSkipped(t *testing.T) {
	repo, dir := initTestRepo(t)
	sha1 := makeCommit(t, repo, dir, "real.txt", "content\n", "init")

	id := newIssueID(t, "nonexistent-file-issue")
	iss := &issue.Issue{
		ID:    id,
		State: issue.StateDone,
		Sessions: []issue.Session{
			{StartSHA: sha1, EndSHA: sha1, Commits: 0},
		},
	}
	issueMap := map[uuid.UUID]*issue.Issue{id: iss}
	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{iss})

	entries := lineage.ComputeLineage(repo, []string{"does_not_exist.txt"}, idx, issueMap)
	if len(entries) != 0 {
		t.Errorf("expected no entries for nonexistent file, got %d", len(entries))
	}
}

// TestComputeLineage_SortedByLineCountDescending verifies that entries are
// returned with highest line count first.
func TestComputeLineage_SortedByLineCountDescending(t *testing.T) {
	repo, dir := initTestRepo(t)

	// Create two issues with different session ranges.
	// Issue A: adds 1 line
	// Issue B: adds 3 lines
	sha0 := makeCommit(t, repo, dir, "sort.txt", "line1\n", "baseline")
	sha1 := makeCommit(t, repo, dir, "sort.txt", "line1\nline2\n", "issue-a adds 1")
	sha2 := makeCommit(t, repo, dir, "sort.txt", "line1\nline2\nline3\nline4\nline5\n", "issue-b adds 3")

	idA := newIssueID(t, "sort-issue-a")
	idB := newIssueID(t, "sort-issue-b")

	issA := &issue.Issue{
		ID:    idA,
		Title: "Issue A",
		State: issue.StateDone,
		Sessions: []issue.Session{
			{StartSHA: sha0, EndSHA: sha1, Commits: 1},
		},
	}
	issB := &issue.Issue{
		ID:    idB,
		Title: "Issue B",
		State: issue.StateDone,
		Sessions: []issue.Session{
			{StartSHA: sha1, EndSHA: sha2, Commits: 1},
		},
	}
	issueMap := map[uuid.UUID]*issue.Issue{idA: issA, idB: issB}
	idx := lineage.BuildSessionIndex(repo, []*issue.Issue{issA, issB})

	entries := lineage.ComputeLineage(repo, []string{"sort.txt"}, idx, issueMap)

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 lineage entries, got %d", len(entries))
	}
	if entries[0].LineCount < entries[1].LineCount {
		t.Errorf("entries not sorted descending: entries[0].LineCount=%d < entries[1].LineCount=%d",
			entries[0].LineCount, entries[1].LineCount)
	}
}
