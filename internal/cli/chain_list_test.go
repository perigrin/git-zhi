// ABOUTME: Tests for chain list command: topo-sorted milestone grouping, critical chain
// ABOUTME: display with arrows, and JSON output modes.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// setupChainListTest creates a temporary git repo with initialized chain state
// and returns an App plus a run function.
func setupChainListTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}

	run := func(args ...string) (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, err
	}

	return app, run
}

// createTestMilestone writes a milestone to the store.
func createTestMilestone(t *testing.T, app *cli.App, name string, due *time.Time) {
	t.Helper()
	ms := &milestone.Milestone{
		Name:    name,
		Due:     due,
		Created: time.Now(),
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	refPath := "refs/zhi/_/milestones/" + name
	if err := app.Store.WriteEntity(refPath, "milestone.yaml", data, "Add test milestone: "+name); err != nil {
		t.Fatalf("WriteEntity milestone: %v", err)
	}
}

// createTestIssueWithDeps writes an issue with explicit deps to the store and returns its UUID.
func createTestIssueWithDeps(t *testing.T, app *cli.App, title string, state issue.State, ms string, blockedBy []uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     state,
		Milestone: ms,
		BlockedBy: blockedBy,
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

// TestChainList_Default creates 3 issues with deps in two milestones and verifies
// the output is grouped by milestone and topo-sorted (dependencies first).
func TestChainList_Default(t *testing.T) {
	app, run := setupChainListTest(t)

	// v0.1 milestone already created by EnsureInitialized.
	createTestMilestone(t, app, "v0.2", nil)

	// Create v0.1 issues: A -> B -> C (A blocks B, B blocks C)
	idA := createTestIssueWithDeps(t, app, "Issue A", issue.StatePending, "v0.1", nil)
	idB := createTestIssueWithDeps(t, app, "Issue B", issue.StatePending, "v0.1", []uuid.UUID{idA})
	_ = createTestIssueWithDeps(t, app, "Issue C", issue.StatePending, "v0.1", []uuid.UUID{idB})

	// Create a v0.2 issue.
	createTestIssueWithDeps(t, app, "Issue D", issue.StatePending, "v0.2", nil)

	stdout, err := run("list")
	if err != nil {
		t.Fatalf("chain list failed: %v", err)
	}

	output := stdout.String()

	// Milestone headers should be present.
	if !strings.Contains(output, "v0.1") {
		t.Errorf("expected 'v0.1' milestone header in output, got:\n%s", output)
	}
	if !strings.Contains(output, "v0.2") {
		t.Errorf("expected 'v0.2' milestone header in output, got:\n%s", output)
	}

	// All issues should appear.
	for _, title := range []string{"Issue A", "Issue B", "Issue C", "Issue D"} {
		if !strings.Contains(output, title) {
			t.Errorf("expected %q in output, got:\n%s", title, output)
		}
	}

	// Topo order: A must appear before B, B before C.
	posA := strings.Index(output, "Issue A")
	posB := strings.Index(output, "Issue B")
	posC := strings.Index(output, "Issue C")
	if posA >= posB {
		t.Errorf("expected Issue A before Issue B in topo order, got:\n%s", output)
	}
	if posB >= posC {
		t.Errorf("expected Issue B before Issue C in topo order, got:\n%s", output)
	}
}

// TestChainList_Critical verifies --critical shows the critical chain with arrow separators.
func TestChainList_Critical(t *testing.T) {
	app, run := setupChainListTest(t)

	// Create a linear chain: A -> B -> C
	idA := createTestIssueWithDeps(t, app, "Crit A", issue.StatePending, "v0.1", nil)
	idB := createTestIssueWithDeps(t, app, "Crit B", issue.StatePending, "v0.1", []uuid.UUID{idA})
	_ = createTestIssueWithDeps(t, app, "Crit C", issue.StatePending, "v0.1", []uuid.UUID{idB})

	stdout, err := run("list", "--critical")
	if err != nil {
		t.Fatalf("chain list --critical failed: %v", err)
	}

	output := stdout.String()

	// Header should be present.
	if !strings.Contains(output, "Critical Chain") {
		t.Errorf("expected 'Critical Chain' header in output, got:\n%s", output)
	}

	// All three critical chain issues should appear.
	for _, title := range []string{"Crit A", "Crit B", "Crit C"} {
		if !strings.Contains(output, title) {
			t.Errorf("expected %q in output, got:\n%s", title, output)
		}
	}

	// Arrow separators between issues.
	if !strings.Contains(output, "↓") {
		t.Errorf("expected ↓ arrow between issues in --critical output, got:\n%s", output)
	}

	// Order: A before B, B before C.
	posA := strings.Index(output, "Crit A")
	posB := strings.Index(output, "Crit B")
	posC := strings.Index(output, "Crit C")
	if posA >= posB || posB >= posC {
		t.Errorf("expected critical chain in order A->B->C, got:\n%s", output)
	}
}

// TestChainList_Json verifies --format json returns a JSON object with an issues array.
func TestChainList_Json(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestIssueWithDeps(t, app, "JSON Issue 1", issue.StatePending, "v0.1", nil)
	createTestIssueWithDeps(t, app, "JSON Issue 2", issue.StateInProgress, "v0.1", nil)

	stdout, err := run("list", "--format", "json")
	if err != nil {
		t.Fatalf("chain list --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	issuesVal, ok := result["issues"]
	if !ok {
		t.Fatalf("expected 'issues' key in JSON output, got keys: %v", keys(result))
	}
	issuesList, ok := issuesVal.([]interface{})
	if !ok {
		t.Fatalf("expected 'issues' to be an array, got: %T", issuesVal)
	}
	if len(issuesList) != 2 {
		t.Fatalf("expected 2 issues in JSON output, got %d\noutput: %s", len(issuesList), stdout.String())
	}
}

// TestChainList_CriticalJson verifies --critical --format json returns an object
// with both issues and critical_chain arrays.
func TestChainList_CriticalJson(t *testing.T) {
	app, run := setupChainListTest(t)

	idA := createTestIssueWithDeps(t, app, "CJ A", issue.StatePending, "v0.1", nil)
	_ = createTestIssueWithDeps(t, app, "CJ B", issue.StatePending, "v0.1", []uuid.UUID{idA})

	stdout, err := run("list", "--critical", "--format", "json")
	if err != nil {
		t.Fatalf("chain list --critical --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if _, ok := result["critical_chain"]; !ok {
		t.Fatalf("expected 'critical_chain' key in JSON output when --critical, got keys: %v", keys(result))
	}
}

// TestChainList_CriticalConstraint verifies that --critical output includes the current
// constraint line and parallel work line when parallel issues exist.
func TestChainList_CriticalConstraint(t *testing.T) {
	app, run := setupChainListTest(t)

	// Linear chain: A -> B.
	idA := createTestIssueWithDeps(t, app, "Constraint A", issue.StatePending, "v0.1", nil)
	_ = createTestIssueWithDeps(t, app, "Constraint B", issue.StatePending, "v0.1", []uuid.UUID{idA})

	// Independent issue (parallel work).
	_ = createTestIssueWithDeps(t, app, "Parallel P", issue.StatePending, "v0.1", nil)

	stdout, err := run("list", "--critical")
	if err != nil {
		t.Fatalf("chain list --critical failed: %v", err)
	}

	output := stdout.String()

	if !strings.Contains(output, "Current constraint:") {
		t.Errorf("expected 'Current constraint:' in --critical output, got:\n%s", output)
	}
	if !strings.Contains(output, "Parallel work available:") {
		t.Errorf("expected 'Parallel work available:' in --critical output, got:\n%s", output)
	}
}

// TestChainList_CriticalConstraintJson verifies that --critical --format json includes
// current_constraint and parallel_work fields.
func TestChainList_CriticalConstraintJson(t *testing.T) {
	app, run := setupChainListTest(t)

	idA := createTestIssueWithDeps(t, app, "CJ Constraint A", issue.StatePending, "v0.1", nil)
	_ = createTestIssueWithDeps(t, app, "CJ Constraint B", issue.StatePending, "v0.1", []uuid.UUID{idA})
	_ = createTestIssueWithDeps(t, app, "CJ Parallel", issue.StatePending, "v0.1", nil)

	stdout, err := run("list", "--critical", "--format", "json")
	if err != nil {
		t.Fatalf("chain list --critical --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if _, ok := result["current_constraint"]; !ok {
		t.Fatalf("expected 'current_constraint' key in JSON output, got keys: %v", keys(result))
	}
	if _, ok := result["parallel_work"]; !ok {
		t.Fatalf("expected 'parallel_work' key in JSON output, got keys: %v", keys(result))
	}
}

// TestChainList_GraphFlagNotImplemented verifies --graph returns an error.
func TestChainList_GraphFlagNotImplemented(t *testing.T) {
	_, run := setupChainListTest(t)

	_, err := run("list", "--graph")
	if err == nil {
		t.Fatal("expected error for --graph flag, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected 'not yet implemented' in error, got: %v", err)
	}
}

// TestListReady verifies --ready shows the ready set with path analysis.
func TestListReady(t *testing.T) {
	app, run := setupChainListTest(t)

	// Create two independent pending issues with context paths.
	createTestIssueWithDeps(t, app, "Parse signatures", issue.StatePending, "v0.1", nil)
	createTestIssueWithDeps(t, app, "Config persistence", issue.StatePending, "v0.1", nil)

	stdout, err := run("list", "--ready")
	if err != nil {
		t.Fatalf("list --ready failed: %v", err)
	}

	output := stdout.String()

	// Output must contain "Ready set" header.
	if !strings.Contains(output, "Ready set") {
		t.Errorf("expected 'Ready set' header in --ready output, got:\n%s", output)
	}

	// Both issues must appear.
	if !strings.Contains(output, "Parse signatures") {
		t.Errorf("expected 'Parse signatures' in --ready output, got:\n%s", output)
	}
	if !strings.Contains(output, "Config persistence") {
		t.Errorf("expected 'Config persistence' in --ready output, got:\n%s", output)
	}
}

// TestListReady_DoneBlockerReleases verifies that a pending issue whose
// blocker is done appears in the ready set.
func TestListReady_DoneBlockerReleases(t *testing.T) {
	app, run := setupChainListTest(t)

	idA := createTestIssueWithDeps(t, app, "Blocker", issue.StateDone, "v0.1", nil)
	createTestIssueWithDeps(t, app, "Unblocked", issue.StatePending, "v0.1", []uuid.UUID{idA})

	stdout, err := run("list", "--ready")
	if err != nil {
		t.Fatalf("list --ready failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Unblocked") {
		t.Errorf("expected 'Unblocked' in --ready output when blocker is done, got:\n%s", output)
	}
	// The done blocker itself must not appear in ready set.
	if strings.Contains(output, "Blocker") {
		t.Errorf("expected done issue 'Blocker' to NOT appear in --ready output, got:\n%s", output)
	}
}

// TestListReady_PathOverlap verifies --ready reports overlap when issues
// share a directory.
func TestListReady_PathOverlap(t *testing.T) {
	app, run := setupChainListTest(t)

	// Both issues declare paths inside internal/parser — same directory.
	bodyA := "## Context\n- paths: internal/parser/signature.go\n"
	bodyB := "## Context\n- paths: internal/parser/error.go\n"
	createTestIssueWithBody(t, app, "Parse sigs", issue.StatePending, "v0.1", bodyA)
	createTestIssueWithBody(t, app, "Parse errors", issue.StatePending, "v0.1", bodyB)

	stdout, err := run("list", "--ready")
	if err != nil {
		t.Fatalf("list --ready failed: %v", err)
	}

	output := stdout.String()
	// Should report overlap warning.
	if !strings.Contains(output, "overlap") && !strings.Contains(output, "Overlap") {
		t.Errorf("expected overlap mention in --ready output for issues sharing a directory, got:\n%s", output)
	}
}

// TestListReady_Json verifies --ready --format json returns structured ready set.
func TestListReady_Json(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestIssueWithDeps(t, app, "Ready JSON A", issue.StatePending, "v0.1", nil)
	createTestIssueWithDeps(t, app, "Ready JSON B", issue.StatePending, "v0.1", nil)

	stdout, err := run("list", "--ready", "--format", "json")
	if err != nil {
		t.Fatalf("list --ready --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if _, ok := result["ready_set"]; !ok {
		t.Fatalf("expected 'ready_set' key in JSON output, got keys: %v", keys(result))
	}
	if _, ok := result["overlap_detected"]; !ok {
		t.Fatalf("expected 'overlap_detected' key in JSON output, got keys: %v", keys(result))
	}
}

// createTestIssueWithBody creates an issue with a specific body in the store.
func createTestIssueWithBody(t *testing.T, app *cli.App, title string, state issue.State, ms string, body string) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     state,
		Milestone: ms,
		Body:      body,
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
