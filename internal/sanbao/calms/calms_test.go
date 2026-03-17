// ABOUTME: Tests for CALMS DevOps indicators derived from issue data.
// ABOUTME: Covers verification coverage, WIP compliance, scope cut frequency, and metric completeness.
package calms_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/calms"
)

// t0 is a fixed base time used across tests for reproducibility.
var t0 = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

// makeIssue builds a minimal in-memory issue for tests.
func makeIssue(state issue.State, body string, sessions []issue.Session, transitions []issue.Transition) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	return &issue.Issue{
		ID:          id,
		Title:       "test issue",
		State:       state,
		Milestone:   "v0.1",
		Created:     t0,
		Updated:     t0,
		Sessions:    sessions,
		Transitions: transitions,
		Body:        body,
	}
}

// bodyWithAC constructs an issue body with a simple Acceptance Criteria section
// containing one executable backtick command.
func bodyWithAC(command string) string {
	return "## Acceptance Criteria\n- [ ] run `" + command + "` to verify\n"
}

// bodyWithContextPaths constructs an issue body with a Context section containing paths.
func bodyWithContextPaths(paths string) string {
	return "## Context\n- paths: " + paths + "\n"
}

// bodyWithACAndContext constructs an issue body with both AC (executable) and Context (paths).
func bodyWithACAndContext(command, paths string) string {
	return "## Acceptance Criteria\n- [ ] run `" + command + "` to verify\n\n## Context\n- paths: " + paths + "\n"
}

func TestVerificationCoverage(t *testing.T) {
	t.Run("empty slice returns zero", func(t *testing.T) {
		got := calms.VerificationCoverage([]*issue.Issue{})
		if got != 0 {
			t.Fatalf("VerificationCoverage: expected 0 for empty slice, got %v", got)
		}
	})

	t.Run("all issues have executable AC returns 100", func(t *testing.T) {
		iss1 := makeIssue(issue.StateDone, bodyWithAC("go test ./..."), nil, nil)
		iss2 := makeIssue(issue.StateDone, bodyWithAC("make lint"), nil, nil)
		got := calms.VerificationCoverage([]*issue.Issue{iss1, iss2})
		if got != 100.0 {
			t.Fatalf("VerificationCoverage: expected 100, got %v", got)
		}
	})

	t.Run("no issues have executable AC returns zero", func(t *testing.T) {
		iss1 := makeIssue(issue.StatePending, "## Acceptance Criteria\n- [ ] manually review the output\n", nil, nil)
		iss2 := makeIssue(issue.StatePending, "## Acceptance Criteria\n- [ ] check the docs\n", nil, nil)
		got := calms.VerificationCoverage([]*issue.Issue{iss1, iss2})
		if got != 0.0 {
			t.Fatalf("VerificationCoverage: expected 0, got %v", got)
		}
	})

	t.Run("half issues have executable AC returns 50", func(t *testing.T) {
		covered := makeIssue(issue.StateDone, bodyWithAC("go build ./..."), nil, nil)
		notCovered := makeIssue(issue.StatePending, "## Acceptance Criteria\n- [ ] review manually\n", nil, nil)
		got := calms.VerificationCoverage([]*issue.Issue{covered, notCovered})
		if got != 50.0 {
			t.Fatalf("VerificationCoverage: expected 50, got %v", got)
		}
	})

	t.Run("issue with no body counts as not covered", func(t *testing.T) {
		iss := makeIssue(issue.StatePending, "", nil, nil)
		got := calms.VerificationCoverage([]*issue.Issue{iss})
		if got != 0.0 {
			t.Fatalf("VerificationCoverage: expected 0 for bodyless issue, got %v", got)
		}
	})
}

