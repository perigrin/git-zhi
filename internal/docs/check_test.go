// ABOUTME: Tests for Check — structural validation of the docs/ directory,
// ABOUTME: including reachability, dead links, ADR numbering, and covers paths.
package docs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/perigrin/git-zhi/internal/docs"
)

// writeFile is a helper that creates a file (and its parent dirs) with content.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// --------------------------------------------------------------------------
// TestCheck_AllOK — fully valid structure returns OK=true with no issues.
// --------------------------------------------------------------------------

func TestCheck_AllOK(t *testing.T) {
	root := t.TempDir()

	// Create CONTRIBUTING.md linking to one doc.
	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\nSee [coding conventions](docs/contributing/coding-conventions.md).\n")

	// Create the linked doc (with a valid covers path pointing to root).
	writeFile(t, root, "docs/contributing/coding-conventions.md", "---\ncovers:\n  - .\n---\n\n# Coding Conventions\n")

	// Create docs/decisions/ with sequential ADRs.
	// ADRs in docs/decisions/ are exempt from reachability — they are
	// indexed by sequential numbering, not by explicit links.
	writeFile(t, root, "docs/decisions/0001-first.md", "# First ADR\n")
	writeFile(t, root, "docs/decisions/0002-second.md", "# Second ADR\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.OK {
		t.Errorf("expected OK=true, got false; unreachable=%v deadLinks=%v adrGaps=%v invalidCovers=%v",
			result.UnreachableFiles, result.DeadLinks, result.ADRGaps, result.InvalidCovers)
	}
}

// --------------------------------------------------------------------------
// TestCheck_UnreachableFiles — files in docs/ not linked from CONTRIBUTING.md.
// --------------------------------------------------------------------------

func TestCheck_UnreachableFiles(t *testing.T) {
	root := t.TempDir()

	// CONTRIBUTING.md links only to one doc.
	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\nSee [linked](docs/linked.md).\n")
	writeFile(t, root, "docs/linked.md", "# Linked\n")

	// This file exists but is not reachable from CONTRIBUTING.md.
	writeFile(t, root, "docs/unreachable.md", "# Unreachable\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if result.OK {
		t.Errorf("expected OK=false when there are unreachable files")
	}

	found := false
	for _, f := range result.UnreachableFiles {
		if filepath.ToSlash(f) == "docs/unreachable.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected docs/unreachable.md in UnreachableFiles, got %v", result.UnreachableFiles)
	}
}

func TestCheck_ReachableViaTransitiveLink(t *testing.T) {
	root := t.TempDir()

	// CONTRIBUTING.md → hub.md → deep.md (transitive).
	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\nSee [hub](docs/hub.md).\n")
	writeFile(t, root, "docs/hub.md", "# Hub\n\nSee [deep](deep.md).\n")
	writeFile(t, root, "docs/deep.md", "# Deep\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	for _, f := range result.UnreachableFiles {
		if filepath.ToSlash(f) == "docs/deep.md" {
			t.Errorf("docs/deep.md should be reachable transitively but was reported as unreachable")
		}
	}
}

func TestCheck_NoDocs_NoUnreachable(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// No docs/ at all.

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.UnreachableFiles) != 0 {
		t.Errorf("expected no unreachable files when docs/ is absent, got %v", result.UnreachableFiles)
	}
}

// --------------------------------------------------------------------------
// TestCheck_DeadLinks — relative markdown links that point to missing files.
// --------------------------------------------------------------------------

func TestCheck_DeadLinks(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// A doc with a broken relative link.
	writeFile(t, root, "docs/broken.md", "# Broken\n\nSee [missing](missing.md).\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if result.OK {
		t.Errorf("expected OK=false when there are dead links")
	}

	found := false
	for _, dl := range result.DeadLinks {
		if dl != "" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one dead link, got %v", result.DeadLinks)
	}
}

