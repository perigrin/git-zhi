// ABOUTME: Tests for the milestone add command: create, duplicate detection,
// ABOUTME: and optional --due date flag.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// setupMilestoneTest creates a temporary git repo with chain initialized and
// returns an App plus a run function that executes the root command.
func setupMilestoneTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
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

func TestMilestoneAdd(t *testing.T) {
	app, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "add", "v0.2")
	if err != nil {
		t.Fatalf("milestone add v0.2 failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.2") {
		t.Errorf("expected 'v0.2' in output, got: %s", output)
	}

	// Verify the ref was created.
	ms, err := milestone.LoadMilestone(app.Store, "v0.2")
	if err != nil {
		t.Fatalf("LoadMilestone after add: %v", err)
	}
	if ms.Name != "v0.2" {
		t.Errorf("milestone Name: got %q, want %q", ms.Name, "v0.2")
	}
}

func TestMilestoneAdd_Duplicate(t *testing.T) {
	_, run := setupMilestoneTest(t)

	// v0.1 is created by EnsureInitialized; adding it again should error.
	_, err := run("milestone", "add", "v0.1")
	if err == nil {
		t.Fatal("expected error when adding duplicate milestone, got nil")
	}
}

func TestMilestoneAdd_WithDue(t *testing.T) {
	app, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "add", "v0.3", "--due", "2026-04-15")
	if err != nil {
		t.Fatalf("milestone add with --due failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.3") {
		t.Errorf("expected 'v0.3' in output, got: %s", output)
	}
	if !strings.Contains(output, "Apr 15") {
		t.Errorf("expected due date in output, got: %s", output)
	}

	ms, err := milestone.LoadMilestone(app.Store, "v0.3")
	if err != nil {
		t.Fatalf("LoadMilestone after add with due: %v", err)
	}
	if ms.Due == nil {
		t.Fatal("expected Due to be set after add --due")
	}
}

func TestMilestoneAdd_Json(t *testing.T) {
	_, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "add", "--format", "json", "v0.4")
	if err != nil {
		t.Fatalf("milestone add --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout.String())
	}
	if result["name"] != "v0.4" {
		t.Errorf("expected name 'v0.4' in JSON, got: %v", result["name"])
	}
}
