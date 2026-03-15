// ABOUTME: Tests for the milestone show command: human-readable detail view
// ABOUTME: with issue list and progress, and JSON output.
package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestMilestoneShow(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")
	createTestIssueWithMilestone(t, app, "Test suite", issue.StateDone, "v0.1", "")

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show v0.1 failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.1") {
		t.Errorf("expected 'v0.1' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Parser feature") {
		t.Errorf("expected issue title in output, got:\n%s", output)
	}
}

func TestMilestoneShow_DefaultsToCurrent(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Add a pending issue in v0.1 so it becomes the "current" milestone.
	createTestIssueWithMilestone(t, app, "Active work", issue.StatePending, "v0.1", "")

	// No arg — should default to current milestone (v0.1).
	stdout, err := run("milestone", "show")
	if err != nil {
		t.Fatalf("milestone show (no arg) failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.1") {
		t.Errorf("expected 'v0.1' in default show output, got:\n%s", output)
	}
}

func TestMilestoneShow_Json(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createTestIssueWithMilestone(t, app, "Issue Alpha", issue.StatePending, "v0.1", "")

	stdout, err := run("milestone", "show", "--format", "json", "v0.1")
	if err != nil {
		t.Fatalf("milestone show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout.String())
	}
	if result["name"] != "v0.1" {
		t.Errorf("expected name 'v0.1' in JSON, got: %v", result["name"])
	}
	issues, ok := result["issues"]
	if !ok {
		t.Fatalf("expected 'issues' key in JSON, got keys: %v", keys(result))
	}
	issueList, ok := issues.([]interface{})
	if !ok || len(issueList) == 0 {
		t.Fatalf("expected non-empty issues array, got: %v", issues)
	}
}

func TestMilestoneShow_NotFound(t *testing.T) {
	_, run := setupMilestoneTest(t)

	_, err := run("milestone", "show", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone, got nil")
	}
}