func TestWIPCompliance(t *testing.T) {
	t.Run("no issues returns zero violations", func(t *testing.T) {
		got := calms.WIPCompliance([]*issue.Issue{})
		if got != 0 {
			t.Fatalf("WIPCompliance: expected 0 for empty slice, got %v", got)
		}
	})

	t.Run("single in-progress issue returns zero violations", func(t *testing.T) {
		iss := makeIssue(issue.StateInProgress, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
		})
		got := calms.WIPCompliance([]*issue.Issue{iss})
		if got != 0 {
			t.Fatalf("WIPCompliance: expected 0 for single in-progress issue, got %v", got)
		}
	})

	t.Run("two sequential issues returns zero violations", func(t *testing.T) {
		// iss1: in-progress t0 to t0+2h (done at t0+2h)
		// iss2: in-progress at t0+3h (after iss1 is done)
		iss1 := makeIssue(issue.StateDone, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		})
		iss2 := makeIssue(issue.StateInProgress, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
		})
		got := calms.WIPCompliance([]*issue.Issue{iss1, iss2})
		if got != 0 {
			t.Fatalf("WIPCompliance: expected 0 for sequential issues, got %v", got)
		}
	})

	t.Run("two overlapping in-progress issues returns one violation", func(t *testing.T) {
		// iss1: in-progress t0 to t0+4h
		// iss2: in-progress at t0+1h (while iss1 is still in-progress)
		iss1 := makeIssue(issue.StateDone, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(4 * time.Hour)},
		})
		iss2 := makeIssue(issue.StateDone, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
		})
		got := calms.WIPCompliance([]*issue.Issue{iss1, iss2})
		if got != 1 {
			t.Fatalf("WIPCompliance: expected 1 violation for overlapping issues, got %v", got)
		}
	})

	t.Run("three simultaneous in-progress issues returns two violations", func(t *testing.T) {
		// All three start at t0 → when iss2 starts, iss1 is already in-progress (1 violation)
		// when iss3 starts, iss1 and iss2 are in-progress (1 more violation)
		iss1 := makeIssue(issue.StateInProgress, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
		})
		iss2 := makeIssue(issue.StateInProgress, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
		})
		iss3 := makeIssue(issue.StateInProgress, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		})
		got := calms.WIPCompliance([]*issue.Issue{iss1, iss2, iss3})
		if got != 2 {
			t.Fatalf("WIPCompliance: expected 2 violations for three simultaneous issues, got %v", got)
		}
	})

	t.Run("cancelled issues count as closing in-progress", func(t *testing.T) {
		// iss1: in-progress t0 to t0+2h (cancelled)
		// iss2: in-progress at t0+3h → no overlap
		iss1 := makeIssue(issue.StateCancelled, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
			{State: string(issue.StateCancelled), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		})
		iss2 := makeIssue(issue.StateInProgress, "", nil, []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
		})
		got := calms.WIPCompliance([]*issue.Issue{iss1, iss2})
		if got != 0 {
			t.Fatalf("WIPCompliance: expected 0 when cancelled closes the gap, got %v", got)
		}
	})

	t.Run("issues with no in-progress transitions return zero violations", func(t *testing.T) {
		iss := makeIssue(issue.StatePending, "", nil, []issue.Transition{})
		got := calms.WIPCompliance([]*issue.Issue{iss})
		if got != 0 {
			t.Fatalf("WIPCompliance: expected 0 for pending issue with no transitions, got %v", got)
		}
	})
}

func TestScopeCutFrequency(t *testing.T) {
	t.Run("empty slice returns zero", func(t *testing.T) {
		got := calms.ScopeCutFrequency([]*issue.Issue{})
		if got != 0 {
			t.Fatalf("ScopeCutFrequency: expected 0 for empty slice, got %v", got)
		}
	})

	t.Run("no cancelled issues returns zero", func(t *testing.T) {
		iss1 := makeIssue(issue.StateDone, "", nil, nil)
		iss2 := makeIssue(issue.StatePending, "", nil, nil)
		got := calms.ScopeCutFrequency([]*issue.Issue{iss1, iss2})
		if got != 0.0 {
			t.Fatalf("ScopeCutFrequency: expected 0 with no cancelled issues, got %v", got)
		}
	})

	t.Run("all cancelled returns 100", func(t *testing.T) {
		iss1 := makeIssue(issue.StateCancelled, "", nil, nil)
		iss2 := makeIssue(issue.StateCancelled, "", nil, nil)
		got := calms.ScopeCutFrequency([]*issue.Issue{iss1, iss2})
		if got != 100.0 {
			t.Fatalf("ScopeCutFrequency: expected 100 when all cancelled, got %v", got)
		}
	})

	t.Run("one out of four cancelled returns 25", func(t *testing.T) {
		iss1 := makeIssue(issue.StateDone, "", nil, nil)
		iss2 := makeIssue(issue.StateDone, "", nil, nil)
		iss3 := makeIssue(issue.StatePending, "", nil, nil)
		iss4 := makeIssue(issue.StateCancelled, "", nil, nil)
		got := calms.ScopeCutFrequency([]*issue.Issue{iss1, iss2, iss3, iss4})
		if got != 25.0 {
			t.Fatalf("ScopeCutFrequency: expected 25, got %v", got)
		}
	})
}

