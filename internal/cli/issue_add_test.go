// ABOUTME: End-to-end tests for the issue add command: single issue, batch
// ABOUTME: creation, JSON output, lazy init, and default milestone.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-chain/internal/cli"
	"github.com/perigrin/git-chain/internal/issue"
)

func setupIssueAddTest(t *testing.T, stdinContent string) (*bytes.Buffer, *bytes.Buffer, *cli.App, func(args ...string) error) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

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
	refs, err := app.Store.ListRefs("refs/chain/_/issues/")
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
	refs, err := app.Store.ListRefs("refs/chain/_/issues/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 issue refs, got %d", len(refs))
	}

	// Verify sequential dependencies were wired
	content0, err := app.Store.ReadEntity(refs[0], "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity[0] failed: %v", err)
	}
	content1, err := app.Store.ReadEntity(refs[1], "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity[1] failed: %v", err)
	}
	iss0, _ := issue.Parse(content0)
	iss1, _ := issue.Parse(content1)
	hasDepLink := (len(iss0.Blocks) == 1 && len(iss1.BlockedBy) == 1) ||
		(len(iss1.Blocks) == 1 && len(iss0.BlockedBy) == 1)
	if !hasDepLink {
		t.Fatalf("expected sequential dependency wiring between batch issues, got blocks=%v/%v blockedBy=%v/%v",
			iss0.Blocks, iss1.Blocks, iss0.BlockedBy, iss1.BlockedBy)
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
	if !app.Store.RefExists("refs/chain/_/config") {
		t.Fatal("expected config ref after lazy init")
	}
	if !app.Store.RefExists("refs/chain/_/milestones/v0.1") {
		t.Fatal("expected default milestone ref after lazy init")
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
	refs, err := app.Store.ListRefs("refs/chain/_/issues/")
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
