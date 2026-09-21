// ABOUTME: Tests for 'issue add --body -': the sentinel must read stdin as
// ABOUTME: the body, matching issue edit and milestone add/edit, never store "-" itself.
package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestIssueAdd_BodyDash_ReadsStdin verifies that --body - reads the body
// from stdin instead of storing the literal string "-". Before the fix,
// this stored a 1-byte body ("-") and exited 0 — silent data loss.
func TestIssueAdd_BodyDash_ReadsStdin(t *testing.T) {
	piped := "Widget is broken.\n\nNeeds repair before the next release goes out.\n"
	_, _, app, run := setupIssueAddTest(t, piped)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add", "Fix the widget", "--body", "-"); err != nil {
		t.Fatalf("issue add --body - failed: %v", err)
	}

	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 issue ref, got %d", len(refs))
	}
	content, err := app.Store.ReadEntity(refs[0], "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity failed: %v", err)
	}
	iss, err := issue.Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Body == "-" {
		t.Fatalf("body stored as literal '-': silent data loss reproduced")
	}
	if !strings.Contains(iss.Body, "Widget is broken.") {
		t.Fatalf("expected body read from stdin, got: %q", iss.Body)
	}
}

// TestIssueAdd_BodyDash_TTY verifies that --body - refuses to read from an
// interactive terminal rather than hanging, the same guard issue edit and
// milestone add/edit already apply via rejectInteractiveStdin.
func TestIssueAdd_BodyDash_TTY(t *testing.T) {
	_, _, app, _ := setupIssueAddTest(t, "")
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	tty, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer tty.Close()

	_, _, ttyErr := bodyEdgeRunWithReader(app, tty, "issue", "add", "Fix the widget", "--body", "-")
	if ttyErr == nil {
		t.Fatalf("expected non-zero exit for --body - on TTY-like stdin, got nil")
	}
	if !strings.Contains(ttyErr.Error(), "--body") {
		t.Fatalf("expected error to name the --body flag, got: %v", ttyErr)
	}

	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no issue created on TTY refusal, got %d", len(refs))
	}
}
