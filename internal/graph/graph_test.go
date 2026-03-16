// ABOUTME: Tests for the Graph DAG: Build, AddEdge, CriticalChain, ReadySet,
// ABOUTME: Head, TopologicalSort, Cancel, and cycle/invariant enforcement.
package graph_test

import (
	"testing"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
)

func makeIssue(title string, state issue.State) *issue.Issue {
	return &issue.Issue{
		ID:        uuid.Must(uuid.NewV7()),
		Title:     title,
		State:     state,
		Milestone: "v0.1",
	}
}

// linkIssues wires the BlockedBy/Blocks fields between blocker and blocked.
// blocker blocks blocked.
func linkIssues(blocker, blocked *issue.Issue) {
	blocker.Blocks = append(blocker.Blocks, blocked.ID)
	blocked.BlockedBy = append(blocked.BlockedBy, blocker.ID)
}

// TestBuild_ValidDAG verifies that a 3-issue linear chain builds correctly.
// A blocks B blocks C — forward and backward edges must be populated.
func TestBuild_ValidDAG(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	linkIssues(a, b)
	linkIssues(b, c)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if g.Len() != 3 {
		t.Fatalf("expected 3 issues, got %d", g.Len())
	}
	// A should block B
	fwd := g.Forward(a.ID)
	if len(fwd) != 1 || fwd[0] != b.ID {
		t.Errorf("expected A to block B, forward(%s) = %v", a.ID, fwd)
	}
	// B should be blocked by A
	bwd := g.Backward(b.ID)
	if len(bwd) != 1 || bwd[0] != a.ID {
		t.Errorf("expected B blocked by A, backward(%s) = %v", b.ID, bwd)
	}
}

// TestBuild_DanglingEdge verifies that a BlockedBy referencing a nonexistent
// UUID is gracefully skipped — Build must not return an error.
func TestBuild_DanglingEdge(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	ghost, _ := uuid.NewV7()
	a.BlockedBy = append(a.BlockedBy, ghost)

	g, err := graph.Build([]*issue.Issue{a})
	if err != nil {
		t.Fatalf("Build with dangling edge must not error, got: %v", err)
	}
	if g.Len() != 1 {
		t.Fatalf("expected 1 issue, got %d", g.Len())
	}
	// The dangling backward edge must not be recorded.
	bwd := g.Backward(a.ID)
	if len(bwd) != 0 {
		t.Errorf("expected no backward edges for A (dangling ref skipped), got %v", bwd)
	}
}

// TestAddEdge_CreatesEdge verifies that AddEdge records forward and backward links.
func TestAddEdge_CreatesEdge(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if err := g.AddEdge(a.ID, b.ID); err != nil {
		t.Fatalf("AddEdge failed: %v", err)
	}
	fwd := g.Forward(a.ID)
	if len(fwd) != 1 || fwd[0] != b.ID {
		t.Errorf("expected a->b in forward, got %v", fwd)
	}
	bwd := g.Backward(b.ID)
	if len(bwd) != 1 || bwd[0] != a.ID {
		t.Errorf("expected a in backward[b], got %v", bwd)
	}
}

// TestAddEdge_CycleRejected verifies that AddEdge refuses an edge that
// would create a cycle (A->B->C, reject C->A).
func TestAddEdge_CycleRejected(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	linkIssues(a, b)
	linkIssues(b, c)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if err := g.AddEdge(c.ID, a.ID); err == nil {
		t.Fatal("expected error adding edge that creates cycle, got nil")
	}
}

// TestAddEdge_DoneIssueRejected verifies invariant 3: cannot add an incoming
// dependency to an already-done issue.
func TestAddEdge_DoneIssueRejected(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StateDone)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if err := g.AddEdge(a.ID, b.ID); err == nil {
		t.Fatal("expected error adding edge to done issue, got nil")
	}
}

// TestCriticalChain_Linear verifies the critical chain of a 3-issue linear
// chain A->B->C is [A, B, C].
func TestCriticalChain_Linear(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	linkIssues(a, b)
	linkIssues(b, c)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	chain := g.CriticalChain()
	if len(chain) != 3 {
		t.Fatalf("expected chain length 3, got %d: %v", len(chain), titlesOf(chain))
	}
	if chain[0].ID != a.ID {
		t.Errorf("expected chain[0]=A, got %s", chain[0].Title)
	}
	if chain[1].ID != b.ID {
		t.Errorf("expected chain[1]=B, got %s", chain[1].Title)
	}
	if chain[2].ID != c.ID {
		t.Errorf("expected chain[2]=C, got %s", chain[2].Title)
	}
}

