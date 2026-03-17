// ABOUTME: Integration tests for the docs CLI commands (init, check, health).
// ABOUTME: Each test runs the Cobra command against a real temp directory.
package docs_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/docs"
)

// runDocsCLI executes the docs root command with the given arguments and
// repoRoot injected via the context. It returns the combined stdout output and
// any error returned by Execute.
func runDocsCLI(t *testing.T, repoRoot string, repo *git.Repository, args ...string) (string, error) {
	t.Helper()
	cmd := docs.NewDocsCommand(repoRoot, repo)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// --------------------------------------------------------------------------
// docs init
// --------------------------------------------------------------------------

func TestDocsCLI_Init_ScaffoldsStructure(t *testing.T) {
	root := t.TempDir()

	out, err := runDocsCLI(t, root, nil, "init")
	if err != nil {
		t.Fatalf("docs init returned error: %v\noutput: %s", err, out)
	}

	// Output must mention the scaffolded directories.
	for _, dir := range []string{"docs/plans/", "docs/decisions/", "docs/architecture/"} {
		if !strings.Contains(out, dir) {
			t.Errorf("docs init output missing %q\nfull output:\n%s", dir, out)
		}
	}

	// The docs/ directory must actually exist.
	info, err := os.Stat(filepath.Join(root, "docs"))
	if err != nil {
		t.Fatalf("docs/ directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("docs is not a directory")
	}
}

func TestDocsCLI_Init_ReportsContributingMDCreated(t *testing.T) {
	root := t.TempDir()

	out, err := runDocsCLI(t, root, nil, "init")
	if err != nil {
		t.Fatalf("docs init returned error: %v", err)
	}

	// Must report CONTRIBUTING.md was created.
	if !strings.Contains(out, "CONTRIBUTING.md") {
		t.Errorf("docs init output does not mention CONTRIBUTING.md:\n%s", out)
	}
}

func TestDocsCLI_Init_ReportsUpdatedWhenContributingMDExists(t *testing.T) {
	root := t.TempDir()

	// Pre-create a CONTRIBUTING.md.
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte("# Contributing\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out, err := runDocsCLI(t, root, nil, "init")
	if err != nil {
		t.Fatalf("docs init returned error: %v", err)
	}

	// Must say "updated" not "created" for the existing file.
	if !strings.Contains(out, "CONTRIBUTING.md") {
		t.Errorf("docs init output does not mention CONTRIBUTING.md:\n%s", out)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("docs init should report CONTRIBUTING.md as updated when it already exists:\n%s", out)
	}
}

func TestDocsCLI_Init_JSONFormat(t *testing.T) {
	root := t.TempDir()

	out, err := runDocsCLI(t, root, nil, "init", "--format", "json")
	if err != nil {
		t.Fatalf("docs init --format json returned error: %v", err)
	}

	var result map[string]interface{}
	if parseErr := json.Unmarshal([]byte(out), &result); parseErr != nil {
		t.Fatalf("docs init --format json output is not valid JSON: %v\noutput: %s", parseErr, out)
	}

	// Must have a "created" or "items" array.
	if _, ok := result["items"]; !ok {
		t.Errorf("JSON output missing 'items' field; keys: %v", keysOf(result))
	}
}

// --------------------------------------------------------------------------
// docs check
// --------------------------------------------------------------------------

func TestDocsCLI_Check_OKWithFullyLinkedStructure(t *testing.T) {
	root := t.TempDir()

	// Create a minimal valid structure: CONTRIBUTING.md explicitly links to a
	// doc, the doc exists, no dead links, no ADR gaps, no invalid covers.
	writeFile(t, root, "CONTRIBUTING.md",
		"# Contributing\n\nSee [coding conventions](docs/contributing/coding-conventions.md).\n")
	writeFile(t, root, "docs/contributing/coding-conventions.md",
		"---\ncovers:\n  - .\n---\n\n# Coding Conventions\n")

	out, err := runDocsCLI(t, root, nil, "check")
	if err != nil {
		t.Fatalf("docs check returned error: %v\noutput: %s", err, out)
	}

	if !strings.Contains(out, "0 issues") {
		t.Errorf("expected '0 issues found' in output; got:\n%s", out)
	}
}

func TestDocsCLI_Check_LazyInitCreatesDocsDir(t *testing.T) {
	root := t.TempDir()
	// No docs/ directory exists yet.

	// Run check — it may return an error (issues found after lazy init),
	// but docs/ must have been created regardless.
	runDocsCLI(t, root, nil, "check") //nolint:errcheck — error is expected

	// docs/ must have been created by lazy init.
	if _, statErr := os.Stat(filepath.Join(root, "docs")); statErr != nil {
		t.Errorf("docs/ was not created by lazy init: %v", statErr)
	}
}

func TestDocsCLI_Check_ExitsNonZeroOnIssues(t *testing.T) {
	root := t.TempDir()

	// Scaffold and then introduce a dead link.
	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("setup scaffold: %v", err)
	}
	// Write a doc that has a dead link.
	deadLinkDoc := "# Guide\n\nSee [missing](docs/missing/nonexistent.md) for details.\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "guides", "guide.md"), []byte(deadLinkDoc), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Ensure guide.md is reachable from CONTRIBUTING.md.
	contribPath := filepath.Join(root, "CONTRIBUTING.md")
	existing, _ := os.ReadFile(contribPath)
	updated := string(existing) + "\n[guide](docs/guides/guide.md)\n"
	if err := os.WriteFile(contribPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("update CONTRIBUTING.md: %v", err)
	}

	_, err := runDocsCLI(t, root, nil, "check")
	if err == nil {
		t.Error("docs check should return error (exit 1) when issues are found")
	}
}

func TestDocsCLI_Check_JSONFormat(t *testing.T) {
	root := t.TempDir()

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("setup scaffold: %v", err)
	}

	out, err := runDocsCLI(t, root, nil, "check", "--format", "json")
	if err != nil {
		// A non-nil error here just means check found issues, which is OK for
		// the JSON format test — we still expect valid JSON output.
		t.Logf("docs check --format json returned error (may be expected): %v", err)
	}

	var result map[string]interface{}
	if parseErr := json.Unmarshal([]byte(out), &result); parseErr != nil {
		t.Fatalf("docs check --format json output is not valid JSON: %v\noutput: %s", parseErr, out)
	}

	// Must have "ok" field.
	if _, ok := result["ok"]; !ok {
		t.Errorf("JSON output missing 'ok' field; keys: %v", keysOf(result))
	}
}

// --------------------------------------------------------------------------
// docs health
// --------------------------------------------------------------------------

func TestDocsCLI_Health_LazyInitCreatesDocsDir(t *testing.T) {
	root := t.TempDir()
	repo := initBareRepo(t, root)

	out, err := runDocsCLI(t, root, repo, "health")
	if err != nil {
		t.Fatalf("docs health should not fail on missing docs/ (lazy init): %v\noutput: %s", err, out)
	}

	// docs/ must have been created by lazy init.
	if _, statErr := os.Stat(filepath.Join(root, "docs")); statErr != nil {
		t.Errorf("docs/ was not created by lazy init: %v", statErr)
	}
}

func TestDocsCLI_Health_ReportsSummary(t *testing.T) {
	root := t.TempDir()
	repo := initBareRepo(t, root)

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("setup scaffold: %v", err)
	}

	out, err := runDocsCLI(t, root, repo, "health")
	if err != nil {
		t.Fatalf("docs health returned error: %v\noutput: %s", err, out)
	}

	// Output must contain a "Summary:" line or equivalent.
	if !strings.Contains(strings.ToLower(out), "summary") {
		t.Errorf("docs health output missing Summary; got:\n%s", out)
	}
}

