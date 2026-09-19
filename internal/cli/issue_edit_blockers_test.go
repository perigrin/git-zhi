// ABOUTME: Tests that issue edit --state start refuses an issue whose blockers
// ABOUTME: are unresolved, and that --force and resume are unaffected.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// Issues are linked through the CLI rather than by setting BlockedBy directly:
// graph.Build derives edges from Blocks declarations, so a fixture that writes
// only one half of the edge produces a graph with no edge in it.

// TestIssueEditStart_RefusesBlockedIssue — the write path had no readiness
// check at all, so a caller could start work whose dependency was still open
// simply by not going through next.
func TestIssueEditStart_RefusesBlockedIssue(t *testing.T) {
	app, run := setupEditTest(t)

	up := createEditTestIssue(t, app, "Upstream work")
	down := createEditTestIssue(t, app, "Downstream work")
	if _, _, err := run("issue", "edit", down, "--after", up); err != nil {
		t.Fatalf("linking failed: %v", err)
	}

	_, _, err := run("issue", "edit", down, "--state", "start")
	if err == nil {
		t.Fatal("expected start to be refused while the upstream is unresolved")
	}
	if !strings.Contains(err.Error(), "Upstream work") {
		t.Errorf("expected the error to name the blocker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected the error to offer --force, got: %v", err)
	}
}

// TestIssueEditStart_ForceOverridesBlockers — the documented escape hatch has
// to reach its own check, which is the defect fixed in the count guard today.
func TestIssueEditStart_ForceOverridesBlockers(t *testing.T) {
	app, run := setupEditTest(t)

	up := createEditTestIssue(t, app, "Upstream work")
	down := createEditTestIssue(t, app, "Downstream work")
	if _, _, err := run("issue", "edit", down, "--after", up); err != nil {
		t.Fatalf("linking failed: %v", err)
	}

	if _, _, err := run("issue", "edit", down, "--state", "start", "--force"); err != nil {
		t.Fatalf("--force should start a blocked issue, got: %v", err)
	}
}

// TestIssueEditStart_ReadyIssueUnaffected — the guard must not refuse work that
// has no blockers at all.
func TestIssueEditStart_ReadyIssueUnaffected(t *testing.T) {
	app, run := setupEditTest(t)

	solo := createEditTestIssue(t, app, "Unblocked work")
	if _, _, err := run("issue", "edit", solo, "--state", "start"); err != nil {
		t.Fatalf("an unblocked issue must start normally, got: %v", err)
	}
}

// TestIssueEditStart_DoneBlockerAllowsStart — a resolved dependency is not a
// dependency.
func TestIssueEditStart_DoneBlockerAllowsStart(t *testing.T) {
	app, run := setupEditTest(t)

	up := createEditTestIssue(t, app, "Upstream work")
	down := createEditTestIssue(t, app, "Downstream work")
	if _, _, err := run("issue", "edit", down, "--after", up); err != nil {
		t.Fatalf("linking failed: %v", err)
	}

	if _, _, err := run("issue", "edit", up, "--state", "start"); err != nil {
		t.Fatalf("starting upstream failed: %v", err)
	}
	makeTestCommit(t, app, "upstream work")
	if _, _, err := run("issue", "edit", up, "--state", "done"); err != nil {
		t.Fatalf("completing upstream failed: %v", err)
	}

	if _, _, err := run("issue", "edit", down, "--state", "start"); err != nil {
		t.Fatalf("downstream should start once upstream is done, got: %v", err)
	}
}

// TestIssueEditStart_CancelledBlockerAllowsStart — cancelled counts as
// resolved, matching ReadySet.
func TestIssueEditStart_CancelledBlockerAllowsStart(t *testing.T) {
	app, run := setupEditTest(t)

	up := createEditTestIssue(t, app, "Upstream work")
	down := createEditTestIssue(t, app, "Downstream work")
	if _, _, err := run("issue", "edit", down, "--after", up); err != nil {
		t.Fatalf("linking failed: %v", err)
	}
	if _, _, err := run("issue", "edit", up, "--state", "cancel"); err != nil {
		t.Fatalf("cancelling upstream failed: %v", err)
	}

	if _, _, err := run("issue", "edit", down, "--state", "start"); err != nil {
		t.Fatalf("downstream should start once upstream is cancelled, got: %v", err)
	}
}

// TestIssueEditResume_NotGatedByBlockers — resume re-opens a session on work
// already in flight. Gating it would strand an issue whose upstream was
// reopened after it started, the same trap the WIP limit avoids.
func TestIssueEditResume_NotGatedByBlockers(t *testing.T) {
	app, run := setupEditTest(t)

	up := createEditTestIssue(t, app, "Upstream work")
	down := createEditTestIssue(t, app, "Downstream work")

	// Start downstream while it is unblocked, then introduce the dependency.
	if _, _, err := run("issue", "edit", down, "--state", "start"); err != nil {
		t.Fatalf("starting downstream failed: %v", err)
	}
	makeTestCommit(t, app, "some work")
	if _, _, err := run("issue", "edit", down, "--state", "pause"); err != nil {
		t.Fatalf("pausing downstream failed: %v", err)
	}
	if _, _, err := run("issue", "edit", down, "--after", up); err != nil {
		t.Fatalf("linking failed: %v", err)
	}

	if _, _, err := run("issue", "edit", down, "--state", "resume"); err != nil {
		t.Fatalf("resume must not be gated by blockers, got: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	for _, iss := range all {
		if iss.Title == "Downstream work" && iss.State != issue.StateInProgress {
			t.Errorf("expected downstream still in progress, got %s", iss.State)
		}
	}
}