func TestCheck_ValidRelativeLinks_NotReported(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\nSee [a](docs/a.md).\n")
	writeFile(t, root, "docs/a.md", "# A\n\nSee [b](b.md).\n")
	writeFile(t, root, "docs/b.md", "# B\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.DeadLinks) != 0 {
		t.Errorf("expected no dead links, got %v", result.DeadLinks)
	}
}

func TestCheck_ExternalLinks_Ignored(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\nSee [external](https://example.com/doc).\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.DeadLinks) != 0 {
		t.Errorf("external links should not be reported as dead; got %v", result.DeadLinks)
	}
}

// --------------------------------------------------------------------------
// TestCheck_ADRGaps — non-sequential ADR numbering detected.
// --------------------------------------------------------------------------

func TestCheck_ADRGaps(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0001-first.md", "# First\n")
	// 0002 is missing — gap.
	writeFile(t, root, "docs/decisions/0003-third.md", "# Third\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if result.OK {
		t.Errorf("expected OK=false with ADR gap")
	}

	found := false
	for _, gap := range result.ADRGaps {
		if gap == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected gap at 2 in ADRGaps, got %v", result.ADRGaps)
	}
}

func TestCheck_ADRSequential_NoGap(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0001-first.md", "# First\n")
	writeFile(t, root, "docs/decisions/0002-second.md", "# Second\n")
	writeFile(t, root, "docs/decisions/0003-third.md", "# Third\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.ADRGaps) != 0 {
		t.Errorf("expected no ADR gaps, got %v", result.ADRGaps)
	}
}

func TestCheck_ADRDirectory_Absent_NoError(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// No docs/decisions/ directory.

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.ADRGaps) != 0 {
		t.Errorf("expected no ADR gaps when decisions/ absent, got %v", result.ADRGaps)
	}
}

func TestCheck_ADRNonConformingFilenames_Ignored(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0001-first.md", "# First\n")
	// README.md does not match the ADR pattern — should be ignored.
	writeFile(t, root, "docs/decisions/README.md", "# Index\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.ADRGaps) != 0 {
		t.Errorf("non-conforming filename should not affect gap detection, got %v", result.ADRGaps)
	}
}

// --------------------------------------------------------------------------
// TestCheck_InvalidCovers — covers: paths that don't resolve.
// --------------------------------------------------------------------------

func TestCheck_InvalidCovers(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// A doc whose covers path does not exist.
	writeFile(t, root, "docs/missing-covers.md", "---\ncovers:\n  - internal/nonexistent\n---\n\n# Doc\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if result.OK {
		t.Errorf("expected OK=false when covers path is invalid")
	}

	if len(result.InvalidCovers) == 0 {
		t.Errorf("expected InvalidCovers to be non-empty, got %v", result.InvalidCovers)
	}
}

