// ABOUTME: Round-trip tests for multi-line frontmatter values, which can
// ABOUTME: otherwise emit a separator that Parse mistakes for the body fence.

package milestone_test

import (
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/milestone"
)

// TestMarshalParse_MultilinePostmortemWithHorizontalRule verifies a postmortem
// containing a markdown horizontal rule survives a round trip. YAML emits a
// multi-line string as an indented block scalar, so a "---" line inside it
// looks like the frontmatter terminator unless the scan is anchored.
func TestMarshalParse_MultilinePostmortemWithHorizontalRule(t *testing.T) {
	original := &milestone.Milestone{
		Name:       "v0.3",
		Created:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Body:       "## Plan\n\noriginal body",
		Postmortem: "Wins\n\n---\n\nActions",
	}

	data, err := milestone.MarshalMilestone(original)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}

	back, err := milestone.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if back.Postmortem != original.Postmortem {
		t.Errorf("postmortem truncated:\n got %q\nwant %q\n\nserialized form:\n%s",
			back.Postmortem, original.Postmortem, data)
	}
	if back.Body != original.Body {
		t.Errorf("body corrupted:\n got %q\nwant %q", back.Body, original.Body)
	}
}

// TestParse_SeparatorToleratesTrailingWhitespace verifies the anchored scan
// still accepts a separator written with trailing spaces, which existing
// milestones may contain.
func TestParse_SeparatorToleratesTrailingWhitespace(t *testing.T) {
	raw := []byte("name: v0.1\ncreated: 2026-01-01T00:00:00Z\n--- \nbody text\n")

	ms, err := milestone.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if ms.Name != "v0.1" {
		t.Errorf("name = %q", ms.Name)
	}
	if ms.Body != "body text" {
		t.Errorf("body = %q, want %q", ms.Body, "body text")
	}
}
