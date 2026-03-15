// ABOUTME: Tests for the milestone list command: human-readable table output
// ABOUTME: with issue counts, current marker, and JSON array output.
package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestMilestoneList(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// EnsureInitialized creates v0.1; add v0.2.
	if _, err := run("milestone", "add", "v0.2"); err != nil {
		t.Fatalf("add v0.2: %v", err)
	}

	// Create issues in v0.1.
	createTestIssueWithMilestone(t, app, "Issue A", issue.StatePending, "v0.1", "")
	createTestIssueWithMilestone(t, app, "Issue B", issue.StateDone, "v0.1", "")

	stdout, err := run("milestone", "list")
	if err != nil {
		t.Fatalf("milestone list failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.1") {
		t.Errorf("expected 'v0.1' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "v0.2") {
		t.Errorf("expected 'v0.2' in output, got:\n%s", output)
	}
	// v0.1 has pending issues so it should be marked as current.
	if !strings.Contains(output, "current") {
		t.Errorf("expected 'current' marker in output, got:\n%s", output)
	}
}

func TestMilestoneList_IssueCounts(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createTestIssueWithMilestone(t, app, "Issue 1", issue.StatePending, "v0.1", "")
	createTestIssueWithMilestone(t, app, "Issue 2", issue.StateDone, "v0.1", "")
	createTestIssueWithMilestone(t, app, "Issue 3", issue.StatePending, "v0.1", "")

	stdout, err := run("milestone", "list")
	if err != nil {
		t.Fatalf("milestone list failed: %v", err)
	}

	output := stdout.String()
	// Should show done/total counts — 1 done out of 3 total.
	if !strings.Contains(output, "1/3") {
		t.Errorf("expected '1/3' issue count in output, got:\n%s", output)
	}
}

func TestMilestoneList_Json(t *testing.T) {
	_, run := setupMilestoneTest(t)

	if _, err := run("milestone", "add", "v0.2"); err != nil {
		t.Fatalf("add v0.2: %v", err)
	}

	stdout, err := run("milestone", "list", "--format", "json")
	if err != nil {
		t.Fatalf("milestone list --format json failed: %v", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout.String())
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 milestones in JSON array, got %d", len(result))
	}
}
