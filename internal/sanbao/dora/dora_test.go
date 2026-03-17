// ABOUTME: Tests for DORA metrics derived from issue state transitions.
// ABOUTME: Covers lead time, cycle time, rework rate, completion frequency, and Compute.
package dora_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/dora"
)

// makeIssue builds a minimal in-memory issue for tests.
func makeIssue(state issue.State, created time.Time, transitions []issue.Transition) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	return &issue.Issue{
		ID:          id,
		Title:       "test issue",
		State:       state,
		Milestone:   "v0.1",
		Created:     created,
		Updated:     created,
		Transitions: transitions,
	}
}

// t0 is a fixed base time used across tests for reproducibility.
var t0 = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

func TestLeadTime(t *testing.T) {
	t.Run("done issue returns created-to-done duration", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
		})
		got := dora.LeadTime(iss)
		want := 3 * time.Hour
		if got != want {
			t.Fatalf("LeadTime: got %v, want %v", got, want)
		}
	})

	t.Run("no done transition returns zero", func(t *testing.T) {
		iss := makeIssue(issue.StateInProgress, t0, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
		})
		got := dora.LeadTime(iss)
		if got != 0 {
			t.Fatalf("LeadTime: expected 0 for issue without done transition, got %v", got)
		}
	})

	t.Run("no transitions returns zero", func(t *testing.T) {
		iss := makeIssue(issue.StatePending, t0, []issue.Transition{})
		got := dora.LeadTime(iss)
		if got != 0 {
			t.Fatalf("LeadTime: expected 0 for issue with no transitions, got %v", got)
		}
	})
}

func TestCycleTime(t *testing.T) {
	t.Run("start to done returns correct duration", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(4 * time.Hour)},
		})
		got := dora.CycleTime(iss)
		want := 3 * time.Hour
		if got != want {
			t.Fatalf("CycleTime: got %v, want %v", got, want)
		}
	})

	t.Run("missing start transition returns zero", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(4 * time.Hour)},
		})
		got := dora.CycleTime(iss)
		if got != 0 {
			t.Fatalf("CycleTime: expected 0 when no start transition, got %v", got)
		}
	})

	t.Run("missing done transition returns zero", func(t *testing.T) {
		iss := makeIssue(issue.StateInProgress, t0, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
		})
		got := dora.CycleTime(iss)
		if got != 0 {
			t.Fatalf("CycleTime: expected 0 when no done transition, got %v", got)
		}
	})

	t.Run("no transitions returns zero", func(t *testing.T) {
		iss := makeIssue(issue.StatePending, t0, []issue.Transition{})
		got := dora.CycleTime(iss)
		if got != 0 {
			t.Fatalf("CycleTime: expected 0 with no transitions, got %v", got)
		}
	})
}

func TestReworkRate(t *testing.T) {
	t.Run("one reopened out of two done is 50 percent", func(t *testing.T) {
		done1 := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
			{State: string(issue.StateReopened), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(5 * time.Hour)},
		})
		done2 := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		})
		got := dora.ReworkRate([]*issue.Issue{done1, done2})
		want := 50.0
		if got != want {
			t.Fatalf("ReworkRate: got %v, want %v", got, want)
		}
	})

	t.Run("no done issues returns zero", func(t *testing.T) {
		pending := makeIssue(issue.StatePending, t0, []issue.Transition{})
		got := dora.ReworkRate([]*issue.Issue{pending})
		if got != 0 {
			t.Fatalf("ReworkRate: expected 0 with no done issues, got %v", got)
		}
	})

	t.Run("all done with no reopens returns zero", func(t *testing.T) {
		done1 := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		})
		done2 := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
		})
		got := dora.ReworkRate([]*issue.Issue{done1, done2})
		if got != 0 {
			t.Fatalf("ReworkRate: expected 0 with no reopened, got %v", got)
		}
	})

	t.Run("empty slice returns zero", func(t *testing.T) {
		got := dora.ReworkRate([]*issue.Issue{})
		if got != 0 {
			t.Fatalf("ReworkRate: expected 0 for empty slice, got %v", got)
		}
	})
}

