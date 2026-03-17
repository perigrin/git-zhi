// ABOUTME: Tests for project domain model: LoadProject, ComputeProjectStatus,
// ABOUTME: NextForActor, and buffer computation across multiple repos.
package project_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/goccy/go-yaml"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/project"
	"github.com/perigrin/git-zhi/internal/storage"
)

// ---------------------------------------------------------------------------
// Test repo helpers
// ---------------------------------------------------------------------------

// makeTestRepo creates a minimal git repo with one commit, initializes chain
// state, and returns the directory path and App.
func makeTestRepo(t *testing.T) (string, *cli.App) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}
	// Make an initial commit so the repo is non-empty.
	wt, _ := repo.Worktree()
	_ = os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0644)
	_, _ = wt.Add("README")
	_, _ = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	return dir, app
}

// writeIssue writes an issue to the store and returns its UUID.
func writeIssue(t *testing.T, app *cli.App, iss *issue.Issue) uuid.UUID {
	t.Helper()
	gen := uuid.NewGen()
	id, err := gen.NewV7()
	if err != nil {
		t.Fatalf("NewV7: %v", err)
	}
	iss.ID = id
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal issue: %v", err)
	}
	ref := issue.RefPrefix + id.String()
	if err := app.Store.WriteEntity(ref, "issue.md", data, "add issue"); err != nil {
		t.Fatalf("WriteEntity issue: %v", err)
	}
	return id
}

// writeMilestone writes a milestone to the store.
func writeMilestone(t *testing.T, app *cli.App, ms *milestone.Milestone) {
	t.Helper()
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	ref := milestone.RefPrefix + ms.Name
	if err := app.Store.WriteEntity(ref, "milestone.yaml", data, "add milestone"); err != nil {
		t.Fatalf("WriteEntity milestone: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LoadProject tests
// ---------------------------------------------------------------------------

func TestLoadProject_ParsesYAMLFile(t *testing.T) {
	def := &project.ProjectDef{
		Name: "Test Project",
		Repos: []project.RepoDef{
			{Path: "/tmp/repo-a", Milestone: "v1.0"},
			{
				Path:      "/tmp/repo-b",
				Milestone: "v2.0",
				Feeds: []project.FeedDef{
					{Repo: "repo-a", Buffer: "3d"},
				},
			},
		},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{"repo-a", "repo-b"}},
		},
	}

	// Write to a temp file.
	tmpFile := filepath.Join(t.TempDir(), "project.yaml")
	data, err := yaml.Marshal(def)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := project.LoadProject(tmpFile)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	if loaded.Name != "Test Project" {
		t.Errorf("expected Name=%q, got %q", "Test Project", loaded.Name)
	}
	if len(loaded.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.Repos))
	}
	if loaded.Repos[0].Path != "/tmp/repo-a" {
		t.Errorf("expected Repos[0].Path=%q, got %q", "/tmp/repo-a", loaded.Repos[0].Path)
	}
	if loaded.Repos[1].Milestone != "v2.0" {
		t.Errorf("expected Repos[1].Milestone=%q, got %q", "v2.0", loaded.Repos[1].Milestone)
	}
	if len(loaded.Repos[1].Feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(loaded.Repos[1].Feeds))
	}
	if loaded.Repos[1].Feeds[0].Buffer != "3d" {
		t.Errorf("expected Feed.Buffer=%q, got %q", "3d", loaded.Repos[1].Feeds[0].Buffer)
	}
	if len(loaded.Workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(loaded.Workers))
	}
	if loaded.Workers[0].Name != "alice" {
		t.Errorf("expected Workers[0].Name=%q, got %q", "alice", loaded.Workers[0].Name)
	}
}

