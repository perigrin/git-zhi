// ABOUTME: Tests for the git-zhi-project CLI command tree.
// ABOUTME: Exercises 'project show' and 'project next' subcommands with real repos.
package project_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/project"
)

// writeProjectFile serializes a ProjectDef to a temp YAML file and returns its path.
func writeProjectFile(t *testing.T, def *project.ProjectDef) string {
	t.Helper()
	data, err := yaml.Marshal(def)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	tmpFile := filepath.Join(t.TempDir(), "project.yaml")
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return tmpFile
}

// executeCommand runs a Cobra command and returns stdout, stderr strings.
func executeCommand(t *testing.T, cmd interface{ Execute() error }, args ...string) (string, string, error) {
	t.Helper()
	// We need to call SetArgs and capture output — use the project.NewProjectCommand API.
	return "", "", nil // placeholder — see actual test below
}

// ---------------------------------------------------------------------------
// show subcommand tests
// ---------------------------------------------------------------------------

func TestProjectShowCommand_HumanOutput(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	writeIssue(t, appA, &issue.Issue{
		Title: "Done task", State: issue.StateDone, Milestone: "v1.0",
		Created: now, Updated: now,
	})
	writeIssue(t, appA, &issue.Issue{
		Title: "Pending task", State: issue.StatePending, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:  "Show Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
	}
	projectFile := writeProjectFile(t, def)

	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"show", projectFile})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Show Test") {
		t.Errorf("expected project name in output, got:\n%s", output)
	}
	if !strings.Contains(output, "v1.0") {
		t.Errorf("expected milestone name in output, got:\n%s", output)
	}
}

func TestProjectShowCommand_JSONOutput(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	writeIssue(t, appA, &issue.Issue{
		Title: "Done task", State: issue.StateDone, Milestone: "v1.0",
		Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:  "JSON Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
	}
	projectFile := writeProjectFile(t, def)

	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"show", projectFile, "--format", "json"})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var status project.ProjectStatus
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("json.Unmarshal: %v\noutput was:\n%s", err, out.String())
	}
	if status.Name != "JSON Test" {
		t.Errorf("expected Name=%q, got %q", "JSON Test", status.Name)
	}
}

func TestProjectShowCommand_MissingFile(t *testing.T) {
	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"show", "/nonexistent/file.yaml"})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ---------------------------------------------------------------------------
// next subcommand tests
// ---------------------------------------------------------------------------

func TestProjectNextCommand_ReturnsIssue(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	writeIssue(t, appA, &issue.Issue{
		Title: "Next task", State: issue.StatePending, Milestone: "v1.0",
		Assigned: "alice", Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:  "Next Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA}},
		},
	}
	projectFile := writeProjectFile(t, def)

	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"next", "--actor", "alice", projectFile})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Next task") {
		t.Errorf("expected issue title in output, got:\n%s", output)
	}
}

func TestProjectNextCommand_MissingActor(t *testing.T) {
	dirA, _ := makeTestRepo(t)

	def := &project.ProjectDef{
		Name:  "Actor Flag Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
	}
	projectFile := writeProjectFile(t, def)

	cmd := project.NewProjectCommand()
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"next", projectFile})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --actor is missing, got nil")
	}
}

func TestProjectNextCommand_JSONOutput(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	msA := &milestone.Milestone{Name: "v1.0", Created: now}
	writeMilestone(t, appA, msA)
	writeIssue(t, appA, &issue.Issue{
		Title: "JSON next", State: issue.StatePending, Milestone: "v1.0",
		Assigned: "alice", Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:  "JSON Next Test",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA}},
		},
	}
	projectFile := writeProjectFile(t, def)

	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"next", "--actor", "alice", "--format", "json", projectFile})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v\noutput was:\n%s", err, out.String())
	}
	if _, ok := result["repo"]; !ok {
		t.Error("expected 'repo' field in JSON output")
	}
	if _, ok := result["issue_id"]; !ok {
		t.Error("expected 'issue_id' field in JSON output")
	}
	if _, ok := result["title"]; !ok {
		t.Error("expected 'title' field in JSON output")
	}
}
