// ABOUTME: Tests that docs health distinguishes an absent covers field from an
// ABOUTME: empty one, and never reports zeros while observing no documents.
package docs_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/docs"
)

// TestHealth_EmptyCoversIsReported — `covers: []` means the doc declares the
// field and watches nothing. That is a finding, not an absence: docs init
// writes exactly this into both templates it scaffolds.
func TestHealth_EmptyCoversIsReported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/contributing/coding-conventions.md",
		"---\nstability: 1\ncovers: []\n---\n\n# Conventions\n")

	report, err := docs.Health(root, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if len(report.Unmonitored) != 1 {
		t.Fatalf("expected 1 unmonitored doc, got %v", report.Unmonitored)
	}
	if got := report.Unmonitored[0]; got != "docs/contributing/coding-conventions.md" {
		t.Errorf("expected the file to be named, got %q", got)
	}
}

// TestHealth_BareCoversKeyIsReported — `covers:` with nothing after it is an
// explicit YAML null, and unmarshals to the same nil as an absent key. It is
// the form a person writes: you type the key, then go looking for the paths.
// Interrupted there, the doc must be flagged, not silently unwatched.
func TestHealth_BareCoversKeyIsReported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/guides/half-written.md",
		"---\nstability: 1\ncovers:\n---\n\n# Half written\n")

	report, err := docs.Health(root, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if len(report.Unmonitored) != 1 {
		t.Fatalf("expected the bare covers key to be reported, got %v", report.Unmonitored)
	}
	if got := report.Unmonitored[0]; got != "docs/guides/half-written.md" {
		t.Errorf("expected the file to be named, got %q", got)
	}
}

// TestHealth_BareCoversKeyCountedInSummary — and it must reach the summary, so
// the blind state cannot read as a clean one.
func TestHealth_BareCoversKeyCountedInSummary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/guides/half-written.md",
		"---\nstability: 1\ncovers:\n---\n\n# Half written\n")

	report, err := docs.Health(root, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if strings.Contains(report.Summary, "no docs with covers frontmatter found") {
		t.Errorf("a declared-but-null covers field is not an absent one, got %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "1 with empty covers") {
		t.Errorf("expected the summary to count it, got %q", report.Summary)
	}
}

// TestHealth_AbsentCoversIsNotReported — a doc that never declares covers is
// not claiming to track code, so it is not a finding.
func TestHealth_AbsentCoversIsNotReported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/guides/how-to.md", "---\nstability: 1\n---\n\n# Guide\n")
	writeFile(t, root, "docs/guides/plain.md", "# No frontmatter at all\n")

	report, err := docs.Health(root, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if len(report.Unmonitored) != 0 {
		t.Errorf("a doc without a covers field is not unmonitored, got %v", report.Unmonitored)
	}
}

// TestHealth_SummaryNamesTheBlindState — the summary must not read as a clean
// bill of health when nothing is being observed.
func TestHealth_SummaryNamesTheBlindState(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/contributing/coding-conventions.md",
		"---\nstability: 1\ncovers: []\n---\n\n# Conventions\n")

	report, err := docs.Health(root, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if !strings.Contains(report.Summary, "0 docs checked") &&
		!strings.Contains(report.Summary, "no docs") {
		t.Errorf("expected the summary to say nothing is being watched, got %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "1") {
		t.Errorf("expected the summary to count the unmonitored doc, got %q", report.Summary)
	}
}

// TestHealth_CoveredDocStillChecked — a doc with real covers entries is
// unaffected by the presence check.
func TestHealth_CoveredDocStillChecked(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/architecture/storage.md",
		"---\nstability: 2\ncovers:\n  - internal/storage\n---\n\n# Storage\n")

	report, err := docs.Health(root, nil)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if len(report.Documents) != 1 {
		t.Fatalf("expected the covered doc to be checked, got %d documents", len(report.Documents))
	}
	if len(report.Unmonitored) != 0 {
		t.Errorf("a doc with real covers entries is not unmonitored, got %v", report.Unmonitored)
	}
}
