// ABOUTME: Tests for the issue list command: default filter, --all, --milestone,
// ABOUTME: --state, and JSON output modes.
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
)

// createTestIssueWithMilestone writes an issue to the store with a specific milestone
// and returns its UUID.
func createTestIssueWithMilestone(t *testing.T, app *cli.App, title string, state issue.State, milestone string, body string) uuid.UUID {
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
		Milestone: milestone,
		Created:   now,
		Updated:   now,
		Body:      body,
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

// setupListTest creates a temporary git repo and returns an App plus a run function.
// The run function executes the root command with the given args using the injected App.
func setupListTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
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

// TestIssueList_Default creates 3 issues (pending, in-progress, done) and verifies
// that the default output includes only pending and in-progress, not done.
func TestIssueList_Default(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssue(t, app, "Alpha", issue.StatePending, "")
	createTestIssue(t, app, "Beta", issue.StateInProgress, "")
	createTestIssue(t, app, "Gamma", issue.StateDone, "")

	stdout, err := run("issue", "list")
	if err != nil {
		t.Fatalf("issue list failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Alpha") {
		t.Errorf("expected 'Alpha' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Beta") {
		t.Errorf("expected 'Beta' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Gamma") {
		t.Errorf("expected 'Gamma' to be excluded from default output, got:\n%s", output)
	}
}

// TestIssueList_All verifies that --all includes done and cancelled issues.
func TestIssueList_All(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssue(t, app, "Alpha", issue.StatePending, "")
	createTestIssue(t, app, "Beta", issue.StateInProgress, "")
	createTestIssue(t, app, "Gamma", issue.StateDone, "")

	stdout, err := run("issue", "list", "--all")
	if err != nil {
		t.Fatalf("issue list --all failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Alpha") {
		t.Errorf("expected 'Alpha' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Beta") {
		t.Errorf("expected 'Beta' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Gamma") {
		t.Errorf("expected 'Gamma' in output with --all, got:\n%s", output)
	}
}

// TestIssueList_FilterMilestone creates issues in two milestones and verifies
// that --milestone filters to only the requested milestone.
func TestIssueList_FilterMilestone(t *testing.T) {
	app, run := setupListTest(t)

	// createTestIssue uses "v0.1" as the default milestone
	createTestIssue(t, app, "In v0.1", issue.StatePending, "")

	// Create a v0.2 issue directly
	id2 := createTestIssueWithMilestone(t, app, "In v0.2", issue.StatePending, "v0.2", "")

	stdout, err := run("issue", "list", "--milestone", "v0.1")
	if err != nil {
		t.Fatalf("issue list --milestone v0.1 failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "In v0.1") {
		t.Errorf("expected 'In v0.1' in output, got:\n%s", output)
	}
	if strings.Contains(output, "In v0.2") {
		t.Errorf("expected 'In v0.2' to be excluded, got:\n%s", output)
	}
	_ = id2
}

// TestIssueList_FilterState creates pending and in-progress issues and verifies
// that --state pending shows only pending issues.
func TestIssueList_FilterState(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssue(t, app, "PendingTask", issue.StatePending, "")
	createTestIssue(t, app, "ActiveTask", issue.StateInProgress, "")

	stdout, err := run("issue", "list", "--state", "pending")
	if err != nil {
		t.Fatalf("issue list --state pending failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PendingTask") {
		t.Errorf("expected 'PendingTask' in output, got:\n%s", output)
	}
	if strings.Contains(output, "ActiveTask") {
		t.Errorf("expected 'ActiveTask' to be excluded with --state pending, got:\n%s", output)
	}
}

// TestIssueList_JsonOutput creates 2 issues and verifies --format json returns
// a valid JSON array with 2 elements.
func TestIssueList_JsonOutput(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssue(t, app, "First Issue", issue.StatePending, "")
	createTestIssue(t, app, "Second Issue", issue.StateInProgress, "")

	stdout, err := run("issue", "list", "--format", "json")
	if err != nil {
		t.Fatalf("issue list --format json failed: %v", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 issues in JSON array, got %d\noutput: %s", len(result), stdout.String())
	}
}
