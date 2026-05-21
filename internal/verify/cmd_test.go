// ABOUTME: Integration tests for the verify Cobra command.
// ABOUTME: Uses real on-disk git repos and exercises the full verify flow.
package verify_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/storage"
	"github.com/perigrin/git-zhi/internal/verify"
)

// setupVerifyTest initialises an in-memory git repo, writes a milestone with
// the given name, and returns the App and a run helper that executes the
// verify command with the supplied args.
func setupVerifyTest(t *testing.T) (*cli.App, func(args ...string) (string, string, error)) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}

	run := func(args ...string) (string, string, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := verify.NewVerifyCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}
	return app, run
}

// writeMilestone writes a milestone ref to the store.
func writeMilestone(t *testing.T, store *storage.Store, ms *milestone.Milestone) {
	t.Helper()
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	refPath := milestone.RefPrefix + ms.Name
	if err := store.WriteEntity(refPath, "milestone.yaml", data, "create milestone"); err != nil {
		t.Fatalf("WriteEntity milestone: %v", err)
	}
}

// writeIssue writes an issue ref to the store and returns the issue ID string.
func writeIssue(t *testing.T, store *storage.Store, iss *issue.Issue) {
	t.Helper()
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("Marshal issue: %v", err)
	}
	refPath := issue.RefPrefix + iss.ID.String()
	if err := store.WriteEntity(refPath, "issue.md", data, "create issue"); err != nil {
		t.Fatalf("WriteEntity issue: %v", err)
	}
}

// newDoneIssueWithAC creates a done issue with a body containing AC commands.
func newDoneIssueWithAC(t *testing.T, milestoneName string, body string) *issue.Issue {
	t.Helper()
	id, err := issueID(t), error(nil)
	_ = err
	now := time.Now()
	return &issue.Issue{
		ID:            id,
		Title:         "Test Issue",
		State:         issue.StateDone,
		Urgency:       issue.UrgencyNormal,
		Milestone:     milestoneName,
		Created:       now,
		Updated:       now,
		Sessions:      []issue.Session{},
		Transitions:   []issue.Transition{},
		ObservedPaths: []string{},
		Body:          body,
	}
}

// TestVerifyCLI_LoadsMilestoneAndRunsCommands verifies the basic flow:
// load milestone, find done issues, extract commands, execute them.
func TestVerifyCLI_LoadsMilestoneAndRunsCommands(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := "## Acceptance Criteria\n\n- [ ] echo works (`echo hello`)\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1")
	if err != nil {
		t.Fatalf("verify v0.1 failed: %v", err)
	}

	if !strings.Contains(stdout, "echo hello") {
		t.Errorf("expected command in output, got:\n%s", stdout)
	}
	// Should show the pass marker (echo exits 0)
	if !strings.Contains(stdout, "✓") {
		t.Errorf("expected pass marker in output, got:\n%s", stdout)
	}
}

// TestVerifyCLI_ExitCode verifies exit code 0 on all pass, 1 on any failure.
func TestVerifyCLI_ExitCode(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	// Issue with a command that always fails
	body := "## Acceptance Criteria\n\n- [ ] always fails (`exit 1`)\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	_, _, err := run("v0.1")
	if err == nil {
		t.Fatal("expected non-zero exit when commands fail, got nil")
	}
}

// TestVerifyCLI_FailFast verifies --fail-fast stops on first failure.
func TestVerifyCLI_FailFast(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	// Issue with a failing command followed by a passing command.
	// With --fail-fast, the second command should not run.
	body := "## Acceptance Criteria\n\n- [ ] fails first (`exit 1`)\n- [ ] passes second (`echo second`)\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1", "--fail-fast")
	if err == nil {
		t.Fatal("expected non-zero exit with --fail-fast on failure")
	}
	// "echo second" should NOT appear in stdout because we stopped early
	if strings.Contains(stdout, "second") {
		t.Errorf("expected fail-fast to stop before 'echo second', but output contains 'second':\n%s", stdout)
	}
}

// TestVerifyCLI_DryRun verifies --dry-run lists commands without executing them.
func TestVerifyCLI_DryRun(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	// A command that would fail if executed.
	body := "## Acceptance Criteria\n\n- [ ] would fail (`exit 1`)\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1", "--dry-run")
	if err != nil {
		t.Fatalf("--dry-run should not fail even with failing commands, got: %v", err)
	}
	if !strings.Contains(stdout, "exit 1") {
		t.Errorf("expected command listed in dry-run output, got:\n%s", stdout)
	}
	// Should indicate dry-run mode
	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected [dry-run] marker in output, got:\n%s", stdout)
	}
}

// TestVerifyCLI_JSONOutput verifies --format json returns structured results.
func TestVerifyCLI_JSONOutput(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := "## Acceptance Criteria\n\n- [ ] echo works (`echo hello`)\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1", "--format", "json")
	if err != nil {
		t.Fatalf("verify --format json failed: %v", err)
	}

	var out verify.JSONReport
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput was:\n%s", err, stdout)
	}

	if out.Milestone != "v0.1" {
		t.Errorf("milestone = %q, want %q", out.Milestone, "v0.1")
	}
	if out.Total == 0 {
		t.Errorf("total = 0, expected > 0")
	}
	if out.Passed != out.Total {
		t.Errorf("passed = %d, total = %d; expected all to pass", out.Passed, out.Total)
	}
	if len(out.Results) == 0 {
		t.Errorf("expected results array to be non-empty")
	}
	if out.Results[0].Command != "echo hello" {
		t.Errorf("results[0].Command = %q, want %q", out.Results[0].Command, "echo hello")
	}
	if !out.Results[0].Passed {
		t.Errorf("results[0].Passed = false, want true")
	}
}

