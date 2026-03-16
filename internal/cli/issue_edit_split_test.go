// ABOUTME: Tests for the 'issue edit --split' command: splitting an issue into
// ABOUTME: multiple via stdin with --- separators, with downstream dep transfer.
package cli_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/cli"
	"github.com/perigrin/git-chain/internal/issue"
)

// setupSplitTest creates a temporary repo with an initial commit and returns
// an App plus a run function that accepts an optional stdin string.
func setupSplitTest(t *testing.T) (*cli.App, func(stdin string, args ...string) (*strings.Builder, *strings.Builder, error)) {
	t.Helper()
	app, _ := setupEditTest(t)

	run := func(stdin string, args ...string) (*strings.Builder, *strings.Builder, error) {
		stdout := new(strings.Builder)
		stderr := new(strings.Builder)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, stderr, err
	}
	return app, run
}

// createSplitTestIssueWithDownstream creates an issue with a downstream dependent.
// Returns the UUID string of the original issue and the UUID string of the downstream dep.
func createSplitTestIssueWithDownstream(t *testing.T, app *cli.App, title string) (origUUID string, downUUID string) {
	t.Helper()

	origID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7 (orig): %v", err)
	}
	downID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7 (down): %v", err)
	}

	now := time.Now()

	// Original issue blocks the downstream one.
	orig := &issue.Issue{
		ID:        origID,
		Title:     title,
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   now,
		Updated:   now,
		Blocks:    []uuid.UUID{downID},
	}
	down := &issue.Issue{
		ID:        downID,
		Title:     "downstream dep",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   now,
		Updated:   now,
		BlockedBy: []uuid.UUID{origID},
	}

	for _, iss := range []*issue.Issue{orig, down} {
		data, err := issue.Marshal(iss)
		if err != nil {
			t.Fatalf("issue.Marshal: %v", err)
		}
		refPath := fmt.Sprintf("refs/chain/_/issues/%s", iss.ID.String())
		if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+iss.Title); err != nil {
			t.Fatalf("WriteEntity: %v", err)
		}
	}

	return origID.String(), downID.String()
}

func TestIssueEdit_Split(t *testing.T) {
	app, run := setupSplitTest(t)

	origUUID, downUUID := createSplitTestIssueWithDownstream(t, app, "Parse basic signatures")

	stdin := `---
title: Parse basic signatures
---

First block body.
---
title: Parse complex signatures
---

Second block body.
`

	stdout, _, err := run(stdin, "issue", "edit", origUUID, "--split")
	if err != nil {
		t.Fatalf("issue edit --split failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Split") {
		t.Fatalf("expected 'Split' in output, got: %s", output)
	}
	if !strings.Contains(output, origUUID[:8]) {
		t.Fatalf("expected original UUID prefix in output, got: %s", output)
	}

	// Original issue keeps its UUID and gets the first block's content.
	origRef := issue.RefPrefix + origUUID
	origData, err := app.Store.ReadEntity(origRef, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity original: %v", err)
	}
	origIss, err := issue.Parse(origData)
	if err != nil {
		t.Fatalf("Parse original: %v", err)
	}
	if origIss.Title != "Parse basic signatures" {
		t.Fatalf("expected original title preserved, got %q", origIss.Title)
	}

	// Original should now block the new issue (not the downstream dep anymore at orig level).
	if len(origIss.Blocks) == 0 {
		t.Fatalf("expected original to block the new issue, got empty Blocks")
	}
	// The original should NOT block the downstream dep directly anymore
	// (downstream dep is transferred to the last new issue).
	downUUIDParsed, err := uuid.FromString(downUUID)
	if err != nil {
		t.Fatalf("parse downUUID: %v", err)
	}
	for _, u := range origIss.Blocks {
		if u == downUUIDParsed {
			t.Fatalf("original should not block downstream dep directly; that should be transferred to last new issue")
		}
	}

	// Find the new issue UUID from the Blocks of the original.
	if len(origIss.Blocks) != 1 {
		t.Fatalf("expected original to block exactly 1 issue (the new one), got %d", len(origIss.Blocks))
	}
	newIssUUID := origIss.Blocks[0]
	newIssRef := issue.RefPrefix + newIssUUID.String()

	newIssData, err := app.Store.ReadEntity(newIssRef, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity new issue: %v", err)
	}
	newIss, err := issue.Parse(newIssData)
	if err != nil {
		t.Fatalf("Parse new issue: %v", err)
	}

	if newIss.Title != "Parse complex signatures" {
		t.Fatalf("expected new issue title 'Parse complex signatures', got %q", newIss.Title)
	}

	// The new (last) issue should now block the downstream dep.
	foundDownstream := false
	for _, u := range newIss.Blocks {
		if u == downUUIDParsed {
			foundDownstream = true
			break
		}
	}
	if !foundDownstream {
		t.Fatalf("expected last new issue to block downstream dep, got Blocks: %v", newIss.Blocks)
	}

	// The downstream dep should now be blocked_by the last new issue (not original).
	downRef := issue.RefPrefix + downUUID
	downData, err := app.Store.ReadEntity(downRef, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity downstream: %v", err)
	}
	downIss, err := issue.Parse(downData)
	if err != nil {
		t.Fatalf("Parse downstream: %v", err)
	}

	origUUIDParsed, err := uuid.FromString(origUUID)
	if err != nil {
		t.Fatalf("parse origUUID: %v", err)
	}
	for _, u := range downIss.BlockedBy {
		if u == origUUIDParsed {
			t.Fatalf("downstream dep should not have original in BlockedBy after split, got BlockedBy: %v", downIss.BlockedBy)
		}
	}
	foundNewInDown := false
	for _, u := range downIss.BlockedBy {
		if u == newIssUUID {
			foundNewInDown = true
			break
		}
	}
	if !foundNewInDown {
		t.Fatalf("downstream dep should be blocked_by the last new issue, got BlockedBy: %v", downIss.BlockedBy)
	}
}

func TestIssueEdit_Split_SingleBlock(t *testing.T) {
	app, run := setupSplitTest(t)

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     "Original title",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   now,
		Updated:   now,
		Body:      "Original body.",
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := issue.RefPrefix + id.String()
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue"); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}

	stdin := `---
title: Replaced title
---

Replaced body.
`

	_, _, err = run(stdin, "issue", "edit", id.String(), "--split")
	if err != nil {
		t.Fatalf("issue edit --split (single block) failed: %v", err)
	}

	// Verify the issue was updated in place (same UUID, new content).
	updData, readErr := app.Store.ReadEntity(refPath, "issue.md")
	if readErr != nil {
		t.Fatalf("ReadEntity after single-block split: %v", readErr)
	}
	updIss, parseErr := issue.Parse(updData)
	if parseErr != nil {
		t.Fatalf("Parse after single-block split: %v", parseErr)
	}

	if updIss.Title != "Replaced title" {
		t.Fatalf("expected title 'Replaced title', got %q", updIss.Title)
	}
	if !strings.Contains(updIss.Body, "Replaced body") {
		t.Fatalf("expected body to contain 'Replaced body', got %q", updIss.Body)
	}
	// No new issues chained — Blocks should be empty.
	if len(updIss.Blocks) != 0 {
		t.Fatalf("expected no Blocks after single-block split, got %v", updIss.Blocks)
	}
}
