// ABOUTME: Tests for issue edit flags: block, unblock, milestone, tag, untag, before, after.
// ABOUTME: Verifies dependency graph edges and tag refs are correctly created/deleted.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
)

// TestIssueEdit_Block creates two issues and verifies that --block adds
// the forward dependency edge in both directions.
func TestIssueEdit_Block(t *testing.T) {
	app, run := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "Issue one")
	id2 := createEditTestIssue(t, app, "Issue two")

	_, _, err := run("issue", "edit", id1, "--block", id2)
	if err != nil {
		t.Fatalf("issue edit --block failed: %v", err)
	}

	// id1 should block id2
	ref1 := issue.RefPrefix + id1
	data1, err := app.Store.ReadEntity(ref1, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id1: %v", err)
	}
	iss1, err := issue.Parse(data1)
	if err != nil {
		t.Fatalf("Parse id1: %v", err)
	}

	found := false
	for _, u := range iss1.Blocks {
		if strings.HasPrefix(u.String(), id2[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id1.Blocks to contain id2, got: %v", iss1.Blocks)
	}

	// id2 should be blocked_by id1
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
		t.Fatalf("expected id2.BlockedBy to contain id1, got: %v", iss2.BlockedBy)
	}
}

// TestIssueEdit_Unblock verifies that --unblock removes the dependency edges
// that were previously created by --block.
func TestIssueEdit_Unblock(t *testing.T) {
	app, run := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "Blocker issue")
	id2 := createEditTestIssue(t, app, "Blocked issue")

	// First block
	if _, _, err := run("issue", "edit", id1, "--block", id2); err != nil {
		t.Fatalf("issue edit --block failed: %v", err)
	}

	// Then unblock
	if _, _, err := run("issue", "edit", id1, "--unblock", id2); err != nil {
		t.Fatalf("issue edit --unblock failed: %v", err)
	}

	// id1.Blocks should no longer contain id2
	ref1 := issue.RefPrefix + id1
	data1, _ := app.Store.ReadEntity(ref1, "issue.md")
	iss1, _ := issue.Parse(data1)
	for _, u := range iss1.Blocks {
		if strings.HasPrefix(u.String(), id2[:8]) {
			t.Fatalf("expected id2 removed from id1.Blocks, but still found: %v", iss1.Blocks)
		}
	}

	// id2.BlockedBy should no longer contain id1
	ref2 := issue.RefPrefix + id2
	data2, _ := app.Store.ReadEntity(ref2, "issue.md")
	iss2, _ := issue.Parse(data2)
	for _, u := range iss2.BlockedBy {
		if strings.HasPrefix(u.String(), id1[:8]) {
			t.Fatalf("expected id1 removed from id2.BlockedBy, but still found: %v", iss2.BlockedBy)
		}
	}
}

// TestIssueEdit_Milestone verifies that --milestone changes the milestone field.
func TestIssueEdit_Milestone(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Milestone test issue")

	_, _, err := run("issue", "edit", id, "--milestone", "v0.2")
	if err != nil {
		t.Fatalf("issue edit --milestone failed: %v", err)
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

	if iss.Milestone != "v0.2" {
		t.Fatalf("expected milestone v0.2, got %q", iss.Milestone)
	}
}

// TestIssueEdit_Tag verifies that --tag creates the expected tag ref.
func TestIssueEdit_Tag(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Tag test issue")

	_, _, err := run("issue", "edit", id, "--tag", "parser")
	if err != nil {
		t.Fatalf("issue edit --tag failed: %v", err)
	}

	tagRef := "refs/chain/_/tags/parser"
	if !app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref %s to exist", tagRef)
	}
}

// TestIssueEdit_Untag verifies that --untag deletes the tag ref.
func TestIssueEdit_Untag(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Untag test issue")

	// Tag first
	if _, _, err := run("issue", "edit", id, "--tag", "mytag"); err != nil {
		t.Fatalf("issue edit --tag failed: %v", err)
	}

	tagRef := "refs/chain/_/tags/mytag"
	if !app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref to exist after --tag")
	}

	// Then untag
	if _, _, err := run("issue", "edit", id, "--untag", "mytag"); err != nil {
		t.Fatalf("issue edit --untag failed: %v", err)
	}

	if app.Store.RefExists(tagRef) {
		t.Fatalf("expected tag ref %s to be deleted after --untag", tagRef)
	}
}

// TestIssueEdit_Before verifies that --before adds a blocks edge from this issue
// to the target (this issue comes before target in the chain).
func TestIssueEdit_Before(t *testing.T) {
	app, run := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "First issue")
	id2 := createEditTestIssue(t, app, "Second issue")

	// id1 --before id2 means: id1 blocks id2
	_, _, err := run("issue", "edit", id1, "--before", id2)
	if err != nil {
		t.Fatalf("issue edit --before failed: %v", err)
	}

	// id1.Blocks should contain id2
	ref1 := issue.RefPrefix + id1
	data1, _ := app.Store.ReadEntity(ref1, "issue.md")
	iss1, _ := issue.Parse(data1)
	found := false
	for _, u := range iss1.Blocks {
		if strings.HasPrefix(u.String(), id2[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id1.Blocks to contain id2 after --before, got: %v", iss1.Blocks)
	}

	// id2.BlockedBy should contain id1
	ref2 := issue.RefPrefix + id2
	data2, _ := app.Store.ReadEntity(ref2, "issue.md")
	iss2, _ := issue.Parse(data2)
	found = false
	for _, u := range iss2.BlockedBy {
		if strings.HasPrefix(u.String(), id1[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id2.BlockedBy to contain id1 after --before, got: %v", iss2.BlockedBy)
	}
}

// TestIssueEdit_After verifies that --after adds a blocked_by edge (target blocks this issue).
func TestIssueEdit_After(t *testing.T) {
	app, run := setupEditTest(t)

	id1 := createEditTestIssue(t, app, "Earlier issue")
	id2 := createEditTestIssue(t, app, "Later issue")

	// id2 --after id1 means: id1 blocks id2 (id2 comes after id1)
	_, _, err := run("issue", "edit", id2, "--after", id1)
	if err != nil {
		t.Fatalf("issue edit --after failed: %v", err)
	}

	// id2.BlockedBy should contain id1
	ref2 := issue.RefPrefix + id2
	data2, _ := app.Store.ReadEntity(ref2, "issue.md")
	iss2, _ := issue.Parse(data2)
	found := false
	for _, u := range iss2.BlockedBy {
		if strings.HasPrefix(u.String(), id1[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id2.BlockedBy to contain id1 after --after, got: %v", iss2.BlockedBy)
	}

	// id1.Blocks should contain id2
	ref1 := issue.RefPrefix + id1
	data1, _ := app.Store.ReadEntity(ref1, "issue.md")
	iss1, _ := issue.Parse(data1)
	found = false
	for _, u := range iss1.Blocks {
		if strings.HasPrefix(u.String(), id2[:8]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id1.Blocks to contain id2 after --after, got: %v", iss1.Blocks)
	}
}
