// ABOUTME: Integration tests for the git-zhi-sanbao Cobra command.
// ABOUTME: Uses real on-disk git repos and exercises format, domain filter, and output.
package sanbao_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/sanbao"
	"github.com/perigrin/git-zhi/internal/storage"
)

// setupSanbaoTest initialises a real on-disk git repo with an initial commit,
// creates a store, and returns an App plus a run helper for the sanbao command.
func setupSanbaoTest(t *testing.T) (*cli.App, func(args ...string) (string, string, error)) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(dir+"/seed.txt", []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if _, err := wt.Add("seed.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}

	run := func(args ...string) (string, string, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := sanbao.NewSanbaoCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}
	return app, run
}

func writeSanbaoMilestone(t *testing.T, store *storage.Store, ms *milestone.Milestone) {
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

func writeSanbaoIssue(t *testing.T, store *storage.Store, iss *issue.Issue) {
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

func newSanbaoDoneIssue(t *testing.T, msName string) *issue.Issue {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	now := time.Now()
	return &issue.Issue{
		ID:        id,
		Title:     "Sanbao Test Issue",
		State:     issue.StateDone,
		Urgency:   issue.UrgencyNormal,
		Milestone: msName,
		Created:   now.Add(-48 * time.Hour),
		Updated:   now,
		Sessions:  []issue.Session{},
		Transitions: []issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:test", Timestamp: now.Add(-24 * time.Hour)},
			{State: string(issue.StateDone), Actor: "human:test", Timestamp: now},
		},
		ObservedPaths: []string{},
		Body:          "## Acceptance Criteria\n\n- [ ] echo works (`echo hello`)\n",
	}
}

// TestSanbaoCLI_HumanOutput verifies the default human-readable output contains
// the expected section headers.
func TestSanbaoCLI_HumanOutput(t *testing.T) {
	app, run := setupSanbaoTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeSanbaoMilestone(t, app.Store, ms)
	writeSanbaoIssue(t, app.Store, newSanbaoDoneIssue(t, "v0.1"))

	stdout, _, err := run("v0.1")
	if err != nil {
		t.Fatalf("sanbao v0.1 failed: %v", err)
	}

	if !strings.Contains(stdout, "DORA") {
		t.Errorf("expected DORA section in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Efficiency") {
		t.Errorf("expected Efficiency section in output, got:\n%s", stdout)
	}
}

// TestSanbaoCLI_JSONOutput verifies --format json produces valid JSON with all
// expected top-level keys.
func TestSanbaoCLI_JSONOutput(t *testing.T) {
	app, run := setupSanbaoTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeSanbaoMilestone(t, app.Store, ms)
	writeSanbaoIssue(t, app.Store, newSanbaoDoneIssue(t, "v0.1"))

	stdout, _, err := run("v0.1", "--format", "json")
	if err != nil {
		t.Fatalf("sanbao --format json failed: %v", err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput was:\n%s", err, stdout)
	}

	for _, key := range []string{"milestone", "dora", "space", "calms", "sentiment", "complexity", "difficulties"} {
		if _, ok := out[key]; !ok {
			t.Errorf("JSON output missing key %q", key)
		}
	}
}

// TestSanbaoCLI_DomainFilterDORA verifies --domain dora shows DORA metrics and
// omits the Efficiency (SPACE) section.
func TestSanbaoCLI_DomainFilterDORA(t *testing.T) {
	app, run := setupSanbaoTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeSanbaoMilestone(t, app.Store, ms)
	writeSanbaoIssue(t, app.Store, newSanbaoDoneIssue(t, "v0.1"))

	stdout, _, err := run("v0.1", "--domain", "dora")
	if err != nil {
		t.Fatalf("sanbao --domain dora failed: %v", err)
	}

	if !strings.Contains(stdout, "DORA") {
		t.Errorf("expected DORA in filtered output, got:\n%s", stdout)
	}
	// SPACE section must NOT appear when domain=dora.
	if strings.Contains(stdout, "Efficiency") {
		t.Errorf("expected Efficiency section to be absent with --domain dora, got:\n%s", stdout)
	}
}

// TestSanbaoCLI_DomainFilterJSON verifies --domain dora with --format json
// returns only the dora sub-object (space, calms etc. must be absent).
func TestSanbaoCLI_DomainFilterJSON(t *testing.T) {
	app, run := setupSanbaoTest(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now(),
		State:   "open",
	}
	writeSanbaoMilestone(t, app.Store, ms)
	writeSanbaoIssue(t, app.Store, newSanbaoDoneIssue(t, "v0.1"))

	stdout, _, err := run("v0.1", "--domain", "dora", "--format", "json")
	if err != nil {
		t.Fatalf("sanbao --domain dora --format json failed: %v", err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput was:\n%s", err, stdout)
	}

	if _, ok := out["dora"]; !ok {
		t.Error("JSON output missing 'dora' key when --domain dora")
	}
	if _, ok := out["space"]; ok {
		t.Error("JSON output contains 'space' key when --domain dora; expected absent")
	}
}

// TestSanbaoCLI_MissingMilestoneArg verifies that omitting the milestone
// positional arg returns an error.
func TestSanbaoCLI_MissingMilestoneArg(t *testing.T) {
	_, run := setupSanbaoTest(t)
	_, _, err := run()
	if err == nil {
		t.Fatal("expected error when milestone arg is missing")
	}
}

// TestSanbaoCLI_UnknownMilestone verifies an error is returned for a milestone
// that does not exist in the store.
func TestSanbaoCLI_UnknownMilestone(t *testing.T) {
	_, run := setupSanbaoTest(t)
	_, _, err := run("nonexistent-milestone")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone")
	}
}
