// ABOUTME: Tests for assignment CLI flags: issue edit --assign/--unassign,
// ABOUTME: issue list --assigned, milestone show capacity, and next --actor assignment preference.
package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// createTestIssueWithAssignment writes an issue with a specific assignment to the store.
func createTestIssueWithAssignment(t *testing.T, app *cli.App, title string, state issue.State, ms, assigned string) uuid.UUID {
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
		Assigned:  assigned,
		Created:   now,
		Updated:   now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := issue.RefPrefix + id.String()
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}

// createTestIssueWithAssignmentAndTransitions writes an issue with assignment and
// transitions to the store, used for testing next --actor preference.
func createTestIssueWithAssignmentAndTransitions(t *testing.T, app *cli.App, title string, state issue.State, assigned string, transitions []issue.Transition) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:          id,
		Title:       title,
		State:       state,
		Milestone:   "v0.1",
		Assigned:    assigned,
		Transitions: transitions,
		Created:     now,
		Updated:     now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := issue.RefPrefix + id.String()
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}

// TestIssueEdit_Assign verifies that --assign sets the Assigned field.
func TestIssueEdit_Assign(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Issue to assign")

	_, _, err := run("issue", "edit", id, "--assign", "agent:claude-code-1")
	if err != nil {
		t.Fatalf("issue edit --assign failed: %v", err)
	}

	ref := issue.RefPrefix + id
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if iss.Assigned != "agent:claude-code-1" {
		t.Fatalf("expected Assigned to be 'agent:claude-code-1', got: %q", iss.Assigned)
	}
}

// TestIssueEdit_Assign_Overwrite verifies that --assign can reassign to a different worker.
func TestIssueEdit_Assign_Overwrite(t *testing.T) {
	app, run := setupEditTest(t)

	id := createTestIssueWithAssignment(t, app, "Already assigned issue", issue.StatePending, "v0.1", "human:alice")

	_, _, err := run("issue", "edit", id.String(), "--assign", "agent:claude-code-1")
	if err != nil {
		t.Fatalf("issue edit --assign (overwrite) failed: %v", err)
	}

	ref := issue.RefPrefix + id.String()
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if iss.Assigned != "agent:claude-code-1" {
		t.Fatalf("expected Assigned to be overwritten to 'agent:claude-code-1', got: %q", iss.Assigned)
	}
}

// TestIssueEdit_Unassign verifies that --unassign clears the Assigned field.
func TestIssueEdit_Unassign(t *testing.T) {
	app, run := setupEditTest(t)

	id := createTestIssueWithAssignment(t, app, "Assigned issue", issue.StatePending, "v0.1", "agent:claude-code-1")

	_, _, err := run("issue", "edit", id.String(), "--unassign")
	if err != nil {
		t.Fatalf("issue edit --unassign failed: %v", err)
	}

	ref := issue.RefPrefix + id.String()
	data, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if iss.Assigned != "" {
		t.Fatalf("expected Assigned to be cleared, got: %q", iss.Assigned)
	}
}

// TestIssueEdit_Unassign_AlreadyClear verifies that --unassign on an already-unassigned
// issue is a no-op and does not error.
func TestIssueEdit_Unassign_AlreadyClear(t *testing.T) {
	app, run := setupEditTest(t)

	id := createEditTestIssue(t, app, "Unassigned issue")

	_, _, err := run("issue", "edit", id, "--unassign")
	if err != nil {
		t.Fatalf("--unassign on unassigned issue should succeed, got: %v", err)
	}

	ref := issue.RefPrefix + id
	data, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(data)

	if iss.Assigned != "" {
		t.Fatalf("expected Assigned to remain empty, got: %q", iss.Assigned)
	}
}

