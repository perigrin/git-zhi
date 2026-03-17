// ABOUTME: Tests for PrioritizeIssues — verifies tier-1 (observed path overlap),
// ABOUTME: tier-2 (lineage connections), and tier-3 (topological order) sorting.
package verify

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
)

// makeIssue is a helper that creates a minimal done Issue with the given
// UUID string, title, and observed paths.
func makeIssue(id string, title string, observedPaths []string) *issue.Issue {
	uid, err := uuid.FromString(id)
	if err != nil {
		panic("makeIssue: invalid UUID: " + id)
	}
	return &issue.Issue{
		ID:            uid,
		Title:         title,
		State:         issue.StateDone,
		Urgency:       issue.UrgencyNormal,
		Created:       time.Now(),
		Updated:       time.Now(),
		ObservedPaths: observedPaths,
		Sessions:      []issue.Session{},
		Transitions:   []issue.Transition{},
	}
}

// mustUUID parses a UUID string and panics on error. Keeps tests concise.
func mustUUID(s string) uuid.UUID {
	u, err := uuid.FromString(s)
	if err != nil {
		panic("mustUUID: " + err.Error())
	}
	return u
}

// wellKnown UUIDs — fixed so tests are deterministic.
const (
	idA = "01920000-0000-0000-0000-000000000001"
	idB = "01920000-0000-0000-0000-000000000002"
	idC = "01920000-0000-0000-0000-000000000003"
	idD = "01920000-0000-0000-0000-000000000004"
)

// TestPriorityOrder_OverlapFirst verifies that issues whose ObservedPaths
// overlap recentChanges appear before issues without overlap.
func TestPriorityOrder_OverlapFirst(t *testing.T) {
	issA := makeIssue(idA, "issue A", []string{"internal/foo/foo.go"})
	issB := makeIssue(idB, "issue B", []string{"internal/bar/bar.go"})

	recentChanges := []string{"internal/foo/foo.go", "cmd/main.go"}
	topoOrder := []uuid.UUID{mustUUID(idA), mustUUID(idB)}

	result := PrioritizeIssues([]*issue.Issue{issA, issB}, recentChanges, topoOrder, nil)

	if len(result) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result))
	}
	if result[0].ID != mustUUID(idA) {
		t.Errorf("expected issue A (overlap) first, got %s (%s)", result[0].ID, result[0].Title)
	}
	if result[1].ID != mustUUID(idB) {
		t.Errorf("expected issue B (no overlap) second, got %s (%s)", result[1].ID, result[1].Title)
	}
}

// TestPriorityOrder_NoOverlapUsesTopoOrder verifies that when no issue has
// overlapping paths, the topological order is used for tier-2 ordering.
func TestPriorityOrder_NoOverlapUsesTopoOrder(t *testing.T) {
	issA := makeIssue(idA, "issue A", []string{"internal/foo/foo.go"})
	issB := makeIssue(idB, "issue B", []string{"internal/bar/bar.go"})
	issC := makeIssue(idC, "issue C", []string{"internal/baz/baz.go"})

	recentChanges := []string{"cmd/main.go"} // overlaps nothing
	// Topo order is C, A, B — reversed from UUID order
	topoOrder := []uuid.UUID{mustUUID(idC), mustUUID(idA), mustUUID(idB)}

	result := PrioritizeIssues([]*issue.Issue{issA, issB, issC}, recentChanges, topoOrder, nil)

	if len(result) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(result))
	}
	// All in tier-2; expected order is C, A, B (matches topoOrder)
	if result[0].ID != mustUUID(idC) {
		t.Errorf("expected issue C first (topo pos 0), got %s", result[0].Title)
	}
	if result[1].ID != mustUUID(idA) {
		t.Errorf("expected issue A second (topo pos 1), got %s", result[1].Title)
	}
	if result[2].ID != mustUUID(idB) {
		t.Errorf("expected issue B third (topo pos 2), got %s", result[2].Title)
	}
}

