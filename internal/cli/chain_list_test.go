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

	"github.com/perigrin/git-chain/internal/cli"
	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/milestone"
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
	refPath := "refs/chain/_/milestones/" + name
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
	refPath := fmt.Sprintf("refs/chain/_/issues/%s", id.String())
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
