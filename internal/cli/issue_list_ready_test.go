// ABOUTME: Tests for `issue list --ready`, the unblocked-work query a DAG
// ABOUTME: driver needs every turn.

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// readyIDs runs issue list --ready --format json and returns the ids.
func readyIDs(t *testing.T, run func(args ...string) (*strings.Builder, error), extra ...string) []string {
	t.Helper()
	args := append([]string{"issue", "list", "--ready", "--format", "json"}, extra...)
	stdout, err := run(args...)
	if err != nil {
		t.Fatalf("issue list --ready: %v", err)
	}
	var raw json.RawMessage = json.RawMessage(stdout.String())
	var asList []map[string]interface{}
	if err := json.Unmarshal(raw, &asList); err != nil {
		var wrapped struct {
			Issues []map[string]interface{} `json:"issues"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil {
			t.Fatalf("unmarshal: %v / %v\n%s", err, err2, stdout.String())
		}
		asList = wrapped.Issues
	}
	ids := make([]string, 0, len(asList))
	for _, m := range asList {
		ids = append(ids, m["id"].(string))
	}
	return ids
}

// TestIssueListReady_ExcludesBlocked verifies --ready returns only issues whose
// blockers are all resolved.
func TestIssueListReady_ExcludesBlocked(t *testing.T) {
	app, run := setupReadyTest(t)

	upstream := createReadyIssue(t, app, "upstream", "m1", nil)
	downstream := createReadyIssue(t, app, "downstream", "m1", []string{upstream})

	ids := readyIDs(t, run)
	if len(ids) != 1 || ids[0] != upstream {
		t.Fatalf("ready = %v, want only the unblocked upstream %s", ids, upstream)
	}
	for _, id := range ids {
		if id == downstream {
			t.Error("blocked issue appeared in the ready set")
		}
	}
}

// TestIssueListReady_UnblocksWhenUpstreamDone verifies finishing a blocker
// moves the downstream issue into the ready set.
func TestIssueListReady_UnblocksWhenUpstreamDone(t *testing.T) {
	app, run := setupReadyTest(t)

	upstream := createReadyIssue(t, app, "upstream", "m1", nil)
	downstream := createReadyIssue(t, app, "downstream", "m1", []string{upstream})

	if _, err := run("issue", "edit", upstream, "--state", "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := run("issue", "edit", upstream, "--state", "done", "--force"); err != nil {
		t.Fatalf("done: %v", err)
	}

	ids := readyIDs(t, run)
	if len(ids) != 1 || ids[0] != downstream {
		t.Fatalf("ready = %v, want the now-unblocked downstream %s", ids, downstream)
	}
}

// TestIssueListReady_SpansMilestonesByDefault verifies the ready set is not
// scoped to one milestone unless asked, which is what a driver wants.
func TestIssueListReady_SpansMilestonesByDefault(t *testing.T) {
	app, run := setupReadyTest(t)

	a := createReadyIssue(t, app, "in m1", "m1", nil)
	b := createReadyIssue(t, app, "in m2", "m2", nil)

	all := readyIDs(t, run)
	if len(all) != 2 {
		t.Fatalf("ready across milestones = %v, want both %s and %s", all, a, b)
	}

	scoped := readyIDs(t, run, "--milestone", "m2")
	if len(scoped) != 1 || scoped[0] != b {
		t.Fatalf("ready --milestone m2 = %v, want only %s", scoped, b)
	}
}

// TestIssueListReady_ExcludesDoneAndInProgress verifies the ready set is work
// that can be picked up, not work already finished or under way.
func TestIssueListReady_ExcludesDoneAndInProgress(t *testing.T) {
	app, run := setupReadyTest(t)

	started := createReadyIssue(t, app, "started", "m1", nil)
	idle := createReadyIssue(t, app, "idle", "m1", nil)

	if _, err := run("issue", "edit", started, "--state", "start"); err != nil {
		t.Fatalf("start: %v", err)
	}

	ids := readyIDs(t, run)
	if len(ids) != 1 || ids[0] != idle {
		t.Fatalf("ready = %v, want only the untouched issue %s", ids, idle)
	}
}

// TestIssueListReady_EmitsBlockedByField verifies the reverse-edge field is
// present and named, so a caller can see the shape without guessing.
func TestIssueListReady_EmitsBlockedByField(t *testing.T) {
	app, run := setupReadyTest(t)

	upstream := createReadyIssue(t, app, "upstream", "m1", nil)
	createReadyIssue(t, app, "downstream", "m1", []string{upstream})

	stdout, err := run("issue", "list", "--format", "json")
	if err != nil {
		t.Fatalf("issue list: %v", err)
	}
	if !strings.Contains(stdout.String(), "blocked_by") {
		t.Errorf("issue list JSON never names blocked_by; a ready query has to guess it:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), string(issue.StatePending)) {
		t.Errorf("expected pending issues in output")
	}
}

// setupReadyTest returns an App and a runner over a fresh chain.
func setupReadyTest(t *testing.T) (*cli.App, func(args ...string) (*strings.Builder, error)) {
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
	// An initial commit so RepoHEAD() resolves for --state transitions.
	makeTestCommit(t, app, "initial commit")

	run := func(args ...string) (*strings.Builder, error) {
		out := new(strings.Builder)
		cmd := cli.NewRootCommand()
		cmd.SetOut(out)
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		return out, cmd.Execute()
	}
	return app, run
}

// createReadyIssue writes a pending issue, wiring blockedBy edges in both
// directions so the graph matches what the CLI would have produced.
func createReadyIssue(t *testing.T, app *cli.App, title, ms string, blockedBy []string) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     issue.StatePending,
		Milestone: ms,
		Created:   now,
		Updated:   now,
	}
	for _, up := range blockedBy {
		upID := uuid.Must(uuid.FromString(up))
		iss.BlockedBy = append(iss.BlockedBy, upID)

		upRef := issue.RefPrefix + up
		raw, err := app.Store.ReadEntity(upRef, "issue.md")
		if err != nil {
			t.Fatalf("read upstream: %v", err)
		}
		upIss, err := issue.Parse(raw)
		if err != nil {
			t.Fatalf("parse upstream: %v", err)
		}
		upIss.Blocks = append(upIss.Blocks, id)
		out, err := issue.Marshal(upIss)
		if err != nil {
			t.Fatalf("marshal upstream: %v", err)
		}
		if err := app.Store.WriteEntity(upRef, "issue.md", out, "link"); err != nil {
			t.Fatalf("write upstream: %v", err)
		}
	}

	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := app.Store.WriteEntity(issue.RefPrefix+id.String(), "issue.md", data, "add "+title); err != nil {
		t.Fatalf("write: %v", err)
	}
	return id.String()
}

// TestIssueAdd_BodyFile verifies --body-file reads the body from a path, so a
// multi-line markdown body never has to survive shell quoting.
func TestIssueAdd_BodyFile(t *testing.T) {
	_, run := setupReadyTest(t)

	path := filepath.Join(t.TempDir(), "body.md")
	body := "## Context\n\nParens (like this) and `backticks` and 100% signs.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	if _, err := run("issue", "add", "Filed from a file", "--body-file", path); err != nil {
		t.Fatalf("issue add --body-file: %v", err)
	}

	stdout, err := run("issue", "list", "--format", "json")
	if err != nil {
		t.Fatalf("issue list: %v", err)
	}
	if !strings.Contains(stdout.String(), "Parens (like this)") {
		t.Errorf("body from file not stored:\n%s", stdout.String())
	}
}

// TestIssueEdit_BodyFile verifies the same flag replaces an existing body
// without consuming stdin, which matters when stdin is already a pipe.
func TestIssueEdit_BodyFile(t *testing.T) {
	app, run := setupReadyTest(t)

	id := createReadyIssue(t, app, "original", "m1", nil)

	path := filepath.Join(t.TempDir(), "new.md")
	if err := os.WriteFile(path, []byte("replaced from a file\n"), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	if _, err := run("issue", "edit", id, "--body-file", path); err != nil {
		t.Fatalf("issue edit --body-file: %v", err)
	}

	raw, err := app.Store.ReadEntity(issue.RefPrefix+id, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity: %v", err)
	}
	iss, err := issue.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(iss.Body, "replaced from a file") {
		t.Errorf("body = %q", iss.Body)
	}
}

// TestIssueListReady_BlockerInAnotherMilestoneStillCounts is the semantic the
// --ready design exists for: readiness is computed over the whole graph, so a
// blocker excluded by --milestone still blocks. Replacing filterReady's
// LoadAllIssues with the already-filtered slice passes every other test here
// but fails this one.
func TestIssueListReady_BlockerInAnotherMilestoneStillCounts(t *testing.T) {
	app, run := setupReadyTest(t)

	up := createReadyIssue(t, app, "upstream", "m1", nil)
	down := createReadyIssue(t, app, "downstream", "m2", []string{up})

	if ids := readyIDs(t, run, "--milestone", "m2"); len(ids) != 0 {
		t.Fatalf("ready --milestone m2 = %v, want empty: %s is blocked by %s in m1", ids, down, up)
	}
}

// TestIssueEdit_BodyFileEmptyIsRefused verifies an empty file errors rather
// than falling through to the $EDITOR sentinel, which would hang a scripted
// caller on vi.
func TestIssueEdit_BodyFileEmptyIsRefused(t *testing.T) {
	app, run := setupReadyTest(t)

	id := createReadyIssue(t, app, "target", "m1", nil)
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, []byte("   \n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := run("issue", "edit", id, "--body-file", empty)
	if err == nil {
		t.Fatal("expected an empty --body-file to be refused")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected an empty-file error, got: %v", err)
	}
}

// TestIssueListReady_IncludesReopened is the CLI-level counterpart to the
// graph test. Widening ReadySet alone was not enough: issue list applied its
// default state filter first, dropping reopened issues before the ready filter
// could consider them, so the flag's own help text was still false.
func TestIssueListReady_IncludesReopened(t *testing.T) {
	app, run := setupReadyTest(t)

	id := createReadyIssue(t, app, "revisit", "m1", nil)
	for _, action := range []string{"start", "done", "reopen"} {
		args := []string{"issue", "edit", id, "--state", action}
		if action == "done" {
			args = append(args, "--force")
		}
		if _, err := run(args...); err != nil {
			t.Fatalf("--state %s: %v", action, err)
		}
	}

	ids := readyIDs(t, run)
	if len(ids) != 1 || ids[0] != id {
		t.Fatalf("ready = %v, want the reopened issue %s", ids, id)
	}
}

// TestIssueAdd_BodyFileWithoutTitleErrors verifies the flag does not fall
// through to a blocking stdin read. On a terminal that hangs until Ctrl-D;
// under a pipe it reports a stdin problem that never mentions the flag.
func TestIssueAdd_BodyFileWithoutTitleErrors(t *testing.T) {
	_, run := setupReadyTest(t)

	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("some body\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := run("issue", "add", "--body-file", path)
	if err == nil {
		t.Fatal("expected --body-file without a title to be refused")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should name the missing title, got: %v", err)
	}
}