func TestDocsCLI_Health_JSONFormat(t *testing.T) {
	root := t.TempDir()
	repo := initBareRepo(t, root)

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("setup scaffold: %v", err)
	}

	out, err := runDocsCLI(t, root, repo, "health", "--format", "json")
	if err != nil {
		t.Fatalf("docs health --format json returned error: %v\noutput: %s", err, out)
	}

	var result map[string]interface{}
	if parseErr := json.Unmarshal([]byte(out), &result); parseErr != nil {
		t.Fatalf("docs health --format json output is not valid JSON: %v\noutput: %s", parseErr, out)
	}

	// Must have "summary" field.
	if _, ok := result["summary"]; !ok {
		t.Errorf("JSON output missing 'summary' field; keys: %v", keysOf(result))
	}
}

func TestDocsCLI_Health_ProducesOutputWithCoversDocs(t *testing.T) {
	root := t.TempDir()
	// Use a nil repo so Health skips git log calls (zero-value drift).
	// This lets us test the output format without needing git history.

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("setup scaffold: %v", err)
	}

	// Write an architecture doc that covers a path.
	archDoc := "---\ncovers:\n  - internal/docs\nstability: 1\n---\n\n# Docs Architecture\n"
	if err := os.MkdirAll(filepath.Join(root, "docs", "architecture"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "architecture", "docs.md"), []byte(archDoc), 0o644); err != nil {
		t.Fatalf("write arch doc: %v", err)
	}

	// nil repo means Health skips git log — all drift will be NONE, which is fine.
	out, err := runDocsCLI(t, root, nil, "health")
	if err != nil {
		t.Fatalf("docs health returned error: %v\noutput: %s", err, out)
	}
	// Output should mention the arch doc and have a Summary line.
	if !strings.Contains(out, "docs/architecture/docs.md") {
		t.Errorf("docs health output missing arch doc entry; got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "summary") {
		t.Errorf("docs health output missing Summary; got:\n%s", out)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// keysOf returns the keys of a map[string]interface{} as a string slice.
func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// initBareRepo initialises a git repository at root (bare init — no commits
// required) and returns the opened go-git Repository. Used by health tests
// that require a non-nil *git.Repository.
func initBareRepo(t *testing.T, root string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("git init %s: %v", root, err)
	}
	return repo
}
