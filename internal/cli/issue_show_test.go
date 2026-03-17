// ABOUTME: Tests for the issue show command: UUID prefix resolution, HEAD resolution,
// ABOUTME: JSON output with parsed sections, and error on unknown ref.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// setupShowTest creates a temporary git repo and returns an App plus a run function.
// The run function executes the root command with the given args using the injected App.
func setupShowTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}

	run := func(args ...string) (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, err
	}

	return app, run
}

// createTestIssue writes an issue to the store and returns its UUID.
func createTestIssue(t *testing.T, app *cli.App, title string, state issue.State, body string) uuid.UUID {
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
		Milestone: "v0.1",
		Created:   now,
		Updated:   now,
		Body:      body,
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

func TestIssueShow_ByUUIDPrefix(t *testing.T) {
	app, run := setupShowTest(t)

	id := createTestIssue(t, app, "Fix the parser", issue.StatePending, "Parser is broken.")

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", prefix)
	if err != nil {
		t.Fatalf("issue show failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Fix the parser") {
		t.Fatalf("expected title in output, got: %s", output)
	}
	if !strings.Contains(output, string(issue.StatePending)) {
		t.Fatalf("expected state 'pending' in output, got: %s", output)
	}
}

func TestIssueShow_HEAD(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "First pending issue", issue.StatePending, "Do something.")

	// No args — resolves HEAD (first pending issue)
	stdout, err := run("issue", "show")
	if err != nil {
		t.Fatalf("issue show (HEAD) failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "First pending issue") {
		t.Fatalf("expected HEAD issue title in output, got: %s", output)
	}
}

func TestIssueShow_JsonOutput(t *testing.T) {
	app, run := setupShowTest(t)

	body := `## Prerequisites
- [ ] Review the spec
- [x] Set up environment

## Context
- paths: internal/issue, internal/cli
- docs: README.md

## Acceptance Criteria
- [ ] All tests pass
- [ ] Code reviewed
`
	id := createTestIssue(t, app, "Implement feature X", issue.StatePending, body)

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if result["title"] != "Implement feature X" {
		t.Fatalf("expected title %q, got %v", "Implement feature X", result["title"])
	}

	// Verify structured sections are present
	prereqs, ok := result["prerequisites"]
	if !ok {
		t.Fatalf("expected 'prerequisites' key in JSON, got keys: %v", keys(result))
	}
	prereqList, ok := prereqs.([]interface{})
	if !ok || len(prereqList) == 0 {
		t.Fatalf("expected non-empty prerequisites list, got: %v", prereqs)
	}

	ac, ok := result["acceptance_criteria"]
	if !ok {
		t.Fatalf("expected 'acceptance_criteria' key in JSON, got keys: %v", keys(result))
	}
	acList, ok := ac.([]interface{})
	if !ok || len(acList) == 0 {
		t.Fatalf("expected non-empty acceptance_criteria list, got: %v", ac)
	}

	// Verify context paths parsed
	ctx, ok := result["context"]
	if !ok {
		t.Fatalf("expected 'context' key in JSON, got keys: %v", keys(result))
	}
	ctxMap, ok := ctx.(map[string]interface{})
	if !ok {
		t.Fatalf("expected context to be an object, got: %v", ctx)
	}
	paths, ok := ctxMap["paths"]
	if !ok {
		t.Fatalf("expected 'paths' in context, got: %v", ctxMap)
	}
	pathList, ok := paths.([]interface{})
	if !ok || len(pathList) == 0 {
		t.Fatalf("expected non-empty paths list, got: %v", paths)
	}

	// Verify raw body is also included
	if _, ok := result["body"]; !ok {
		t.Fatalf("expected 'body' key in JSON output, got keys: %v", keys(result))
	}
}

func TestIssueShow_JsonOutput_ACSubsections(t *testing.T) {
	app, run := setupShowTest(t)

	body := `## Acceptance Criteria

### Positive Scenarios
- [ ] positional params work
- [x] error messages include line numbers

### Negative Scenarios
- [ ] rejects duplicate param names
- [ ] handles EOF mid-signature without panic
`
	id := createTestIssue(t, app, "AC subsections test", issue.StatePending, body)

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	// positive_scenarios must be present with 2 items
	pos, ok := result["positive_scenarios"]
	if !ok {
		t.Fatalf("expected 'positive_scenarios' key in JSON, got keys: %v", keys(result))
	}
	posList, ok := pos.([]interface{})
	if !ok || len(posList) != 2 {
		t.Fatalf("expected 2 positive_scenarios, got: %v", pos)
	}

	// negative_scenarios must be present with 2 items
	neg, ok := result["negative_scenarios"]
	if !ok {
		t.Fatalf("expected 'negative_scenarios' key in JSON, got keys: %v", keys(result))
	}
	negList, ok := neg.([]interface{})
	if !ok || len(negList) != 2 {
		t.Fatalf("expected 2 negative_scenarios, got: %v", neg)
	}

	// acceptance_criteria backward-compat field must contain all 4 items
	ac, ok := result["acceptance_criteria"]
	if !ok {
		t.Fatalf("expected 'acceptance_criteria' key in JSON, got keys: %v", keys(result))
	}
	acList, ok := ac.([]interface{})
	if !ok || len(acList) != 4 {
		t.Fatalf("expected 4 acceptance_criteria (union), got: %v", ac)
	}
}

func TestIssueShow_NotFound(t *testing.T) {
	_, run := setupShowTest(t)

	_, err := run("issue", "show", "deadbeef")
	if err == nil {
		t.Fatal("expected error for nonexistent ref, got nil")
	}
}

// keys returns the keys of a map for use in error messages.
func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// createTestMilestoneWithBody writes a milestone with an optional markdown body
// to the store. Used to verify milestone_context in issue show JSON output.
func createTestMilestoneWithBody(t *testing.T, app *cli.App, name string, body string) {
	t.Helper()
	ms := &milestone.Milestone{
		Name:    name,
		Created: time.Now(),
		Body:    body,
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	refPath := "refs/zhi/_/milestones/" + name
	if err := app.Store.WriteEntity(refPath, "milestone.yaml", data, "Add test milestone: "+name); err != nil {
		t.Fatalf("WriteEntity milestone: %v", err)
	}
}

func TestIssueShowIncludesMilestoneContext(t *testing.T) {
	app, run := setupShowTest(t)

	milestoneBody := "## Context\n\nThis milestone delivers the parser MVP.\n\n## Goals\n\n- Fast parsing\n- Good errors\n"
	createTestMilestoneWithBody(t, app, "v0.1", milestoneBody)

	id := createTestIssue(t, app, "Parse tokens", issue.StatePending, "Parse all the tokens.")

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	ctx, ok := result["milestone_context"]
	if !ok {
		t.Fatalf("expected 'milestone_context' key in JSON output, got keys: %v", keys(result))
	}
	ctxStr, ok := ctx.(string)
	if !ok {
		t.Fatalf("expected milestone_context to be a string, got %T: %v", ctx, ctx)
	}
	if !strings.Contains(ctxStr, "parser MVP") {
		t.Fatalf("expected milestone body in milestone_context, got: %q", ctxStr)
	}
}

func TestIssueShowMilestoneContext_NoMilestone(t *testing.T) {
	app, run := setupShowTest(t)

	// createTestIssue sets milestone to "v0.1" but we do NOT create that milestone,
	// so LoadMilestone will fail and milestone_context should be empty string.
	id := createTestIssue(t, app, "Orphan issue", issue.StatePending, "No milestone exists.")

	prefix := id.String()[:8]
	stdout, err := run("issue", "show", "--format", "json", prefix)
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	// milestone_context should be absent (omitempty empty string) or an empty string.
	if ctx, exists := result["milestone_context"]; exists {
		ctxStr, ok := ctx.(string)
		if !ok || ctxStr != "" {
			t.Fatalf("expected milestone_context to be empty string or absent, got: %v", ctx)
		}
	}
	// If the key is absent entirely (omitempty), that is also acceptable.
}

// makeShowTestCommit writes a file to the working tree, stages it, and commits.
// Returns the resulting commit SHA string. Used for lineage tests that require
// actual committed files to run git blame on.
func makeShowTestCommit(t *testing.T, repo *git.Repository, dir, filename, content, message string) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", filename, err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("git add %s: %v", filename, err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return hash.String()
}

// createDoneIssueWithSession writes a done issue to the store with a session
// covering the given SHA range. Returns the issue UUID.
func createDoneIssueWithSession(t *testing.T, app *cli.App, title, startSHA, endSHA string) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	now := time.Now()
	iss := &issue.Issue{
		ID:       id,
		Title:    title,
		State:    issue.StateDone,
		Milestone: "v0.1",
		Created:  now,
		Updated:  now,
		Sessions: []issue.Session{
			{StartSHA: startSHA, EndSHA: endSHA, Commits: 1},
		},
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("issue.Marshal: %v", err)
	}
	refPath := fmt.Sprintf("refs/zhi/_/issues/%s", id.String())
	if err := app.Store.WriteEntity(refPath, "issue.md", data, "Add done issue: "+title); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}
	return id
}

