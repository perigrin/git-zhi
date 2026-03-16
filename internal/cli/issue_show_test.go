// ABOUTME: Tests for the issue show command: UUID prefix resolution, HEAD resolution,
// ABOUTME: JSON output with parsed sections, and error on unknown ref.
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
)

// setupShowTest creates a temporary git repo and returns an App plus a run function.
// The run function executes the root command with the given args using the injected App.
func setupShowTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
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
		stderr := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, err
	}

	return app, run
}

// createTestIssue writes an issue to the store and returns its UUID.
func createTestIssue(t *testing.T, app *cli.App, title string, state issue.State, body string) uuid.UUID {
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

func TestIssueShow_ByUUIDPrefix(t *testing.T) {
	app, run := setupShowTest(t)

	id := createTestIssue(t, app, "Fix the parser", issue.StatePending, "Parser is broken.")

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", prefix)
	if err != nil {
		t.Fatalf("issue show failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Fix the parser") {
		t.Fatalf("expected title in output, got: %s", output)
	}
	if !strings.Contains(output, string(issue.StatePending)) {
		t.Fatalf("expected state 'pending' in output, got: %s", output)
	}
}

func TestIssueShow_HEAD(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "First pending issue", issue.StatePending, "Do something.")

	// No args — resolves HEAD (first pending issue)
	stdout, err := run("issue", "show")
	if err != nil {
		t.Fatalf("issue show (HEAD) failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "First pending issue") {
		t.Fatalf("expected HEAD issue title in output, got: %s", output)
	}
}

func TestIssueShow_JsonOutput(t *testing.T) {
	app, run := setupShowTest(t)

	body := `## Prerequisites
- [ ] Review the spec
- [x] Set up environment

## Context
- paths: internal/issue, internal/cli
- docs: README.md

## Acceptance Criteria
- [ ] All tests pass
- [ ] Code reviewed
`
	id := createTestIssue(t, app, "Implement feature X", issue.StatePending, body)

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if result["title"] != "Implement feature X" {
		t.Fatalf("expected title %q, got %v", "Implement feature X", result["title"])
	}

	// Verify structured sections are present
	prereqs, ok := result["prerequisites"]
	if !ok {
		t.Fatalf("expected 'prerequisites' key in JSON, got keys: %v", keys(result))
	}
	prereqList, ok := prereqs.([]interface{})
	if !ok || len(prereqList) == 0 {
		t.Fatalf("expected non-empty prerequisites list, got: %v", prereqs)
	}

	ac, ok := result["acceptance_criteria"]
	if !ok {
		t.Fatalf("expected 'acceptance_criteria' key in JSON, got keys: %v", keys(result))
	}
	acList, ok := ac.([]interface{})
	if !ok || len(acList) == 0 {
		t.Fatalf("expected non-empty acceptance_criteria list, got: %v", ac)
	}

	// Verify context paths parsed
	ctx, ok := result["context"]
	if !ok {
		t.Fatalf("expected 'context' key in JSON, got keys: %v", keys(result))
	}
	ctxMap, ok := ctx.(map[string]interface{})
	if !ok {
		t.Fatalf("expected context to be an object, got: %v", ctx)
	}
	paths, ok := ctxMap["paths"]
	if !ok {
		t.Fatalf("expected 'paths' in context, got: %v", ctxMap)
	}
	pathList, ok := paths.([]interface{})
	if !ok || len(pathList) == 0 {
		t.Fatalf("expected non-empty paths list, got: %v", paths)
	}

	// Verify raw body is also included
	if _, ok := result["body"]; !ok {
		t.Fatalf("expected 'body' key in JSON output, got keys: %v", keys(result))
	}
}

func TestIssueShow_NotFound(t *testing.T) {
	_, run := setupShowTest(t)

	_, err := run("issue", "show", "deadbeef")
	if err == nil {
		t.Fatal("expected error for nonexistent ref, got nil")
	}
}

// keys returns the keys of a map for use in error messages.
func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
