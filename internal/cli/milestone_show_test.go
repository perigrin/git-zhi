// ABOUTME: Tests for the milestone show command: human-readable detail view
// ABOUTME: with issue list, progress, telemetry signals, and JSON output.
package cli_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// createTestIssueWithSessionsAndMilestone writes an issue with explicit sessions
// to the store and returns its UUID.
func createTestIssueWithSessionsAndMilestone(t *testing.T, app *cli.App, title string, state issue.State, ms string, sessions []issue.Session) uuid.UUID {
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
		Sessions:  sessions,
		Created:   now,
		Updated:   now,
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
	// Telemetry should be present in JSON output.
	if _, ok := result["telemetry"]; !ok {
		t.Errorf("expected 'telemetry' key in JSON output, got keys: %v", keys(result))
	}
}

// TestMilestoneShow_DefaultsToHeadIssueMilestone verifies that when no
// milestone name is provided, milestone show resolves to the milestone of
// the current HEAD issue rather than scanning for the first with pending issues.
func TestMilestoneShow_DefaultsToHeadIssueMilestone(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create a second milestone.
	if _, err := run("milestone", "add", "v0.2"); err != nil {
		t.Fatalf("milestone add v0.2 failed: %v", err)
	}

	// Add a pending issue in v0.1 (earlier, would win by scan-order).
	createTestIssueWithMilestone(t, app, "Old milestone work", issue.StatePending, "v0.1", "")

	// Add an in-progress issue in v0.2 (HEAD resolves here).
	createTestIssueWithMilestone(t, app, "Current work", issue.StateInProgress, "v0.2", "")

	// No arg — should resolve to v0.2 (HEAD issue's milestone), not v0.1.
	stdout, err := run("milestone", "show")
	if err != nil {
		t.Fatalf("milestone show (no arg) failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.2") {
		t.Errorf("expected 'v0.2' (HEAD issue's milestone) in output, got:\n%s", output)
	}
}

func TestMilestoneShow_TimeInChain(t *testing.T) {
	// Verify that when sessions have timestamps, the human output includes
	// Time-in-chain and Shadow work lines.
	app, run := setupMilestoneTest(t)

	// Build a done issue with a timestamped session directly in the store.
	sessStart := time.Now().Add(-2 * time.Hour)
	sessEnd := time.Now().Add(-1 * time.Hour)
	id := createTestIssueWithSessionsAndMilestone(t, app, "Timed work", issue.StateDone, "v0.1",
		[]issue.Session{{
			StartSHA:  "aaa",
			EndSHA:    "bbb",
			Commits:   3,
			StartedAt: &sessStart,
			EndedAt:   &sessEnd,
		}},
	)
	_ = id

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show v0.1 failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Time-in-chain:") {
		t.Errorf("expected 'Time-in-chain:' in human output when sessions have timestamps, got:\n%s", output)
	}
	if !strings.Contains(output, "Shadow work:") {
		t.Errorf("expected 'Shadow work:' in human output when sessions have timestamps, got:\n%s", output)
	}
}

func TestMilestoneShow_NotFound(t *testing.T) {
	_, run := setupMilestoneTest(t)

	_, err := run("milestone", "show", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone, got nil")
	}
}

func TestMilestoneShow_HumanTelemetry(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// A done issue with sessions so telemetry has non-zero values.
	createTestIssueWithMilestone(t, app, "Completed work", issue.StateDone, "v0.1", "")
	createTestIssueWithMilestone(t, app, "Pending work", issue.StatePending, "v0.1", "")

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show v0.1 failed: %v", err)
	}

	output := stdout.String()
	// Human output should include a Fever section.
	if !strings.Contains(output, "Fever:") {
		t.Errorf("expected 'Fever:' in human output, got:\n%s", output)
	}
}
