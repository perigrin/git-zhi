// ABOUTME: Tests the chain-wide WIP limit that caps how many issues may be
// ABOUTME: in progress at once, enforced when starting an issue.

package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestWIPLimit_BlocksStartBeyondLimit verifies that once wip_limit issues are
// in progress, starting another is refused. A fleet of agents driving the DAG
// otherwise starts every ready issue at once.
func TestWIPLimit_BlocksStartBeyondLimit(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "2"); err != nil {
		t.Fatalf("set wip_limit: %v", err)
	}

	var ids []string
	for _, title := range []string{"one", "two", "three"} {
		ids = append(ids, createEditTestIssue(t, app, title))
	}

	for i := 0; i < 2; i++ {
		if _, _, err := run("issue", "edit", ids[i], "--state", "start"); err != nil {
			t.Fatalf("start %d should be allowed under the limit: %v", i+1, err)
		}
	}

	_, _, err := run("issue", "edit", ids[2], "--state", "start")
	if err == nil {
		t.Fatal("expected the third start to be refused at wip_limit 2")
	}
	if !strings.Contains(err.Error(), "WIP limit") {
		t.Errorf("expected a WIP limit error, got: %v", err)
	}
}

// TestWIPLimit_ForceOverrides verifies --force is the documented escape hatch,
// consistent with how --force already overrides the done-with-zero-commits check.
func TestWIPLimit_ForceOverrides(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "1"); err != nil {
		t.Fatalf("set wip_limit: %v", err)
	}

	a := createEditTestIssue(t, app, "one")
	b := createEditTestIssue(t, app, "two")

	if _, _, err := run("issue", "edit", a, "--state", "start"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, _, err := run("issue", "edit", b, "--state", "start", "--force"); err != nil {
		t.Errorf("--force should override the WIP limit: %v", err)
	}
}

// TestWIPLimit_UnsetMeansUnlimited verifies the default stays permissive, so
// existing chains are unaffected until someone opts in.
func TestWIPLimit_UnsetMeansUnlimited(t *testing.T) {
	app, run := setupEditTest(t)

	for _, title := range []string{"one", "two", "three"} {
		id := createEditTestIssue(t, app, title)
		if _, _, err := run("issue", "edit", id, "--state", "start"); err != nil {
			t.Fatalf("start %q with no limit configured: %v", title, err)
		}
	}
}

// TestWIPLimit_FinishedIssuesFreeASlot verifies the cap tracks work still in
// flight. Note that pause does NOT free a slot: it closes the measurement
// session but leaves the state at in-progress, which is why the limit error
// points at finish or cancel.
func TestWIPLimit_FinishedIssuesFreeASlot(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "1"); err != nil {
		t.Fatalf("set wip_limit: %v", err)
	}

	a := createEditTestIssue(t, app, "one")
	b := createEditTestIssue(t, app, "two")

	if _, _, err := run("issue", "edit", a, "--state", "start"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, _, err := run("issue", "edit", a, "--state", "done", "--force"); err != nil {
		t.Fatalf("finish first: %v", err)
	}
	if _, _, err := run("issue", "edit", b, "--state", "start"); err != nil {
		t.Errorf("finishing an issue should free a WIP slot: %v", err)
	}

	data, err := app.Store.ReadEntity("refs/zhi/_/issues/"+b, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	loaded, err := issue.Parse(data)
	if err != nil {
		t.Fatalf("issue.Parse: %v", err)
	}
	if loaded.State != issue.StateInProgress {
		t.Errorf("second issue state = %q, want in-progress", loaded.State)
	}
}

// TestWIPLimit_ResumeAtCapIsAllowed verifies an issue already in progress can
// be resumed when the chain is at its cap. It already occupies a slot, so
// counting it against itself would make the only issue you are working on
// impossible to resume.
func TestWIPLimit_ResumeAtCapIsAllowed(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "1"); err != nil {
		t.Fatalf("set wip_limit: %v", err)
	}

	id := createEditTestIssue(t, app, "only")
	if _, _, err := run("issue", "edit", id, "--state", "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := run("issue", "edit", id, "--state", "pause"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	if _, _, err := run("issue", "edit", id, "--state", "resume"); err != nil {
		t.Errorf("resume at the cap must be allowed; the issue already holds the slot: %v", err)
	}
}

// TestWIPLimit_ConfigValidation verifies the new config key rejects values it
// cannot honour, rather than storing them and silently disabling the cap.
func TestWIPLimit_ConfigValidation(t *testing.T) {
	_, run := setupEditTest(t)

	for _, bad := range []string{"abc", "-1", "1.5", ""} {
		if _, _, err := run("config", "wip_limit", bad); err == nil {
			t.Errorf("config wip_limit %q was accepted; expected an error", bad)
		}
	}

	if _, _, err := run("config", "wip_limit", "0"); err != nil {
		t.Errorf("0 must be accepted as the disable value: %v", err)
	}

	if _, _, err := run("config", "wip_limit", "3"); err != nil {
		t.Fatalf("set wip_limit 3: %v", err)
	}
	stdout, _, err := run("config")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if !strings.Contains(stdout.String(), "wip_limit: 3") {
		t.Errorf("config display does not show the configured limit:\n%s", stdout.String())
	}
}

// TestWIPLimit_ResumeWhenOverCapIsAllowed covers the case the at-cap test
// cannot: lowering the limit below the number already in flight. Gating resume
// on the count left every in-flight issue unresumable without --force, which
// is the state an operator lands in the moment they tighten the cap.
func TestWIPLimit_ResumeWhenOverCapIsAllowed(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "3"); err != nil {
		t.Fatalf("set wip_limit: %v", err)
	}

	var ids []string
	for _, title := range []string{"one", "two", "three"} {
		id := createEditTestIssue(t, app, title)
		ids = append(ids, id)
		if _, _, err := run("issue", "edit", id, "--state", "start"); err != nil {
			t.Fatalf("start %s: %v", title, err)
		}
	}

	// Operator tightens the cap mid-flight: 3 in progress, limit now 1.
	if _, _, err := run("config", "wip_limit", "1"); err != nil {
		t.Fatalf("lower wip_limit: %v", err)
	}
	if _, _, err := run("issue", "edit", ids[0], "--state", "pause"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	if _, _, err := run("issue", "edit", ids[0], "--state", "resume"); err != nil {
		t.Errorf("resume over cap must be allowed; the issue already holds its slot: %v", err)
	}
}
