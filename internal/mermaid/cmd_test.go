// ABOUTME: Tests for the Cobra CLI command wiring in the mermaid package.
// ABOUTME: Covers stdin and --file input dispatch for gantt and dag subcommands.
package mermaid

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildJSON encodes a slice of IssueInput to JSON bytes.
func buildJSON(t *testing.T, issues []IssueInput) []byte {
	t.Helper()
	b, err := json.Marshal(issues)
	if err != nil {
		t.Fatalf("marshal issues: %v", err)
	}
	return b
}

// parseDate2 is a test helper to avoid collision with the other test file.
func parseDate2(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// ---------------------------------------------------------------------------
// gantt subcommand tests
// ---------------------------------------------------------------------------

func TestCmdGantt_Stdin(t *testing.T) {
	issues := []IssueInput{
		{
			ID:       "019444a100000000000000000001",
			Title:    "Implement lexer",
			State:    "done",
			Assigned: "dev-a",
			Created:  parseDate2("2026-03-01"),
			Updated:  parseDate2("2026-03-03"),
		},
	}
	data := buildJSON(t, issues)

	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// Provide the JSON via stdin simulation.
	cmd.SetIn(bytes.NewReader(data))
	cmd.SetArgs([]string{"gantt"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("gantt command failed: %v\noutput: %s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "gantt") {
		t.Errorf("expected 'gantt' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Implement lexer") {
		t.Errorf("expected issue title in output, got:\n%s", got)
	}
}

func TestCmdGantt_File(t *testing.T) {
	issues := []IssueInput{
		{
			ID:      "019444a100000000000000000001",
			Title:   "File-based issue",
			State:   "pending",
			Created: parseDate2("2026-03-04"),
			Updated: parseDate2("2026-03-05"),
		},
	}
	data := buildJSON(t, issues)

	// Write to a temp file.
	dir := t.TempDir()
	fpath := filepath.Join(dir, "issues.json")
	if err := os.WriteFile(fpath, data, 0600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gantt", "--file", fpath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("gantt --file command failed: %v\noutput: %s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "File-based issue") {
		t.Errorf("expected issue title in output, got:\n%s", got)
	}
}

func TestCmdGantt_InvalidJSON(t *testing.T) {
	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("not valid json"))
	cmd.SetArgs([]string{"gantt"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid JSON input, got nil")
	}
}

// ---------------------------------------------------------------------------
// dag subcommand tests
// ---------------------------------------------------------------------------

func TestCmdDAG_Stdin(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "Implement lexer",
			State: "done",
		},
		{
			ID:        "019444a200000000000000000002",
			Title:     "Parse declarations",
			State:     "in-progress",
			BlockedBy: []string{"019444a100000000000000000001"},
		},
	}
	data := buildJSON(t, issues)

	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(bytes.NewReader(data))
	cmd.SetArgs([]string{"dag"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dag command failed: %v\noutput: %s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "graph TD") {
		t.Errorf("expected 'graph TD' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "-->") {
		t.Errorf("expected '-->' edge in output, got:\n%s", got)
	}
}

func TestCmdDAG_File(t *testing.T) {
	issues := []IssueInput{
		{
			ID:    "019444a100000000000000000001",
			Title: "From file",
			State: "pending",
		},
	}
	data := buildJSON(t, issues)

	dir := t.TempDir()
	fpath := filepath.Join(dir, "dag.json")
	if err := os.WriteFile(fpath, data, 0600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"dag", "--file", fpath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dag --file command failed: %v\noutput: %s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "From file") {
		t.Errorf("expected issue title in output, got:\n%s", got)
	}
}

func TestCmdDAG_InvalidJSON(t *testing.T) {
	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("{bad json}"))
	cmd.SetArgs([]string{"dag"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid JSON input, got nil")
	}
}

func TestCmdDAG_EmptyInput(t *testing.T) {
	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("[]"))
	cmd.SetArgs([]string{"dag"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dag with empty array failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "graph TD") {
		t.Errorf("expected 'graph TD' for empty input, got:\n%s", got)
	}
}

func TestCmdGantt_EmptyInput(t *testing.T) {
	cmd := NewMermaidCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("[]"))
	cmd.SetArgs([]string{"gantt"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("gantt with empty array failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "gantt") {
		t.Errorf("expected 'gantt' for empty input, got:\n%s", got)
	}
}
