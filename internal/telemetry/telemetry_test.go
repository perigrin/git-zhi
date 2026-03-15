// ABOUTME: Tests for telemetry types: fever chart status constants,
// ABOUTME: Stats struct zero-value, and Compute function with various issue scenarios.
package telemetry_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/milestone"
	"github.com/perigrin/git-chain/internal/telemetry"
)

func TestStatusConstants(t *testing.T) {
	if telemetry.StatusGreen != "GREEN" {
		t.Fatalf("expected %q, got %q", "GREEN", telemetry.StatusGreen)
	}
	if telemetry.StatusYellow != "YELLOW" {
		t.Fatalf("expected %q, got %q", "YELLOW", telemetry.StatusYellow)
	}
	if telemetry.StatusRed != "RED" {
		t.Fatalf("expected %q, got %q", "RED", telemetry.StatusRed)
	}
}

func TestStats_ZeroValue(t *testing.T) {
	s := telemetry.Stats{}
	if s.FeverStatus != "" {
		t.Fatalf("expected zero-value FeverStatus to be empty, got %q", s.FeverStatus)
	}
}

// makeIssue is a helper to build an in-memory issue for tests.
func makeIssue(msName string, state issue.State, sessions []issue.Session) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	now := time.Now()
	return &issue.Issue{
		ID:        id,
		Title:     "test issue",
		State:     state,
		Milestone: msName,
		Sessions:  sessions,
		Created:   now,
		Updated:   now,
	}
}

// makeMilestone builds an in-memory Milestone for tests.
func makeMilestone(name string) *milestone.Milestone {
	return &milestone.Milestone{
		Name:    name,
		Created: time.Now(),
	}
}

func TestCompute_Empty(t *testing.T) {
	ms := makeMilestone("v0.1")
	stats := telemetry.Compute(nil, ms)
	if stats == nil {
		t.Fatal("expected non-nil Stats")
	}
	if stats.MPG != 0 {
		t.Errorf("expected MPG=0, got %f", stats.MPG)
	}
	if stats.Speed != 0 {
		t.Errorf("expected Speed=0, got %f", stats.Speed)
	}
	if stats.FeverStatus != "" {
		t.Errorf("expected empty FeverStatus, got %q", stats.FeverStatus)
	}
}

func TestCompute_AllPending(t *testing.T) {
	ms := makeMilestone("v0.1")
	issues := []*issue.Issue{
		makeIssue("v0.1", issue.StatePending, nil),
		makeIssue("v0.1", issue.StatePending, nil),
	}
	stats := telemetry.Compute(issues, ms)
	if stats.MPG != 0 {
		t.Errorf("expected MPG=0 with no done issues, got %f", stats.MPG)
	}
	if stats.Speed != 0 {
		t.Errorf("expected Speed=0 with no done issues, got %f", stats.Speed)
	}
}

func TestCompute_WithDoneIssues(t *testing.T) {
	ms := makeMilestone("v0.1")

	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	id3, _ := gen.NewV7()

	base := time.Now().Add(-14 * 24 * time.Hour) // two weeks ago

	issue1 := &issue.Issue{
		ID:        id1,
		Title:     "Issue One",
		State:     issue.StateDone,
		Milestone: "v0.1",
		Sessions:  []issue.Session{{StartSHA: "abc", EndSHA: "def", Commits: 3}},
		Created:   base,
		Updated:   base.Add(3 * 24 * time.Hour),
	}
	issue2 := &issue.Issue{
		ID:        id2,
		Title:     "Issue Two",
		State:     issue.StateDone,
		Milestone: "v0.1",
		Sessions:  []issue.Session{{StartSHA: "ghi", EndSHA: "jkl", Commits: 5}},
		Created:   base.Add(24 * time.Hour),
		Updated:   base.Add(10 * 24 * time.Hour),
	}
	issue3 := &issue.Issue{
		ID:        id3,
		Title:     "Issue Three",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Sessions:  nil,
		Created:   base.Add(2 * 24 * time.Hour),
		Updated:   base.Add(2 * 24 * time.Hour),
	}

	stats := telemetry.Compute([]*issue.Issue{issue1, issue2, issue3}, ms)

	// 2 done issues with 3+5=8 total commits: MPG = 8/2 = 4
	if stats.MPG <= 0 {
		t.Errorf("expected MPG > 0, got %f", stats.MPG)
	}
	// 2 done issues over elapsed time: Speed > 0
	if stats.Speed <= 0 {
		t.Errorf("expected Speed > 0, got %f", stats.Speed)
	}
	// BufferTotal = 3 * 0.5 = 1.5
	if stats.BufferTotal != 1.5 {
		t.Errorf("expected BufferTotal=1.5, got %f", stats.BufferTotal)
	}
	// With 2 done / 3 total = 66% progress, should be GREEN
	if stats.FeverStatus != telemetry.StatusGreen {
		t.Errorf("expected FeverStatus=GREEN, got %q", stats.FeverStatus)
	}
}

