// ABOUTME: End-to-end tests for the issue add command: single issue, batch
// ABOUTME: creation, JSON output, lazy init, and default milestone.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

func setupIssueAddTest(t *testing.T, stdinContent string) (*bytes.Buffer, *bytes.Buffer, *cli.App, func(args ...string) error) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	// App is injected into the command context directly, so PersistentPreRunE
	// skips repo detection. No os.Chdir needed.
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	run := func(args ...string) error {
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetIn(strings.NewReader(stdinContent))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		return cmd.Execute()
	}

	return stdout, stderr, app, run
}

func TestIssueAdd_SingleIssue(t *testing.T) {
	input := "---\ntitle: \"Fix the bug\"\n---\n\nBug description here.\n"
	stdout, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Created") {
		t.Fatalf("expected 'Created' in output, got: %s", output)
	}
	if !strings.Contains(output, "Fix the bug") {
		t.Fatalf("expected title in output, got: %s", output)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 issue ref, got %d", len(refs))
	}
}

func TestIssueAdd_BatchIssues(t *testing.T) {
	input := "---\ntitle: \"First task\"\n---\n\nFirst body\n\n---\ntitle: \"Second task\"\n---\n\nSecond body\n"
	stdout, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "2 issues") {
		t.Fatalf("expected '2 issues' in output, got: %s", output)
	}
	// Output should NOT say "chained sequentially"
	if strings.Contains(output, "chained sequentially") {
		t.Fatalf("output should not say 'chained sequentially', got: %s", output)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 issue refs, got %d", len(refs))
	}

	// Batch-add defaults to parallel: both issues should be independent.
	for _, ref := range refs {
		content, readErr := app.Store.ReadEntity(ref, "issue.md")
		if readErr != nil {
			t.Fatalf("ReadEntity failed: %v", readErr)
		}
		iss, parseErr := issue.Parse(content)
		if parseErr != nil {
			t.Fatalf("Parse failed: %v", parseErr)
		}
		if len(iss.BlockedBy) != 0 {
			t.Fatalf("issue %q should have no BlockedBy, got %d", iss.Title, len(iss.BlockedBy))
		}
		if len(iss.Blocks) != 0 {
			t.Fatalf("issue %q should have no Blocks, got %d", iss.Title, len(iss.Blocks))
		}
	}
}

func TestIssueAdd_JsonOutput(t *testing.T) {
	input := "---\ntitle: \"JSON test\"\n---\n\nBody.\n"
	stdout, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add", "--format", "json"); err != nil {
		t.Fatalf("issue add --format json failed: %v", err)
	}
	var result []issue.Issue
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 issue in JSON, got %d", len(result))
	}
	if result[0].Title != "JSON test" {
		t.Fatalf("expected title %q, got %q", "JSON test", result[0].Title)
	}
}

func TestIssueAdd_LazyInit(t *testing.T) {
	input := "---\ntitle: \"First ever issue\"\n---\n"
	_, _, app, run := setupIssueAddTest(t, input)

	// Do NOT call EnsureInitialized — let issue add trigger it
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	if !app.Store.RefExists("refs/zhi/_/config") {
		t.Fatal("expected config ref after lazy init")
	}
	if !app.Store.RefExists("refs/zhi/_/milestones/v0.1") {
		t.Fatal("expected default milestone ref after lazy init")
	}
}

func TestIssueAdd_BatchParallelDefault(t *testing.T) {
	input := "---\ntitle: \"Task A\"\n---\n\nBody A\n\n---\ntitle: \"Task B\"\n---\n\nBody B\n\n---\ntitle: \"Task C\"\n---\n\nBody C\n"
	_, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 issue refs, got %d", len(refs))
	}
	// All issues should be independent — no blocked_by or blocks
	for _, ref := range refs {
		content, readErr := app.Store.ReadEntity(ref, "issue.md")
		if readErr != nil {
			t.Fatalf("ReadEntity failed: %v", readErr)
		}
		iss, parseErr := issue.Parse(content)
		if parseErr != nil {
			t.Fatalf("Parse failed: %v", parseErr)
		}
		if len(iss.BlockedBy) != 0 {
			t.Fatalf("issue %q should have no BlockedBy, got %d", iss.Title, len(iss.BlockedBy))
		}
		if len(iss.Blocks) != 0 {
			t.Fatalf("issue %q should have no Blocks, got %d", iss.Title, len(iss.Blocks))
		}
	}
}