func TestLoadProject_MissingFile(t *testing.T) {
	_, err := project.LoadProject("/nonexistent/path/project.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadProject_InvalidYAML(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(tmpFile, []byte("not: valid: yaml: [broken"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := project.LoadProject(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// ---------------------------------------------------------------------------
// ComputeProjectStatus tests
// ---------------------------------------------------------------------------

func TestComputeProjectStatus_BasicAggregation(t *testing.T) {
	dirA, appA := makeTestRepo(t)
	dirB, appB := makeTestRepo(t)

	now := time.Now()

	// Milestone v1.0 in repo A: 2 issues, 1 done.
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	writeIssue(t, appA, &issue.Issue{
		Title: "A done", State: issue.StateDone, Milestone: "v1.0",
		Created: now, Updated: now,
	})
	writeIssue(t, appA, &issue.Issue{
		Title: "A pending", State: issue.StatePending, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	// Milestone v2.0 in repo B: 3 issues, 3 done.
	msB := &milestone.Milestone{Name: "v2.0", Created: now}
	writeMilestone(t, appB, msB)
	for i := 0; i < 3; i++ {
		writeIssue(t, appB, &issue.Issue{
			Title: "B done", State: issue.StateDone, Milestone: "v2.0",
			Created: now, Updated: now,
		})
	}

	def := &project.ProjectDef{
		Name: "Auth Overhaul",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
			{Path: dirB, Milestone: "v2.0"},
		},
	}

	status, err := project.ComputeProjectStatus(def)
	if err != nil {
		t.Fatalf("ComputeProjectStatus: %v", err)
	}

	if status.Name != "Auth Overhaul" {
		t.Errorf("expected Name=%q, got %q", "Auth Overhaul", status.Name)
	}
	if len(status.Repos) != 2 {
		t.Fatalf("expected 2 repo statuses, got %d", len(status.Repos))
	}

	// Repo A: 1/2 done = 50%.
	var repoA, repoB project.RepoStatus
	for _, r := range status.Repos {
		if r.Milestone == "v1.0" {
			repoA = r
		} else if r.Milestone == "v2.0" {
			repoB = r
		}
	}

	if repoA.IssuesTotal != 2 {
		t.Errorf("expected repoA.IssuesTotal=2, got %d", repoA.IssuesTotal)
	}
	if repoA.IssuesDone != 1 {
		t.Errorf("expected repoA.IssuesDone=1, got %d", repoA.IssuesDone)
	}
	if repoA.Progress != 0.5 {
		t.Errorf("expected repoA.Progress=0.5, got %f", repoA.Progress)
	}

	// Repo B: 3/3 done = 100%.
	if repoB.IssuesTotal != 3 {
		t.Errorf("expected repoB.IssuesTotal=3, got %d", repoB.IssuesTotal)
	}
	if repoB.IssuesDone != 3 {
		t.Errorf("expected repoB.IssuesDone=3, got %d", repoB.IssuesDone)
	}
	if repoB.Progress != 1.0 {
		t.Errorf("expected repoB.Progress=1.0, got %f", repoB.Progress)
	}
}

func TestComputeProjectStatus_MissingRepoPath(t *testing.T) {
	def := &project.ProjectDef{
		Name: "Bad Project",
		Repos: []project.RepoDef{
			{Path: "/nonexistent/repo/path", Milestone: "v1.0"},
		},
	}
	_, err := project.ComputeProjectStatus(def)
	if err == nil {
		t.Fatal("expected error for missing repo path, got nil")
	}
}

func TestComputeProjectStatus_ProjectBuffer(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)

	// 4 issues total, 0 done — buffer should be non-zero and GREEN.
	for i := 0; i < 4; i++ {
		writeIssue(t, appA, &issue.Issue{
			Title: "pending issue", State: issue.StatePending, Milestone: "v1.0",
			Created: now, Updated: now,
		})
	}

	def := &project.ProjectDef{
		Name:  "Buffer Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
	}
	status, err := project.ComputeProjectStatus(def)
	if err != nil {
		t.Fatalf("ComputeProjectStatus: %v", err)
	}

	// Project buffer size should be 50% of critical chain (> 0).
	if status.ProjectBuffer.SizeDays <= 0 {
		t.Errorf("expected ProjectBuffer.SizeDays > 0, got %f", status.ProjectBuffer.SizeDays)
	}
	// With 0% consumed and a full buffer, should be GREEN.
	if status.ProjectBuffer.Status != "GREEN" {
		t.Errorf("expected ProjectBuffer.Status=GREEN, got %q", status.ProjectBuffer.Status)
	}
}

func TestComputeProjectStatus_BufferThresholds(t *testing.T) {
	// Test buffer status thresholds: GREEN < 1/3, YELLOW 1/3-2/3, RED > 2/3
	testCases := []struct {
		name       string
		consumed   float64
		size       float64
		wantStatus string
	}{
		{"green_zero", 0, 9, "GREEN"},
		{"green_low", 2, 9, "GREEN"},            // 2/9 ≈ 22% < 1/3
		{"yellow_mid", 4, 9, "YELLOW"},           // 4/9 ≈ 44% between 1/3 and 2/3
		{"red_high", 7, 9, "RED"},                // 7/9 ≈ 78% > 2/3
		{"red_full", 9, 9, "RED"},                // 100% > 2/3
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := project.BufferStatusFromValues(tc.consumed, tc.size)
			if status != tc.wantStatus {
				t.Errorf("BufferStatusFromValues(%f, %f) = %q, want %q",
					tc.consumed, tc.size, status, tc.wantStatus)
			}
		})
	}
}

func TestComputeProjectStatus_FeedingBuffer(t *testing.T) {
	dirA, appA := makeTestRepo(t)
	dirB, appB := makeTestRepo(t)

	now := time.Now()

	// Repo A has a milestone with pending issues.
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	for i := 0; i < 3; i++ {
		writeIssue(t, appA, &issue.Issue{
			Title: "A pending", State: issue.StatePending, Milestone: "v1.0",
			Created: now, Updated: now,
		})
	}

	// Repo B has a milestone and declares a feeding buffer from repo A.
	msB := &milestone.Milestone{Name: "v2.0", Created: now}
	writeMilestone(t, appB, msB)
	writeIssue(t, appB, &issue.Issue{
		Title: "B pending", State: issue.StatePending, Milestone: "v2.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name: "Feed Test",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
			{
				Path:      dirB,
				Milestone: "v2.0",
				Feeds: []project.FeedDef{
					{Repo: dirA, Buffer: "3d"},
				},
			},
		},
	}
	status, err := project.ComputeProjectStatus(def)
	if err != nil {
		t.Fatalf("ComputeProjectStatus: %v", err)
	}

	if len(status.FeedingBuffers) != 1 {
		t.Fatalf("expected 1 feeding buffer, got %d", len(status.FeedingBuffers))
	}
	fb := status.FeedingBuffers[0]
	if fb.Buffer.SizeDays <= 0 {
		t.Errorf("expected FeedingBuffer.SizeDays > 0, got %f", fb.Buffer.SizeDays)
	}
}

func TestComputeProjectStatus_ResourceBuffer(t *testing.T) {
	dirA, appA := makeTestRepo(t)
	dirB, appB := makeTestRepo(t)

	now := time.Now()

	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	// Write an in-progress issue assigned to "alice" in repo A.
	writeIssue(t, appA, &issue.Issue{
		Title: "A in-progress", State: issue.StateInProgress, Milestone: "v1.0",
		Assigned: "alice", Created: now, Updated: now,
	})

	msB := &milestone.Milestone{Name: "v2.0", Created: now}
	writeMilestone(t, appB, msB)
	writeIssue(t, appB, &issue.Issue{
		Title: "B pending", State: issue.StatePending, Milestone: "v2.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name: "Resource Test",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
			{Path: dirB, Milestone: "v2.0"},
		},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA, dirB}},
		},
	}
	status, err := project.ComputeProjectStatus(def)
	if err != nil {
		t.Fatalf("ComputeProjectStatus: %v", err)
	}

	if len(status.ResourceBuffers) != 1 {
		t.Fatalf("expected 1 resource buffer, got %d", len(status.ResourceBuffers))
	}
	rb := status.ResourceBuffers[0]
	if rb.Worker != "alice" {
		t.Errorf("expected Worker=%q, got %q", "alice", rb.Worker)
	}
	// Alice has an in-progress issue in repo A — she is "at risk" for repo B.
	if rb.Status != "at risk" {
		t.Errorf("expected Status=%q, got %q", "at risk", rb.Status)
	}
}

