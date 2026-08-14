// ABOUTME: Tests for the milestone body/resolution/postmortem write path,
// ABOUTME: which previously existed only on the read side.

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// setupMilestoneWriteTest returns a runner over a fresh chain.
func setupMilestoneWriteTest(t *testing.T) (*cli.App, func(args ...string) (*strings.Builder, error)) {
	t.Helper()
	app, runStdin := setupMilestoneWriteTestWithStdin(t)
	return app, func(args ...string) (*strings.Builder, error) {
		return runStdin("", args...)
	}
}

// setupMilestoneWriteTestWithStdin returns a runner that feeds stdin, needed
// for the '-' sentinel.
func setupMilestoneWriteTestWithStdin(t *testing.T) (*cli.App, func(stdin string, args ...string) (*strings.Builder, error)) {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	run := func(stdin string, args ...string) (*strings.Builder, error) {
		out := new(strings.Builder)
		cmd := cli.NewRootCommand()
		cmd.SetOut(out)
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		return out, cmd.Execute()
	}
	return app, run
}

// showMilestoneJSON runs milestone show --format json and decodes it.
func showMilestoneJSON(t *testing.T, run func(args ...string) (*strings.Builder, error), name string) map[string]interface{} {
	t.Helper()
	stdout, err := run("milestone", "show", name, "--format", "json")
	if err != nil {
		t.Fatalf("milestone show %s --format json: %v", name, err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout.String())
	}
	return out
}

// TestMilestoneAdd_BodyAndResolution verifies milestone add can set the body
// and resolution command. Both round-trip through milestone show already; only
// the write path was missing, which forced callers to write refs/zhi directly.
func TestMilestoneAdd_BodyAndResolution(t *testing.T) {
	_, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "rel",
		"--body", "## Context\n\nwhy this exists",
		"--resolution", "make test"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}

	out := showMilestoneJSON(t, run, "rel")
	if got, _ := out["resolution"].(string); got != "make test" {
		t.Errorf("resolution = %q, want %q", got, "make test")
	}
	if got, _ := out["body"].(string); !strings.Contains(got, "why this exists") {
		t.Errorf("body = %q, want it to contain the supplied text", got)
	}
}

// TestMilestoneEdit_BodyResolutionPostmortem verifies the same fields can be
// set after creation, and that a postmortem can be attached and read back.
func TestMilestoneEdit_BodyResolutionPostmortem(t *testing.T) {
	_, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "rel"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}

	if _, err := run("milestone", "edit", "rel", "--body", "replaced body"); err != nil {
		t.Fatalf("--body: %v", err)
	}
	if _, err := run("milestone", "edit", "rel", "--resolution", "prove -lr t/"); err != nil {
		t.Fatalf("--resolution: %v", err)
	}
	if _, err := run("milestone", "edit", "rel", "--postmortem", "what we learned"); err != nil {
		t.Fatalf("--postmortem: %v", err)
	}

	out := showMilestoneJSON(t, run, "rel")
	if got, _ := out["body"].(string); !strings.Contains(got, "replaced body") {
		t.Errorf("body = %q", got)
	}
	if got, _ := out["resolution"].(string); got != "prove -lr t/" {
		t.Errorf("resolution = %q", got)
	}
	if got, _ := out["postmortem"].(string); !strings.Contains(got, "what we learned") {
		t.Errorf("postmortem = %q", got)
	}
}

// TestMilestoneEdit_BodyFromStdin verifies the '-' sentinel, matching how
// issue edit --body reads a multi-line body without shell quoting hazards.
func TestMilestoneEdit_BodyFromStdin(t *testing.T) {
	_, runStdin := setupMilestoneWriteTestWithStdin(t)

	if _, err := runStdin("", "milestone", "add", "rel"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}
	if _, err := runStdin("## Design\n\nmultiline body\n", "milestone", "edit", "rel", "--body", "-"); err != nil {
		t.Fatalf("--body -: %v", err)
	}

	stdout, err := runStdin("", "milestone", "show", "rel", "--format", "json")
	if err != nil {
		t.Fatalf("milestone show: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, _ := out["body"].(string); !strings.Contains(got, "multiline body") {
		t.Errorf("body = %q", got)
	}
}

// TestMilestoneEdit_PostmortemSurvivesStateComplete verifies the content flags
// are applied before --state dispatches. Attaching a retrospective while
// closing the milestone out is the natural invocation; applying writes after
// the state branch returned would discard it and still exit 0.
func TestMilestoneEdit_PostmortemSurvivesStateComplete(t *testing.T) {
	_, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "rel"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}
	// No issues in the milestone, so the issue gate passes and the verify gate
	// is skipped (git-zhi is not on PATH under test).
	if _, err := run("milestone", "edit", "rel", "--state", "complete", "--postmortem", "closing notes"); err != nil {
		t.Fatalf("--state complete --postmortem: %v", err)
	}

	out := showMilestoneJSON(t, run, "rel")
	if got, _ := out["postmortem"].(string); !strings.Contains(got, "closing notes") {
		t.Errorf("postmortem discarded by --state complete: %q", got)
	}
	if got, _ := out["state"].(string); got != "completed" {
		t.Errorf("state = %q, want completed", got)
	}
}

// TestMilestoneEdit_ContentSurvivesRename verifies a body change combined with
// --name lands on the renamed ref rather than the deleted one.
func TestMilestoneEdit_ContentSurvivesRename(t *testing.T) {
	_, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "old"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}
	if _, err := run("milestone", "edit", "old", "--name", "new", "--body", "carried across"); err != nil {
		t.Fatalf("rename with body: %v", err)
	}

	out := showMilestoneJSON(t, run, "new")
	if got, _ := out["body"].(string); !strings.Contains(got, "carried across") {
		t.Errorf("body lost in rename: %q", got)
	}
}

