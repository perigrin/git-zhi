// ABOUTME: Tests for SPACE framework metrics derived from issue data.
// ABOUTME: Covers agent autonomy rate, review cycle count, session count, and Compute.
package space_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/actor"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/space"
)

// makeIssue builds a minimal in-memory issue for tests.
func makeIssue(state issue.State, transitions []issue.Transition, sessions []issue.Session) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	now := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	return &issue.Issue{
		ID:          id,
		Title:       "test issue",
		State:       state,
		Milestone:   "v0.1",
		Created:     now,
		Updated:     now,
		Transitions: transitions,
		Sessions:    sessions,
	}
}

// t0 is a fixed base time used across tests for reproducibility.
var t0 = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

func agentActor() string {
	return actor.Actor{Type: actor.TypeAgent, ID: "claude-code-1"}.String()
}

func humanActor() string {
	return actor.Actor{Type: actor.TypeHuman, ID: "perigrin"}.String()
}

// TestAgentAutonomyRate verifies percentage of done issues completed entirely by agents.
func TestAgentAutonomyRate(t *testing.T) {
	t.Run("all done issues agent-only returns 100 percent", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, nil)
		got := space.AgentAutonomyRate([]*issue.Issue{iss})
		if got != 100.0 {
			t.Fatalf("AgentAutonomyRate: got %v, want 100.0", got)
		}
	})

	t.Run("done issue with human transition returns 0 percent", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: humanActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, nil)
		got := space.AgentAutonomyRate([]*issue.Issue{iss})
		if got != 0.0 {
			t.Fatalf("AgentAutonomyRate: got %v, want 0.0", got)
		}
	})

	t.Run("two done issues one agent-only one mixed returns 50 percent", func(t *testing.T) {
		agentOnly := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, nil)
		mixed := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: humanActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, nil)
		got := space.AgentAutonomyRate([]*issue.Issue{agentOnly, mixed})
		if got != 50.0 {
			t.Fatalf("AgentAutonomyRate: got %v, want 50.0", got)
		}
	})

	t.Run("no done issues returns 0", func(t *testing.T) {
		pending := makeIssue(issue.StatePending, []issue.Transition{}, nil)
		got := space.AgentAutonomyRate([]*issue.Issue{pending})
		if got != 0.0 {
			t.Fatalf("AgentAutonomyRate: got %v, want 0.0 for no done issues", got)
		}
	})

	t.Run("empty slice returns 0", func(t *testing.T) {
		got := space.AgentAutonomyRate([]*issue.Issue{})
		if got != 0.0 {
			t.Fatalf("AgentAutonomyRate: got %v, want 0.0 for empty slice", got)
		}
	})

	t.Run("done issue with no transitions returns 0 percent agent-autonomous", func(t *testing.T) {
		// An issue with a done state but no transitions cannot be confirmed as
		// agent-autonomous — treat it as not autonomous.
		iss := makeIssue(issue.StateDone, []issue.Transition{}, nil)
		got := space.AgentAutonomyRate([]*issue.Issue{iss})
		if got != 0.0 {
			t.Fatalf("AgentAutonomyRate: got %v, want 0.0 for done issue with no transitions", got)
		}
	})
}

// TestReviewCycleCount verifies counting of done→reopened cycles.
func TestReviewCycleCount(t *testing.T) {
	t.Run("issue with one reopen returns 1", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateReopened), Actor: humanActor(), Timestamp: t0.Add(2 * time.Hour)},
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(3 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(4 * time.Hour)},
		}, nil)
		got := space.ReviewCycleCount(iss)
		if got != 1 {
			t.Fatalf("ReviewCycleCount: got %d, want 1", got)
		}
	})

	t.Run("issue with two reopens returns 2", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateReopened), Actor: humanActor(), Timestamp: t0.Add(2 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(3 * time.Hour)},
			{State: string(issue.StateReopened), Actor: humanActor(), Timestamp: t0.Add(4 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(5 * time.Hour)},
		}, nil)
		got := space.ReviewCycleCount(iss)
		if got != 2 {
			t.Fatalf("ReviewCycleCount: got %d, want 2", got)
		}
	})

	t.Run("issue with no reopens returns 0", func(t *testing.T) {
		iss := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, nil)
		got := space.ReviewCycleCount(iss)
		if got != 0 {
			t.Fatalf("ReviewCycleCount: got %d, want 0", got)
		}
	})

	t.Run("issue with no transitions returns 0", func(t *testing.T) {
		iss := makeIssue(issue.StatePending, []issue.Transition{}, nil)
		got := space.ReviewCycleCount(iss)
		if got != 0 {
			t.Fatalf("ReviewCycleCount: got %d, want 0", got)
		}
	})
}