func TestIssueAdd_BatchWithAfter(t *testing.T) {
	input := "---\ntitle: \"First task\"\n---\n\nFirst body\n\n---\ntitle: \"Second task\"\nafter: \"First task\"\n---\n\nSecond body\n"
	stdout, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	// Verify output shows the dependency annotation
	output := stdout.String()
	if !strings.Contains(output, "(after: First task)") {
		t.Fatalf("expected '(after: First task)' in output, got: %s", output)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	var first, second *issue.Issue
	for _, ref := range refs {
		content, readErr := app.Store.ReadEntity(ref, "issue.md")
		if readErr != nil {
			t.Fatalf("ReadEntity failed: %v", readErr)
		}
		iss, parseErr := issue.Parse(content)
		if parseErr != nil {
			t.Fatalf("Parse failed: %v", parseErr)
		}
		switch iss.Title {
		case "First task":
			first = iss
		case "Second task":
			second = iss
		}
	}
	if first == nil || second == nil {
		t.Fatal("could not find both issues by title")
	}
	if len(first.Blocks) != 1 {
		t.Fatalf("expected First to block 1 issue, got %d", len(first.Blocks))
	}
	if len(second.BlockedBy) != 1 {
		t.Fatalf("expected Second to be blocked by 1 issue, got %d", len(second.BlockedBy))
	}
	if len(first.BlockedBy) != 0 {
		t.Fatalf("expected First to have no BlockedBy, got %d", len(first.BlockedBy))
	}
	if len(second.Blocks) != 0 {
		t.Fatalf("expected Second to have no Blocks, got %d", len(second.Blocks))
	}
}

func TestIssueAdd_BatchWithBefore(t *testing.T) {
	input := "---\ntitle: \"First task\"\nbefore: \"Second task\"\n---\n\nFirst body\n\n---\ntitle: \"Second task\"\n---\n\nSecond body\n"
	_, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	var first, second *issue.Issue
	for _, ref := range refs {
		content, readErr := app.Store.ReadEntity(ref, "issue.md")
		if readErr != nil {
			t.Fatalf("ReadEntity failed: %v", readErr)
		}
		iss, parseErr := issue.Parse(content)
		if parseErr != nil {
			t.Fatalf("Parse failed: %v", parseErr)
		}
		switch iss.Title {
		case "First task":
			first = iss
		case "Second task":
			second = iss
		}
	}
	if first == nil || second == nil {
		t.Fatal("could not find both issues by title")
	}
	// before means First blocks Second
	if len(first.Blocks) != 1 {
		t.Fatalf("expected First to block 1 issue, got %d", len(first.Blocks))
	}
	if len(second.BlockedBy) != 1 {
		t.Fatalf("expected Second to be blocked by 1 issue, got %d", len(second.BlockedBy))
	}
}

func TestIssueAdd_BatchMixed(t *testing.T) {
	input := "---\ntitle: \"Independent\"\n---\n\nNo deps\n\n---\ntitle: \"Depends on independent\"\nafter: \"Independent\"\n---\n\nHas dep\n\n---\ntitle: \"Also independent\"\n---\n\nNo deps either\n"
	_, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}
	issues := make(map[string]*issue.Issue)
	for _, ref := range refs {
		content, _ := app.Store.ReadEntity(ref, "issue.md")
		iss, _ := issue.Parse(content)
		issues[iss.Title] = iss
	}
	// "Independent" blocks 1, is not blocked
	if len(issues["Independent"].Blocks) != 1 {
		t.Fatalf("expected Independent to block 1, got %d", len(issues["Independent"].Blocks))
	}
	if len(issues["Independent"].BlockedBy) != 0 {
		t.Fatalf("expected Independent to have no BlockedBy, got %d", len(issues["Independent"].BlockedBy))
	}
	// "Depends on independent" is blocked by 1, blocks none
	if len(issues["Depends on independent"].BlockedBy) != 1 {
		t.Fatalf("expected 'Depends on independent' to be blocked by 1, got %d", len(issues["Depends on independent"].BlockedBy))
	}
	// "Also independent" has no deps
	if len(issues["Also independent"].Blocks) != 0 || len(issues["Also independent"].BlockedBy) != 0 {
		t.Fatal("expected 'Also independent' to have no deps")
	}
}

func TestIssueAdd_AfterNotFound(t *testing.T) {
	input := "---\ntitle: \"Orphan\"\nafter: \"Nonexistent task\"\n---\n\nBody\n"
	_, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	err := run("issue", "add")
	if err == nil {
		t.Fatal("expected error for after referencing nonexistent issue")
	}
	if !strings.Contains(err.Error(), "Nonexistent task") {
		t.Fatalf("expected error to mention the missing title, got: %v", err)
	}
}

func TestIssueAdd_DefaultMilestone(t *testing.T) {
	input := "---\ntitle: \"No milestone specified\"\n---\n"
	_, _, app, run := setupIssueAddTest(t, input)

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := run("issue", "add"); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}
	refs, err := app.Store.ListRefs("refs/zhi/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	content, err := app.Store.ReadEntity(refs[0], "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity failed: %v", err)
	}
	iss, err := issue.Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if iss.Milestone != "v0.1" {
		t.Fatalf("expected milestone 'v0.1', got %q", iss.Milestone)
	}
}
