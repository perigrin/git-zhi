# Issue Show Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `issue show <ref>` with UUID prefix + HEAD resolution, human-readable and JSON output with structured body section parsing.

**Architecture:** Resolve package gains `ResolveRef` for UUID prefix and HEAD lookups. Issue package gains `ParseSections` for extracting Prerequisites/Context/Acceptance Criteria from markdown body. CLI gets a new `issue_show.go` with human and JSON formatters.

**Tech Stack:** Go 1.24+, existing storage/issue/resolve packages

---

## File Structure

```
internal/
  resolve/
    resolve.go               # Modify: add ResolveRef, resolveHead, resolveUUIDPrefix
    resolve_test.go           # Modify: add 6 resolution tests
  issue/
    sections.go              # Create: ParseSections, Checkbox, StructuredContext types
    sections_test.go         # Create: 5 section parsing tests
  cli/
    issue.go                 # Modify: replace show stub, wire to issue_show.go
    issue_show.go            # Create: runIssueShow, human/JSON formatters, IssueJSON type
    issue_show_test.go       # Create: 4 end-to-end tests
```

---

## Task 1: Implement ResolveRef in resolve package

**Files:**
- Modify: `internal/resolve/resolve.go`
- Modify: `internal/resolve/resolve_test.go`

- [ ] **Step 1: Write resolve tests**

Add to `internal/resolve/resolve_test.go`. Need to import storage, issue, git, and uuid packages. Add a helper to create a test repo with issues:

```go
func setupResolveRepo(t *testing.T) (*storage.Store, []string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Create three issues with known UUIDs via issue.Marshal
	var refPaths []string
	states := []issue.State{issue.StatePending, issue.StateInProgress, issue.StatePending}
	titles := []string{"First issue", "Second issue", "Third issue"}
	for i := 0; i < 3; i++ {
		id, _ := uuid.NewV7()
		iss := &issue.Issue{
			ID:        id,
			Title:     titles[i],
			State:     states[i],
			Milestone: "v0.1",
			Created:   time.Now(),
			Updated:   time.Now(),
		}
		data, _ := issue.Marshal(iss)
		refPath := "refs/chain/_/issues/" + id.String()
		store.WriteEntity(refPath, "issue.md", data, "create "+titles[i])
		refPaths = append(refPaths, refPath)
	}
	return store, refPaths
}

func TestResolveRef_UUIDPrefix(t *testing.T) {
	store, refPaths := setupResolveRepo(t)
	// Extract UUID from first ref path
	fullUUID := strings.TrimPrefix(refPaths[0], "refs/chain/_/issues/")
	prefix := fullUUID[:8]

	result, err := resolve.ResolveRef(store, prefix)
	if err != nil {
		t.Fatalf("ResolveRef failed: %v", err)
	}
	if result != refPaths[0] {
		t.Fatalf("expected %s, got %s", refPaths[0], result)
	}
}

func TestResolveRef_NotFound(t *testing.T) {
	store, _ := setupResolveRepo(t)
	_, err := resolve.ResolveRef(store, "ffffffff")
	if err == nil {
		t.Fatal("expected error for nonexistent prefix")
	}
}

func TestResolveRef_HEAD_InProgress(t *testing.T) {
	store, refPaths := setupResolveRepo(t)
	// Second issue is in-progress
	result, err := resolve.ResolveRef(store, "")
	if err != nil {
		t.Fatalf("HEAD resolve failed: %v", err)
	}
	if result != refPaths[1] {
		t.Fatalf("expected HEAD to resolve to in-progress issue %s, got %s", refPaths[1], result)
	}
}

func TestResolveRef_HEAD_Pending(t *testing.T) {
	dir := t.TempDir()
	repo, _ := git.PlainInit(dir, false)
	store, _ := storage.NewStore(repo)

	// Create only pending issues
	for _, title := range []string{"Alpha", "Beta"} {
		id, _ := uuid.NewV7()
		iss := &issue.Issue{
			ID: id, Title: title, State: issue.StatePending,
			Milestone: "v0.1", Created: time.Now(), Updated: time.Now(),
		}
		data, _ := issue.Marshal(iss)
		store.WriteEntity("refs/chain/_/issues/"+id.String(), "issue.md", data, "create")
		time.Sleep(time.Millisecond) // ensure distinct UUIDv7 timestamps
	}

	result, err := resolve.ResolveRef(store, "HEAD")
	if err != nil {
		t.Fatalf("HEAD resolve failed: %v", err)
	}
	// Should resolve to first pending by UUID sort (Alpha, created first)
	content, _ := store.ReadEntity(result, "issue.md")
	iss, _ := issue.Parse(content)
	if iss.Title != "Alpha" {
		t.Fatalf("expected HEAD to resolve to first pending 'Alpha', got %q", iss.Title)
	}
}

func TestResolveRef_HEAD_Empty(t *testing.T) {
	dir := t.TempDir()
	repo, _ := git.PlainInit(dir, false)
	store, _ := storage.NewStore(repo)

	_, err := resolve.ResolveRef(store, "")
	if err == nil {
		t.Fatal("expected error when no issues exist")
	}
}
```