// TestSessionCountPerIssue verifies that session count equals len(iss.Sessions).
func TestSessionCountPerIssue(t *testing.T) {
	t.Run("issue with two sessions returns 2", func(t *testing.T) {
		sessions := []issue.Session{
			{StartSHA: "abc", EndSHA: "def", Commits: 3},
			{StartSHA: "ghi", EndSHA: "jkl", Commits: 2},
		}
		iss := makeIssue(issue.StateDone, nil, sessions)
		got := space.SessionCountPerIssue(iss)
		if got != 2 {
			t.Fatalf("SessionCountPerIssue: got %d, want 2", got)
		}
	})

	t.Run("issue with no sessions returns 0", func(t *testing.T) {
		iss := makeIssue(issue.StatePending, nil, nil)
		got := space.SessionCountPerIssue(iss)
		if got != 0 {
			t.Fatalf("SessionCountPerIssue: got %d, want 0", got)
		}
	})

	t.Run("issue with one session returns 1", func(t *testing.T) {
		sessions := []issue.Session{
			{StartSHA: "abc", EndSHA: "def", Commits: 5},
		}
		iss := makeIssue(issue.StateDone, nil, sessions)
		got := space.SessionCountPerIssue(iss)
		if got != 1 {
			t.Fatalf("SessionCountPerIssue: got %d, want 1", got)
		}
	})
}

// TestCompute verifies the aggregate SPACEMetrics computation.
func TestCompute(t *testing.T) {
	t.Run("two done issues computes correct averages", func(t *testing.T) {
		// Issue 1: agent-only (all transitions by agent, including reopen), 1 reopen, 2 sessions.
		iss1 := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
			{State: string(issue.StateReopened), Actor: agentActor(), Timestamp: t0.Add(3 * time.Hour)},
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(4 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(5 * time.Hour)},
		}, []issue.Session{
			{StartSHA: "a", EndSHA: "b", Commits: 2},
			{StartSHA: "c", EndSHA: "d", Commits: 3},
		})
		// Issue 2: human-touched, 0 reopens, 1 session.
		iss2 := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: humanActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: humanActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, []issue.Session{
			{StartSHA: "e", EndSHA: "f", Commits: 1},
		})

		metrics := space.Compute([]*issue.Issue{iss1, iss2})

		// AgentAutonomyRate: 1 of 2 done issues is agent-only = 50%
		if metrics.AgentAutonomyRate != 50.0 {
			t.Errorf("Compute AgentAutonomyRate: got %v, want 50.0", metrics.AgentAutonomyRate)
		}
		// AvgReviewCycles: (1 + 0) / 2 = 0.5
		if metrics.AvgReviewCycles != 0.5 {
			t.Errorf("Compute AvgReviewCycles: got %v, want 0.5", metrics.AvgReviewCycles)
		}
		// AvgSessionCount: (2 + 1) / 2 = 1.5
		if metrics.AvgSessionCount != 1.5 {
			t.Errorf("Compute AvgSessionCount: got %v, want 1.5", metrics.AvgSessionCount)
		}
	})

	t.Run("empty slice returns zero metrics", func(t *testing.T) {
		metrics := space.Compute([]*issue.Issue{})
		if metrics.AgentAutonomyRate != 0 {
			t.Errorf("Compute empty: AgentAutonomyRate should be 0, got %v", metrics.AgentAutonomyRate)
		}
		if metrics.AvgReviewCycles != 0 {
			t.Errorf("Compute empty: AvgReviewCycles should be 0, got %v", metrics.AvgReviewCycles)
		}
		if metrics.AvgSessionCount != 0 {
			t.Errorf("Compute empty: AvgSessionCount should be 0, got %v", metrics.AvgSessionCount)
		}
	})

	t.Run("non-done issues are excluded from averages", func(t *testing.T) {
		// Only the done issue contributes to metrics.
		done := makeIssue(issue.StateDone, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: agentActor(), Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: agentActor(), Timestamp: t0.Add(2 * time.Hour)},
		}, []issue.Session{
			{StartSHA: "a", EndSHA: "b", Commits: 4},
			{StartSHA: "c", EndSHA: "d", Commits: 2},
			{StartSHA: "e", EndSHA: "f", Commits: 1},
		})
		pending := makeIssue(issue.StatePending, []issue.Transition{}, nil)

		metrics := space.Compute([]*issue.Issue{done, pending})

		if metrics.AgentAutonomyRate != 100.0 {
			t.Errorf("Compute: AgentAutonomyRate should be 100.0, got %v", metrics.AgentAutonomyRate)
		}
		if metrics.AvgReviewCycles != 0.0 {
			t.Errorf("Compute: AvgReviewCycles should be 0.0, got %v", metrics.AvgReviewCycles)
		}
		if metrics.AvgSessionCount != 3.0 {
			t.Errorf("Compute: AvgSessionCount should be 3.0, got %v", metrics.AvgSessionCount)
		}
	})
}
