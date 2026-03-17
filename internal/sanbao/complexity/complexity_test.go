// ABOUTME: Tests for language-agnostic code complexity metrics.
// ABOUTME: Covers FileChurn, FileSize, ChurnSizeHotspots, ChangeCoupling, and IndentationDepth.
package complexity_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/complexity"
)

// makeSession creates a Session with the given start and end SHAs.
func makeSession(startSHA, endSHA string) issue.Session {
	return issue.Session{StartSHA: startSHA, EndSHA: endSHA}
}

// makeIssueWithSessions creates a minimal issue with the given sessions.
func makeIssueWithSessions(sessions []issue.Session) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	return &issue.Issue{
		ID:       id,
		Title:    "test issue",
		State:    issue.StateDone,
		Milestone: "v0.2",
		Created:  time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		Updated:  time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		Sessions: sessions,
	}
}

// initRepoWithFileCommits creates a real on-disk git repo, makes commits that
// modify real files, and returns the repo, its root dir, and the commit SHAs
// (oldest first).
//
// commitFiles is a slice of maps: each map is { "filename": "content" } for one
// commit. Only listed files are added in that commit.
func initRepoWithFileCommits(t *testing.T, commitFiles []map[string]string) (*git.Repository, string, []string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("repo.Config: %v", err)
	}
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	var shas []string
	ts := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	for i, files := range commitFiles {
		for name, content := range files {
			fullPath := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile %s: %v", name, err)
			}
			if _, err := wt.Add(name); err != nil {
				t.Fatalf("wt.Add %s: %v", name, err)
			}
		}
		sig := &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  ts.Add(time.Duration(i) * time.Minute),
		}
		h, err := wt.Commit("commit "+string(rune('A'+i)), &git.CommitOptions{
			Author: sig,
		})
		if err != nil {
			t.Fatalf("Commit[%d]: %v", i, err)
		}
		shas = append(shas, h.String())
	}
	return repo, dir, shas
}

// TestFileChurn verifies modification counts per file across session ranges.
func TestFileChurn(t *testing.T) {
	t.Run("single session counts file modifications", func(t *testing.T) {
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1"},
			{"a.go": "v2"},
			{"b.go": "v1"},
			{"a.go": "v3"},
		})
		// Session covers commits 1–3 (after shas[0], through shas[3]).
		sessions := []issue.Session{makeSession(shas[0], shas[3])}
		churn := complexity.FileChurn(repo, sessions)

		// a.go was modified in commits 1, 3 = 2 times in range
		if churn["a.go"] != 2 {
			t.Errorf("expected a.go churn=2, got %d", churn["a.go"])
		}
		// b.go was modified in commit 2 = 1 time in range
		if churn["b.go"] != 1 {
			t.Errorf("expected b.go churn=1, got %d", churn["b.go"])
		}
	})

	t.Run("multiple sessions accumulate churn", func(t *testing.T) {
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1"},
			{"a.go": "v2"},
			{"a.go": "v3"},
			{"a.go": "v4"},
		})
		// Two sessions: [0→1] and [2→3]
		sessions := []issue.Session{
			makeSession(shas[0], shas[1]),
			makeSession(shas[2], shas[3]),
		}
		churn := complexity.FileChurn(repo, sessions)
		// a.go appears in both sessions (1 commit each) = 2
		if churn["a.go"] != 2 {
			t.Errorf("expected a.go churn=2 across sessions, got %d", churn["a.go"])
		}
	})

	t.Run("empty sessions returns empty map", func(t *testing.T) {
		repo, _, _ := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1"},
		})
		churn := complexity.FileChurn(repo, []issue.Session{})
		if len(churn) != 0 {
			t.Errorf("expected empty churn map, got %v", churn)
		}
	})

	t.Run("open session with empty EndSHA is skipped", func(t *testing.T) {
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1"},
			{"a.go": "v2"},
		})
		sessions := []issue.Session{makeSession(shas[0], "")}
		churn := complexity.FileChurn(repo, sessions)
		if len(churn) != 0 {
			t.Errorf("expected empty churn for open session, got %v", churn)
		}
	})
}

// TestFileSize verifies line counting on real files.
func TestFileSize(t *testing.T) {
	t.Run("file with known line count returns correct count", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		content := "line1\nline2\nline3\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		size := complexity.FileSize(dir, "test.go")
		if size != 3 {
			t.Errorf("expected 3 lines, got %d", size)
		}
	})

	t.Run("file without trailing newline counts all lines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		content := "line1\nline2\nline3"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		size := complexity.FileSize(dir, "test.go")
		if size != 3 {
			t.Errorf("expected 3 lines for no-trailing-newline, got %d", size)
		}
	})

	t.Run("empty file returns zero", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.go")
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		size := complexity.FileSize(dir, "empty.go")
		if size != 0 {
			t.Errorf("expected 0 for empty file, got %d", size)
		}
	})

	t.Run("missing file returns zero", func(t *testing.T) {
		dir := t.TempDir()
		size := complexity.FileSize(dir, "nonexistent.go")
		if size != 0 {
			t.Errorf("expected 0 for missing file, got %d", size)
		}
	})
}

