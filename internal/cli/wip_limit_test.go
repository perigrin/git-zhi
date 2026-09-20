// ABOUTME: Tests the chain-wide WIP limit that caps how many issues may be
// ABOUTME: in progress at once, enforced when starting an issue.

package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/perigrin/git-zhi/internal/config"
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

// The tests below cover the acceptance criteria for issue 019f2a2e-252e-78a0-8913-ab59ea084c02
// ("Add wip.limit chain config key and enforce it in --state start"), each
// named for the go test -run pattern its checkbox names.

// TestChainConfig_SetWIPLimit verifies that 'config wip_limit 3' persists and
// is reflected both in the human-readable display and the JSON output.
func TestChainConfig_SetWIPLimit(t *testing.T) {
	_, run := setupChainConfigTest(t)

	if _, err := run("config", "wip_limit", "3"); err != nil {
		t.Fatalf("chain config set wip_limit failed: %v", err)
	}

	stdout, err := run("config")
	if err != nil {
		t.Fatalf("chain config display failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "wip_limit: 3") {
		t.Fatalf("expected 'wip_limit: 3' in config output, got:\n%s", stdout.String())
	}

	jsonOut, err := run("config", "--format", "json")
	if err != nil {
		t.Fatalf("chain config --format json failed: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, jsonOut.String())
	}
	if wl, ok := result["wip_limit"]; !ok || wl != float64(3) {
		t.Fatalf("expected wip_limit: 3 in JSON output, got: %v", result)
	}
}

// TestChainConfig_WIPLimit_Zero verifies that a wip_limit of 0 is accepted as
// "unlimited" and does not block a subsequent start, even with several issues
// already in progress.
func TestChainConfig_WIPLimit_Zero(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "0"); err != nil {
		t.Fatalf("chain config set wip_limit 0 failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		createTestIssue(t, app, "Already Running", issue.StateInProgress, "")
	}
	newUUID := createEditTestIssue(t, app, "New Work Under Zero Limit")

	// Full UUID rather than a short prefix: several issues created in quick
	// succession can share a UUIDv7 time-based prefix, which resolve treats
	// as ambiguous.
	if _, _, err := run("issue", "edit", "--state", "start", newUUID); err != nil {
		t.Fatalf("expected start to succeed with wip_limit=0 (unlimited), got: %v", err)
	}
}

// TestChainConfig_WIPLimit_Negative verifies that a negative wip_limit is
// rejected, the existing config is left unchanged, and a subsequent start is
// unaffected by the rejected attempt.
func TestChainConfig_WIPLimit_Negative(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "-1"); err == nil {
		t.Fatal("expected error for negative wip_limit, got nil")
	}

	data, err := app.Store.ReadEntity("refs/zhi/_/config", "config.yaml")
	if err != nil {
		t.Fatalf("ReadEntity config: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.WIPLimit != 0 {
		t.Fatalf("expected wip_limit to remain 0 after rejected negative set, got %d", cfg.WIPLimit)
	}

	newUUID := createEditTestIssue(t, app, "Unaffected By Rejected Set")
	if _, _, err := run("issue", "edit", "--state", "start", newUUID); err != nil {
		t.Fatalf("expected start to succeed since the negative wip_limit set was rejected, got: %v", err)
	}
}

// TestChainConfig_WIPLimit_NonInteger verifies that a non-integer wip_limit
// value returns a parse error and leaves the existing config unchanged.
func TestChainConfig_WIPLimit_NonInteger(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "notanumber"); err == nil {
		t.Fatal("expected error for non-integer wip_limit, got nil")
	}

	data, err := app.Store.ReadEntity("refs/zhi/_/config", "config.yaml")
	if err != nil {
		t.Fatalf("ReadEntity config: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.WIPLimit != 0 {
		t.Fatalf("expected wip_limit to remain 0 after rejected non-integer set, got %d", cfg.WIPLimit)
	}
}

// TestIssueEdit_Start_WIPLimitRefusal verifies that starting a second issue
// while wip_limit=1 and one issue is already in progress is refused with an
// error referencing the WIP limit.
func TestIssueEdit_Start_WIPLimitRefusal(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "1"); err != nil {
		t.Fatalf("set wip_limit failed: %v", err)
	}

	createTestIssue(t, app, "Already In Progress", issue.StateInProgress, "")
	otherUUID := createEditTestIssue(t, app, "Other Work")

	_, _, err := run("issue", "edit", "--state", "start", otherUUID)
	if err == nil {
		t.Fatal("expected WIP limit refusal, got success")
	}
	if !strings.Contains(err.Error(), "WIP limit") {
		t.Fatalf("expected error to reference the WIP limit, got: %v", err)
	}
}

// TestIssueEdit_Start_WIPLimitAllowed verifies that starting a second issue
// succeeds when wip_limit=2 and only one issue is currently in progress.
func TestIssueEdit_Start_WIPLimitAllowed(t *testing.T) {
	app, run := setupEditTest(t)

	if _, _, err := run("config", "wip_limit", "2"); err != nil {
		t.Fatalf("set wip_limit failed: %v", err)
	}

	createTestIssue(t, app, "Already In Progress", issue.StateInProgress, "")
	otherUUID := createEditTestIssue(t, app, "Other Work")

	if _, _, err := run("issue", "edit", "--state", "start", otherUUID); err != nil {
		t.Fatalf("expected start to succeed under wip_limit=2 with 1 in progress, got: %v", err)
	}
}

// TestIssueEdit_Start_AlreadyInProgress verifies that double-starting an
// already in-progress issue returns a clear state-transition error and does
// not record a second session (i.e. does not double-count against the WIP
// limit).
func TestIssueEdit_Start_AlreadyInProgress(t *testing.T) {
	app, run := setupEditTest(t)

	uuidStr := createEditTestIssue(t, app, "Double Start")
	prefix := uuidStr[:8]

	if _, _, err := run("issue", "edit", "--state", "start", prefix); err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	_, _, err := run("issue", "edit", "--state", "start", prefix)
	if err == nil {
		t.Fatal("expected error on double-start, got nil")
	}
	if !strings.Contains(err.Error(), "cannot start") {
		t.Fatalf("expected a clear state-transition error, got: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	data, err2 := app.Store.ReadEntity(ref, "issue.md")
	if err2 != nil {
		t.Fatalf("ReadEntity: %v", err2)
	}
	iss, err2 := issue.Parse(data)
	if err2 != nil {
		t.Fatalf("Parse: %v", err2)
	}
	if len(iss.Sessions) != 1 {
		t.Fatalf("expected exactly 1 session after a rejected double-start, got %d", len(iss.Sessions))
	}
}

// TestChainNext_PerActorCapHit verifies that 'next --actor' does not hand a
// second, different ready issue to an agent that already holds an
// in-progress issue (per-actor WIP=1 already reached), even though the
// chain-wide wip_limit still has room.
//
// wip.limit chain config key issue (019f2a2e), positive scenario: "next
// --actor <agent> when the per-actor WIP=1 is already reached for that agent
// but the chain-wide cap still has room returns nothing for that agent."
func TestChainNext_PerActorCapHit(t *testing.T) {
	app, run := setupShowTest(t)

	if _, err := run("config", "wip_limit", "5"); err != nil {
		t.Fatalf("set wip_limit failed: %v", err)
	}

	actorName := "agent:wip-limit-cap-hit"
	transitions := []issue.Transition{{State: "start", Actor: actorName, Timestamp: time.Now()}}
	createTestIssueWithTransitions(t, app, "Held By Agent", issue.StateInProgress, transitions)
	createTestIssue(t, app, "Unassigned Ready Work", issue.StatePending, "")

	stdout, err := run("next", "--actor", actorName)
	if err != nil {
		t.Fatalf("next --actor failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Held By Agent") {
		t.Fatalf("expected the agent's own in-progress issue (per-actor WIP=1 already reached), got:\n%s", output)
	}
	if strings.Contains(output, "Unassigned Ready Work") {
		t.Fatalf("agent already at per-actor WIP=1 must not be handed new ready work, got:\n%s", output)
	}
}