func TestCheck_ValidCovers(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// Create the covered directory.
	if err := os.MkdirAll(filepath.Join(root, "internal", "cli"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Doc covering a directory that exists.
	writeFile(t, root, "docs/arch.md", "---\ncovers:\n  - internal/cli\n---\n\n# Architecture\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.InvalidCovers) != 0 {
		t.Errorf("expected no invalid covers, got %v", result.InvalidCovers)
	}
}

func TestCheck_CoversFile_Valid(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// Create a specific file to cover.
	writeFile(t, root, "internal/cli/root.go", "package cli\n")
	writeFile(t, root, "docs/root-doc.md", "---\ncovers:\n  - internal/cli/root.go\n---\n\n# Root Doc\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.InvalidCovers) != 0 {
		t.Errorf("expected no invalid covers for existing file, got %v", result.InvalidCovers)
	}
}

func TestCheck_NoFrontmatter_NoCoversCheck(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	// Doc with no frontmatter — should not trigger covers check.
	writeFile(t, root, "docs/plain.md", "# Plain doc with no frontmatter\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.InvalidCovers) != 0 {
		t.Errorf("doc without frontmatter should produce no InvalidCovers, got %v", result.InvalidCovers)
	}
}

// --------------------------------------------------------------------------
// TestCheck_Combined — combined check over a realistic structure.
// --------------------------------------------------------------------------

func TestCheck_Combined(t *testing.T) {
	root := t.TempDir()

	// CONTRIBUTING.md links to a doc.
	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\nSee [arch](docs/architecture/overview.md).\n")

	// Linked doc with a dead sub-link.
	writeFile(t, root, "docs/architecture/overview.md", "# Overview\n\nSee [missing](missing.md).\n")

	// An unreachable doc.
	writeFile(t, root, "docs/guides/orphan.md", "# Orphan\n")

	// ADR with a gap.
	writeFile(t, root, "docs/decisions/0001-init.md", "# Init\n")
	writeFile(t, root, "docs/decisions/0003-skip.md", "# Skip\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if result.OK {
		t.Errorf("expected OK=false in combined scenario")
	}
	if len(result.DeadLinks) == 0 {
		t.Errorf("expected at least one dead link")
	}
	if len(result.ADRGaps) == 0 {
		t.Errorf("expected at least one ADR gap")
	}
}

// --------------------------------------------------------------------------
// Directory links — a link to a directory makes the files inside it
// reachable, the same way a link to an index file would.
// --------------------------------------------------------------------------

// TestCheck_ScaffoldedTreePasses — the index Scaffold generates must satisfy
// the checker that reads it. Scaffold's Short Links section links directories,
// and the two living documents it writes live inside one of them.
func TestCheck_ScaffoldedTreePasses(t *testing.T) {
	root := t.TempDir()

	if err := docs.Scaffold(root); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.UnreachableFiles) != 0 {
		t.Errorf("a freshly scaffolded tree should have no unreachable files, got %v",
			result.UnreachableFiles)
	}
	if !result.OK {
		t.Errorf("expected OK=true for a freshly scaffolded tree; deadLinks=%v adrGaps=%v invalidCovers=%v",
			result.DeadLinks, result.ADRGaps, result.InvalidCovers)
	}
}

// TestCheck_DirectoryLink_ReachesContents — the general case, independent of
// Scaffold. docs/guides/ is not an archive directory and is not exempt, so it
// proves directory links work rather than that something was exempted.
func TestCheck_DirectoryLink_ReachesContents(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md",
		"# Contributing\n\n- [Plans](docs/plans)\n- [Guides](docs/guides)\n")
	writeFile(t, root, "docs/plans/2026-q1-roadmap.md", "# Roadmap\n")
	writeFile(t, root, "docs/guides/how-to-release.md", "# Release\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.UnreachableFiles) != 0 {
		t.Errorf("files under a linked directory should be reachable, got %v",
			result.UnreachableFiles)
	}
}

// TestCheck_DirectoryLink_Nested — a linked directory is walked all the way
// down, so nesting needs no separate handling.
func TestCheck_DirectoryLink_Nested(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\n- [Guides](docs/guides)\n")
	writeFile(t, root, "docs/guides/deploy/staging.md", "# Staging\n")
	writeFile(t, root, "docs/guides/deploy/rollback/steps.md", "# Rollback\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.UnreachableFiles) != 0 {
		t.Errorf("nested files under a linked directory should be reachable, got %v",
			result.UnreachableFiles)
	}
}

// TestCheck_DirectoryLink_FollowsLinksInside — files inside a linked directory
// keep propagating reachability through their own links, so a doc linked only
// from inside a linked directory is reachable too.
func TestCheck_DirectoryLink_FollowsLinksInside(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\n- [Guides](docs/guides)\n")
	writeFile(t, root, "docs/guides/index.md", "# Guides\n\nSee [the runbook](../reference/runbook.md).\n")
	writeFile(t, root, "docs/reference/runbook.md", "# Runbook\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.UnreachableFiles) != 0 {
		t.Errorf("a doc linked from inside a linked directory should be reachable, got %v",
			result.UnreachableFiles)
	}
}

