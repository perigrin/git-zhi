// ABOUTME: Tests for Scaffold — directory scaffolding, CONTRIBUTING.md creation,
// ABOUTME: idempotency, and Short Links section injection for existing files.
package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/docs"
)

// expectedDirs lists every directory Scaffold must create under repoRoot.
var expectedDirs = []string{
	"docs/plans",
	"docs/decisions",
	"docs/contributing",
	"docs/guides",
	"docs/architecture",
	"docs/reference",
}

// expectedLivingDocs lists living documents Scaffold must create with content.
var expectedLivingDocs = []string{
	"docs/contributing/coding-conventions.md",
	"docs/contributing/development-workflow.md",
}

func TestScaffold_CreatesCanonicalDirectoryStructure(t *testing.T) {
	root := t.TempDir()

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold(%q) returned error: %v", root, err)
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil {
			t.Errorf("expected dir %q to exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory, got file", dir)
		}
	}
}

func TestScaffold_CreatesLivingDocuments(t *testing.T) {
	root := t.TempDir()

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold(%q) returned error: %v", root, err)
	}

	for _, path := range expectedLivingDocs {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Errorf("expected living doc %q to exist: %v", path, err)
			continue
		}
		// Each living doc must have the stability frontmatter.
		if !strings.Contains(string(content), "stability:") {
			t.Errorf("living doc %q missing stability frontmatter", path)
		}
		// Each must have at least one section heading.
		if !strings.Contains(string(content), "## ") {
			t.Errorf("living doc %q missing section headings", path)
		}
	}
}

func TestScaffold_CreatesContributingMD_WhenAbsent(t *testing.T) {
	root := t.TempDir()

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold(%q) returned error: %v", root, err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("expected CONTRIBUTING.md to exist: %v", err)
	}

	// Hub-style CONTRIBUTING.md must contain short links section.
	if !strings.Contains(string(content), "Short Links") {
		t.Errorf("CONTRIBUTING.md missing Short Links section, got:\n%s", string(content))
	}
	// Must link to each docs/ subdirectory.
	for _, subdir := range []string{"plans", "decisions", "contributing", "guides", "architecture", "reference"} {
		if !strings.Contains(string(content), subdir) {
			t.Errorf("CONTRIBUTING.md missing link to docs/%s", subdir)
		}
	}
}

func TestScaffold_SkipsExistingDirectories(t *testing.T) {
	root := t.TempDir()

	// Pre-create docs/plans with a sentinel file.
	plansDir := filepath.Join(root, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sentinelPath := filepath.Join(plansDir, "existing.md")
	if err := os.WriteFile(sentinelPath, []byte("sentinel content"), 0o644); err != nil {
		t.Fatalf("setup sentinel: %v", err)
	}

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold(%q) returned error: %v", root, err)
	}

	// Sentinel must still exist — Scaffold must not clobber or remove it.
	content, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("sentinel file should still exist: %v", err)
	}
	if string(content) != "sentinel content" {
		t.Errorf("sentinel file content changed, got %q", string(content))
	}
}

func TestScaffold_PreservesExistingContributingMD(t *testing.T) {
	root := t.TempDir()

	existing := "# Contributing\n\nThis is an existing CONTRIBUTING.md.\n"
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold(%q) returned error: %v", root, err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("CONTRIBUTING.md should still exist: %v", err)
	}

	// Original content must be preserved.
	if !strings.Contains(string(content), "This is an existing CONTRIBUTING.md.") {
		t.Errorf("existing content was lost, got:\n%s", string(content))
	}
	// Short links section must have been appended.
	if !strings.Contains(string(content), "Short Links") {
		t.Errorf("Short Links section was not appended, got:\n%s", string(content))
	}
}

func TestScaffold_DoesNotDuplicateShortLinksSection(t *testing.T) {
	root := t.TempDir()

	// Scaffold once.
	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("first Scaffold: %v", err)
	}

	// Scaffold again — second run must be a no-op.
	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("second Scaffold: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("CONTRIBUTING.md should exist: %v", err)
	}

	// "Short Links" must appear exactly once.
	count := strings.Count(string(content), "Short Links")
	if count != 1 {
		t.Errorf("Short Links section appears %d times, expected 1:\n%s", count, string(content))
	}
}

func TestScaffold_Idempotent(t *testing.T) {
	root := t.TempDir()

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("first Scaffold: %v", err)
	}

	// Capture state after first run.
	firstContent := map[string]string{}
	for _, p := range append(expectedLivingDocs, "CONTRIBUTING.md") {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatalf("read %s after first Scaffold: %v", p, err)
		}
		firstContent[p] = string(b)
	}

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("second Scaffold: %v", err)
	}

	// All files must be byte-identical after second run.
	for _, p := range append(expectedLivingDocs, "CONTRIBUTING.md") {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatalf("read %s after second Scaffold: %v", p, err)
		}
		if string(b) != firstContent[p] {
			t.Errorf("file %s changed on second Scaffold run:\nbefore:\n%s\nafter:\n%s", p, firstContent[p], string(b))
		}
	}
}

func TestScaffold_ExistingContributingMD_AlreadyHasShortLinks(t *testing.T) {
	root := t.TempDir()

	existing := "# Contributing\n\n## Short Links to Important Resources\n\n- [Plans](docs/plans)\n"
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold(%q) returned error: %v", root, err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("CONTRIBUTING.md should still exist: %v", err)
	}

	// Short Links must still appear exactly once.
	count := strings.Count(string(content), "Short Links")
	if count != 1 {
		t.Errorf("Short Links section appears %d times after Scaffold on file that already had it:\n%s", count, string(content))
	}
}
