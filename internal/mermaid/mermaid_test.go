// ABOUTME: Tests for Gantt and DAG Mermaid chart rendering from IssueInput slices.
// ABOUTME: Covers section grouping, date formatting, state markers, and edge generation.
package mermaid

import (
	"strings"
	"testing"
	"time"
)

// parseDate is a test helper that panics on invalid date strings.
func parseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// ---------------------------------------------------------------------------
// RenderGantt tests
// ---------------------------------------------------------------------------

func TestRenderGantt_EmptyInput(t *testing.T) {
	got := RenderGantt(nil, "")
	// Must produce a valid (but empty) Gantt header.
	if !strings.Contains(got, "gantt") {
		t.Errorf("expected 'gantt' header, got:\n%s", got)
	}
	if !strings.Contains(got, "dateFormat YYYY-MM-DD") {
		t.Errorf("expected dateFormat line, got:\n%s", got)
	}
}

func TestRenderGantt_TitleDefault(t *testing.T) {
	got := RenderGantt(nil, "")
	if !strings.Contains(got, "title Chain") {
		t.Errorf("expected default title 'Chain', got:\n%s", got)
	}
}

func TestRenderGantt_TitleOverride(t *testing.T) {
	got := RenderGantt(nil, "My Project")
	if !strings.Contains(got, "title My Project") {
		t.Errorf("expected title 'My Project', got:\n%s", got)
	}
}

func TestRenderGantt_DoneStateMarker(t *testing.T) {
	issues := []IssueInput{
		{
			ID:      "019444a100000000000000000001",
			Title:   "Implement lexer",
			State:   "done",
			Created: parseDate("2026-03-01"),
			Updated: parseDate("2026-03-03"),
		},
	}
	got := RenderGantt(issues, "")
	if !strings.Contains(got, ":done,") {
		t.Errorf("expected ':done,' state marker, got:\n%s", got)
	}
	if !strings.Contains(got, "Implement lexer") {
		t.Errorf("expected issue title in output, got:\n%s", got)
	}
}

func TestRenderGantt_InProgressStateMarker(t *testing.T) {
	issues := []IssueInput{
		{
			ID:      "019444a100000000000000000002",
			Title:   "Parse declarations",
			State:   "in-progress",
			Created: parseDate("2026-03-04"),
			Updated: parseDate("2026-03-06"),
		},
	}
	got := RenderGantt(issues, "")
	if !strings.Contains(got, ":active,") {
		t.Errorf("expected ':active,' state marker, got:\n%s", got)
	}
}

func TestRenderGantt_PendingNoMarker(t *testing.T) {
	issues := []IssueInput{
		{
			ID:      "019444a100000000000000000003",
			Title:   "Tag resolution",
			State:   "pending",
			Created: parseDate("2026-03-04"),
			Updated: parseDate("2026-03-05"),
		},
	}
	got := RenderGantt(issues, "")
	// Pending issues should appear but with no :done, or :active, prefix.
	if !strings.Contains(got, "Tag resolution") {
		t.Errorf("expected issue title in output, got:\n%s", got)
	}
	if strings.Contains(got, ":done,") || strings.Contains(got, ":active,") {
		t.Errorf("pending issue should have no state marker, got:\n%s", got)
	}
}

func TestRenderGantt_CancelledNoMarker(t *testing.T) {
	issues := []IssueInput{
		{
			ID:      "019444a100000000000000000004",
			Title:   "Old approach",
			State:   "cancelled",
			Created: parseDate("2026-03-01"),
			Updated: parseDate("2026-03-02"),
		},
	}
	got := RenderGantt(issues, "")
	if strings.Contains(got, ":done,") || strings.Contains(got, ":active,") {
		t.Errorf("cancelled issue should have no state marker, got:\n%s", got)
	}
}

func TestRenderGantt_AssignedSections(t *testing.T) {
	issues := []IssueInput{
		{
			ID:       "019444a100000000000000000001",
			Title:    "Implement lexer",
			State:    "done",
			Assigned: "dev-a",
			Created:  parseDate("2026-03-01"),
			Updated:  parseDate("2026-03-03"),
		},
		{
			ID:       "019444a100000000000000000002",
			Title:    "Config persistence",
			State:    "done",
			Assigned: "agent-1",
			Created:  parseDate("2026-03-02"),
			Updated:  parseDate("2026-03-03"),
		},
	}
	got := RenderGantt(issues, "")
	if !strings.Contains(got, "section dev-a") {
		t.Errorf("expected 'section dev-a', got:\n%s", got)
	}
	if !strings.Contains(got, "section agent-1") {
		t.Errorf("expected 'section agent-1', got:\n%s", got)
	}
}

func TestRenderGantt_UnassignedSection(t *testing.T) {
	issues := []IssueInput{
		{
			ID:      "019444a100000000000000000003",
			Title:   "Tag resolution",
			State:   "pending",
			Created: parseDate("2026-03-04"),
			Updated: parseDate("2026-03-05"),
		},
	}
	got := RenderGantt(issues, "")
	if !strings.Contains(got, "section Unassigned") {
		t.Errorf("expected 'section Unassigned', got:\n%s", got)
	}
}