// TestCheck_UnlinkedDirectory_StillUnreachable — the fix must not make the
// reachability check vacuous. A directory nobody links stays unreachable.
func TestCheck_UnlinkedDirectory_StillUnreachable(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n\n- [Guides](docs/guides)\n")
	writeFile(t, root, "docs/guides/how-to-release.md", "# Release\n")
	writeFile(t, root, "docs/orphans/nobody-links-me.md", "# Orphan\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	want := []string{"docs/orphans/nobody-links-me.md"}
	if !equalStrings(result.UnreachableFiles, want) {
		t.Errorf("expected UnreachableFiles %v, got %v", want, result.UnreachableFiles)
	}
}

// equalStrings reports whether two string slices hold the same values in order.
func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if filepath.ToSlash(got[i]) != want[i] {
			return false
		}
	}
	return true
}

// --------------------------------------------------------------------------
// ADR numbering base — the gap scan starts at the lowest number present,
// clamped to 1, so neither the 0-based nor the 1-based convention is assumed.
// --------------------------------------------------------------------------

// TestCheck_ADRZeroBased_GapReported — a 0000 meta-document is part of the
// sequence, not invisible to the check. The hole at 3 is still reported.
func TestCheck_ADRZeroBased_GapReported(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0000-meta.md", "# Meta\n")
	writeFile(t, root, "docs/decisions/0001-first.md", "# First\n")
	writeFile(t, root, "docs/decisions/0002-second.md", "# Second\n")
	// 0003 is missing — gap.
	writeFile(t, root, "docs/decisions/0004-fourth.md", "# Fourth\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if want := []int{3}; !equalInts(result.ADRGaps, want) {
		t.Errorf("expected ADRGaps %v with a 0000 document present, got %v", want, result.ADRGaps)
	}
}

// TestCheck_ADRZeroBasedSequential_NoGap — a complete 0-based run reports
// nothing: 0 is present, so it is not missing.
func TestCheck_ADRZeroBasedSequential_NoGap(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0000-meta.md", "# Meta\n")
	writeFile(t, root, "docs/decisions/0001-first.md", "# First\n")
	writeFile(t, root, "docs/decisions/0002-second.md", "# Second\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.ADRGaps) != 0 {
		t.Errorf("expected no ADR gaps for a complete 0-based sequence, got %v", result.ADRGaps)
	}
}

// TestCheck_ADROneBased_ZeroNotReported — the 1-based convention must not
// grow a permanent false gap at 0.
func TestCheck_ADROneBased_ZeroNotReported(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0001-first.md", "# First\n")
	writeFile(t, root, "docs/decisions/0002-second.md", "# Second\n")
	writeFile(t, root, "docs/decisions/0003-third.md", "# Third\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if len(result.ADRGaps) != 0 {
		t.Errorf("expected no ADR gaps for a 1-based sequence, got %v", result.ADRGaps)
	}
}

// TestCheck_ADRStartsAboveOne_LowNumbersMissing — decisions are superseded,
// never deleted, so a sequence that begins at 0007 is missing 1 through 6.
func TestCheck_ADRStartsAboveOne_LowNumbersMissing(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "CONTRIBUTING.md", "# Contributing\n")
	writeFile(t, root, "docs/decisions/0007-seventh.md", "# Seventh\n")
	writeFile(t, root, "docs/decisions/0008-eighth.md", "# Eighth\n")

	result, err := docs.Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if want := []int{1, 2, 3, 4, 5, 6}; !equalInts(result.ADRGaps, want) {
		t.Errorf("expected ADRGaps %v for a sequence starting at 0007, got %v", want, result.ADRGaps)
	}
}

// equalInts reports whether two int slices hold the same values in order.
func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