func TestMetricCompleteness(t *testing.T) {
	t.Run("empty slice returns zero", func(t *testing.T) {
		got := calms.MetricCompleteness([]*issue.Issue{})
		if got != 0 {
			t.Fatalf("MetricCompleteness: expected 0 for empty slice, got %v", got)
		}
	})

	t.Run("issue with sessions and executable AC and context paths is complete", func(t *testing.T) {
		sessions := []issue.Session{{StartSHA: "abc", EndSHA: "def", Commits: 2}}
		body := bodyWithACAndContext("go test ./...", "internal/pkg")
		iss := makeIssue(issue.StateDone, body, sessions, nil)
		got := calms.MetricCompleteness([]*issue.Issue{iss})
		if got != 100.0 {
			t.Fatalf("MetricCompleteness: expected 100 for complete issue, got %v", got)
		}
	})

	t.Run("issue missing sessions is not complete", func(t *testing.T) {
		body := bodyWithACAndContext("go test ./...", "internal/pkg")
		iss := makeIssue(issue.StateDone, body, nil, nil)
		got := calms.MetricCompleteness([]*issue.Issue{iss})
		if got != 0.0 {
			t.Fatalf("MetricCompleteness: expected 0 when sessions missing, got %v", got)
		}
	})

	t.Run("issue missing AC commands is not complete", func(t *testing.T) {
		sessions := []issue.Session{{StartSHA: "abc", EndSHA: "def", Commits: 2}}
		body := "## Acceptance Criteria\n- [ ] manually review\n\n## Context\n- paths: internal/pkg\n"
		iss := makeIssue(issue.StateDone, body, sessions, nil)
		got := calms.MetricCompleteness([]*issue.Issue{iss})
		if got != 0.0 {
			t.Fatalf("MetricCompleteness: expected 0 when AC has no commands, got %v", got)
		}
	})

	t.Run("issue missing context paths is not complete", func(t *testing.T) {
		sessions := []issue.Session{{StartSHA: "abc", EndSHA: "def", Commits: 2}}
		body := bodyWithAC("go test ./...")
		iss := makeIssue(issue.StateDone, body, sessions, nil)
		got := calms.MetricCompleteness([]*issue.Issue{iss})
		if got != 0.0 {
			t.Fatalf("MetricCompleteness: expected 0 when context paths missing, got %v", got)
		}
	})

	t.Run("half complete returns 50", func(t *testing.T) {
		sessions := []issue.Session{{StartSHA: "abc", EndSHA: "def", Commits: 2}}
		body := bodyWithACAndContext("go test ./...", "internal/pkg")
		complete := makeIssue(issue.StateDone, body, sessions, nil)
		incomplete := makeIssue(issue.StatePending, "", nil, nil)
		got := calms.MetricCompleteness([]*issue.Issue{complete, incomplete})
		if got != 50.0 {
			t.Fatalf("MetricCompleteness: expected 50, got %v", got)
		}
	})
}

func TestCompute(t *testing.T) {
	sessions := []issue.Session{{StartSHA: "abc", EndSHA: "def", Commits: 2}}
	body := bodyWithACAndContext("go test ./...", "internal/pkg")

	// iss1: complete — has sessions, AC command, context paths; done with no overlap
	iss1 := makeIssue(issue.StateDone, body, sessions, []issue.Transition{
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
	})
	// iss2: cancelled — no AC command, no sessions
	iss2 := makeIssue(issue.StateCancelled, "", nil, []issue.Transition{
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(3 * time.Hour)},
		{State: string(issue.StateCancelled), Actor: "human:perigrin", Timestamp: t0.Add(5 * time.Hour)},
	})

	indicators := calms.Compute([]*issue.Issue{iss1, iss2})

	// VerificationCoverage: only iss1 has executable AC → 50%
	if indicators.VerificationCoverage != 50.0 {
		t.Errorf("Compute VerificationCoverage: got %v, want 50.0", indicators.VerificationCoverage)
	}
	// WIPViolations: sequential — no overlap
	if indicators.WIPViolations != 0 {
		t.Errorf("Compute WIPViolations: got %v, want 0", indicators.WIPViolations)
	}
	// ScopeCutFrequency: 1 out of 2 cancelled → 50%
	if indicators.ScopeCutFrequency != 50.0 {
		t.Errorf("Compute ScopeCutFrequency: got %v, want 50.0", indicators.ScopeCutFrequency)
	}
	// MetricCompleteness: only iss1 is complete → 50%
	if indicators.MetricCompleteness != 50.0 {
		t.Errorf("Compute MetricCompleteness: got %v, want 50.0", indicators.MetricCompleteness)
	}
}

func TestCompute_EmptySlice(t *testing.T) {
	indicators := calms.Compute([]*issue.Issue{})
	if indicators.VerificationCoverage != 0 {
		t.Errorf("Compute empty: VerificationCoverage should be 0, got %v", indicators.VerificationCoverage)
	}
	if indicators.WIPViolations != 0 {
		t.Errorf("Compute empty: WIPViolations should be 0, got %v", indicators.WIPViolations)
	}
	if indicators.ScopeCutFrequency != 0 {
		t.Errorf("Compute empty: ScopeCutFrequency should be 0, got %v", indicators.ScopeCutFrequency)
	}
	if indicators.MetricCompleteness != 0 {
		t.Errorf("Compute empty: MetricCompleteness should be 0, got %v", indicators.MetricCompleteness)
	}
}