// ---------------------------------------------------------------------------
// NextForActor tests
// ---------------------------------------------------------------------------

func TestNextForActor_PicksFromCorrectRepo(t *testing.T) {
	dirA, appA := makeTestRepo(t)
	dirB, appB := makeTestRepo(t)

	now := time.Now()

	// Repo A: milestone with pending issues assigned to bob.
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	idA := writeIssue(t, appA, &issue.Issue{
		Title: "A pending", State: issue.StatePending, Milestone: "v1.0",
		Assigned: "bob", Created: now, Updated: now,
	})

	// Repo B: milestone with pending issues assigned to alice.
	msB := &milestone.Milestone{Name: "v2.0", Created: now}
	writeMilestone(t, appB, msB)
	writeIssue(t, appB, &issue.Issue{
		Title: "B pending", State: issue.StatePending, Milestone: "v2.0",
		Assigned: "alice", Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name: "Next Test",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
			{Path: dirB, Milestone: "v2.0"},
		},
		Workers: []project.WorkerDef{
			{Name: "bob", Repos: []string{dirA}},
			{Name: "alice", Repos: []string{dirB}},
		},
	}

	repoPath, issueID, err := project.NextForActor(def, "bob")
	if err != nil {
		t.Fatalf("NextForActor: %v", err)
	}
	if repoPath != dirA {
		t.Errorf("expected repoPath=%q, got %q", dirA, repoPath)
	}
	if issueID != idA {
		t.Errorf("expected issueID=%v, got %v", idA, issueID)
	}
}