// TestIssueList_FilterAssigned verifies that 'issue list --assigned' shows only
// issues assigned to the specified worker.
func TestIssueList_FilterAssigned(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssueWithAssignment(t, app, "Agent issue", issue.StatePending, "v0.1", "agent:claude-code-1")
	createTestIssueWithAssignment(t, app, "Human issue", issue.StatePending, "v0.1", "human:alice")
	createTestIssueWithAssignment(t, app, "Unassigned issue", issue.StatePending, "v0.1", "")

	stdout, err := run("issue", "list", "--assigned", "agent:claude-code-1")
	if err != nil {
		t.Fatalf("issue list --assigned failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Agent issue") {
		t.Errorf("expected 'Agent issue' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Human issue") {
		t.Errorf("expected 'Human issue' to be excluded, got:\n%s", output)
	}
	if strings.Contains(output, "Unassigned issue") {
		t.Errorf("expected 'Unassigned issue' to be excluded, got:\n%s", output)
	}
}

// TestIssueList_FilterAssigned_JSON verifies that 'issue list --assigned --format json'
// returns only the matching issues in JSON format.
func TestIssueList_FilterAssigned_JSON(t *testing.T) {
	app, run := setupListTest(t)

	createTestIssueWithAssignment(t, app, "Assigned JSON issue", issue.StatePending, "v0.1", "agent:claude-code-1")
	createTestIssueWithAssignment(t, app, "Other JSON issue", issue.StatePending, "v0.1", "human:bob")

	stdout, err := run("issue", "list", "--format", "json", "--assigned", "agent:claude-code-1")
	if err != nil {
		t.Fatalf("issue list --assigned --format json failed: %v", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout.String())
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 issue in JSON, got %d: %v", len(result), result)
	}
	if result[0]["title"] != "Assigned JSON issue" {
		t.Fatalf("expected 'Assigned JSON issue', got: %v", result[0]["title"])
	}
}

// TestMilestoneShow_WorkerCapacity verifies that 'milestone show' renders a
// "Worker capacity" section when workers have assigned or in-progress issues.
func TestMilestoneShow_WorkerCapacity(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// dev-a: 1 in-progress (transitions mark dev-a as actor), 1 assigned
	// agent-1: 0 in-progress, 1 assigned (idle)
	transitions := []issue.Transition{
		{State: "in-progress", Actor: "agent:dev-a", Timestamp: time.Now()},
	}
	createTestIssueWithAssignmentTransitionsMilestone(t, app, "dev-a active work", issue.StateInProgress, "v0.1", "", transitions)
	createTestIssueWithAssignment(t, app, "dev-a assigned work", issue.StatePending, "v0.1", "agent:dev-a")
	createTestIssueWithAssignment(t, app, "agent-1 assigned work", issue.StatePending, "v0.1", "agent:agent-1")

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show v0.1 failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Worker capacity") {
		t.Errorf("expected 'Worker capacity' section in output, got:\n%s", output)
	}
	if !strings.Contains(output, "agent:dev-a") {
		t.Errorf("expected 'agent:dev-a' in capacity output, got:\n%s", output)
	}
	if !strings.Contains(output, "agent:agent-1") {
		t.Errorf("expected 'agent:agent-1' in capacity output, got:\n%s", output)
	}
	if !strings.Contains(output, "idle") {
		t.Errorf("expected 'idle' for agent-1 in capacity output, got:\n%s", output)
	}
}

// TestMilestoneShow_WorkerCapacity_JSON verifies that 'milestone show --format json'
// includes a 'worker_capacity' map in the JSON output.
func TestMilestoneShow_WorkerCapacity_JSON(t *testing.T) {
	app, run := setupMilestoneTest(t)

	transitions := []issue.Transition{
		{State: "in-progress", Actor: "agent:dev-a", Timestamp: time.Now()},
	}
	createTestIssueWithAssignmentTransitionsMilestone(t, app, "dev-a active work", issue.StateInProgress, "v0.1", "", transitions)
	createTestIssueWithAssignment(t, app, "dev-a assigned work", issue.StatePending, "v0.1", "agent:dev-a")

	stdout, err := run("milestone", "show", "--format", "json", "v0.1")
	if err != nil {
		t.Fatalf("milestone show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout.String())
	}

	if _, ok := result["worker_capacity"]; !ok {
		t.Fatalf("expected 'worker_capacity' key in JSON output, got keys: %v", keys(result))
	}
}

// TestMilestoneShow_NoCapacityWhenNoWorkers verifies that the capacity section
// is omitted when no workers have in-progress or assigned issues.
func TestMilestoneShow_NoCapacityWhenNoWorkers(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createTestIssueWithMilestone(t, app, "Unassigned pending", issue.StatePending, "v0.1", "")

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show v0.1 failed: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "Worker capacity") {
		t.Errorf("expected NO 'Worker capacity' section when no workers are active, got:\n%s", output)
	}
}

// TestMilestoneShow_WorkerCapacity_NoDoubleCount verifies that an in-progress
// issue that is also assigned to the same worker is NOT double-counted in both
// InProgress and Assigned.
func TestMilestoneShow_WorkerCapacity_NoDoubleCount(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Create one in-progress issue assigned to dev-a with dev-a as the
	// transition actor. This should count as 1 in-progress, NOT also 1 assigned.
	transitions := []issue.Transition{
		{State: "in-progress", Actor: "agent:dev-a", Timestamp: time.Now()},
	}
	createTestIssueWithAssignmentTransitionsMilestone(t, app, "dev-a in-progress+assigned", issue.StateInProgress, "v0.1", "agent:dev-a", transitions)

	stdout, err := run("milestone", "show", "--format", "json", "v0.1")
	if err != nil {
		t.Fatalf("milestone show --format json failed: %v", err)
	}

	var result struct {
		WorkerCapacity map[string]struct {
			InProgress int `json:"in_progress"`
			Assigned   int `json:"assigned"`
		} `json:"worker_capacity"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout.String())
	}

	wc, ok := result.WorkerCapacity["agent:dev-a"]
	if !ok {
		t.Fatal("expected worker capacity for agent:dev-a")
	}
	if wc.InProgress != 1 {
		t.Errorf("InProgress = %d, want 1", wc.InProgress)
	}
	// An in-progress issue should NOT also count as assigned.
	if wc.Assigned != 0 {
		t.Errorf("Assigned = %d, want 0 (in-progress issue should not be double-counted as assigned)", wc.Assigned)
	}
}

// TestChainNext_Actor_PrefersAssigned verifies that next --actor prefers issues
// assigned to the requesting actor over unassigned ready issues.
func TestChainNext_Actor_PrefersAssigned(t *testing.T) {
	app, run := setupListTest(t)

	// An unassigned ready issue (no dependencies, comes first by UUID sort typically).
	createTestIssue(t, app, "Unassigned ready issue", issue.StatePending, "")

	// An issue specifically assigned to the target actor.
	createTestIssueWithAssignment(t, app, "Assigned to actor issue", issue.StatePending, "v0.1", "agent:claude-code-1")

	stdout, err := run("next", "--actor", "agent:claude-code-1")
	if err != nil {
		t.Fatalf("next --actor with assigned issue failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Assigned to actor issue") {
		t.Errorf("expected 'Assigned to actor issue' to be preferred, got:\n%s", output)
	}
}

// createTestIssueWithAssignmentTransitionsMilestone writes an issue with assignment,
// transitions, and a milestone to the store.
func createTestIssueWithAssignmentTransitionsMilestone(t *testing.T, app *cli.App, title string, state issue.State, ms, assigned string, transitions []issue.Transition) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:          id,
		Title:       title,
		State:       state,
		Milestone:   ms,
		Assigned:    assigned,
		Transitions: transitions,
		Created:     now,
		Updated:     now,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := issue.RefPrefix + id.String()
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add test issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}