Note: `TestResolveRef_AmbiguousPrefix` is hard to construct deterministically with UUIDv7 (you can't control the prefix). Skip it for now — the code handles it but the test is impractical. The 5 tests above provide solid coverage.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/issue-show && go test ./internal/resolve/...`

- [ ] **Step 3: Implement ResolveRef**

Replace `internal/resolve/resolve.go`:

```go
// ABOUTME: Ref argument resolution for CLI commands. Resolves user input
// ABOUTME: (HEAD, UUID prefix) to an entity ref path. Tags and title added in issue 10.
package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/storage"
)

// IsHead returns true if the input resolves to the HEAD reference.
// An empty ref argument means the caller passed no explicit target,
// which resolves to HEAD — the current in-progress issue or next on
// the critical chain.
func IsHead(input string) bool {
	return input == "" || input == "HEAD"
}

// ResolveRef resolves a user-provided ref to a full entity ref path.
// Resolution order: HEAD → UUID prefix scan.
// Tag and title resolution added in issue 10.
func ResolveRef(store *storage.Store, input string) (string, error) {
	if IsHead(input) {
		return resolveHead(store)
	}
	return resolveUUIDPrefix(store, input)
}

// resolveHead finds the current HEAD issue: first in-progress issue,
// or first pending issue by lexicographic UUID order (UUIDv7 sorts by
// creation time). This is a pre-graph placeholder; issue 7 replaces
// it with critical-chain-aware HEAD resolution.
func resolveHead(store *storage.Store) (string, error) {
	refs, err := store.ListRefs("refs/chain/_/issues/")
	if err != nil {
		return "", fmt.Errorf("list issues: %w", err)
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("no issues found")
	}

	sort.Strings(refs)

	// First pass: find in-progress issue
	for _, ref := range refs {
		content, err := store.ReadEntity(ref, "issue.md")
		if err != nil {
			continue
		}
		iss, err := issue.Parse(content)
		if err != nil {
			continue
		}
		if iss.State == issue.StateInProgress {
			return ref, nil
		}
	}

	// Second pass: first pending issue by UUID sort
	for _, ref := range refs {
		content, err := store.ReadEntity(ref, "issue.md")
		if err != nil {
			continue
		}
		iss, err := issue.Parse(content)
		if err != nil {
			continue
		}
		if iss.State == issue.StatePending {
			return ref, nil
		}
	}

	return "", fmt.Errorf("no in-progress or pending issues found")
}

// resolveUUIDPrefix scans issue refs for a unique prefix match.
func resolveUUIDPrefix(store *storage.Store, prefix string) (string, error) {
	refs, err := store.ListRefs("refs/chain/_/issues/")
	if err != nil {
		return "", fmt.Errorf("list issues: %w", err)
	}

	var matches []string
	for _, ref := range refs {
		uuid := strings.TrimPrefix(ref, "refs/chain/_/issues/")
		if strings.HasPrefix(uuid, prefix) {
			matches = append(matches, ref)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no issue found matching prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = strings.TrimPrefix(m, "refs/chain/_/issues/")[:8]
		}
		return "", fmt.Errorf("ambiguous prefix %q matches %d issues: %s", prefix, len(matches), strings.Join(ids, ", "))
	}
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/issue-show && go test ./internal/resolve/... -v`
Expected: PASS (6 tests — 1 existing + 5 new)

- [ ] **Step 5: Commit**

```bash
git add internal/resolve/
git commit -m "Implement ResolveRef with UUID prefix and HEAD resolution

HEAD resolves to first in-progress issue, or first pending by UUID
sort order. UUID prefix scans issue refs for unique match. Ambiguous
prefixes return error with candidates."
```

---

## Task 2: Implement ParseSections in issue package

**Files:**
- Create: `internal/issue/sections.go`
- Create: `internal/issue/sections_test.go`

- [ ] **Step 1: Write sections tests**

Create `internal/issue/sections_test.go`:

```go
// ABOUTME: Tests for body section parsing: extracts Prerequisites, Context,
// ABOUTME: and Acceptance Criteria from markdown body into structured types.
package issue_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestParseSections_Full(t *testing.T) {
	body := `## Prerequisites

- [x] lexer complete
- [ ] staging deployed

## Context

- paths: lib/Parser.pm, t/parser/
- docs: docs/parser-design.md
- commands: prove -lv t/parser/
- entrypoints: lib/Parser.pm

Implement basic signature parsing.

## Acceptance Criteria

- [ ] positional params work
- [x] error messages include line numbers`

	s := issue.ParseSections(body)

	if len(s.Prerequisites) != 2 {
		t.Fatalf("expected 2 prerequisites, got %d", len(s.Prerequisites))
	}
	if !s.Prerequisites[0].Checked || s.Prerequisites[0].Text != "lexer complete" {
		t.Fatalf("prereq 0: expected checked 'lexer complete', got checked=%v text=%q",
			s.Prerequisites[0].Checked, s.Prerequisites[0].Text)
	}
	if s.Prerequisites[1].Checked {
		t.Fatal("prereq 1 should be unchecked")
	}

	if s.Context == nil {
		t.Fatal("expected non-nil context")
	}
	if len(s.Context.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(s.Context.Paths))
	}
	if len(s.Context.Commands) != 1 || s.Context.Commands[0] != "prove -lv t/parser/" {
		t.Fatalf("expected 1 command 'prove -lv t/parser/', got %v", s.Context.Commands)
	}

	if len(s.AcceptanceCriteria) != 2 {
		t.Fatalf("expected 2 acceptance criteria, got %d", len(s.AcceptanceCriteria))
	}

	if s.Description == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestParseSections_CheckboxParsing(t *testing.T) {
	body := `## Prerequisites

- [x] done item
- [ ] pending item
- [X] also done (capital X)`

	s := issue.ParseSections(body)
	if len(s.Prerequisites) != 3 {
		t.Fatalf("expected 3 checkboxes, got %d", len(s.Prerequisites))
	}
	if !s.Prerequisites[0].Checked {
		t.Fatal("first should be checked")
	}
	if s.Prerequisites[1].Checked {
		t.Fatal("second should be unchecked")
	}
	if !s.Prerequisites[2].Checked {
		t.Fatal("third (capital X) should be checked")
	}
}

func TestParseSections_ContextParsing(t *testing.T) {
	body := `## Context

- paths: src/main.go, src/util.go
- docs: README.md
- commands: go test ./..., go vet ./...
- entrypoints: src/main.go`

	s := issue.ParseSections(body)
	if s.Context == nil {
		t.Fatal("expected non-nil context")
	}
	if len(s.Context.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(s.Context.Paths), s.Context.Paths)
	}
	if len(s.Context.Docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(s.Context.Docs))
	}
	if len(s.Context.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(s.Context.Commands))
	}
	if len(s.Context.Entrypoints) != 1 {
		t.Fatalf("expected 1 entrypoint, got %d", len(s.Context.Entrypoints))
	}
}

func TestParseSections_EmptyBody(t *testing.T) {
	s := issue.ParseSections("")
	if len(s.Prerequisites) != 0 {
		t.Fatalf("expected 0 prerequisites, got %d", len(s.Prerequisites))
	}
	if s.Context != nil {
		t.Fatal("expected nil context for empty body")
	}
	if len(s.AcceptanceCriteria) != 0 {
		t.Fatalf("expected 0 acceptance criteria, got %d", len(s.AcceptanceCriteria))
	}
}

func TestParseSections_NoSections(t *testing.T) {
	body := "Just some plain text description without any sections."
	s := issue.ParseSections(body)
	if s.Description != body {
		t.Fatalf("expected body as description, got %q", s.Description)
	}
	if len(s.Prerequisites) != 0 || len(s.AcceptanceCriteria) != 0 {
		t.Fatal("expected no prerequisites or acceptance criteria")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement ParseSections**

Create `internal/issue/sections.go`:

```go
// ABOUTME: Parses structured sections from issue markdown body. Extracts
// ABOUTME: Prerequisites, Context, and Acceptance Criteria for JSON output.
package issue

import (
	"strings"
)

// Sections represents the parsed body sections of an issue.
type Sections struct {
	Prerequisites      []Checkbox         `json:"prerequisites,omitempty"`
	Context            *StructuredContext  `json:"context,omitempty"`
	AcceptanceCriteria []Checkbox         `json:"acceptance_criteria,omitempty"`
	Description        string             `json:"description,omitempty"`
}

// Checkbox represents a markdown checkbox item: - [x] or - [ ].
type Checkbox struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// StructuredContext holds extracted context fields from the ## Context section.
type StructuredContext struct {
	Paths       []string `json:"paths,omitempty"`
	Docs        []string `json:"docs,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Entrypoints []string `json:"entrypoints,omitempty"`
}

// ParseSections extracts structured sections from a markdown body.
// Splits on ## headings and matches Prerequisites, Context, and
// Acceptance Criteria. Everything else becomes the description.
// Malformed sections produce empty fields, not errors.
func ParseSections(body string) *Sections {
	s := &Sections{}
	if strings.TrimSpace(body) == "" {
		return s
	}

	// Split body into sections by ## headings
	type section struct {
		name string
		body string
	}
	var sections []section
	lines := strings.Split(body, "\n")
	var currentName string
	var currentLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			// Save previous section
			if currentName != "" || len(currentLines) > 0 {
				sections = append(sections, section{currentName, strings.TrimSpace(strings.Join(currentLines, "\n"))})
			}
			currentName = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	// Save last section
	if currentName != "" || len(currentLines) > 0 {
		sections = append(sections, section{currentName, strings.TrimSpace(strings.Join(currentLines, "\n"))})
	}

	var descParts []string
	for _, sec := range sections {
		switch strings.ToLower(sec.name) {
		case "prerequisites":
			s.Prerequisites = parseCheckboxes(sec.body)
		case "acceptance criteria":
			s.AcceptanceCriteria = parseCheckboxes(sec.body)
		case "context":
			s.Context = parseContext(sec.body)
			// Any non-key lines in context become description
			for _, line := range strings.Split(sec.body, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" && !strings.HasPrefix(trimmed, "- paths:") &&
					!strings.HasPrefix(trimmed, "- docs:") &&
					!strings.HasPrefix(trimmed, "- commands:") &&
					!strings.HasPrefix(trimmed, "- entrypoints:") {
					descParts = append(descParts, trimmed)
				}
			}
		default:
			// Unnamed section or unrecognized heading → description
			if sec.body != "" {
				descParts = append(descParts, sec.body)
			}
		}
	}
	s.Description = strings.TrimSpace(strings.Join(descParts, "\n"))
	return s
}

func parseCheckboxes(text string) []Checkbox {
	var boxes []Checkbox
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			boxes = append(boxes, Checkbox{
				Text:    strings.TrimSpace(trimmed[6:]),
				Checked: true,
			})
		} else if strings.HasPrefix(trimmed, "- [ ] ") {
			boxes = append(boxes, Checkbox{
				Text:    strings.TrimSpace(trimmed[6:]),
				Checked: false,
			})
		}
	}
	return boxes
}

func parseContext(text string) *StructuredContext {
	ctx := &StructuredContext{}
	hasContent := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- paths:") {
			ctx.Paths = splitCSV(strings.TrimPrefix(trimmed, "- paths:"))
			hasContent = true
		} else if strings.HasPrefix(trimmed, "- docs:") {
			ctx.Docs = splitCSV(strings.TrimPrefix(trimmed, "- docs:"))
			hasContent = true
		} else if strings.HasPrefix(trimmed, "- commands:") {
			ctx.Commands = splitCSV(strings.TrimPrefix(trimmed, "- commands:"))
			hasContent = true
		} else if strings.HasPrefix(trimmed, "- entrypoints:") {
			ctx.Entrypoints = splitCSV(strings.TrimPrefix(trimmed, "- entrypoints:"))
			hasContent = true
		}
	}
	if !hasContent {
		return nil
	}
	return ctx
}

func splitCSV(s string) []string {
	parts := strings.Split(strings.TrimSpace(s), ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests, verify pass**

Expected: PASS (15 issue tests — 10 existing + 5 new)

- [ ] **Step 5: Commit**

```bash
git add internal/issue/sections.go internal/issue/sections_test.go
git commit -m "Implement ParseSections for structured body extraction

Extracts Prerequisites and Acceptance Criteria as checkbox arrays,
Context as structured paths/docs/commands/entrypoints, and remaining
text as description. Loose parsing: malformed sections produce empty
fields, not errors."
```

---

## Task 3: Implement issue show command

**Files:**
- Create: `internal/cli/issue_show.go`
- Create: `internal/cli/issue_show_test.go`
- Modify: `internal/cli/issue.go` (wire show stub to real impl)

- [ ] **Step 1: Write issue show tests**

Create `internal/cli/issue_show_test.go`:

```go
// ABOUTME: End-to-end tests for the issue show command: display by UUID prefix,
// ABOUTME: HEAD resolution, JSON output with structured sections, and not-found error.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/cli"
	"github.com/perigrin/git-chain/internal/issue"
)

// createTestIssue is a helper that creates an issue in the store and returns its UUID.
func createTestIssue(t *testing.T, app *cli.App, title string, state issue.State, body string) uuid.UUID {
	t.Helper()
	id, _ := uuid.NewV7()
	iss := &issue.Issue{
		ID:        id,
		Title:     title,
		State:     state,
		Milestone: "v0.1",
		Created:   time.Now(),
		Updated:   time.Now(),
		Body:      body,
	}
	data, err := issue.Marshal(iss)
	if err != nil {
		t.Fatalf("marshal issue: %v", err)
	}
	if err := app.Store.WriteEntity("refs/chain/_/issues/"+id.String(), "issue.md", data, "create"); err != nil {
		t.Fatalf("write issue: %v", err)
	}
	return id
}

func setupShowTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
	t.Helper()
	dir := t.TempDir()
	repo, _ := git.PlainInit(dir, false)
	app, _ := cli.OpenRepo(dir)
	_ = repo

	run := func(args ...string) (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, err
	}

	return app, run
}