// TestChurnSizeHotspots verifies ranking by churn * size product.
func TestChurnSizeHotspots(t *testing.T) {
	t.Run("hotspots ranked by churn times size descending", func(t *testing.T) {
		dir := t.TempDir()
		// big.go: 100 lines, churn=5 -> score=500
		// small.go: 10 lines, churn=3 -> score=30
		bigContent := ""
		for i := 0; i < 100; i++ {
			bigContent += "line\n"
		}
		smallContent := ""
		for i := 0; i < 10; i++ {
			smallContent += "line\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(bigContent), 0o644); err != nil {
			t.Fatalf("WriteFile big.go: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte(smallContent), 0o644); err != nil {
			t.Fatalf("WriteFile small.go: %v", err)
		}

		churn := map[string]int{
			"big.go":   5,
			"small.go": 3,
		}
		hotspots := complexity.ChurnSizeHotspots(churn, dir)

		if len(hotspots) < 2 {
			t.Fatalf("expected at least 2 hotspots, got %d", len(hotspots))
		}
		// First hotspot must be big.go (score 500 > 30)
		if hotspots[0].File != "big.go" {
			t.Errorf("expected big.go as top hotspot, got %s", hotspots[0].File)
		}
		if hotspots[0].Score != 500 {
			t.Errorf("expected score=500 for big.go, got %d", hotspots[0].Score)
		}
		if hotspots[1].File != "small.go" {
			t.Errorf("expected small.go as second hotspot, got %s", hotspots[1].File)
		}
	})

	t.Run("missing file in churn map gets zero size and zero score", func(t *testing.T) {
		dir := t.TempDir()
		churn := map[string]int{"missing.go": 5}
		hotspots := complexity.ChurnSizeHotspots(churn, dir)
		if len(hotspots) != 1 {
			t.Fatalf("expected 1 hotspot, got %d", len(hotspots))
		}
		if hotspots[0].Size != 0 {
			t.Errorf("expected size=0 for missing file, got %d", hotspots[0].Size)
		}
		if hotspots[0].Score != 0 {
			t.Errorf("expected score=0 for missing file, got %d", hotspots[0].Score)
		}
	})

	t.Run("empty churn returns empty hotspots", func(t *testing.T) {
		dir := t.TempDir()
		hotspots := complexity.ChurnSizeHotspots(map[string]int{}, dir)
		if len(hotspots) != 0 {
			t.Errorf("expected empty hotspots for empty churn, got %v", hotspots)
		}
	})

	t.Run("ChurnSizeHotspot struct has expected fields", func(t *testing.T) {
		dir := t.TempDir()
		content := "line1\nline2\n"
		if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		churn := map[string]int{"f.go": 3}
		hotspots := complexity.ChurnSizeHotspots(churn, dir)
		if len(hotspots) != 1 {
			t.Fatalf("expected 1 hotspot, got %d", len(hotspots))
		}
		h := hotspots[0]
		if h.File != "f.go" {
			t.Errorf("expected File=f.go, got %s", h.File)
		}
		if h.Churn != 3 {
			t.Errorf("expected Churn=3, got %d", h.Churn)
		}
		if h.Size != 2 {
			t.Errorf("expected Size=2, got %d", h.Size)
		}
		if h.Score != 6 {
			t.Errorf("expected Score=6 (3*2), got %d", h.Score)
		}
	})
}

// TestChangeCoupling verifies co-occurrence counting across commits.
func TestChangeCoupling(t *testing.T) {
	t.Run("two files modified together multiple times appear in coupling map", func(t *testing.T) {
		// Three commits: the first is the session start (excluded from range).
		// Commits 1 and 2 both touch a.go and b.go, giving count=2 which
		// clears the >1 threshold.
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1", "b.go": "v1"},   // session start (excluded)
			{"a.go": "v2", "b.go": "v2"},   // in range
			{"a.go": "v3", "b.go": "v3"},   // in range
		})
		// Session covers commits 1 and 2 (after shas[0], through shas[2]).
		sessions := []issue.Session{makeSession(shas[0], shas[2])}
		coupling := complexity.ChangeCoupling(repo, sessions)

		// Find the pair [a.go, b.go] regardless of key order
		found := false
		for pair, count := range coupling {
			files := []string{pair[0], pair[1]}
			sort.Strings(files)
			if files[0] == "a.go" && files[1] == "b.go" {
				found = true
				if count != 2 {
					t.Errorf("expected a.go+b.go coupling count=2, got %d", count)
				}
			}
		}
		if !found {
			t.Errorf("expected a.go+b.go pair in coupling map, got %v", coupling)
		}
	})

	t.Run("pair modified together multiple times has count equal to co-occurrences", func(t *testing.T) {
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1", "b.go": "v1"},
			{"a.go": "v2", "b.go": "v2"},
			{"a.go": "v3", "b.go": "v3"},
		})
		sessions := []issue.Session{makeSession(shas[0], shas[2])}
		coupling := complexity.ChangeCoupling(repo, sessions)

		found := false
		for pair, count := range coupling {
			files := []string{pair[0], pair[1]}
			sort.Strings(files)
			if files[0] == "a.go" && files[1] == "b.go" {
				found = true
				// commits 1 and 2 are in range (after shas[0]) = 2 co-occurrences
				if count != 2 {
					t.Errorf("expected coupling count=2, got %d", count)
				}
			}
		}
		if !found {
			t.Errorf("expected a.go+b.go in coupling map, got %v", coupling)
		}
	})

	t.Run("pair with count of 1 is excluded (only count > 1 returned)", func(t *testing.T) {
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1", "b.go": "v1"},  // single co-occurrence
			{"c.go": "v1"},                 // c alone (no pair)
		})
		sessions := []issue.Session{makeSession(shas[0], shas[1])}
		coupling := complexity.ChangeCoupling(repo, sessions)

		for pair, count := range coupling {
			if count <= 1 {
				t.Errorf("expected all pairs to have count > 1, got pair %v with count %d", pair, count)
			}
		}
	})

	t.Run("single file per commit produces no coupling", func(t *testing.T) {
		repo, _, shas := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1"},
			{"b.go": "v1"},
			{"c.go": "v1"},
		})
		sessions := []issue.Session{makeSession(shas[0], shas[2])}
		coupling := complexity.ChangeCoupling(repo, sessions)
		if len(coupling) != 0 {
			t.Errorf("expected no coupling for isolated file changes, got %v", coupling)
		}
	})

	t.Run("empty sessions returns empty coupling", func(t *testing.T) {
		repo, _, _ := initRepoWithFileCommits(t, []map[string]string{
			{"a.go": "v1"},
		})
		coupling := complexity.ChangeCoupling(repo, []issue.Session{})
		if len(coupling) != 0 {
			t.Errorf("expected empty coupling for no sessions, got %v", coupling)
		}
	})
}