// TestPriorityOrder_MixedTiers verifies tier-1 issues all appear before tier-2,
// and tier-2 is sorted by topoOrder.
func TestPriorityOrder_MixedTiers(t *testing.T) {
	issA := makeIssue(idA, "issue A", []string{"internal/parser/parse.go"})
	issB := makeIssue(idB, "issue B", []string{"internal/parser/ast.go"}) // also overlaps
	issC := makeIssue(idC, "issue C", []string{"internal/storage/store.go"})
	issD := makeIssue(idD, "issue D", []string{"internal/graph/graph.go"})

	recentChanges := []string{"internal/parser/parse.go", "internal/parser/ast.go"}
	// Topo order: D, C, B, A — but B and A are tier-1 so only D, C matter for tier-2
	topoOrder := []uuid.UUID{mustUUID(idD), mustUUID(idC), mustUUID(idB), mustUUID(idA)}

	result := PrioritizeIssues([]*issue.Issue{issA, issB, issC, issD}, recentChanges, topoOrder, nil)

	if len(result) != 4 {
		t.Fatalf("expected 4 issues, got %d", len(result))
	}
	// Tier-1: A and B (in topo order within tier-1: B at pos 3, A at pos 4 → B before A)
	// Tier-2: D and C (in topo order: D at pos 0, C at pos 1)
	tier1IDs := []uuid.UUID{result[0].ID, result[1].ID}
	tier2IDs := []uuid.UUID{result[2].ID, result[3].ID}

	for _, id := range tier1IDs {
		if id != mustUUID(idA) && id != mustUUID(idB) {
			t.Errorf("expected tier-1 to contain only A and B, got %s", id)
		}
	}
	// Within tier-1, B (topo pos 2) should come before A (topo pos 3)
	if result[0].ID != mustUUID(idB) {
		t.Errorf("expected B first in tier-1 (topo pos 2), got %s", result[0].Title)
	}
	if result[1].ID != mustUUID(idA) {
		t.Errorf("expected A second in tier-1 (topo pos 3), got %s", result[1].Title)
	}

	for _, id := range tier2IDs {
		if id != mustUUID(idC) && id != mustUUID(idD) {
			t.Errorf("expected tier-2 to contain only C and D, got %s", id)
		}
	}
	// Within tier-2, D (topo pos 0) should come before C (topo pos 1)
	if result[2].ID != mustUUID(idD) {
		t.Errorf("expected D first in tier-2 (topo pos 0), got %s", result[2].Title)
	}
	if result[3].ID != mustUUID(idC) {
		t.Errorf("expected C second in tier-2 (topo pos 1), got %s", result[3].Title)
	}
}

// TestPriorityOrder_EmptyInputs verifies the function handles degenerate cases
// without panicking.
func TestPriorityOrder_EmptyInputs(t *testing.T) {
	// Empty issues
	result := PrioritizeIssues(nil, []string{"foo.go"}, nil, nil)
	if result == nil {
		result = []*issue.Issue{}
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil issues, got %d", len(result))
	}

	// Empty recent changes — all tier-3
	issA := makeIssue(idA, "issue A", []string{"foo.go"})
	result = PrioritizeIssues([]*issue.Issue{issA}, nil, []uuid.UUID{mustUUID(idA)}, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result))
	}
	if result[0].ID != mustUUID(idA) {
		t.Errorf("expected A in result")
	}
}

// TestPriorityOrder_IssueNotInTopoOrder verifies that issues not present in
// topoOrder are sorted after those that are (they go to the end of tier-2).
func TestPriorityOrder_IssueNotInTopoOrder(t *testing.T) {
	issA := makeIssue(idA, "issue A", []string{"other.go"})
	issB := makeIssue(idB, "issue B", []string{"other2.go"})

	recentChanges := []string{"unrelated.go"}
	// Only A is in topo order; B is absent
	topoOrder := []uuid.UUID{mustUUID(idA)}

	result := PrioritizeIssues([]*issue.Issue{issA, issB}, recentChanges, topoOrder, nil)

	if len(result) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result))
	}
	// A is in topoOrder so it comes first; B is not in topoOrder so it comes after
	if result[0].ID != mustUUID(idA) {
		t.Errorf("expected A first (in topoOrder), got %s", result[0].Title)
	}
	if result[1].ID != mustUUID(idB) {
		t.Errorf("expected B second (not in topoOrder), got %s", result[1].Title)
	}
}

