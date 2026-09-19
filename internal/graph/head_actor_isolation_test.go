// ABOUTME: Tests that per-worker head resolution never hands out an issue whose
// ABOUTME: blocker is in progress under another actor.
package graph_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
)

// heldBy returns transitions marking an issue in progress under actor.
func heldBy(actor string) []issue.Transition {
	return []issue.Transition{
		{State: string(issue.StateInProgress), Actor: actor},
	}
}

// TestHeadForActor_BlockerHeldByAnother — the defect. Excluding another
// worker's in-progress issue used to remove it from the graph entirely, taking
// its edges with it, so its downstream looked unblocked to everyone else.
func TestHeadForActor_BlockerHeldByAnother(t *testing.T) {
	a := makeIssueWithTransitions("A upstream", issue.StateInProgress, heldBy("agent:one"))
	b := makeIssue("B downstream", issue.StatePending)
	linkIssues(a, b)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("agent:two")
	if err == nil && head != nil && head.ID == b.ID {
		t.Fatalf("handed B to agent:two while its blocker A is in progress under agent:one")
	}
	if err == nil && head != nil {
		t.Fatalf("expected no actionable issue for agent:two, got %q", head.Title)
	}
}

// TestHeadForActor_BlockerHeldByAnother_WithOtherWork — the same exclusion must
// not hide work that genuinely is available. C is unrelated and unblocked.
func TestHeadForActor_BlockerHeldByAnother_WithOtherWork(t *testing.T) {
	a := makeIssueWithTransitions("A upstream", issue.StateInProgress, heldBy("agent:one"))
	b := makeIssue("B downstream", issue.StatePending)
	c := makeIssue("C unrelated", issue.StatePending)
	linkIssues(a, b)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("agent:two")
	if err != nil {
		t.Fatalf("expected C to be available for agent:two, got error: %v", err)
	}
	if head.ID != c.ID {
		t.Errorf("expected C, got %q", head.Title)
	}
}

// TestHeadForActor_DoneBlockerStillUnblocks — a completed blocker must not be
// confused with a held one. B is genuinely ready.
func TestHeadForActor_DoneBlockerStillUnblocks(t *testing.T) {
	a := makeIssue("A upstream", issue.StateDone)
	b := makeIssue("B downstream", issue.StatePending)
	linkIssues(a, b)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("agent:two")
	if err != nil {
		t.Fatalf("expected B to be ready once A is done: %v", err)
	}
	if head.ID != b.ID {
		t.Errorf("expected B, got %q", head.Title)
	}
}

// TestHeadForActor_ResumesOwnWork — step 1 must survive the change: an actor
// holding an in-progress issue gets it back, blocked or not.
func TestHeadForActor_ResumesOwnWork(t *testing.T) {
	a := makeIssueWithTransitions("A mine", issue.StateInProgress, heldBy("agent:one"))
	b := makeIssue("B other", issue.StatePending)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("agent:one")
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if head.ID != a.ID {
		t.Errorf("expected the actor's own in-progress issue, got %q", head.Title)
	}
}

// TestHeadForActor_PrefersAssigned — step 3 must survive: an assigned, ready
// issue wins over an unassigned one.
func TestHeadForActor_PrefersAssigned(t *testing.T) {
	a := makeIssue("A unassigned", issue.StatePending)
	b := makeIssue("B assigned", issue.StatePending)
	b.Assigned = "agent:two"

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("agent:two")
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if head.ID != b.ID {
		t.Errorf("expected the assigned issue, got %q", head.Title)
	}
}

// TestHeadForActor_NeverReturnsAnothersInProgress — mutual exclusion itself.
func TestHeadForActor_NeverReturnsAnothersInProgress(t *testing.T) {
	a := makeIssueWithTransitions("A theirs", issue.StateInProgress, heldBy("agent:one"))

	g, err := graph.Build([]*issue.Issue{a})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("agent:two")
	if err == nil && head != nil {
		t.Errorf("agent:two was handed agent:one's in-progress issue %q", head.Title)
	}
}

// TestHeadNoActor_UnchangedByIsolation — the global path keeps v0.1 semantics:
// an in-progress issue is returned regardless of who holds it.
func TestHeadNoActor_UnchangedByIsolation(t *testing.T) {
	a := makeIssueWithTransitions("A held", issue.StateInProgress, heldBy("agent:one"))
	b := makeIssue("B pending", issue.StatePending)
	linkIssues(a, b)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	head, err := g.Head("")
	if err != nil {
		t.Fatalf("Head(\"\") failed: %v", err)
	}
	if head.ID != a.ID {
		t.Errorf("global head should return the in-progress issue, got %q", head.Title)
	}
}