// TestVerifyCLI_NoMilestoneArg verifies that missing milestone arg returns error.
func TestVerifyCLI_NoMilestoneArg(t *testing.T) {
	_, run := setupVerifyTest(t)
	_, _, err := run()
	if err == nil {
		t.Fatal("expected error when milestone arg is missing")
	}
}

// TestVerifyCLI_UnknownMilestone verifies error on non-existent milestone.
func TestVerifyCLI_UnknownMilestone(t *testing.T) {
	_, run := setupVerifyTest(t)
	_, _, err := run("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone")
	}
}

// TestVerifyCLI_NoDoneIssues verifies output when milestone has no done issues.
func TestVerifyCLI_NoDoneIssues(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	// Pending issue — should be skipped
	now := time.Now()
	id := issueID(t)
	iss := &issue.Issue{
		ID:            id,
		Title:         "Pending Issue",
		State:         issue.StatePending,
		Urgency:       issue.UrgencyNormal,
		Milestone:     "v0.1",
		Created:       now,
		Updated:       now,
		Sessions:      []issue.Session{},
		Transitions:   []issue.Transition{},
		ObservedPaths: []string{},
		Body:          "## Acceptance Criteria\n\n- [ ] test (`echo hi`)\n",
	}
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1")
	if err != nil {
		t.Fatalf("verify with no done issues should succeed (exit 0), got: %v", err)
	}
	if !strings.Contains(stdout, "0") {
		t.Logf("output was: %s", stdout)
	}
}

// TestVerifyCLI_PositiveNegativeSubsections verifies that positive and negative
// subsections are shown distinctly in human-readable output.
func TestVerifyCLI_PositiveNegativeSubsections(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := `## Acceptance Criteria

### Positive Scenarios
- [ ] passes (` + "`echo positive`" + `)

### Negative Scenarios
- [ ] also passes (` + "`echo negative`" + `)`

	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	if !strings.Contains(stdout, "Positive") {
		t.Errorf("expected 'Positive' in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Negative") {
		t.Errorf("expected 'Negative' in output, got:\n%s", stdout)
	}
}

// TestVerifyCLI_DroppedACFailsGate reproduces git-zhi#3: a done issue whose AC
// command is written in the colon form (not the (`...`) convention) must make
// verify exit non-zero and report the item as unverifiable, instead of silently
// reporting 0/0 and exiting 0.
func TestVerifyCLI_DroppedACFailsGate(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := "## Acceptance Criteria\n\n- [ ] greet works: `bash test.sh`\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1")
	if err == nil {
		t.Fatalf("expected non-zero exit for a dropped AC command, got nil\noutput:\n%s", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "unverifiable") {
		t.Errorf("expected 'unverifiable' in output, got:\n%s", stdout)
	}
}

// TestVerifyCLI_DroppedACReportedSeparately verifies a dropped AC command is not
// counted as a regression: a clean passing command alongside it still shows the
// dropped item as unverifiable, distinct from any regression count.
func TestVerifyCLI_DroppedACReportedSeparately(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := "## Acceptance Criteria\n\n" +
		"- [ ] echo works (`echo hello`)\n" +
		"- [ ] greet works: `bash test.sh`\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1")
	if err == nil {
		t.Fatalf("expected non-zero exit, got nil\noutput:\n%s", stdout)
	}
	if strings.Contains(stdout, "regression") {
		t.Errorf("dropped AC should not be reported as a regression, got:\n%s", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "unverifiable") {
		t.Errorf("expected 'unverifiable' in output, got:\n%s", stdout)
	}
}

// TestVerifyCLI_DroppedACJSON verifies the JSON report surfaces dropped AC
// commands in dedicated fields and still exits non-zero.
func TestVerifyCLI_DroppedACJSON(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := "## Acceptance Criteria\n\n- [ ] greet works: `bash test.sh`\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	stdout, _, err := run("v0.1", "--format", "json")
	if err == nil {
		t.Fatalf("expected non-zero exit for dropped AC in JSON mode, got nil\noutput:\n%s", stdout)
	}

	var out verify.JSONReport
	if e := json.Unmarshal([]byte(stdout), &out); e != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput was:\n%s", e, stdout)
	}
	if out.Unverifiable != 1 {
		t.Errorf("Unverifiable = %d, want 1", out.Unverifiable)
	}
	if len(out.UnverifiableItems) != 1 {
		t.Fatalf("UnverifiableItems len = %d, want 1", len(out.UnverifiableItems))
	}
	if !strings.Contains(out.UnverifiableItems[0].Item, "bash test.sh") {
		t.Errorf("UnverifiableItems[0].Item = %q, want it to contain the AC item text", out.UnverifiableItems[0].Item)
	}
}

// TestVerifyCLI_Timeout verifies --timeout is accepted.
func TestVerifyCLI_Timeout(t *testing.T) {
	app, run := setupVerifyTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeMilestone(t, app.Store, ms)

	body := "## Acceptance Criteria\n\n- [ ] quick (`echo quick`)\n"
	iss := newDoneIssueWithAC(t, "v0.1", body)
	writeIssue(t, app.Store, iss)

	// --timeout 30 should be accepted without error
	_, _, err := run("v0.1", "--timeout", "30")
	if err != nil {
		t.Fatalf("verify with --timeout 30 failed: %v", err)
	}
}