// TestIssueShowLineage_HumanOutput verifies that issue show displays an
// "Upstream lineage" section when done upstream issues exist that touched
// the files declared in the issue's context paths.
func TestIssueShowLineage_HumanOutput(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	run := func(args ...string) (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		return stdout, cmd.Execute()
	}

	// Create a file with an initial commit (session start, exclusive), then
	// a second commit adding lines (within the session, will be blamed to upstream issue).
	sha1 := makeShowTestCommit(t, repo, dir, "lexer.go", "package lexer\n", "baseline")
	sha2 := makeShowTestCommit(t, repo, dir, "lexer.go", "package lexer\n\nfunc Lex() {}\n", "implement lexer")

	// Create an upstream done issue whose session covers sha1→sha2.
	upstreamID := createDoneIssueWithSession(t, app, "Implement lexer", sha1, sha2)
	_ = upstreamID

	// Create the issue-under-test with context paths pointing to lexer.go.
	body := "## Context\n- paths: lexer.go\n\n## Acceptance Criteria\n- [ ] lexer works\n"
	id := createTestIssue(t, app, "Add error handling", issue.StatePending, body)

	// Use full UUID to avoid prefix ambiguity when issues are created within the
	// same millisecond (UUIDv7 shares millisecond prefix in that case).
	stdout, err := run("issue", "show", id.String())
	if err != nil {
		t.Fatalf("issue show failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Upstream lineage") {
		t.Fatalf("expected 'Upstream lineage' section in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Implement lexer") {
		t.Fatalf("expected upstream issue title 'Implement lexer' in lineage, got:\n%s", output)
	}
	if !strings.Contains(output, "lexer.go") {
		t.Fatalf("expected filename 'lexer.go' in lineage, got:\n%s", output)
	}
}

// TestIssueShowLineage_JSONOutput verifies that --format json includes a
// "lineage" field with upstream issue details when done issues exist that
// touched the files in the issue's context paths.
func TestIssueShowLineage_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	run := func(args ...string) (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		return stdout, cmd.Execute()
	}

	sha1 := makeShowTestCommit(t, repo, dir, "sig.go", "package sig\n", "baseline")
	sha2 := makeShowTestCommit(t, repo, dir, "sig.go", "package sig\n\nfunc Parse() {}\n", "implement parser")

	upstreamID := createDoneIssueWithSession(t, app, "Parse signatures", sha1, sha2)

	body := "## Context\n- paths: sig.go\n\n## Acceptance Criteria\n- [ ] all cases handled\n"
	id := createTestIssue(t, app, "Add signature validation", issue.StatePending, body)

	// Use full UUID to avoid prefix ambiguity when issues are created within the
	// same millisecond (UUIDv7 shares millisecond prefix in that case).
	stdout, err := run("issue", "show", "--format", "json", id.String())
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	lin, ok := result["lineage"]
	if !ok {
		t.Fatalf("expected 'lineage' key in JSON output, got keys: %v", keys(result))
	}
	linList, ok := lin.([]interface{})
	if !ok || len(linList) == 0 {
		t.Fatalf("expected non-empty lineage list, got: %v", lin)
	}

	entry, ok := linList[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected lineage entry to be object, got: %T", linList[0])
	}

	if entry["issue_id"] == nil {
		t.Fatalf("expected issue_id in lineage entry, got: %v", entry)
	}
	if entry["title"] != "Parse signatures" {
		t.Fatalf("expected title 'Parse signatures', got: %v", entry["title"])
	}
	if entry["file"] != "sig.go" {
		t.Fatalf("expected file 'sig.go', got: %v", entry["file"])
	}
	if entry["line_count"] == nil {
		t.Fatalf("expected line_count in lineage entry, got: %v", entry)
	}

	// Verify issue_id matches the upstream issue UUID.
	if idStr, ok := entry["issue_id"].(string); ok {
		if idStr != upstreamID.String() {
			t.Fatalf("expected issue_id %q, got %q", upstreamID.String(), idStr)
		}
	}
}

// TestIssueShowLineage_NoContextPaths verifies that when an issue has no
// context paths declared, the lineage section is omitted entirely from both
// human and JSON output.
func TestIssueShowLineage_NoContextPaths(t *testing.T) {
	app, run := setupShowTest(t)

	// Issue with no context paths at all.
	body := "## Acceptance Criteria\n- [ ] nothing here\n"
	id := createTestIssue(t, app, "No paths issue", issue.StatePending, body)

	// Human output: no lineage section.
	stdout, err := run("issue", "show", id.String())
	if err != nil {
		t.Fatalf("issue show failed: %v", err)
	}
	if strings.Contains(stdout.String(), "Upstream lineage") {
		t.Fatalf("unexpected 'Upstream lineage' section for issue with no context paths, got:\n%s", stdout.String())
	}

	// JSON output: no lineage key.
	stdout, err = run("issue", "show", "--format", "json", id.String())
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}
	if lin, exists := result["lineage"]; exists {
		linList, ok := lin.([]interface{})
		if ok && len(linList) > 0 {
			t.Fatalf("expected lineage to be absent or empty for no-paths issue, got: %v", lin)
		}
	}
}
