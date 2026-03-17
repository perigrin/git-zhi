// ABOUTME: Tests for the Milestone domain model: struct construction
// ABOUTME: with name and description fields. Includes marshal round-trip tests.
package milestone_test

import (
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/milestone"
)

func TestNewMilestone(t *testing.T) {
	ms := &milestone.Milestone{
		Name:        "v0.1",
		Description: "Initial parser implementation",
		Created:     time.Now(),
	}

	if ms.Name != "v0.1" {
		t.Fatalf("expected name %q, got %q", "v0.1", ms.Name)
	}
}

func TestMarshalMilestone(t *testing.T) {
	ms := &milestone.Milestone{
		Name:        "v0.1",
		Description: "Initial implementation",
		Created:     time.Now(),
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty marshaled milestone")
	}
	s := string(data)
	if !strings.Contains(s, "v0.1") {
		t.Fatal("expected marshaled milestone to contain 'v0.1'")
	}
}

func TestParseMilestoneBody(t *testing.T) {
	// Verify that Parse() correctly handles YAML frontmatter + markdown body.
	raw := []byte(`name: "v0.1"
state: open
due: "2026-04-15T00:00:00Z"
resolution: "make integration-test"
created: "2026-03-14T10:00:00Z"
completed: null
---

## Context

Single Go binary at cmd/git-zhi/main.go.

## File Structure

  cmd/git-zhi/main.go
  internal/cli/root.go

## Design Rationale

Storage built first, then domain types, then CLI.
`)
	ms, err := milestone.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if ms.Name != "v0.1" {
		t.Fatalf("expected name %q, got %q", "v0.1", ms.Name)
	}
	if ms.State != "open" {
		t.Fatalf("expected state %q, got %q", "open", ms.State)
	}
	if ms.Resolution != "make integration-test" {
		t.Fatalf("expected resolution %q, got %q", "make integration-test", ms.Resolution)
	}
	if ms.Body == "" {
		t.Fatal("expected non-empty body")
	}
	if !strings.Contains(ms.Body, "## Context") {
		t.Fatalf("expected body to contain '## Context', got: %q", ms.Body)
	}
	if !strings.Contains(ms.Body, "## Design Rationale") {
		t.Fatalf("expected body to contain '## Design Rationale', got: %q", ms.Body)
	}
	if ms.Completed != nil {
		t.Fatalf("expected Completed to be nil, got %v", ms.Completed)
	}
}

func TestParseMilestone_BackwardCompat_PureYAML(t *testing.T) {
	// Pure YAML input (v0.1 format) must still parse correctly with no body.
	raw := []byte("name: v0.1\ndescription: Initial release\ncreated: \"2026-03-14T10:00:00Z\"\n")
	ms, err := milestone.Parse(raw)
	if err != nil {
		t.Fatalf("Parse of pure YAML failed: %v", err)
	}
	if ms.Name != "v0.1" {
		t.Fatalf("expected name %q, got %q", "v0.1", ms.Name)
	}
	if ms.Description != "Initial release" {
		t.Fatalf("expected description %q, got %q", "Initial release", ms.Description)
	}
	if ms.Body != "" {
		t.Fatalf("expected empty body for pure YAML input, got %q", ms.Body)
	}
}

func TestParseMilestone_DefaultState(t *testing.T) {
	// A milestone without a state field must default to "open".
	raw := []byte("name: v0.1\ncreated: \"2026-03-14T10:00:00Z\"\n")
	ms, err := milestone.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if ms.State != "open" {
		t.Fatalf("expected default state %q, got %q", "open", ms.State)
	}
}

func TestMarshalMilestone_FrontmatterBody(t *testing.T) {
	// Marshal must produce frontmatter + body output for milestones with a body.
	created, _ := time.Parse(time.RFC3339, "2026-03-14T10:00:00Z")
	ms := &milestone.Milestone{
		Name:       "v0.1",
		State:      "open",
		Resolution: "make test",
		Created:    created,
		Body:       "## Context\n\nSome context here.",
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "---") {
		t.Fatal("expected frontmatter separator '---' in output")
	}
	if !strings.Contains(s, "## Context") {
		t.Fatalf("expected body in output, got: %q", s)
	}
	if !strings.Contains(s, "name: v0.1") {
		t.Fatalf("expected 'name: v0.1' in output, got: %q", s)
	}
}

func TestMarshalMilestone_RoundTrip_WithBody(t *testing.T) {
	// Parse → Marshal → Parse must preserve all fields including the body.
	created, _ := time.Parse(time.RFC3339, "2026-03-14T10:00:00Z")
	ms := &milestone.Milestone{
		Name:       "v0.2",
		State:      "open",
		Resolution: "make integration-test",
		Created:    created,
		Body:       "## Context\n\nSingle Go binary.\n\n## File Structure\n\n  cmd/git-zhi/main.go\n\n## Design Rationale\n\nStorage built first.",
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone failed: %v", err)
	}
	ms2, err := milestone.Parse(data)
	if err != nil {
		t.Fatalf("re-Parse failed: %v", err)
	}
	if ms.Name != ms2.Name {
		t.Fatalf("Name mismatch: %q vs %q", ms.Name, ms2.Name)
	}
	if ms.State != ms2.State {
		t.Fatalf("State mismatch: %q vs %q", ms.State, ms2.State)
	}
	if ms.Resolution != ms2.Resolution {
		t.Fatalf("Resolution mismatch: %q vs %q", ms.Resolution, ms2.Resolution)
	}
	if ms.Body != ms2.Body {
		t.Fatalf("Body mismatch:\ngot:  %q\nwant: %q", ms2.Body, ms.Body)
	}
}

func TestParseMilestone_CompletedTimestamp(t *testing.T) {
	// A milestone with a completed timestamp must parse it correctly.
	raw := []byte(`name: "v0.1"
state: completed
created: "2026-03-14T10:00:00Z"
completed: "2026-04-01T12:00:00Z"
---

## Context

Done.
`)
	ms, err := milestone.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if ms.State != "completed" {
		t.Fatalf("expected state %q, got %q", "completed", ms.State)
	}
	if ms.Completed == nil {
		t.Fatal("expected Completed to be set")
	}
	expected, _ := time.Parse(time.RFC3339, "2026-04-01T12:00:00Z")
	if !ms.Completed.Equal(expected) {
		t.Fatalf("expected Completed %v, got %v", expected, *ms.Completed)
	}
}