// TestCriticalChain_Diamond verifies critical chain picks the longest path.
// Diamond: A->{B,C}->D, with B also blocked by E (extra dep on B's side).
// Paths:
//
//	A->B->D length 3, A->C->D length 3
//	E->B->D length 3
//	Longest starting from a root: E->B->D (or A->B->D / A->C->D depending on tiebreak).
//
// We just verify chain length >= 3 and all returned issues are non-nil.
func TestCriticalChain_Diamond(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	d := makeIssue("D", issue.StatePending)
	e := makeIssue("E", issue.StatePending)
	linkIssues(a, b)
	linkIssues(a, c)
	linkIssues(b, d)
	linkIssues(c, d)
	linkIssues(e, b)

	g, err := graph.Build([]*issue.Issue{a, b, c, d, e})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	chain := g.CriticalChain()
	if len(chain) < 3 {
		t.Fatalf("expected critical chain length >= 3, got %d: %v", len(chain), titlesOf(chain))
	}
	for _, iss := range chain {
		if iss == nil {
			t.Fatal("nil issue in critical chain")
		}
	}
}

// TestReadySet verifies that a pending issue whose only blocker is done
// appears in the ready set.
func TestReadySet(t *testing.T) {
	a := makeIssue("A", issue.StateDone)
	b := makeIssue("B", issue.StatePending)
	linkIssues(a, b)

	g, err := graph.Build([]*issue.Issue{a, b})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	ready := g.ReadySet()
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready issue, got %d: %v", len(ready), titlesOf(ready))
	}
	if ready[0].ID != b.ID {
		t.Errorf("expected B in ready set, got %s", ready[0].Title)
	}
}

// TestHead_InProgress verifies Head returns the in-progress issue when one exists.
func TestHead_InProgress(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StateInProgress)
	c := makeIssue("C", issue.StatePending)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	head, err := g.Head()
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if head.ID != b.ID {
		t.Errorf("expected in-progress B, got %s", head.Title)
	}
}

// TestHead_CriticalChainLeader verifies Head returns the critical chain issue
// with the most downstream dependencies when no issue is in-progress.
// Setup: A->B->C (all pending). A has 2 downstream (B and C), B has 1 (C).
// Head should return A.
func TestHead_CriticalChainLeader(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	linkIssues(a, b)
	linkIssues(b, c)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	head, err := g.Head()
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if head.ID != a.ID {
		t.Errorf("expected critical chain leader A, got %s", head.Title)
	}
}

// TestTopologicalSort verifies that dependencies appear before the issues
// that depend on them.
func TestTopologicalSort(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	linkIssues(a, b)
	linkIssues(b, c)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	sorted := g.TopologicalSort()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 issues in topological sort, got %d", len(sorted))
	}
	pos := make(map[uuid.UUID]int, 3)
	for i, iss := range sorted {
		pos[iss.ID] = i
	}
	if pos[a.ID] >= pos[b.ID] {
		t.Errorf("A must appear before B in topological order")
	}
	if pos[b.ID] >= pos[c.ID] {
		t.Errorf("B must appear before C in topological order")
	}
}

// TestCancel_ReconnectsGraph verifies that cancelling B in A->B->C
// causes a direct A->C edge to be added.
func TestCancel_ReconnectsGraph(t *testing.T) {
	a := makeIssue("A", issue.StatePending)
	b := makeIssue("B", issue.StatePending)
	c := makeIssue("C", issue.StatePending)
	linkIssues(a, b)
	linkIssues(b, c)

	g, err := graph.Build([]*issue.Issue{a, b, c})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	// Mark B as cancelled (caller sets state, then calls Cancel).
	b.State = issue.StateCancelled
	if err := g.Cancel(b.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// A should now block C directly.
	fwd := g.Forward(a.ID)
	foundC := false
	for _, id := range fwd {
		if id == c.ID {
			foundC = true
		}
	}
	if !foundC {
		t.Errorf("expected A->C after cancelling B, forward[A] = %v", fwd)
	}
	// B should have no edges.
	if len(g.Forward(b.ID)) != 0 {
		t.Errorf("expected no forward edges from cancelled B")
	}
	if len(g.Backward(b.ID)) != 0 {
		t.Errorf("expected no backward edges to cancelled B")
	}
}

// titlesOf is a helper that returns issue titles for readable failure messages.
func titlesOf(issues []*issue.Issue) []string {
	titles := make([]string, len(issues))
	for i, iss := range issues {
		titles[i] = iss.Title
	}
	return titles
}
