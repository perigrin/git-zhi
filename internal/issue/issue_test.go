// ABOUTME: Tests for the Issue domain model: struct construction with UUIDv7,
// ABOUTME: state constant values, and Parse/Marshal/SplitBatch functions.
package issue_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
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

func TestParseUrgency_High(t *testing.T) {
	raw := []byte("---\ntitle: \"Urgent issue\"\nstate: pending\nurgency: high\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Urgency != issue.UrgencyHigh {
		t.Fatalf("expected urgency %q, got %q", issue.UrgencyHigh, iss.Urgency)
	}
}

func TestParseUrgency_Low(t *testing.T) {
	raw := []byte("---\ntitle: \"Low priority issue\"\nstate: pending\nurgency: low\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Urgency != issue.UrgencyLow {
		t.Fatalf("expected urgency %q, got %q", issue.UrgencyLow, iss.Urgency)
	}
}

func TestParseUrgency_Normal(t *testing.T) {
	raw := []byte("---\ntitle: \"Normal priority issue\"\nstate: pending\nurgency: normal\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Urgency != issue.UrgencyNormal {
		t.Fatalf("expected urgency %q, got %q", issue.UrgencyNormal, iss.Urgency)
	}
}

func TestParseUrgency_Default(t *testing.T) {
	// Issues without an urgency field must default to UrgencyNormal for
	// backward compatibility with v0.1 issues.
	raw := []byte("---\ntitle: \"Quick fix\"\nstate: pending\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Urgency != issue.UrgencyNormal {
		t.Fatalf("expected default urgency %q, got %q", issue.UrgencyNormal, iss.Urgency)
	}
}

func TestMarshal_RoundTrip_Urgency(t *testing.T) {
	raw := []byte("---\ntitle: Urgency round trip\nstate: pending\nmilestone: v0.2\nurgency: high\n---\n\nBody content.\n")
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
	if iss.Urgency != iss2.Urgency {
		t.Fatalf("urgency mismatch: %q vs %q", iss.Urgency, iss2.Urgency)
	}
}

func TestParseTransitions(t *testing.T) {
	raw := []byte(`---
title: "Issue with transitions"
state: done
transitions:
  - state: start
    actor: "human:perigrin"
    timestamp: "2026-03-16T10:30:00Z"
  - state: done
    actor: "agent:claude-code-1"
    timestamp: "2026-03-16T14:22:00Z"
---
`)
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(iss.Transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(iss.Transitions))
	}
	if iss.Transitions[0].State != "start" {
		t.Fatalf("expected first transition state %q, got %q", "start", iss.Transitions[0].State)
	}
	if iss.Transitions[0].Actor != "human:perigrin" {
		t.Fatalf("expected first transition actor %q, got %q", "human:perigrin", iss.Transitions[0].Actor)
	}
	expectedTime0, _ := time.Parse(time.RFC3339, "2026-03-16T10:30:00Z")
	if !iss.Transitions[0].Timestamp.Equal(expectedTime0) {
		t.Fatalf("expected first transition timestamp %v, got %v", expectedTime0, iss.Transitions[0].Timestamp)
	}
	if iss.Transitions[1].State != "done" {
		t.Fatalf("expected second transition state %q, got %q", "done", iss.Transitions[1].State)
	}
	if iss.Transitions[1].Actor != "agent:claude-code-1" {
		t.Fatalf("expected second transition actor %q, got %q", "agent:claude-code-1", iss.Transitions[1].Actor)
	}
}