// TestIndentationDepth verifies average and max indentation level computation.
func TestIndentationDepth(t *testing.T) {
	t.Run("tabs counted as one level each", func(t *testing.T) {
		dir := t.TempDir()
		// Line 1: 0 tabs (depth 0)
		// Line 2: 1 tab (depth 1)
		// Line 3: 2 tabs (depth 2)
		content := "top\n\tindent1\n\t\tindent2\n"
		if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		avg, max := complexity.IndentationDepth(dir, "f.go")
		// avg = (0+1+2)/3 = 1.0
		if avg < 0.99 || avg > 1.01 {
			t.Errorf("expected avg=1.0, got %f", avg)
		}
		if max != 2 {
			t.Errorf("expected max=2, got %d", max)
		}
	})

	t.Run("spaces counted as one level per 4 spaces", func(t *testing.T) {
		dir := t.TempDir()
		// depth 0, depth 1 (4 spaces), depth 2 (8 spaces)
		content := "top\n    indent1\n        indent2\n"
		if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		avg, max := complexity.IndentationDepth(dir, "f.go")
		if avg < 0.99 || avg > 1.01 {
			t.Errorf("expected avg=1.0 for 4-space indent, got %f", avg)
		}
		if max != 2 {
			t.Errorf("expected max=2 for 4-space indent, got %d", max)
		}
	})

	t.Run("empty lines are skipped in computation", func(t *testing.T) {
		dir := t.TempDir()
		// Only non-empty lines should count. Two non-empty lines at depths 0 and 1.
		content := "top\n\n\tindented\n\n"
		if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		avg, max := complexity.IndentationDepth(dir, "f.go")
		// avg = (0+1)/2 = 0.5
		if avg < 0.49 || avg > 0.51 {
			t.Errorf("expected avg=0.5, got %f", avg)
		}
		if max != 1 {
			t.Errorf("expected max=1, got %d", max)
		}
	})

	t.Run("all zero depth returns avg=0 and max=0", func(t *testing.T) {
		dir := t.TempDir()
		content := "line1\nline2\nline3\n"
		if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		avg, max := complexity.IndentationDepth(dir, "f.go")
		if avg != 0.0 {
			t.Errorf("expected avg=0.0, got %f", avg)
		}
		if max != 0 {
			t.Errorf("expected max=0, got %d", max)
		}
	})

	t.Run("missing file returns zeros", func(t *testing.T) {
		dir := t.TempDir()
		avg, max := complexity.IndentationDepth(dir, "nonexistent.go")
		if avg != 0.0 || max != 0 {
			t.Errorf("expected (0.0, 0) for missing file, got (%f, %d)", avg, max)
		}
	})

	t.Run("empty file returns zeros", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "empty.go"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		avg, max := complexity.IndentationDepth(dir, "empty.go")
		if avg != 0.0 || max != 0 {
			t.Errorf("expected (0.0, 0) for empty file, got (%f, %d)", avg, max)
		}
	})
}
