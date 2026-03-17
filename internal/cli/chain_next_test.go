// ABOUTME: Tests for chain next command: verifies it shows the next issue to work on,
// ABOUTME: equivalent to 'issue show HEAD', and --actor flag for per-worker resolution.
package cli_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// TestChainNext shows a pending issue via 'chain next' and verifies
// it returns the same HEAD issue as 'issue show'.
func TestChainNext(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "Next Issue To Work On", issue.StatePending, "Important work.")

	stdout, err := run("next")
	if err != nil {
		t.Fatalf("chain next failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Next Issue To Work On") {
		t.Fatalf("expected issue title in chain next output, got:\n%s", output)
	}
}

// TestChainNext_InProgress verifies chain next returns the in-progress issue
// when one exists, since HEAD resolves to in-progress first.
func TestChainNext_InProgress(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "Pending One", issue.StatePending, "")
	createTestIssue(t, app, "Active Work", issue.StateInProgress, "")

	stdout, err := run("next")
	if err != nil {
		t.Fatalf("chain next failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Active Work") {
		t.Fatalf("expected in-progress issue 'Active Work' from chain next, got:\n%s", output)
	}
}

// createTestIssueWithTransitions writes an issue with actor transitions to the store.
func createTestIssueWithTransitions(t *testing.T, app *cli.App, title string, state issue.State, transitions []issue.Transition) uuid.UUID {
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
		Transitions: transitions,
		Created:     now,
		Updated:     now,
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

// TestChainNext_Actor verifies that --actor returns the in-progress issue
// for the named actor when one exists.
func TestChainNext_Actor(t *testing.T) {
	app, run := setupShowTest(t)

	actor := "agent:claude-code-1"
	transitions := []issue.Transition{
		{State: "start", Actor: actor, Timestamp: time.Now()},
	}
	createTestIssueWithTransitions(t, app, "Agent Active Work", issue.StateInProgress, transitions)
	createTestIssue(t, app, "Human Pending", issue.StatePending, "")

	stdout, err := run("next", "--actor", actor)
	if err != nil {
		t.Fatalf("next --actor failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Agent Active Work") {
		t.Fatalf("expected 'Agent Active Work' for actor %q, got:\n%s", actor, output)
	}
}

// TestChainNext_ActorNoneInProgress verifies that --actor returns a pending
// issue from the ready set when the actor has no in-progress issue.
func TestChainNext_ActorNoneInProgress(t *testing.T) {
	app, run := setupShowTest(t)

	// No in-progress issue; one pending issue is available.
	createTestIssue(t, app, "Available For Agent", issue.StatePending, "")

	stdout, err := run("next", "--actor", "agent:claude-code-1")
	if err != nil {
		t.Fatalf("next --actor with no in-progress issue failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Available For Agent") {
		t.Fatalf("expected 'Available For Agent' from ready set for actor, got:\n%s", output)
	}
}

// TestChainNext_NoActorFlagPreservesV1Behavior verifies that omitting --actor
// preserves v0.1 global WIP=1 semantics.
func TestChainNext_NoActorFlagPreservesV1Behavior(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "Global In Progress", issue.StateInProgress, "")
	createTestIssue(t, app, "Other Pending", issue.StatePending, "")

	stdout, err := run("next")
	if err != nil {
		t.Fatalf("next (no --actor) failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Global In Progress") {
		t.Fatalf("expected global in-progress issue without --actor, got:\n%s", output)
	}
}
