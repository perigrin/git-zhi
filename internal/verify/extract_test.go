// ABOUTME: Tests for ExtractCommands: pulls backtick-delimited commands from AC items.
// ABOUTME: Covers positive/negative subsections, flat v0.1 format, and items without commands.
package verify_test

import (
	"testing"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/verify"
)

func issueID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("failed to generate uuid: %v", err)
	}
	return id
}

func TestExtractCommands_PositiveAndNegative(t *testing.T) {
	body := `## Acceptance Criteria

### Positive Scenarios
- [ ] positional params work (` + "`go test -run TestSignature_Positional -v`" + `)
- [ ] error messages include line numbers (` + "`go test -run TestSignature_Errors -v`" + `)

### Negative Scenarios
- [ ] rejects duplicate param names (` + "`go test -run TestSignature_DuplicateParam -v`" + `)
- [ ] handles EOF mid-signature without panic (` + "`go test -run TestSignature_EOF -v`" + `)`

	sections := issue.ParseSections(body)
	id := issueID(t)
	cmds := verify.ExtractCommands(sections, id, "My Issue")

	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(cmds))
	}

	// First two should be tagged as positive
	if cmds[0].Text != "go test -run TestSignature_Positional -v" {
		t.Errorf("cmds[0].Text = %q", cmds[0].Text)
	}
	if cmds[0].Subsection != "positive" {
		t.Errorf("cmds[0].Subsection = %q, want %q", cmds[0].Subsection, "positive")
	}
	if cmds[0].IssueID != id {
		t.Errorf("cmds[0].IssueID = %v, want %v", cmds[0].IssueID, id)
	}
	if cmds[0].IssueTitle != "My Issue" {
		t.Errorf("cmds[0].IssueTitle = %q, want %q", cmds[0].IssueTitle, "My Issue")
	}

	if cmds[1].Text != "go test -run TestSignature_Errors -v" {
		t.Errorf("cmds[1].Text = %q", cmds[1].Text)
	}
	if cmds[1].Subsection != "positive" {
		t.Errorf("cmds[1].Subsection = %q, want %q", cmds[1].Subsection, "positive")
	}

	// Last two should be tagged as negative
	if cmds[2].Text != "go test -run TestSignature_DuplicateParam -v" {
		t.Errorf("cmds[2].Text = %q", cmds[2].Text)
	}
	if cmds[2].Subsection != "negative" {
		t.Errorf("cmds[2].Subsection = %q, want %q", cmds[2].Subsection, "negative")
	}

	if cmds[3].Text != "go test -run TestSignature_EOF -v" {
		t.Errorf("cmds[3].Text = %q", cmds[3].Text)
	}
	if cmds[3].Subsection != "negative" {
		t.Errorf("cmds[3].Subsection = %q, want %q", cmds[3].Subsection, "negative")
	}
}

func TestExtractCommands_FlatListTaggedPositive(t *testing.T) {
	// v0.1 flat AC list — no subsections; all commands tagged as positive
	body := `## Acceptance Criteria

- [ ] positional params work (` + "`go test -run TestPositional -v`" + `)
- [ ] variadic params work (` + "`go test -run TestVariadic -v`" + `)`

	sections := issue.ParseSections(body)
	id := issueID(t)
	cmds := verify.ExtractCommands(sections, id, "Flat Issue")

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands for flat AC list, got %d", len(cmds))
	}
	for i, cmd := range cmds {
		if cmd.Subsection != "positive" {
			t.Errorf("cmds[%d].Subsection = %q, want %q", i, cmd.Subsection, "positive")
		}
	}
	if cmds[0].Text != "go test -run TestPositional -v" {
		t.Errorf("cmds[0].Text = %q", cmds[0].Text)
	}
	if cmds[1].Text != "go test -run TestVariadic -v" {
		t.Errorf("cmds[1].Text = %q", cmds[1].Text)
	}
}

func TestExtractCommands_SkipsItemsWithoutBackticks(t *testing.T) {
	body := `## Acceptance Criteria

- [ ] positional params work (` + "`go test -run TestPositional -v`" + `)
- [ ] this item has no command
- [ ] variadic params work (` + "`go test -run TestVariadic -v`" + `)`

	sections := issue.ParseSections(body)
	id := issueID(t)
	cmds := verify.ExtractCommands(sections, id, "Skip Issue")

	// Only 2 commands: the item without backticks is silently skipped
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands (item without backticks skipped), got %d", len(cmds))
	}
	if cmds[0].Text != "go test -run TestPositional -v" {
		t.Errorf("cmds[0].Text = %q", cmds[0].Text)
	}
	if cmds[1].Text != "go test -run TestVariadic -v" {
		t.Errorf("cmds[1].Text = %q", cmds[1].Text)
	}
}

func TestExtractCommands_EmptySections(t *testing.T) {
	sections := issue.ParseSections("")
	id := issueID(t)
	cmds := verify.ExtractCommands(sections, id, "Empty Issue")

	if len(cmds) != 0 {
		t.Fatalf("expected 0 commands from empty sections, got %d", len(cmds))
	}
}
