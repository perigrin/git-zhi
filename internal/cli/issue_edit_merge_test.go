// ABOUTME: Tests for the 'issue edit --merge <ref>' command: merging one issue
// ABOUTME: into another by combining title/body/sessions and transferring deps.
package cli_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// setupMergeTest creates a temporary repo with an initial commit.
func setupMergeTest(t *testing.T) (*cli.App, func(args ...string) (*strings.Builder, *strings.Builder, error)) {
	t.Helper()
	app, _ := setupEditTest(t)

	run := func(args ...string) (*strings.Builder, *strings.Builder, error) {
		stdout := new(strings.Builder)
		stderr := new(strings.Builder)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, stderr, err
	}
	return app, run
}

// createMergeTestIssue writes a pending issue directly to the store and returns its UUID.
func createMergeTestIssue(t *testing.T, app *cli.App, title, body string) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   now,
		Updated:   now,
		Body:      body,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := fmt.Sprintf("refs/zhi/_/issues/%s", id.String())
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}

func TestIssueEdit_Merge(t *testing.T) {
	app, run := setupMergeTest(t)

	id1 := createMergeTestIssue(t, app, "Error recovery", "Body of issue one.")
	id2 := createMergeTestIssue(t, app, "Full method modifier support", "Body of issue two.")

	// Merge id2 into id1.
	stdout, _, err := run("issue", "edit", id1.String(), "--merge", id2.String())
	if err != nil {
		t.Fatalf("issue edit --merge failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merged") {
		t.Fatalf("expected 'Merged' in output, got: %s", output)
	}
	if !strings.Contains(output, id2.String()[:8]) {
		t.Fatalf("expected merged UUID prefix in output, got: %s", output)
	}
	if !strings.Contains(output, id1.String()[:8]) {
		t.Fatalf("expected target UUID prefix in output, got: %s", output)
	}

	// Verify id1 has combined title.
	ref1 := issue.RefPrefix + id1.String()
	data1, err := app.Store.ReadEntity(ref1, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id1: %v", err)
	}
	iss1, err := issue.Parse(data1)
	if err != nil {
		t.Fatalf("Parse id1: %v", err)
	}

	if !strings.Contains(iss1.Title, "Error recovery") {
		t.Fatalf("expected id1 title to contain 'Error recovery', got %q", iss1.Title)
	}
	if !strings.Contains(iss1.Title, "Full method modifier support") {
		t.Fatalf("expected id1 title to contain 'Full method modifier support', got %q", iss1.Title)
	}
	if !strings.Contains(iss1.Body, "Body of issue one") {
		t.Fatalf("expected id1 body to contain original body, got %q", iss1.Body)
	}
	if !strings.Contains(iss1.Body, "Body of issue two") {
		t.Fatalf("expected id1 body to contain merged body, got %q", iss1.Body)
	}

	// Verify id2 is cancelled.
	ref2 := issue.RefPrefix + id2.String()
	data2, err := app.Store.ReadEntity(ref2, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity id2: %v", err)
	}
	iss2, err := issue.Parse(data2)
	if err != nil {
		t.Fatalf("Parse id2: %v", err)
	}

	if iss2.State != issue.StateCancelled {
		t.Fatalf("expected merged issue to be cancelled, got %s", iss2.State)
	}
	if len(iss2.Blocks) != 0 {
		t.Fatalf("expected merged issue Blocks to be cleared, got %v", iss2.Blocks)
	}
	if len(iss2.BlockedBy) != 0 {
		t.Fatalf("expected merged issue BlockedBy to be cleared, got %v", iss2.BlockedBy)
	}
}

func TestIssueEdit_Merge_TransfersDeps(t *testing.T) {
	app, run := setupMergeTest(t)

	// Create: id1 (target), id2 (to be merged), id3 (downstream dep of id2).
	id1 := createMergeTestIssue(t, app, "Target issue", "")
	id2 := createMergeTestIssue(t, app, "Source issue", "")
	id3 := createMergeTestIssue(t, app, "Downstream dep", "")

	// Wire id2 -> id3 (id2 blocks id3).
	now := time.Now()
	ref2 := issue.RefPrefix + id2.String()
	data2, _ := app.Store.ReadEntity(ref2, "issue.md")
	iss2, _ := issue.Parse(data2)
	iss2.Blocks = append(iss2.Blocks, id3)
	iss2.Updated = now
	out2, _ := issue.Marshal(iss2)
	_ = app.Store.WriteEntity(ref2, "issue.md", out2, "wire dep")

	ref3 := issue.RefPrefix + id3.String()
	data3, _ := app.Store.ReadEntity(ref3, "issue.md")
	iss3, _ := issue.Parse(data3)
	iss3.BlockedBy = append(iss3.BlockedBy, id2)
	iss3.Updated = now
	out3, _ := issue.Marshal(iss3)
	_ = app.Store.WriteEntity(ref3, "issue.md", out3, "wire dep")

	// Merge id2 into id1.
	_, _, err := run("issue", "edit", id1.String(), "--merge", id2.String())
	if err != nil {
		t.Fatalf("issue edit --merge failed: %v", err)
	}

	// id1 should now block id3.
	ref1 := issue.RefPrefix + id1.String()
	data1r, _ := app.Store.ReadEntity(ref1, "issue.md")
	iss1r, _ := issue.Parse(data1r)

	found := false
	for _, u := range iss1r.Blocks {
		if u == id3 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected id1 to block id3 after merge, got id1.Blocks: %v", iss1r.Blocks)
	}

	// id3 should now be blocked_by id1 (not id2).
	data3r, _ := app.Store.ReadEntity(ref3, "issue.md")
	iss3r, _ := issue.Parse(data3r)

	foundID1 := false
	for _, u := range iss3r.BlockedBy {
		if u == id1 {
			foundID1 = true
		}
		if u == id2 {
			t.Fatalf("id3 should not be blocked_by id2 after merge, got BlockedBy: %v", iss3r.BlockedBy)
		}
	}
	if !foundID1 {
		t.Fatalf("expected id3 to be blocked_by id1 after merge, got BlockedBy: %v", iss3r.BlockedBy)
	}
}