func TestCompute_CompletionRatio(t *testing.T) {
	ms := makeMilestone("v0.1")

	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	id3, _ := gen.NewV7()

	now := time.Now()
	issues := []*issue.Issue{
		{ID: id1, Title: "Done 1", State: issue.StateDone, Milestone: "v0.1",
			Sessions: []issue.Session{{Commits: 2}}, Created: now, Updated: now},
		{ID: id2, Title: "Done 2", State: issue.StateDone, Milestone: "v0.1",
			Sessions: []issue.Session{{Commits: 2}}, Created: now, Updated: now},
		{ID: id3, Title: "Pending", State: issue.StatePending, Milestone: "v0.1",
			Created: now, Updated: now},
	}

	stats := telemetry.Compute(issues, ms)

	// 2 done / 3 total = 2/3 ≈ 0.666...
	expected := 2.0 / 3.0
	if stats.CompletionRatio < 0.66 || stats.CompletionRatio > 0.68 {
		t.Errorf("expected CompletionRatio ≈ %.3f, got %.3f", expected, stats.CompletionRatio)
	}
}

func TestCompute_SpeedUsesDoneIssueTimes(t *testing.T) {
	ms := makeMilestone("v0.1")

	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()

	// Created 30 days ago but done within a 7-day window.
	farPast := time.Now().Add(-30 * 24 * time.Hour)
	doneStart := time.Now().Add(-7 * 24 * time.Hour)
	doneEnd := time.Now()

	issues := []*issue.Issue{
		{ID: id1, Title: "Old Done 1", State: issue.StateDone, Milestone: "v0.1",
			Sessions: []issue.Session{{Commits: 1}}, Created: farPast, Updated: doneStart},
		{ID: id2, Title: "Old Done 2", State: issue.StateDone, Milestone: "v0.1",
			Sessions: []issue.Session{{Commits: 1}}, Created: farPast, Updated: doneEnd},
	}

	stats := telemetry.Compute(issues, ms)

	// Speed should be based on done-issue Updated times (7-day window), not
	// creation times (30-day window). Roughly 2 issues / 1 week = ~2 issues/week.
	if stats.Speed <= 0 {
		t.Errorf("expected Speed > 0, got %f", stats.Speed)
	}
	// Speed based on 7-day window: ~2 issues/week. If it used the 30-day span
	// it would be ~0.47 issues/week. Assert Speed > 1 to distinguish.
	if stats.Speed < 1.0 {
		t.Errorf("expected Speed > 1.0 (7-day done-issue window), got %f — may be using creation times instead", stats.Speed)
	}
}

func TestCompute_FeverRed(t *testing.T) {
	ms := makeMilestone("v0.1")

	gen := uuid.NewGen()
	base := time.Now().Add(-30 * 24 * time.Hour)

	// Scenario: 10 issues total, 5 done, 5 pending.
	// 4 done issues with 1 commit each and 1 done issue with 100 commits.
	// MPG = (4 + 100) / 5 = 20.8.
	// BufferBurned: only the 100-commit issue exceeds MPG:
	//   (100 - 20.8) / 20.8 ≈ 3.8
	// BufferTotal = 10 * 0.5 = 5.0.
	// bufferPct = 3.8 / 5.0 * 100 = 76%.
	// progress = 5 / 10 * 100 = 50%.
	// 76 > 50 + 20 → RED.
	var issues []*issue.Issue

	// Four normal done issues: 1 commit each.
	for i := 0; i < 4; i++ {
		id, _ := gen.NewV7()
		issues = append(issues, &issue.Issue{
			ID:        id,
			Title:     "Normal Done",
			State:     issue.StateDone,
			Milestone: "v0.1",
			Sessions:  []issue.Session{{Commits: 1}},
			Created:   base.Add(time.Duration(i) * time.Hour),
			Updated:   base.Add(time.Duration(i+1) * time.Hour),
		})
	}

	// One over-budget done issue: 100 commits, far above MPG ≈ 20.8.
	overID, _ := gen.NewV7()
	issues = append(issues, &issue.Issue{
		ID:        overID,
		Title:     "Over Budget Done",
		State:     issue.StateDone,
		Milestone: "v0.1",
		Sessions:  []issue.Session{{Commits: 100}},
		Created:   base.Add(5 * time.Hour),
		Updated:   base.Add(6 * time.Hour),
	})

	// Five pending issues.
	for i := 0; i < 5; i++ {
		id, _ := gen.NewV7()
		issues = append(issues, &issue.Issue{
			ID:        id,
			Title:     "Pending",
			State:     issue.StatePending,
			Milestone: "v0.1",
			Sessions:  nil,
			Created:   base.Add(time.Duration(i+7) * time.Hour),
			Updated:   base.Add(time.Duration(i+7) * time.Hour),
		})
	}

	stats := telemetry.Compute(issues, ms)

	// 50% progress but buffer burned at 76% → RED.
	if stats.FeverStatus != telemetry.StatusRed {
		t.Errorf("expected FeverStatus=RED for 50%% progress + 76%% buffer burn, got %q (MPG=%.2f BufferBurned=%.2f BufferTotal=%.2f)",
			stats.FeverStatus, stats.MPG, stats.BufferBurned, stats.BufferTotal)
	}
}
