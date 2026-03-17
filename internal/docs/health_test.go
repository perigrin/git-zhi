// ABOUTME: Tests for Health — churn-relative documentation staleness detection.
// ABOUTME: Covers drift classification, stability modulation, ADR exemption, and coverage gaps.
package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-zhi/internal/docs"
)

// initHealthRepo creates a real on-disk git repo with an initial commit, so
// git log operations can run against it. Returns the repo and its root dir.
func initHealthRepo(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("repo.Config: %v", err)
	}
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	return repo, dir
}

// commitFile writes a file to disk, stages it, and commits. Returns the commit SHA.
func commitFile(t *testing.T, repo *git.Repository, dir, relPath, content string, when time.Time) string {
	t.Helper()
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", relPath, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add(relPath); err != nil {
		t.Fatalf("wt.Add %s: %v", relPath, err)
	}
	sig := &object.Signature{
		Name:  "Test User",
		Email: "test@example.com",
		When:  when,
	}
	h, err := wt.Commit("update "+relPath, &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("Commit %s: %v", relPath, err)
	}
	return h.String()
}

// --------------------------------------------------------------------------
// TestHealth_DriftNone — doc with covers, no subsequent code commits → NONE.
// --------------------------------------------------------------------------

func TestHealth_DriftNone(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	// Commit a source file first.
	commitFile(t, repo, dir, "internal/cli/root.go", "package cli\n", base)

	// Commit the doc after the source file (stable, covers internal/cli).
	docContent := "---\nstability: 2\ncovers:\n  - internal/cli\n---\n\n# CLI Architecture\n"
	commitFile(t, repo, dir, "docs/architecture/cli.md", docContent, base.Add(time.Minute))

	// No further commits to internal/cli after the doc was written.

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	if len(report.Documents) == 0 {
		t.Fatalf("expected at least one document in report, got none")
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "cli.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected cli.md in report documents, got %v", report.Documents)
	}

	if found.Drift != docs.DriftNone {
		t.Errorf("expected DriftNone when no code churn after doc update, got %s (churn=%d)", found.Drift, found.CodeChurn)
	}
	if found.CodeChurn != 0 {
		t.Errorf("expected CodeChurn=0, got %d", found.CodeChurn)
	}
	if found.Stability != 2 {
		t.Errorf("expected Stability=2, got %d", found.Stability)
	}
}

// --------------------------------------------------------------------------
// TestHealth_DriftLow — stable doc (threshold=5), small churn → LOW.
// --------------------------------------------------------------------------