func TestIssueShow_ByUUIDPrefix(t *testing.T) {
	app, run := setupShowTest(t)
	app.EnsureInitialized()
	id := createTestIssue(t, app, "Show me this issue", issue.StatePending, "Some body text.")
	prefix := id.String()[:8]

	stdout, err := run("issue", "show", prefix)
	if err != nil {
		t.Fatalf("issue show failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Show me this issue") {
		t.Fatalf("expected title in output, got:\n%s", output)
	}
	if !strings.Contains(output, "pending") {
		t.Fatalf("expected state in output, got:\n%s", output)
	}
}

func TestIssueShow_HEAD(t *testing.T) {
	app, run := setupShowTest(t)
	app.EnsureInitialized()
	createTestIssue(t, app, "The pending issue", issue.StatePending, "")

	// No arg = HEAD
	stdout, err := run("issue", "show")
	if err != nil {
		t.Fatalf("issue show HEAD failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "The pending issue") {
		t.Fatalf("expected HEAD to resolve to pending issue, got:\n%s", stdout.String())
	}
}

func TestIssueShow_JsonOutput(t *testing.T) {
	app, run := setupShowTest(t)
	app.EnsureInitialized()
	body := "## Prerequisites\n\n- [x] done thing\n- [ ] pending thing\n\n## Context\n\n- paths: src/main.go\n- commands: go test\n\nDo the work.\n\n## Acceptance Criteria\n\n- [ ] it works"
	id := createTestIssue(t, app, "JSON show test", issue.StatePending, body)

	stdout, err := run("issue", "show", id.String()[:8], "--format", "json")
	if err != nil {
		t.Fatalf("issue show --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if result["title"] != "JSON show test" {
		t.Fatalf("expected title 'JSON show test', got %v", result["title"])
	}
	// Verify structured sections exist
	if result["prerequisites"] == nil {
		t.Fatal("expected prerequisites in JSON output")
	}
	if result["context"] == nil {
		t.Fatal("expected context in JSON output")
	}
	if result["acceptance_criteria"] == nil {
		t.Fatal("expected acceptance_criteria in JSON output")
	}
}

func TestIssueShow_NotFound(t *testing.T) {
	app, run := setupShowTest(t)
	app.EnsureInitialized()

	_, err := run("issue", "show", "ffffffff")
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement issue_show.go**

Create `internal/cli/issue_show.go`:

```go
// ABOUTME: Implementation of the issue show command. Resolves a ref argument,
// ABOUTME: loads the issue, and displays it in human-readable or JSON format.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/resolve"
)

// IssueJSON is the presentation struct for --format json output.
// Combines issue metadata with parsed body sections.
type IssueJSON struct {
	ID                 uuid.UUID                `json:"id"`
	Title              string                   `json:"title"`
	State              issue.State              `json:"state"`
	Milestone          string                   `json:"milestone"`
	BlockedBy          []uuid.UUID              `json:"blocked_by,omitempty"`
	Blocks             []uuid.UUID              `json:"blocks,omitempty"`
	Created            time.Time                `json:"created"`
	Updated            time.Time                `json:"updated"`
	Sessions           []issue.Session          `json:"sessions,omitempty"`
	Prerequisites      []issue.Checkbox         `json:"prerequisites,omitempty"`
	Context            *issue.StructuredContext  `json:"context,omitempty"`
	AcceptanceCriteria []issue.Checkbox         `json:"acceptance_criteria,omitempty"`
	Description        string                   `json:"description,omitempty"`
	Body               string                   `json:"body,omitempty"`
}

func runIssueShow(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	// Determine ref argument (default to HEAD)
	input := ""
	if len(args) > 0 {
		input = args[0]
	}

	refPath, err := resolve.ResolveRef(app.Store, input)
	if err != nil {
		return fmt.Errorf("resolve ref: %w", err)
	}

	// Load the issue
	content, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return fmt.Errorf("read issue: %w", err)
	}
	iss, err := issue.Parse(content)
	if err != nil {
		return fmt.Errorf("parse issue: %w", err)
	}

	// Extract ID from ref path
	uuidStr := strings.TrimPrefix(refPath, "refs/chain/_/issues/")
	id, err := uuid.FromString(uuidStr)
	if err != nil {
		return fmt.Errorf("parse issue ID from ref: %w", err)
	}
	iss.ID = id

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		return showJSON(cmd, iss)
	}
	return showHuman(cmd, app, iss)
}

func showJSON(cmd *cobra.Command, iss *issue.Issue) error {
	sections := issue.ParseSections(iss.Body)
	out := IssueJSON{
		ID:                 iss.ID,
		Title:              iss.Title,
		State:              iss.State,
		Milestone:          iss.Milestone,
		BlockedBy:          iss.BlockedBy,
		Blocks:             iss.Blocks,
		Created:            iss.Created,
		Updated:            iss.Updated,
		Sessions:           iss.Sessions,
		Prerequisites:      sections.Prerequisites,
		Context:            sections.Context,
		AcceptanceCriteria: sections.AcceptanceCriteria,
		Description:        sections.Description,
		Body:               iss.Body,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func showHuman(cmd *cobra.Command, app *App, iss *issue.Issue) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "%s: %s\n\n", iss.ID.String()[:8], iss.Title)
	fmt.Fprintf(w, "  State:      %s\n", iss.State)
	fmt.Fprintf(w, "  Milestone:  %s\n", iss.Milestone)
	fmt.Fprintf(w, "  Created:    %s\n", iss.Created.Format("2006-01-02"))

	// Show dependencies with titles
	if len(iss.BlockedBy) > 0 {
		fmt.Fprintf(w, "\n  Blocked by:\n")
		for _, depID := range iss.BlockedBy {
			title := resolveIssueTitle(app, depID)
			fmt.Fprintf(w, "    %s  %s\n", depID.String()[:8], title)
		}
	}
	if len(iss.Blocks) > 0 {
		fmt.Fprintf(w, "\n  Blocks:\n")
		for _, depID := range iss.Blocks {
			title := resolveIssueTitle(app, depID)
			fmt.Fprintf(w, "    %s  %s\n", depID.String()[:8], title)
		}
	}

	// Show body as-is (markdown is already human-readable)
	if iss.Body != "" {
		fmt.Fprintf(w, "\n%s\n", iss.Body)
	}

	return nil
}

// resolveIssueTitle loads an issue by UUID and returns its title.
// Returns a placeholder on error.
func resolveIssueTitle(app *App, id uuid.UUID) string {
	refPath := "refs/chain/_/issues/" + id.String()
	content, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return "(unknown)"
	}
	iss, err := issue.Parse(content)
	if err != nil {
		return "(unknown)"
	}
	return iss.Title
}
```

- [ ] **Step 4: Wire the show command**

In `internal/cli/issue.go`, replace the `newIssueShowCommand` stub:

```go
func newIssueShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [ref]",
		Short: "View an issue with full context",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runIssueShow,
	}
}
```

- [ ] **Step 5: Run all tests, verify pass**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/issue-show && go vet ./... && go test ./...`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/issue_show.go internal/cli/issue_show_test.go internal/cli/issue.go
git commit -m "Implement issue show command with human and JSON output

Resolves ref via UUID prefix or HEAD. Human output shows title, state,
milestone, dependencies with titles, and raw body. JSON output includes
parsed sections (prerequisites, context, acceptance criteria) per PRD."
```

---

## Task 4: Final verification

- [ ] **Step 1: Run full suite with race detector**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/issue-show && go vet ./... && go test -race ./...`

- [ ] **Step 2: Build and smoke test**

```bash
go build -o git-chain ./cmd/git-chain/
cd $(mktemp -d) && git init
echo -e '---\ntitle: "Test issue"\n---\n\n## Prerequisites\n\n- [x] ready\n\n## Context\n\n- paths: main.go\n\nDo the thing.\n\n## Acceptance Criteria\n\n- [ ] it works' | ../git-chain issue add
../git-chain issue show
../git-chain issue show --format json
```

- [ ] **Step 3: Clean up and verify git status**

```bash
rm -f /home/perigrin/dev/git-chain/.worktrees/issue-show/git-chain
cd /home/perigrin/dev/git-chain/.worktrees/issue-show && git status
```
Expected: clean working tree.
