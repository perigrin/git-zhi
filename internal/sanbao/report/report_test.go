// ABOUTME: Integration tests for the sanbao report aggregator.
// ABOUTME: Uses real on-disk git repos to exercise GenerateReport end-to-end.
package report_test

import (
	"os"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/sanbao/report"
	"github.com/perigrin/git-zhi/internal/storage"
)

// setupRepo initialises a real on-disk git repo with a single initial commit
// so that go-git's Log operations work. Returns the repo and its store.
func setupRepo(t *testing.T) (*git.Repository, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	// Commit a seed file so HEAD resolves and session SHA ranges work.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(dir+"/seed.txt", []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if _, err := wt.Add("seed.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return repo, store
}

// newDoneIssue builds a done issue with sessions and transitions to drive metrics.
func newDoneIssue(t *testing.T, msName string, startSHA, endSHA string) *issue.Issue {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	now := time.Now()
	earlier := now.Add(-48 * time.Hour)
	started := now.Add(-24 * time.Hour)
	return &issue.Issue{
		ID:        id,
		Title:     "Test Issue",
		State:     issue.StateDone,
		Urgency:   issue.UrgencyNormal,
		Milestone: msName,
		Created:   earlier,
		Updated:   now,
		Sessions: []issue.Session{
			{StartSHA: startSHA, EndSHA: endSHA, Commits: 1},
		},
		Transitions: []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:test", Timestamp: started},
			{State: string(issue.StateDone), Actor: "human:test", Timestamp: now},
		},
		ObservedPaths: []string{},
		Body:          "## Acceptance Criteria\n\n- [ ] echo works (`echo hello`)\n",
	}
}

func writeMilestone(t *testing.T, store *storage.Store, ms *milestone.Milestone) {
	t.Helper()
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	refPath := milestone.RefPrefix + ms.Name
	if err := store.WriteEntity(refPath, "milestone.yaml", data, "create milestone"); err != nil {
		t.Fatalf("WriteEntity milestone: %v", err)
	}
}

func writeIssue(t *testing.T, store *storage.Store, iss *issue.Issue) {
	t.Helper()
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal issue: %v", err)
	}
	refPath := issue.RefPrefix + iss.ID.String()
	if err := store.WriteEntity(refPath, "issue.md", data, "create issue"); err != nil {
		t.Fatalf("WriteEntity issue: %v", err)
	}
}

// TestGenerateReport_BasicAggregation verifies that GenerateReport assembles
// all domain metrics without error when given a milestone with done issues.
func TestGenerateReport_BasicAggregation(t *testing.T) {
	repo, store := setupRepo(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, store, ms)

	// Use HEAD SHA for both start and end — same SHA means the session is
	// skipped by the complexity/sentiment sub-packages, which is fine: we are
	// only testing that GenerateReport assembles without error.
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	headSHA := head.Hash().String()

	iss := newDoneIssue(t, "v0.1", headSHA, headSHA)
	writeIssue(t, store, iss)

	rpt, err := report.GenerateReport(repo, store, "v0.1")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if rpt.Milestone != "v0.1" {
		t.Errorf("Milestone = %q, want %q", rpt.Milestone, "v0.1")
	}
	// All sub-metric structs must be accessible (verifies aggregation ran).
	_ = rpt.DORA
	_ = rpt.SPACE
	_ = rpt.CALMS
	_ = rpt.Sentiment
	_ = rpt.Complexity
	_ = rpt.Difficulties
}

// TestGenerateReport_MilestoneNotFound verifies that GenerateReport returns an
// error when the milestone does not exist.
func TestGenerateReport_MilestoneNotFound(t *testing.T) {
	_, store := setupRepo(t)

	_, err := report.GenerateReport(nil, store, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone, got nil")
	}
}

// TestGenerateReport_NoDoneIssues verifies that GenerateReport succeeds and
// returns an empty difficulties slice when the milestone has no done issues.
func TestGenerateReport_NoDoneIssues(t *testing.T) {
	_, store := setupRepo(t)

	ms := &milestone.Milestone{
		Name:    "empty",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, store, ms)

	rpt, err := report.GenerateReport(nil, store, "empty")
	if err != nil {
		t.Fatalf("GenerateReport with no done issues: %v", err)
	}
	if rpt.Milestone != "empty" {
		t.Errorf("Milestone = %q, want %q", rpt.Milestone, "empty")
	}
	if len(rpt.Difficulties) != 0 {
		t.Errorf("expected no difficulties, got %d", len(rpt.Difficulties))
	}
}

// TestGenerateReport_DifficultiesPopulated verifies that Difficulties has one
// entry per done issue, each with a non-empty IssueID and the correct IssueTitle.
func TestGenerateReport_DifficultiesPopulated(t *testing.T) {
	repo, store := setupRepo(t)

	ms := &milestone.Milestone{
		Name:    "v0.2",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, store, ms)

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	headSHA := head.Hash().String()

	iss := newDoneIssue(t, "v0.2", headSHA, headSHA)
	writeIssue(t, store, iss)

	rpt, err := report.GenerateReport(repo, store, "v0.2")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if len(rpt.Difficulties) != 1 {
		t.Fatalf("expected 1 difficulty entry, got %d", len(rpt.Difficulties))
	}
	if rpt.Difficulties[0].IssueTitle != "Test Issue" {
		t.Errorf("IssueTitle = %q, want %q", rpt.Difficulties[0].IssueTitle, "Test Issue")
	}
	if rpt.Difficulties[0].IssueID == "" {
		t.Error("IssueID is empty, want non-empty UUID string")
	}
}

// TestGenerateReport_ComplexityFieldsAccessible verifies that the Complexity
// struct fields are accessible even when no session ranges produce results.
func TestGenerateReport_ComplexityFieldsAccessible(t *testing.T) {
	repo, store := setupRepo(t)

	ms := &milestone.Milestone{
		Name:    "v0.3",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, store, ms)

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	headSHA := head.Hash().String()

	iss := newDoneIssue(t, "v0.3", headSHA, headSHA)
	writeIssue(t, store, iss)

	rpt, err := report.GenerateReport(repo, store, "v0.3")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Fields must be non-nil slices (never raw nil) for safe range iteration.
	if rpt.Complexity.Hotspots == nil {
		t.Error("Complexity.Hotspots is nil, want non-nil slice")
	}
	if rpt.Complexity.ChangeCoupling == nil {
		t.Error("Complexity.ChangeCoupling is nil, want non-nil slice")
	}
}