func TestCompletionFrequency(t *testing.T) {
	t.Run("two done issues over two weeks with weekly period", func(t *testing.T) {
		week := 7 * 24 * time.Hour
		// created at t0, done at t0+2weeks → span = 2 weeks
		done1 := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(week)},
		})
		done2 := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * week)},
		})
		got := dora.CompletionFrequency([]*issue.Issue{done1, done2}, week)
		// 2 done over span of 2 weeks = 1.0 per week
		want := 1.0
		if got != want {
			t.Fatalf("CompletionFrequency: got %v, want %v", got, want)
		}
	})

	t.Run("no done issues returns zero", func(t *testing.T) {
		got := dora.CompletionFrequency([]*issue.Issue{
			makeIssue(issue.StatePending, t0, []issue.Transition{}),
		}, 7*24*time.Hour)
		if got != 0 {
			t.Fatalf("CompletionFrequency: expected 0 with no done issues, got %v", got)
		}
	})

	t.Run("zero span returns zero", func(t *testing.T) {
		// created and done at same instant → span = 0
		iss := makeIssue(issue.StateDone, t0, []issue.Transition{
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0},
		})
		got := dora.CompletionFrequency([]*issue.Issue{iss}, 7*24*time.Hour)
		if got != 0 {
			t.Fatalf("CompletionFrequency: expected 0 when span is zero, got %v", got)
		}
	})

	t.Run("empty slice returns zero", func(t *testing.T) {
		got := dora.CompletionFrequency([]*issue.Issue{}, 7*24*time.Hour)
		if got != 0 {
			t.Fatalf("CompletionFrequency: expected 0 for empty slice, got %v", got)
		}
	})
}

func TestCompute(t *testing.T) {
	week := 7 * 24 * time.Hour

	// Two issues: one reopened (rework), one clean.
	done1 := makeIssue(issue.StateDone, t0, []issue.Transition{
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(6 * time.Hour)},
		{State: string(issue.StateReopened), Actor: "human:perigrin", Timestamp: t0.Add(7 * time.Hour)},
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(8 * time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(week)},
	})
	done2 := makeIssue(issue.StateDone, t0, []issue.Transition{
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * week)},
	})

	metrics := dora.Compute([]*issue.Issue{done1, done2})

	// AvgLeadTime uses the FIRST done transition per issue.
	// done1: first done at t0+6h → lead = 6h
	// done2: first done at t0+2weeks → lead = 2weeks = 336h
	// avg = (6h + 336h) / 2 = 171h
	wantAvgLead := (6*time.Hour + 2*week) / 2
	if metrics.AvgLeadTime != wantAvgLead {
		t.Errorf("Compute AvgLeadTime: got %v, want %v", metrics.AvgLeadTime, wantAvgLead)
	}

	// AvgCycleTime: first in-progress → first done per issue.
	// done1: start at t0+2h, first done at t0+6h → cycle = 4h
	// done2: start at t0+1h, first done at t0+2weeks → cycle = 2weeks - 1h = 335h
	// avg = (4h + 335h) / 2 = 169.5h = 169h30m
	wantAvgCycle := (4*time.Hour + (2*week - time.Hour)) / 2
	if metrics.AvgCycleTime != wantAvgCycle {
		t.Errorf("Compute AvgCycleTime: got %v, want %v", metrics.AvgCycleTime, wantAvgCycle)
	}

	// ReworkRate: 1 out of 2 done issues reopened = 50%
	if metrics.ReworkRate != 50.0 {
		t.Errorf("Compute ReworkRate: got %v, want 50.0", metrics.ReworkRate)
	}

	// CompletionFrequency: 2 done over span of 2 weeks = 1.0 per week
	if metrics.CompletionFrequency != 1.0 {
		t.Errorf("Compute CompletionFrequency: got %v, want 1.0", metrics.CompletionFrequency)
	}
}

func TestCompute_EmptySlice(t *testing.T) {
	metrics := dora.Compute([]*issue.Issue{})
	if metrics.AvgLeadTime != 0 {
		t.Errorf("Compute empty: AvgLeadTime should be 0, got %v", metrics.AvgLeadTime)
	}
	if metrics.AvgCycleTime != 0 {
		t.Errorf("Compute empty: AvgCycleTime should be 0, got %v", metrics.AvgCycleTime)
	}
	if metrics.ReworkRate != 0 {
		t.Errorf("Compute empty: ReworkRate should be 0, got %v", metrics.ReworkRate)
	}
	if metrics.CompletionFrequency != 0 {
		t.Errorf("Compute empty: CompletionFrequency should be 0, got %v", metrics.CompletionFrequency)
	}
}