func TestNextForActor_ActorNotFound(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	writeIssue(t, appA, &issue.Issue{
		Title: "A pending", State: issue.StatePending, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name: "Actor Test",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
		},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA}},
		},
	}

	// Unknown actor should return an error.
	_, _, err := project.NextForActor(def, "nobody")
	if err == nil {
		t.Fatal("expected error for unknown actor, got nil")
	}
}

func TestNextForActor_FallsBackToAnyRepo(t *testing.T) {
	// When a worker is assigned to multiple repos, NextForActor should return
	// the first actionable issue across any of their repos.
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	idA := writeIssue(t, appA, &issue.Issue{
		Title: "A pending", State: issue.StatePending, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name: "Fallback Test",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
		},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA}},
		},
	}

	repoPath, issueID, err := project.NextForActor(def, "alice")
	if err != nil {
		t.Fatalf("NextForActor: %v", err)
	}
	if repoPath != dirA {
		t.Errorf("expected repoPath=%q, got %q", dirA, repoPath)
	}
	if issueID != idA {
		t.Errorf("expected issueID=%v, got %v", idA, issueID)
	}
}

func TestNextForActor_NoActionableIssues(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	// All issues are done — no actionable work.
	writeIssue(t, appA, &issue.Issue{
		Title: "A done", State: issue.StateDone, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name: "Done Test",
		Repos: []project.RepoDef{
			{Path: dirA, Milestone: "v1.0"},
		},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA}},
		},
	}

	_, _, err := project.NextForActor(def, "alice")
	if err == nil {
		t.Fatal("expected error when no actionable issues, got nil")
	}
}

// ---------------------------------------------------------------------------
// FeverChart / progress derivation
// ---------------------------------------------------------------------------

func TestComputeProjectStatus_FeverChart(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)

	// 1 done, 1 pending — progress is 50%.
	writeIssue(t, appA, &issue.Issue{
		Title: "done", State: issue.StateDone, Milestone: "v1.0",
		Created: now, Updated: now,
	})
	writeIssue(t, appA, &issue.Issue{
		Title: "pending", State: issue.StatePending, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:  "Fever Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
	}
	status, err := project.ComputeProjectStatus(def)
	if err != nil {
		t.Fatalf("ComputeProjectStatus: %v", err)
	}

	// Find the repo status for v1.0.
	var rs project.RepoStatus
	for _, r := range status.Repos {
		if r.Milestone == "v1.0" {
			rs = r
			break
		}
	}
	// FeverChart should be set (GREEN, YELLOW, or RED — not empty).
	if rs.FeverChart == "" {
		t.Error("expected FeverChart to be non-empty")
	}
}