// TestPriorityWithLineage verifies that issues with lineage connections to
// recently completed issues are placed in tier 2, between tier-1 (path
// overlap) issues and tier-3 (remaining topo-order) issues.
func TestPriorityWithLineage(t *testing.T) {
	// issA: overlaps recent changes → tier 1
	issA := makeIssue(idA, "issue A", []string{"internal/parser/parse.go"})
	// issB: lineage connection but no path overlap → tier 2
	issB := makeIssue(idB, "issue B", []string{"internal/config/cfg.go"})
	// issC: no overlap, no lineage → tier 3
	issC := makeIssue(idC, "issue C", []string{"internal/graph/graph.go"})
	// issD: no overlap, no lineage → tier 3
	issD := makeIssue(idD, "issue D", []string{"internal/storage/store.go"})

	recentChanges := []string{"internal/parser/parse.go"}
	topoOrder := []uuid.UUID{mustUUID(idA), mustUUID(idB), mustUUID(idC), mustUUID(idD)}

	// B has a lineage connection; C and D do not.
	lineageConnections := map[uuid.UUID]bool{
		mustUUID(idB): true,
	}

	result := PrioritizeIssues(
		[]*issue.Issue{issA, issB, issC, issD},
		recentChanges,
		topoOrder,
		lineageConnections,
	)

	if len(result) != 4 {
		t.Fatalf("expected 4 issues, got %d", len(result))
	}

	// Tier 1: A (path overlap)
	if result[0].ID != mustUUID(idA) {
		t.Errorf("expected A first (tier-1 path overlap), got %s (%s)", result[0].ID, result[0].Title)
	}

	// Tier 2: B (lineage connection)
	if result[1].ID != mustUUID(idB) {
		t.Errorf("expected B second (tier-2 lineage), got %s (%s)", result[1].ID, result[1].Title)
	}

	// Tier 3: C and D in topo order
	if result[2].ID != mustUUID(idC) {
		t.Errorf("expected C third (tier-3 topo pos 2), got %s (%s)", result[2].ID, result[2].Title)
	}
	if result[3].ID != mustUUID(idD) {
		t.Errorf("expected D fourth (tier-3 topo pos 3), got %s (%s)", result[3].ID, result[3].Title)
	}
}

// TestPriorityWithLineage_NilFallback verifies that passing nil for
// lineageConnections produces the same two-tier behavior as before: tier-1
// (path overlap) followed by tier-2 (topo order, now called tier 3 when
// lineage is active).
func TestPriorityWithLineage_NilFallback(t *testing.T) {
	issA := makeIssue(idA, "issue A", []string{"internal/foo/foo.go"})
	issB := makeIssue(idB, "issue B", []string{"internal/bar/bar.go"})
	issC := makeIssue(idC, "issue C", []string{"internal/baz/baz.go"})

	recentChanges := []string{"internal/foo/foo.go"}
	topoOrder := []uuid.UUID{mustUUID(idA), mustUUID(idB), mustUUID(idC)}

	// nil lineageConnections: fallback to two-tier behavior.
	result := PrioritizeIssues([]*issue.Issue{issA, issB, issC}, recentChanges, topoOrder, nil)

	if len(result) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(result))
	}
	// Tier 1: A (path overlap)
	if result[0].ID != mustUUID(idA) {
		t.Errorf("expected A first (tier-1 path overlap), got %s", result[0].Title)
	}
	// Former tier-2 (now tier-3 when lineage active, but fallback keeps order):
	// B (topo pos 1) before C (topo pos 2)
	if result[1].ID != mustUUID(idB) {
		t.Errorf("expected B second (topo pos 1), got %s", result[1].Title)
	}
	if result[2].ID != mustUUID(idC) {
		t.Errorf("expected C third (topo pos 2), got %s", result[2].Title)
	}
}

// TestPriorityWithLineage_OverlapTakesPrecedence verifies that an issue in
// both lineageConnections and with overlapping paths is placed in tier 1,
// not tier 2.
func TestPriorityWithLineage_OverlapTakesPrecedence(t *testing.T) {
	// issA: overlaps AND has lineage → should go to tier 1
	issA := makeIssue(idA, "issue A", []string{"internal/parser/parse.go"})
	// issB: lineage only → tier 2
	issB := makeIssue(idB, "issue B", []string{"internal/config/cfg.go"})
	// issC: neither → tier 3
	issC := makeIssue(idC, "issue C", []string{"internal/graph/graph.go"})

	recentChanges := []string{"internal/parser/parse.go"}
	topoOrder := []uuid.UUID{mustUUID(idA), mustUUID(idB), mustUUID(idC)}

	// Both A and B have lineage connections; only A also has path overlap.
	lineageConnections := map[uuid.UUID]bool{
		mustUUID(idA): true,
		mustUUID(idB): true,
	}

	result := PrioritizeIssues(
		[]*issue.Issue{issA, issB, issC},
		recentChanges,
		topoOrder,
		lineageConnections,
	)

	if len(result) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(result))
	}
	// A goes to tier 1 (path overlap takes precedence over lineage)
	if result[0].ID != mustUUID(idA) {
		t.Errorf("expected A first (tier-1 overlap takes precedence), got %s", result[0].Title)
	}
	// B goes to tier 2 (lineage only)
	if result[1].ID != mustUUID(idB) {
		t.Errorf("expected B second (tier-2 lineage), got %s", result[1].Title)
	}
	// C goes to tier 3
	if result[2].ID != mustUUID(idC) {
		t.Errorf("expected C third (tier-3), got %s", result[2].Title)
	}
}
