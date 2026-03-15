// ABOUTME: Tests for ParseSections: structured section extraction from issue markdown bodies.
// ABOUTME: Covers full bodies, checkbox variants, context parsing, empty input, and plain text.
package issue_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestParseSections_Full(t *testing.T) {
	body := `## Prerequisites

- [x] lexer complete
- [ ] parser wired up

## Context

- paths: internal/lexer, internal/parser
- docs: docs/design.md
- commands: go build ./..., go test ./...
- entrypoints: cmd/git-chain/main.go

Some extra description text here.

## Acceptance Criteria

- [X] positional params work
- [ ] variadic params work`

	s := issue.ParseSections(body)

	// Prerequisites
	if len(s.Prerequisites) != 2 {
		t.Fatalf("expected 2 prerequisites, got %d", len(s.Prerequisites))
	}
	if !s.Prerequisites[0].Checked {
		t.Errorf("expected Prerequisites[0].Checked = true")
	}
	if s.Prerequisites[0].Text != "lexer complete" {
		t.Errorf("expected Prerequisites[0].Text = %q, got %q", "lexer complete", s.Prerequisites[0].Text)
	}
	if s.Prerequisites[1].Checked {
		t.Errorf("expected Prerequisites[1].Checked = false")
	}
	if s.Prerequisites[1].Text != "parser wired up" {
		t.Errorf("expected Prerequisites[1].Text = %q, got %q", "parser wired up", s.Prerequisites[1].Text)
	}

	// Context
	if s.Context == nil {
		t.Fatal("expected non-nil Context")
	}
	if len(s.Context.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(s.Context.Paths))
	}
	if len(s.Context.Docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(s.Context.Docs))
	}
	if len(s.Context.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(s.Context.Commands))
	}
	if len(s.Context.Entrypoints) != 1 {
		t.Errorf("expected 1 entrypoint, got %d", len(s.Context.Entrypoints))
	}

	// Acceptance Criteria
	if len(s.AcceptanceCriteria) != 2 {
		t.Fatalf("expected 2 acceptance criteria, got %d", len(s.AcceptanceCriteria))
	}
	if !s.AcceptanceCriteria[0].Checked {
		t.Errorf("expected AcceptanceCriteria[0].Checked = true")
	}
	if s.AcceptanceCriteria[1].Checked {
		t.Errorf("expected AcceptanceCriteria[1].Checked = false")
	}

	// Description should contain the extra text
	if s.Description == "" {
		t.Error("expected non-empty Description from extra context text")
	}
}

func TestParseSections_CheckboxParsing(t *testing.T) {
	body := `## Prerequisites

- [x] lowercase x checked
- [X] uppercase X checked
- [ ] unchecked item`

	s := issue.ParseSections(body)

	if len(s.Prerequisites) != 3 {
		t.Fatalf("expected 3 prerequisites, got %d", len(s.Prerequisites))
	}

	cases := []struct {
		idx     int
		text    string
		checked bool
	}{
		{0, "lowercase x checked", true},
		{1, "uppercase X checked", true},
		{2, "unchecked item", false},
	}

	for _, tc := range cases {
		got := s.Prerequisites[tc.idx]
		if got.Text != tc.text {
			t.Errorf("item %d: expected text %q, got %q", tc.idx, tc.text, got.Text)
		}
		if got.Checked != tc.checked {
			t.Errorf("item %d: expected checked=%v, got %v", tc.idx, tc.checked, got.Checked)
		}
	}
}

func TestParseSections_ContextParsing(t *testing.T) {
	body := `## Context

- paths: internal/lexer, internal/parser
- docs: docs/design.md
- commands: go build ./..., go test ./...
- entrypoints: cmd/git-chain/main.go`

	s := issue.ParseSections(body)

	if s.Context == nil {
		t.Fatal("expected non-nil Context")
	}

	if len(s.Context.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d: %v", len(s.Context.Paths), s.Context.Paths)
	}
	if s.Context.Paths[0] != "internal/lexer" {
		t.Errorf("expected Paths[0]=%q, got %q", "internal/lexer", s.Context.Paths[0])
	}
	if s.Context.Paths[1] != "internal/parser" {
		t.Errorf("expected Paths[1]=%q, got %q", "internal/parser", s.Context.Paths[1])
	}

	if len(s.Context.Docs) != 1 {
		t.Errorf("expected 1 doc, got %d: %v", len(s.Context.Docs), s.Context.Docs)
	}
	if s.Context.Docs[0] != "docs/design.md" {
		t.Errorf("expected Docs[0]=%q, got %q", "docs/design.md", s.Context.Docs[0])
	}

	if len(s.Context.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d: %v", len(s.Context.Commands), s.Context.Commands)
	}
	if s.Context.Commands[0] != "go build ./..." {
		t.Errorf("expected Commands[0]=%q, got %q", "go build ./...", s.Context.Commands[0])
	}
	if s.Context.Commands[1] != "go test ./..." {
		t.Errorf("expected Commands[1]=%q, got %q", "go test ./...", s.Context.Commands[1])
	}

	if len(s.Context.Entrypoints) != 1 {
		t.Errorf("expected 1 entrypoint, got %d: %v", len(s.Context.Entrypoints), s.Context.Entrypoints)
	}
	if s.Context.Entrypoints[0] != "cmd/git-chain/main.go" {
		t.Errorf("expected Entrypoints[0]=%q, got %q", "cmd/git-chain/main.go", s.Context.Entrypoints[0])
	}
}

func TestParseSections_EmptyBody(t *testing.T) {
	s := issue.ParseSections("")

	if s == nil {
		t.Fatal("expected non-nil Sections for empty body")
	}
	if len(s.Prerequisites) != 0 {
		t.Errorf("expected 0 prerequisites, got %d", len(s.Prerequisites))
	}
	if s.Context != nil {
		t.Errorf("expected nil Context, got %+v", s.Context)
	}
	if len(s.AcceptanceCriteria) != 0 {
		t.Errorf("expected 0 acceptance criteria, got %d", len(s.AcceptanceCriteria))
	}
	if s.Description != "" {
		t.Errorf("expected empty description, got %q", s.Description)
	}
}

func TestParseSections_CaseInsensitive(t *testing.T) {
	body := "## prerequisites\n\n- [x] done\n\n## context\n\n- paths: main.go\n\n## acceptance criteria\n\n- [ ] works"
	s := issue.ParseSections(body)
	if len(s.Prerequisites) != 1 {
		t.Fatalf("expected 1 prerequisite with lowercase heading, got %d", len(s.Prerequisites))
	}
	if s.Context == nil {
		t.Fatal("expected context with lowercase heading")
	}
	if len(s.AcceptanceCriteria) != 1 {
		t.Fatalf("expected 1 AC with lowercase heading, got %d", len(s.AcceptanceCriteria))
	}
}

func TestParseSections_NoSections(t *testing.T) {
	body := "This is a plain text body with no headings.\n\nJust some prose."

	s := issue.ParseSections(body)

	if s == nil {
		t.Fatal("expected non-nil Sections")
	}
	if len(s.Prerequisites) != 0 {
		t.Errorf("expected 0 prerequisites, got %d", len(s.Prerequisites))
	}
	if s.Context != nil {
		t.Errorf("expected nil Context, got %+v", s.Context)
	}
	if len(s.AcceptanceCriteria) != 0 {
		t.Errorf("expected 0 acceptance criteria, got %d", len(s.AcceptanceCriteria))
	}
	if s.Description == "" {
		t.Error("expected non-empty description from plain text body")
	}
}
