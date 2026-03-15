// ABOUTME: Tests for the Issue domain model: struct construction with UUIDv7,
// ABOUTME: state constant values, and Parse/Marshal/SplitBatch functions.
package issue_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestNewIssue(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("failed to generate UUIDv7: %v", err)
	}

	iss := &issue.Issue{
		ID:        id,
		Title:     "Test issue",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   time.Now(),
		Updated:   time.Now(),
	}

	if iss.State != issue.StatePending {
		t.Fatalf("expected state %q, got %q", issue.StatePending, iss.State)
	}
}

func TestStateConstants(t *testing.T) {
	states := []issue.State{
		issue.StatePending,
		issue.StateInProgress,
		issue.StateDone,
		issue.StateCancelled,
	}
	expected := []string{"pending", "in-progress", "done", "cancelled"}

	for i, s := range states {
		if string(s) != expected[i] {
			t.Errorf("state %d: expected %q, got %q", i, expected[i], string(s))
		}
	}
}

func TestParse_ValidIssue(t *testing.T) {
	raw := []byte("---\ntitle: \"Parse basic signatures\"\nstate: pending\nmilestone: \"v0.1\"\nblocked_by:\n  - \"019444a1-b2c3-7def-0000-000000000001\"\n---\n\n## Prerequisites\n\n- [x] lexer complete\n\n## Context\n\nImplement parsing for basic subroutine signatures.\n\n## Acceptance Criteria\n\n- [ ] positional params work\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Title != "Parse basic signatures" {
		t.Fatalf("expected title %q, got %q", "Parse basic signatures", iss.Title)
	}
	if iss.State != issue.StatePending {
		t.Fatalf("expected state %q, got %q", issue.StatePending, iss.State)
	}
	if iss.Milestone != "v0.1" {
		t.Fatalf("expected milestone %q, got %q", "v0.1", iss.Milestone)
	}
	if len(iss.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocked_by, got %d", len(iss.BlockedBy))
	}
	if iss.Body == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestParse_MinimalIssue(t *testing.T) {
	raw := []byte("---\ntitle: \"Quick fix\"\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Title != "Quick fix" {
		t.Fatalf("expected title %q, got %q", "Quick fix", iss.Title)
	}
	if iss.State != "" {
		t.Fatalf("expected empty state from minimal parse, got %q", iss.State)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	raw := []byte("---\ntitle: [invalid yaml\n---\n")
	_, err := issue.Parse(raw)
	if err == nil {
		t.Fatal("expected error parsing invalid YAML")
	}
}

func TestMarshal_RoundTrip(t *testing.T) {
	raw := []byte("---\ntitle: Round trip test\nstate: pending\nmilestone: v0.1\n---\n\nBody content here.\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	out, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	iss2, err := issue.Parse(out)
	if err != nil {
		t.Fatalf("re-Parse failed: %v", err)
	}
	if iss.Title != iss2.Title {
		t.Fatalf("title mismatch: %q vs %q", iss.Title, iss2.Title)
	}
	if iss.State != iss2.State {
		t.Fatalf("state mismatch: %q vs %q", iss.State, iss2.State)
	}
	if iss.Milestone != iss2.Milestone {
		t.Fatalf("milestone mismatch: %q vs %q", iss.Milestone, iss2.Milestone)
	}
	if iss.Body != iss2.Body {
		t.Fatalf("body mismatch:\ngot:  %q\nwant: %q", iss2.Body, iss.Body)
	}
}

func TestMarshal_RoundTrip_WithSessions(t *testing.T) {
	startedAt := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	endedAt := time.Now().Truncate(time.Second)

	iss := &issue.Issue{
		Title:     "Session round trip",
		State:     issue.StateInProgress,
		Milestone: "v0.1",
		Sessions: []issue.Session{
			{StartSHA: "abc123", EndSHA: "def456", Commits: 5, StartedAt: &startedAt, EndedAt: &endedAt},
			{StartSHA: "ghi789", EndSHA: "", Commits: 0},
		},
		Created: time.Now().Truncate(time.Second),
		Updated: time.Now().Truncate(time.Second),
	}
	out, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	iss2, err := issue.Parse(out)
	if err != nil {
		t.Fatalf("re-Parse failed: %v", err)
	}
	if len(iss2.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(iss2.Sessions))
	}
	if iss2.Sessions[0].Commits != 5 {
		t.Fatalf("expected Commits=5, got %d", iss2.Sessions[0].Commits)
	}
	if iss2.Sessions[0].StartSHA != "abc123" {
		t.Fatalf("expected StartSHA 'abc123', got %q", iss2.Sessions[0].StartSHA)
	}
	// Verify timestamps round-trip correctly.
	if iss2.Sessions[0].StartedAt == nil {
		t.Fatal("expected StartedAt to be set after round-trip")
	}
	if !iss2.Sessions[0].StartedAt.Equal(startedAt) {
		t.Fatalf("expected StartedAt %v, got %v", startedAt, *iss2.Sessions[0].StartedAt)
	}
	if iss2.Sessions[0].EndedAt == nil {
		t.Fatal("expected EndedAt to be set after round-trip")
	}
	if !iss2.Sessions[0].EndedAt.Equal(endedAt) {
		t.Fatalf("expected EndedAt %v, got %v", endedAt, *iss2.Sessions[0].EndedAt)
	}
	// Second session (no timestamps) should have nil pointers.
	if iss2.Sessions[1].StartedAt != nil {
		t.Fatalf("expected StartedAt nil for session without timestamps, got %v", iss2.Sessions[1].StartedAt)
	}
	if iss2.Sessions[1].EndedAt != nil {
		t.Fatalf("expected EndedAt nil for session without timestamps, got %v", iss2.Sessions[1].EndedAt)
	}
}

func TestSplitBatch_SingleIssue(t *testing.T) {
	raw := []byte("---\ntitle: \"Single issue\"\n---\n\nBody\n")
	blocks := issue.SplitBatch(raw)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
}

func TestSplitBatch_MultipleIssues(t *testing.T) {
	raw := []byte("---\ntitle: \"First issue\"\n---\n\nFirst body\n\n---\ntitle: \"Second issue\"\n---\n\nSecond body\n")
	blocks := issue.SplitBatch(raw)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	for i, block := range blocks {
		_, err := issue.Parse(block)
		if err != nil {
			t.Fatalf("block %d failed to parse: %v", i, err)
		}
	}
}

func TestSplitBatch_BodyWithDashesAndColons(t *testing.T) {
	raw := []byte("---\ntitle: \"Single issue with tricky body\"\n---\n\nSome text here.\n\n---\n\nNote: this is not a new issue, just a horizontal rule followed by a note.\n")
	blocks := issue.SplitBatch(raw)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (body contains --- + colon line), got %d", len(blocks))
	}
}

func TestSplitBatch_EmptyInput(t *testing.T) {
	blocks := issue.SplitBatch([]byte{})
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty input, got %d", len(blocks))
	}
}
