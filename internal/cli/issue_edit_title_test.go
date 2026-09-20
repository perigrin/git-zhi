// ABOUTME: Tests for issue edit --title: retitling an issue in place without
// ABOUTME: rewriting its ref, so the id and its graph edges survive.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestIssueEdit_Title verifies that --title changes the title and leaves the
// issue's id (ref) unchanged.
func TestIssueEdit_Title(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Original title")

	_, _, err := run("issue", "edit", id, "--title", "New title")
	if err != nil {
		t.Fatalf("issue edit --title failed: %v", err)
	}

	ref := issue.RefPrefix + id
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if iss.Title != "New title" {
		t.Fatalf("expected title %q, got %q", "New title", iss.Title)
	}

	// The ref is the id — reading at the same ref proves the id survived.
	if !app.Store.RefExists(ref) {
		t.Fatalf("expected ref %s to still exist after retitle", ref)
	}
}

// TestIssueEdit_TitleEmpty verifies that --title "" is rejected rather than
// persisted as an empty title.
func TestIssueEdit_TitleEmpty(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Keep this title")

	_, _, err := run("issue", "edit", id, "--title", "")
	if err == nil {
		t.Fatal("expected error for empty --title, got nil")
	}

	ref := issue.RefPrefix + id
	data, readErr := app.Store.ReadEntity(ref, "issue.md")
	if readErr != nil {
		t.Fatalf("ReadEntity: %v", readErr)
	}
	iss, parseErr := issue.Parse(data)
	if parseErr != nil {
		t.Fatalf("Parse: %v", parseErr)
	}
	if iss.Title != "Keep this title" {
		t.Fatalf("expected title unchanged, got %q", iss.Title)
	}
}

// TestIssueEdit_NoFlags verifies that bare `issue edit <ref>` with no flags at
// all still falls through to the not-yet-implemented $EDITOR message, rather
// than treating the absence of --title as a completed edit.
func TestIssueEdit_NoFlags(t *testing.T) {
	app, run := setupEditTest(t)
	id := createEditTestIssue(t, app, "Untouched title")

	stdout, _, err := run("issue", "edit", id)
	if err != nil {
		t.Fatalf("issue edit with no flags failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "not yet implemented") {
		t.Fatalf("expected 'not yet implemented' message, got: %q", stdout.String())
	}
}

// TestIssueEdit_TitlePreservesGraph builds a real blocks/blocked_by edge
// between two issues, retitles one of them, and proves the edge still
// resolves from both sides afterward.
func TestIssueEdit_TitlePreservesGraph(t *testing.T) {
	app, run := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "Blocker issue")
	id2 := createEditTestIssue(t, app, "Blocked issue")

	if _, _, err := run("issue", "edit", id1, "--block", id2); err != nil {
		t.Fatalf("issue edit --block failed: %v", err)
	}

	if _, _, err := run("issue", "edit", id1, "--title", "Retitled blocker"); err != nil {
		t.Fatalf("issue edit --title failed: %v", err)
	}

	ref1 := issue.RefPrefix + id1
	data1, err := app.Store.ReadEntity(ref1, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id1: %v", err)
	}
	iss1, err := issue.Parse(data1)
	if err != nil {
		t.Fatalf("Parse id1: %v", err)
	}
	if iss1.Title != "Retitled blocker" {
		t.Fatalf("expected retitled blocker's title to persist, got %q", iss1.Title)
	}
	found := false
	for _, u := range iss1.Blocks {
		if strings.HasPrefix(u.String(), id2[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id1.Blocks to still contain id2 after retitle, got: %v", iss1.Blocks)
	}

	ref2 := issue.RefPrefix + id2
	data2, err := app.Store.ReadEntity(ref2, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id2: %v", err)
	}
	iss2, err := issue.Parse(data2)
	if err != nil {
		t.Fatalf("Parse id2: %v", err)
	}
	found = false
	for _, u := range iss2.BlockedBy {
		if strings.HasPrefix(u.String(), id1[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id2.BlockedBy to still contain id1 after id1's retitle, got: %v", iss2.BlockedBy)
	}
}
