// ABOUTME: Tests for the 'issue edit --purge' command: permanently deleting an issue
// ABOUTME: with --yes confirmation, cleaning up all dependency edges.
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

// setupPurgeTest creates a temporary repo with an initial commit.
func setupPurgeTest(t *testing.T) (*cli.App, func(args ...string) (*strings.Builder, *strings.Builder, error)) {
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

// createPurgeTestIssue writes a pending issue directly and returns its UUID.
func createPurgeTestIssue(t *testing.T, app *cli.App, title string) uuid.UUID {
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

func TestIssueEdit_Purge(t *testing.T) {
	app, run := setupPurgeTest(t)

	// Create three issues: idA blocks idB, which blocks idC.
	// We will purge idB and verify edges are cleaned up.
	idA := createPurgeTestIssue(t, app, "Issue A (upstream)")
	idB := createPurgeTestIssue(t, app, "Issue B (to be purged)")
	idC := createPurgeTestIssue(t, app, "Issue C (downstream)")

	// Wire: A -> B -> C
	now := time.Now()
	refA := issue.RefPrefix + idA.String()
	dataA, _ := app.Store.ReadEntity(refA, "issue.md")
	issA, _ := issue.Parse(dataA)
	issA.Blocks = append(issA.Blocks, idB)
	issA.Updated = now
	outA, _ := issue.Marshal(issA)
	_ = app.Store.WriteEntity(refA, "issue.md", outA, "wire A->B")

	refB := issue.RefPrefix + idB.String()
	dataB, _ := app.Store.ReadEntity(refB, "issue.md")
	issB, _ := issue.Parse(dataB)
	issB.BlockedBy = append(issB.BlockedBy, idA)
	issB.Blocks = append(issB.Blocks, idC)
	issB.Updated = now
	outB, _ := issue.Marshal(issB)
	_ = app.Store.WriteEntity(refB, "issue.md", outB, "wire A->B->C")

	refC := issue.RefPrefix + idC.String()
	dataC, _ := app.Store.ReadEntity(refC, "issue.md")
	issC, _ := issue.Parse(dataC)
	issC.BlockedBy = append(issC.BlockedBy, idB)
	issC.Updated = now
	outC, _ := issue.Marshal(issC)
	_ = app.Store.WriteEntity(refC, "issue.md", outC, "wire B->C")

	// Purge idB with --yes.
	stdout, _, err := run("issue", "edit", idB.String(), "--purge", "--yes")
	if err != nil {
		t.Fatalf("issue edit --purge --yes failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Purged") {
		t.Fatalf("expected 'Purged' in output, got: %s", output)
	}
	if !strings.Contains(output, idB.String()[:8]) {
		t.Fatalf("expected purged UUID prefix in output, got: %s", output)
	}

	// idB ref should no longer exist.
	if app.Store.RefExists(refB) {
		t.Fatalf("expected ref %s to be deleted after purge", refB)
	}

	// idA.Blocks should no longer contain idB.
	dataAr, err := app.Store.ReadEntity(refA, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity idA after purge: %v", err)
	}
	issAr, err := issue.Parse(dataAr)
	if err != nil {
		t.Fatalf("Parse idA after purge: %v", err)
	}
	for _, u := range issAr.Blocks {
		if u == idB {
			t.Fatalf("expected idB removed from idA.Blocks after purge, got: %v", issAr.Blocks)
		}
	}

	// idC.BlockedBy should no longer contain idB.
	dataCr, err := app.Store.ReadEntity(refC, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity idC after purge: %v", err)
	}
	issCr, err := issue.Parse(dataCr)
	if err != nil {
		t.Fatalf("Parse idC after purge: %v", err)
	}
	for _, u := range issCr.BlockedBy {
		if u == idB {
			t.Fatalf("expected idB removed from idC.BlockedBy after purge, got: %v", issCr.BlockedBy)
		}
	}
}

func TestIssueEdit_Purge_NoYes(t *testing.T) {
	app, run := setupPurgeTest(t)

	id := createPurgeTestIssue(t, app, "Issue to maybe purge")

	// Attempt purge without --yes.
	_, _, err := run("issue", "edit", id.String(), "--purge")
	if err == nil {
		t.Fatal("expected error when purging without --yes, got nil")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected '--yes' mentioned in error, got: %v", err)
	}

	// Issue ref should still exist.
	ref := issue.RefPrefix + id.String()
	if !app.Store.RefExists(ref) {
		t.Fatalf("expected issue ref to still exist after failed purge without --yes")
	}
}

// TestIssueEdit_Purge_DependencyWarning verifies that purging an issue with
// upstream and downstream dependents prints a "Dependency cleanup:" section.
func TestIssueEdit_Purge_DependencyWarning(t *testing.T) {
	app, run := setupPurgeTest(t)

	idA := createPurgeTestIssue(t, app, "Upstream issue")
	idB := createPurgeTestIssue(t, app, "Target to purge")
	idC := createPurgeTestIssue(t, app, "Downstream issue")

	// Wire: A -> B -> C
	now := time.Now()
	refA := issue.RefPrefix + idA.String()
	dataA, _ := app.Store.ReadEntity(refA, "issue.md")
	issA, _ := issue.Parse(dataA)
	issA.Blocks = append(issA.Blocks, idB)
	issA.Updated = now
	outA, _ := issue.Marshal(issA)
	_ = app.Store.WriteEntity(refA, "issue.md", outA, "wire A->B")

	refB := issue.RefPrefix + idB.String()
	dataB, _ := app.Store.ReadEntity(refB, "issue.md")
	issB, _ := issue.Parse(dataB)
	issB.BlockedBy = append(issB.BlockedBy, idA)
	issB.Blocks = append(issB.Blocks, idC)
	issB.Updated = now
	outB, _ := issue.Marshal(issB)
	_ = app.Store.WriteEntity(refB, "issue.md", outB, "wire A->B->C")

	refC := issue.RefPrefix + idC.String()
	dataC, _ := app.Store.ReadEntity(refC, "issue.md")
	issC, _ := issue.Parse(dataC)
	issC.BlockedBy = append(issC.BlockedBy, idB)
	issC.Updated = now
	outC, _ := issue.Marshal(issC)
	_ = app.Store.WriteEntity(refC, "issue.md", outC, "wire B->C")

	stdout, _, err := run("issue", "edit", idB.String(), "--purge", "--yes")
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}

	output := stdout.String()

	// Should contain "Dependency cleanup:" section.
	if !strings.Contains(output, "Dependency cleanup:") {
		t.Errorf("expected 'Dependency cleanup:' in output, got: %q", output)
	}

	// Should mention the upstream issue.
	if !strings.Contains(output, idA.String()[:8]) {
		t.Errorf("expected upstream UUID prefix %s in output, got: %q", idA.String()[:8], output)
	}

	// Should mention the downstream issue.
	if !strings.Contains(output, idC.String()[:8]) {
		t.Errorf("expected downstream UUID prefix %s in output, got: %q", idC.String()[:8], output)
	}

	// "Dependency cleanup:" must appear before "Purged".
	cleanupIdx := strings.Index(output, "Dependency cleanup:")
	purgedIdx := strings.Index(output, "Purged")
	if cleanupIdx >= purgedIdx {
		t.Errorf("expected 'Dependency cleanup:' before 'Purged' in output, got: %q", output)
	}
}

// TestIssueEdit_Purge_NoDependents_NoWarning verifies that purging an issue
// with no dependents does not print a "Dependency cleanup:" section.
func TestIssueEdit_Purge_NoDependents_NoWarning(t *testing.T) {
	app, run := setupPurgeTest(t)

	id := createPurgeTestIssue(t, app, "Lonely issue")

	stdout, _, err := run("issue", "edit", id.String(), "--purge", "--yes")
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "Dependency cleanup:") {
		t.Errorf("expected no 'Dependency cleanup:' for issue with no dependents, got: %q", output)
	}
	if !strings.Contains(output, "Purged") {
		t.Errorf("expected 'Purged' in output, got: %q", output)
	}
}

// TestIssueEdit_Purge_NoYes_WithDependents verifies that purging without --yes
// when dependents exist includes the dependent count in the error message.
func TestIssueEdit_Purge_NoYes_WithDependents(t *testing.T) {
	app, run := setupPurgeTest(t)

	idA := createPurgeTestIssue(t, app, "Upstream")
	idB := createPurgeTestIssue(t, app, "Target")

	// Wire: A -> B (B is blocked by A)
	now := time.Now()
	refA := issue.RefPrefix + idA.String()
	dataA, _ := app.Store.ReadEntity(refA, "issue.md")
	issA, _ := issue.Parse(dataA)
	issA.Blocks = append(issA.Blocks, idB)
	issA.Updated = now
	outA, _ := issue.Marshal(issA)
	_ = app.Store.WriteEntity(refA, "issue.md", outA, "wire A->B")

	refB := issue.RefPrefix + idB.String()
	dataB, _ := app.Store.ReadEntity(refB, "issue.md")
	issB, _ := issue.Parse(dataB)
	issB.BlockedBy = append(issB.BlockedBy, idA)
	issB.Updated = now
	outB, _ := issue.Marshal(issB)
	_ = app.Store.WriteEntity(refB, "issue.md", outB, "wire A->B blocked_by")

	// Try to purge B without --yes.
	_, _, err := run("issue", "edit", idB.String(), "--purge")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "--yes") {
		t.Errorf("expected '--yes' in error, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "1 dependent") {
		t.Errorf("expected '1 dependent' in error, got: %v", errMsg)
	}
}

// TestIssueEdit_Purge_NoYes_NoDependents verifies that purging without --yes
// when no dependents exist uses the original error message without a count.
func TestIssueEdit_Purge_NoYes_NoDependents(t *testing.T) {
	app, run := setupPurgeTest(t)

	id := createPurgeTestIssue(t, app, "Standalone issue")

	_, _, err := run("issue", "edit", id.String(), "--purge")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "--yes") {
		t.Errorf("expected '--yes' in error, got: %v", errMsg)
	}
	// Should NOT contain "dependent" since there are no dependents.
	if strings.Contains(errMsg, "dependent") {
		t.Errorf("expected no 'dependent' in error for standalone issue, got: %v", errMsg)
	}
}