func TestMarshal_RoundTrip_Transitions(t *testing.T) {
	ts1, _ := time.Parse(time.RFC3339, "2026-03-16T10:30:00Z")
	ts2, _ := time.Parse(time.RFC3339, "2026-03-16T14:22:00Z")
	iss := &issue.Issue{
		Title:   "Transition round trip",
		State:   issue.StateDone,
		Urgency: issue.UrgencyNormal,
		Transitions: []issue.Transition{
			{State: "start", Actor: "human:perigrin", Timestamp: ts1},
			{State: "done", Actor: "agent:claude-code-1", Timestamp: ts2},
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
	if len(iss2.Transitions) != 2 {
		t.Fatalf("expected 2 transitions after round-trip, got %d", len(iss2.Transitions))
	}
	if iss2.Transitions[0].State != "start" {
		t.Fatalf("expected first transition state %q, got %q", "start", iss2.Transitions[0].State)
	}
	if iss2.Transitions[0].Actor != "human:perigrin" {
		t.Fatalf("expected first transition actor %q, got %q", "human:perigrin", iss2.Transitions[0].Actor)
	}
	if !iss2.Transitions[0].Timestamp.Equal(ts1) {
		t.Fatalf("expected first transition timestamp %v, got %v", ts1, iss2.Transitions[0].Timestamp)
	}
	if iss2.Transitions[1].State != "done" {
		t.Fatalf("expected second transition state %q, got %q", "done", iss2.Transitions[1].State)
	}
	if iss2.Transitions[1].Actor != "agent:claude-code-1" {
		t.Fatalf("expected second transition actor %q, got %q", "agent:claude-code-1", iss2.Transitions[1].Actor)
	}
}

func TestParseTransitions_BackwardCompat(t *testing.T) {
	// Issues without a transitions field must produce an empty (non-nil) slice
	// for backward compatibility with v0.1 issues that predate this field.
	raw := []byte("---\ntitle: \"Old issue\"\nstate: done\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Transitions == nil {
		t.Fatal("expected non-nil Transitions slice for backward compat, got nil")
	}
	if len(iss.Transitions) != 0 {
		t.Fatalf("expected 0 transitions for issue without transitions field, got %d", len(iss.Transitions))
	}
}

func TestParseObservedPaths(t *testing.T) {
	raw := []byte(`---
title: "Issue with observed paths"
state: done
observed_paths:
  - "internal/issue/issue.go"
  - "internal/issue/issue_test.go"
---
`)
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(iss.ObservedPaths) != 2 {
		t.Fatalf("expected 2 observed_paths, got %d", len(iss.ObservedPaths))
	}
	if iss.ObservedPaths[0] != "internal/issue/issue.go" {
		t.Fatalf("expected first path %q, got %q", "internal/issue/issue.go", iss.ObservedPaths[0])
	}
	if iss.ObservedPaths[1] != "internal/issue/issue_test.go" {
		t.Fatalf("expected second path %q, got %q", "internal/issue/issue_test.go", iss.ObservedPaths[1])
	}
}

func TestMarshal_RoundTrip_ObservedPaths(t *testing.T) {
	iss := &issue.Issue{
		Title:   "Observed paths round trip",
		State:   issue.StateDone,
		Urgency: issue.UrgencyNormal,
		ObservedPaths: []string{
			"internal/issue/issue.go",
			"internal/cli/issue_edit.go",
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
	if len(iss2.ObservedPaths) != 2 {
		t.Fatalf("expected 2 observed_paths after round-trip, got %d", len(iss2.ObservedPaths))
	}
	if iss2.ObservedPaths[0] != "internal/issue/issue.go" {
		t.Fatalf("expected first path %q, got %q", "internal/issue/issue.go", iss2.ObservedPaths[0])
	}
	if iss2.ObservedPaths[1] != "internal/cli/issue_edit.go" {
		t.Fatalf("expected second path %q, got %q", "internal/cli/issue_edit.go", iss2.ObservedPaths[1])
	}
}

func TestParseObservedPaths_BackwardCompat(t *testing.T) {
	// Issues without an observed_paths field must produce an empty (non-nil)
	// slice for backward compatibility with v0.1 issues that predate this field.
	raw := []byte("---\ntitle: \"Old issue\"\nstate: done\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.ObservedPaths == nil {
		t.Fatal("expected non-nil ObservedPaths slice for backward compat, got nil")
	}
	if len(iss.ObservedPaths) != 0 {
		t.Fatalf("expected 0 observed_paths for issue without the field, got %d", len(iss.ObservedPaths))
	}
}

func TestParseLabels(t *testing.T) {
	raw := []byte(`---
title: "Issue with labels"
state: pending
labels:
  - "LOPS"
  - "microservices"
---
`)
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(iss.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(iss.Labels))
	}
	if iss.Labels[0] != "LOPS" {
		t.Fatalf("expected first label %q, got %q", "LOPS", iss.Labels[0])
	}
	if iss.Labels[1] != "microservices" {
		t.Fatalf("expected second label %q, got %q", "microservices", iss.Labels[1])
	}
}

func TestMarshal_RoundTrip_Labels(t *testing.T) {
	iss := &issue.Issue{
		Title:   "Labels round trip",
		State:   issue.StatePending,
		Urgency: issue.UrgencyNormal,
		Labels:  []string{"LOPS", "microservices"},
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
	if len(iss2.Labels) != 2 {
		t.Fatalf("expected 2 labels after round-trip, got %d", len(iss2.Labels))
	}
	if iss2.Labels[0] != "LOPS" {
		t.Fatalf("expected first label %q, got %q", "LOPS", iss2.Labels[0])
	}
	if iss2.Labels[1] != "microservices" {
		t.Fatalf("expected second label %q, got %q", "microservices", iss2.Labels[1])
	}
}

func TestParseLabels_BackwardCompat(t *testing.T) {
	// Issues without a labels field must produce an empty (non-nil) slice for
	// backward compatibility with v0.1/v0.2 issues that predate this field.
	raw := []byte("---\ntitle: \"Old issue\"\nstate: done\n---\n")
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Labels == nil {
		t.Fatal("expected non-nil Labels slice for backward compat, got nil")
	}
	if len(iss.Labels) != 0 {
		t.Fatalf("expected 0 labels for issue without the field, got %d", len(iss.Labels))
	}
}

func TestMarshal_Labels_OmitWhenEmpty(t *testing.T) {
	// Empty labels must not appear in the serialized YAML (omitempty).
	iss := &issue.Issue{
		Title:   "No labels issue",
		State:   issue.StatePending,
		Urgency: issue.UrgencyNormal,
		Labels:  []string{},
		Created: time.Now().Truncate(time.Second),
		Updated: time.Now().Truncate(time.Second),
	}
	out, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if bytes.Contains(out, []byte("labels:")) {
		t.Fatalf("expected labels field to be omitted when empty, but found it in:\n%s", out)
	}
}