// TestMilestoneEdit_EmptyStdinLeavesFieldAlone verifies that '-' reading an
// empty stream is a no-op. A generator that fails and emits nothing must not
// erase the field it was meant to fill.
func TestMilestoneEdit_EmptyStdinLeavesFieldAlone(t *testing.T) {
	_, runStdin := setupMilestoneWriteTestWithStdin(t)

	if _, err := runStdin("", "milestone", "add", "rel", "--body", "original body"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}
	if _, err := runStdin("", "milestone", "edit", "rel", "--body", "-"); err != nil {
		t.Fatalf("--body - with empty stdin: %v", err)
	}

	stdout, err := runStdin("", "milestone", "show", "rel", "--format", "json")
	if err != nil {
		t.Fatalf("milestone show: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, _ := out["body"].(string); !strings.Contains(got, "original body") {
		t.Errorf("empty stdin wiped the body: %q", got)
	}
}

// TestMilestoneEdit_OnlyOneFlagMayReadStdin verifies the contention is refused
// rather than silently giving the stream to whichever flag is evaluated first.
func TestMilestoneEdit_OnlyOneFlagMayReadStdin(t *testing.T) {
	_, runStdin := setupMilestoneWriteTestWithStdin(t)

	if _, err := runStdin("", "milestone", "add", "rel"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}

	_, err := runStdin("text", "milestone", "edit", "rel", "--body", "-", "--postmortem", "-")
	if err == nil {
		t.Fatal("expected an error when two flags both ask for stdin")
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Errorf("expected a stdin-contention error, got: %v", err)
	}
}

// TestMilestoneEdit_ResolutionNoneClears verifies the clear sentinel, which
// matches how --due none works.
func TestMilestoneEdit_ResolutionNoneClears(t *testing.T) {
	_, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "rel", "--resolution", "make test"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}
	if _, err := run("milestone", "edit", "rel", "--resolution", "none"); err != nil {
		t.Fatalf("--resolution none: %v", err)
	}

	out := showMilestoneJSON(t, run, "rel")
	if got, ok := out["resolution"].(string); ok && got != "" {
		t.Errorf("resolution = %q, want cleared", got)
	}
}

// TestMilestoneEdit_ContentSurvivesFailedCompleteGate verifies content flags
// persist even when the completion gate rejects the transition. Gates failing
// is the routine case — that is what they are for — and an LLM-generated
// postmortem must not be lost to one after stdout already reported it attached.
func TestMilestoneEdit_ContentSurvivesFailedCompleteGate(t *testing.T) {
	app, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "rel"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}
	// A pending issue in the milestone makes gate 1 fail.
	createTestIssueInMilestoneWithState(t, app, "still open", issue.StatePending, "rel")

	if _, err := run("milestone", "edit", "rel", "--state", "complete", "--postmortem", "hard-won notes"); err == nil {
		t.Fatal("expected the completion gate to reject a milestone with a pending issue")
	}

	out := showMilestoneJSON(t, run, "rel")
	if got, _ := out["postmortem"].(string); !strings.Contains(got, "hard-won notes") {
		t.Errorf("postmortem lost to the failed gate: %q", got)
	}
	if got, _ := out["state"].(string); got == "completed" {
		t.Error("milestone completed despite the gate failing")
	}
}

// TestMilestoneEdit_DueAndBodyWriteOnce verifies combining --due with a content
// flag produces a single commit on the milestone ref. Two writes leave a
// phantom intermediate revision that never matched any user intent, which
// anything reading the ref's history would report as a real edit.
func TestMilestoneEdit_DueAndBodyWriteOnce(t *testing.T) {
	app, run := setupMilestoneWriteTest(t)

	if _, err := run("milestone", "add", "rel"); err != nil {
		t.Fatalf("milestone add: %v", err)
	}

	before := countRefCommits(t, app, milestone.RefPrefix+"rel")
	if _, err := run("milestone", "edit", "rel", "--due", "2026-09-01", "--body", "new plan"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	after := countRefCommits(t, app, milestone.RefPrefix+"rel")

	if got := after - before; got != 1 {
		t.Errorf("--due with --body produced %d commits, want 1", got)
	}

	out := showMilestoneJSON(t, run, "rel")
	if body, _ := out["body"].(string); !strings.Contains(body, "new plan") {
		t.Errorf("body = %q", body)
	}
	if due, _ := out["due"].(string); !strings.Contains(due, "2026-09-01") {
		t.Errorf("due = %q", due)
	}
}

// countRefCommits returns the number of commits reachable from a zhi ref.
func countRefCommits(t *testing.T, app *cli.App, refPath string) int {
	t.Helper()
	ref, err := app.Repo.Reference(plumbing.ReferenceName(refPath), true)
	if err != nil {
		t.Fatalf("resolve %s: %v", refPath, err)
	}
	iter, err := app.Repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		t.Fatalf("log %s: %v", refPath, err)
	}
	defer iter.Close()
	n := 0
	for {
		if _, err := iter.Next(); err != nil {
			break
		}
		n++
	}
	return n
}

// TestMilestoneAdd_OnlyOneFlagMayReadStdin verifies add rejects stdin
// contention too. The guard previously lived only inside edit's helper, so on
// add the second flag silently received EOF and was left unset.
func TestMilestoneAdd_OnlyOneFlagMayReadStdin(t *testing.T) {
	_, runStdin := setupMilestoneWriteTestWithStdin(t)

	_, err := runStdin("context", "milestone", "add", "rel", "--body", "-", "--resolution", "-")
	if err == nil {
		t.Fatal("expected an error when two flags both ask for stdin")
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Errorf("expected a stdin-contention error, got: %v", err)
	}
}