func TestRenderGantt_DateFormatting(t *testing.T) {
	issues := []IssueInput{
		{
			ID:       "019444a100000000000000000001",
			Title:    "Implement lexer",
			State:    "done",
			Assigned: "dev-a",
			Created:  parseDate("2026-03-01"),
			Updated:  parseDate("2026-03-03"),
		},
	}
	got := RenderGantt(issues, "")
	// Should contain YYYY-MM-DD formatted dates.
	if !strings.Contains(got, "2026-03-01") {
		t.Errorf("expected start date 2026-03-01, got:\n%s", got)
	}
	if !strings.Contains(got, "2026-03-03") {
		t.Errorf("expected end date 2026-03-03, got:\n%s", got)
	}
}

func TestRenderGantt_MultipleIssuesSameSection(t *testing.T) {
	issues := []IssueInput{
		{
			ID:       "019444a100000000000000000001",
			Title:    "Implement lexer",
			State:    "done",
			Assigned: "dev-a",
			Created:  parseDate("2026-03-01"),
			Updated:  parseDate("2026-03-03"),
		},
		{
			ID:       "019444a100000000000000000002",
			Title:    "Parse declarations",
			State:    "in-progress",
			Assigned: "dev-a",
			Created:  parseDate("2026-03-04"),
			Updated:  parseDate("2026-03-06"),
		},
	}
	got := RenderGantt(issues, "")
	// Both issues should appear; section header should appear once.
	count := strings.Count(got, "section dev-a")
	if count != 1 {
		t.Errorf("expected exactly 1 'section dev-a', got %d in:\n%s", count, got)
	}
	if !strings.Contains(got, "Implement lexer") {
		t.Errorf("expected 'Implement lexer' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Parse declarations") {
		t.Errorf("expected 'Parse declarations' in output, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// RenderDAG tests
// ---------------------------------------------------------------------------

func TestRenderDAG_EmptyInput(t *testing.T) {
	got := RenderDAG(nil)
	if !strings.Contains(got, "graph TD") {
		t.Errorf("expected 'graph TD' header, got:\n%s", got)
	}
}

func TestRenderDAG_NodeLabels(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
	}
	got := RenderDAG(issues)
	// Node ID is first 8 chars of issue ID.
	if !strings.Contains(got, `019444a1`) {
		t.Errorf("expected node ID prefix '019444a1', got:\n%s", got)
	}
	if !strings.Contains(got, "Implement lexer") {
		t.Errorf("expected issue title in node label, got:\n%s", got)
	}
}

func TestRenderDAG_DoneStatusIcon(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
	}
	got := RenderDAG(issues)
	if !strings.Contains(got, "✓") {
		t.Errorf("expected ✓ icon for done issue, got:\n%s", got)
	}
}

func TestRenderDAG_InProgressStatusIcon(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a200000000000000000002",
			Title: "Parse declarations",
			State: "in-progress",
		},
	}
	got := RenderDAG(issues)
	if !strings.Contains(got, "🔵") {
		t.Errorf("expected 🔵 icon for in-progress issue, got:\n%s", got)
	}
}

func TestRenderDAG_PendingStatusIcon(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a300000000000000000003",
			Title: "Error recovery",
			State: "pending",
		},
	}
	got := RenderDAG(issues)
	if !strings.Contains(got, "◻️") {
		t.Errorf("expected ◻️ icon for pending issue, got:\n%s", got)
	}
}

func TestRenderDAG_CancelledStatusIcon(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a400000000000000000004",
			Title: "Old approach",
			State: "cancelled",
		},
	}
	got := RenderDAG(issues)
	if !strings.Contains(got, "✗") {
		t.Errorf("expected ✗ icon for cancelled issue, got:\n%s", got)
	}
}

func TestRenderDAG_BlockedByEdges(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
		{
			ID:        "019444a200000000000000000002",
			Title:     "Parse declarations",
			State:     "in-progress",
			BlockedBy: []string{"019444a100000000000000000001"},
		},
	}
	got := RenderDAG(issues)
	// Edge: blocker --> blocked
	if !strings.Contains(got, "-->") {
		t.Errorf("expected '-->' edge, got:\n%s", got)
	}
}

func TestRenderDAG_NodeIDPrefix(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
		{
			ID:        "019444a200000000000000000002",
			Title:     "Parse declarations",
			State:     "in-progress",
			BlockedBy: []string{"019444a100000000000000000001"},
		},
	}
	got := RenderDAG(issues)
	// Both node IDs (first 8 chars) should appear.
	if !strings.Contains(got, "019444a1") {
		t.Errorf("expected node ID '019444a1', got:\n%s", got)
	}
	if !strings.Contains(got, "019444a2") {
		t.Errorf("expected node ID '019444a2', got:\n%s", got)
	}
}

func TestRenderDAG_MultipleBlockers(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
		{
			ID:    "019444a200000000000000000002",
			Title: "Config persistence",
			State: "done",
		},
		{
			ID:        "019444a300000000000000000003",
			Title:     "Error recovery",
			State:     "pending",
			BlockedBy: []string{"019444a100000000000000000001", "019444a200000000000000000002"},
		},
	}
	got := RenderDAG(issues)
	// Two edges leading to error recovery.
	edgeCount := strings.Count(got, "-->")
	if edgeCount != 2 {
		t.Errorf("expected 2 edges, got %d in:\n%s", edgeCount, got)
	}
}

func TestRenderDAG_NoEdgesForUnblockedIssues(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
		{
			ID:    "019444a200000000000000000002",
			Title: "Config persistence",
			State: "pending",
		},
	}
	got := RenderDAG(issues)
	// No edges — neither issue blocks the other.
	if strings.Contains(got, "-->") {
		t.Errorf("expected no edges for unblocked issues, got:\n%s", got)
	}
}