func TestHealth_DriftLow_Stable(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	// Initial source file commit.
	commitFile(t, repo, dir, "internal/graph/graph.go", "package graph\n", base)

	// Doc commit (stable).
	docContent := "---\nstability: 2\ncovers:\n  - internal/graph\n---\n\n# Graph Architecture\n"
	commitFile(t, repo, dir, "docs/architecture/graph.md", docContent, base.Add(time.Minute))

	// Three more commits to the covered path after the doc was written.
	for i := 0; i < 3; i++ {
		commitFile(t, repo, dir, "internal/graph/graph.go",
			"package graph // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "graph.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected graph.md in report documents, got %v", report.Documents)
	}

	if found.CodeChurn != 3 {
		t.Errorf("expected CodeChurn=3, got %d", found.CodeChurn)
	}
	if found.Drift != docs.DriftLow {
		t.Errorf("expected DriftLow for stable doc with 3 commits (threshold=5), got %s", found.Drift)
	}
}

// --------------------------------------------------------------------------
// TestHealth_DriftHigh — stable doc with >5 commits → HIGH.
// --------------------------------------------------------------------------

func TestHealth_DriftHigh_Stable(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/storage/store.go", "package storage\n", base)

	docContent := "---\nstability: 2\ncovers:\n  - internal/storage\n---\n\n# Storage Architecture\n"
	commitFile(t, repo, dir, "docs/architecture/storage.md", docContent, base.Add(time.Minute))

	// Six commits to covered path after doc — exceeds stable threshold of 5.
	for i := 0; i < 6; i++ {
		commitFile(t, repo, dir, "internal/storage/store.go",
			"package storage // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "storage.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected storage.md in report documents, got %v", report.Documents)
	}

	if found.CodeChurn != 6 {
		t.Errorf("expected CodeChurn=6, got %d", found.CodeChurn)
	}
	if found.Drift != docs.DriftHigh {
		t.Errorf("expected DriftHigh for stable doc with 6 commits (threshold=5), got %s", found.Drift)
	}
}

// --------------------------------------------------------------------------
// TestHealth_StabilityModulation — experimental doc (threshold=2) flagged sooner.
// --------------------------------------------------------------------------

func TestHealth_DriftLow_Experimental(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/verify/extract.go", "package verify\n", base)

	docContent := "---\nstability: 1\ncovers:\n  - internal/verify\n---\n\n# Verify Architecture\n"
	commitFile(t, repo, dir, "docs/architecture/verify.md", docContent, base.Add(time.Minute))

	// Two commits to covered path — equals experimental threshold of 2 → LOW.
	for i := 0; i < 2; i++ {
		commitFile(t, repo, dir, "internal/verify/extract.go",
			"package verify // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "verify.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected verify.md in report documents, got %v", report.Documents)
	}

	if found.Drift != docs.DriftLow {
		t.Errorf("expected DriftLow for experimental doc with 2 commits (threshold=2), got %s (churn=%d)", found.Drift, found.CodeChurn)
	}
}

func TestHealth_DriftHigh_Experimental(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/actor/actor.go", "package actor\n", base)

	docContent := "---\nstability: 1\ncovers:\n  - internal/actor\n---\n\n# Actor Architecture\n"
	commitFile(t, repo, dir, "docs/architecture/actor.md", docContent, base.Add(time.Minute))

	// Three commits after doc — exceeds experimental threshold of 2 → HIGH.
	for i := 0; i < 3; i++ {
		commitFile(t, repo, dir, "internal/actor/actor.go",
			"package actor // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "actor.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected actor.md in report documents, got %v", report.Documents)
	}

	if found.Drift != docs.DriftHigh {
		t.Errorf("expected DriftHigh for experimental doc with 3 commits (threshold=2), got %s (churn=%d)", found.Drift, found.CodeChurn)
	}
}

// --------------------------------------------------------------------------
// TestHealth_DeprecatedDoc — stability=0 always produces DriftNone.
// --------------------------------------------------------------------------

func TestHealth_DeprecatedDoc_AlwaysNone(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/oldpkg/old.go", "package oldpkg\n", base)

	// Deprecated doc (stability=0).
	docContent := "---\nstability: 0\ncovers:\n  - internal/oldpkg\n---\n\n# Old Package\n"
	commitFile(t, repo, dir, "docs/architecture/old.md", docContent, base.Add(time.Minute))

	// Many commits to covered path — should still be NONE for deprecated.
	for i := 0; i < 10; i++ {
		commitFile(t, repo, dir, "internal/oldpkg/old.go",
			"package oldpkg // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "old.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected old.md in report documents, got %v", report.Documents)
	}

	if found.Drift != docs.DriftNone {
		t.Errorf("expected DriftNone for deprecated doc, got %s", found.Drift)
	}
}

// --------------------------------------------------------------------------
// TestHealth_ADRExempt — docs in docs/decisions/ are not included in health report.
// --------------------------------------------------------------------------

func TestHealth_ADRExempt(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/graph/graph.go", "package graph\n", base)

	// ADR with covers field in docs/decisions/.
	adrContent := "---\nstability: 2\ncovers:\n  - internal/graph\n---\n\n# ADR 0001\n"
	commitFile(t, repo, dir, "docs/decisions/0001-graph.md", adrContent, base.Add(time.Minute))

	// Commits to covered path after ADR.
	for i := 0; i < 10; i++ {
		commitFile(t, repo, dir, "internal/graph/graph.go",
			"package graph // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	// ADR should not appear in Documents.
	for _, d := range report.Documents {
		if strings.Contains(d.File, "0001-graph.md") {
			t.Errorf("ADR docs/decisions/0001-graph.md should be exempt from health report, but appeared: %+v", d)
		}
	}
}

// --------------------------------------------------------------------------
// TestHealth_PostmortemExempt — docs in docs/postmortems/ are exempt.
// --------------------------------------------------------------------------

func TestHealth_PostmortemExempt(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/cli/root.go", "package cli\n", base)

	// Postmortem with covers field.
	pmContent := "---\nstability: 2\ncovers:\n  - internal/cli\n---\n\n# Postmortem v0.1\n"
	commitFile(t, repo, dir, "docs/postmortems/v0.1-postmortem.md", pmContent, base.Add(time.Minute))

	// Commits to covered path.
	for i := 0; i < 5; i++ {
		commitFile(t, repo, dir, "internal/cli/root.go",
			"package cli // v"+string(rune('2'+i))+"\n",
			base.Add(time.Duration(2+i)*time.Minute))
	}

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	for _, d := range report.Documents {
		if strings.Contains(d.File, "postmortem") {
			t.Errorf("postmortem doc should be exempt from health report, but appeared: %+v", d)
		}
	}
}

// --------------------------------------------------------------------------
// TestHealth_CoverageGaps — packages in internal/ without a docs/architecture/ doc.
// --------------------------------------------------------------------------

func TestHealth_CoverageGaps(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	// Two internal packages.
	commitFile(t, repo, dir, "internal/graph/graph.go", "package graph\n", base)
	commitFile(t, repo, dir, "internal/storage/store.go", "package storage\n", base.Add(time.Minute))

	// Only graph has an architecture doc.
	docContent := "---\nstability: 2\ncovers:\n  - internal/graph\n---\n\n# Graph\n"
	commitFile(t, repo, dir, "docs/architecture/graph.md", docContent, base.Add(2*time.Minute))

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	// storage should be in CoverageGaps, graph should not.
	foundStorage := false
	for _, gap := range report.CoverageGaps {
		if gap == "internal/storage" {
			foundStorage = true
		}
		if gap == "internal/graph" {
			t.Errorf("internal/graph should not be in CoverageGaps (it has a doc)")
		}
	}
	if !foundStorage {
		t.Errorf("expected internal/storage in CoverageGaps, got %v", report.CoverageGaps)
	}
}

// --------------------------------------------------------------------------
// TestHealth_NoCoversFrontmatter — docs without covers are skipped.
// --------------------------------------------------------------------------

func TestHealth_NoCoversFrontmatter(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	// Doc without covers field.
	docContent := "---\nstability: 2\n---\n\n# No Covers\n"
	commitFile(t, repo, dir, "docs/architecture/nocov.md", docContent, base)

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	for _, d := range report.Documents {
		if strings.Contains(d.File, "nocov.md") {
			t.Errorf("doc without covers should not appear in health report, got %+v", d)
		}
	}
}

// --------------------------------------------------------------------------
// TestHealth_DocHealthFields — verify all DocHealth fields are populated.
// --------------------------------------------------------------------------

func TestHealth_DocHealthFields(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/cli/cli.go", "package cli\n", base)

	docContent := "---\nstability: 2\ncovers:\n  - internal/cli\n---\n\n# CLI Doc\n"
	commitFile(t, repo, dir, "docs/architecture/cli-detail.md", docContent, base.Add(time.Minute))

	// Two more commits to covered path.
	commitFile(t, repo, dir, "internal/cli/cli.go", "package cli // v2\n", base.Add(2*time.Minute))
	commitFile(t, repo, dir, "internal/cli/cli.go", "package cli // v3\n", base.Add(3*time.Minute))

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "cli-detail.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected cli-detail.md in report, got %v", report.Documents)
	}

	if found.File == "" {
		t.Error("expected File field to be set")
	}
	if len(found.Covers) == 0 {
		t.Error("expected Covers to have at least one entry")
	}
	if found.DocModified.IsZero() {
		t.Error("expected DocModified to be set")
	}
	if found.CodeChurn != 2 {
		t.Errorf("expected CodeChurn=2, got %d", found.CodeChurn)
	}
	if found.Drift != docs.DriftLow {
		t.Errorf("expected DriftLow (2 commits, stable threshold=5), got %s", found.Drift)
	}
	if found.Stability != 2 {
		t.Errorf("expected Stability=2, got %d", found.Stability)
	}
}

// --------------------------------------------------------------------------
// TestHealth_SummaryField — report has a non-empty summary.
// --------------------------------------------------------------------------

func TestHealth_SummaryField(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/cli/cmd.go", "package cli\n", base)

	docContent := "---\nstability: 2\ncovers:\n  - internal/cli\n---\n\n# CLI\n"
	commitFile(t, repo, dir, "docs/architecture/cli-cmd.md", docContent, base.Add(time.Minute))

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	if report.Summary == "" {
		t.Error("expected non-empty Summary in health report")
	}
}

// --------------------------------------------------------------------------
// TestHealth_EmptyRepo — no docs, no internal packages → empty report.
// --------------------------------------------------------------------------

func TestHealth_EmptyRepo(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	// Initial commit with no relevant files.
	commitFile(t, repo, dir, "README.md", "# Hello\n", base)

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	if len(report.Documents) != 0 {
		t.Errorf("expected no documents in empty repo, got %v", report.Documents)
	}
	if len(report.CoverageGaps) != 0 {
		t.Errorf("expected no coverage gaps in empty repo, got %v", report.CoverageGaps)
	}
}

// --------------------------------------------------------------------------
// TestHealth_DocModifiedTime — DocModified matches when the doc was committed.
// --------------------------------------------------------------------------

func TestHealth_DocModifiedTime(t *testing.T) {
	repo, dir := initHealthRepo(t)
	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	commitFile(t, repo, dir, "internal/graph/g.go", "package graph\n", base)

	docTime := time.Date(2026, 3, 11, 9, 30, 0, 0, time.UTC)
	docContent := "---\nstability: 2\ncovers:\n  - internal/graph\n---\n\n# Graph Doc\n"
	commitFile(t, repo, dir, "docs/architecture/g.md", docContent, docTime)

	report, err := docs.Health(dir, repo)
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	var found *docs.DocHealth
	for i := range report.Documents {
		if strings.Contains(report.Documents[i].File, "g.md") {
			found = &report.Documents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected g.md in report, got %v", report.Documents)
	}

	// DocModified must be close to docTime (within 1 second, since git stores
	// author time at second precision).
	diff := found.DocModified.Sub(docTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("expected DocModified≈%v, got %v (diff=%v)", docTime, found.DocModified, diff)
	}
}
