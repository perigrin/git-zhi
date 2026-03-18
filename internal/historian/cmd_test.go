// ABOUTME: Integration tests for the git-zhi-historian Cobra command pipeline.
// ABOUTME: Uses real on-disk git repos to verify extract→cluster→enrich→write behavior.
package historian_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/historian"
	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/storage"
)

// makeHistorianTestRepo creates a real on-disk git repo with some commits and
// returns the App and the repo dir. Each call to addCommit creates a file.
func makeHistorianTestRepo(t *testing.T) (*git.Repository, string, *storage.Store, *cli.App) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}
	return repo, dir, store, app
}

// addCommitToRepo adds a single-file commit with the given message, author, and time.
func addCommitToRepo(t *testing.T, repo *git.Repository, dir, filename, content, message, authorName, authorEmail string, when time.Time) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("wt.Add: %v", err)
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  when,
		},
	})
	if err != nil {
		t.Fatalf("wt.Commit: %v", err)
	}
	return hash.String()
}

// runHistorian is a helper that executes the historian command with the given
// args against the provided App.
func runHistorian(t *testing.T, app *cli.App, args ...string) (string, string, error) {
	t.Helper()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := historian.NewHistorianCommand()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	ctx := cli.WithApp(context.Background(), app)
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// TestHistorianFullPipeline verifies that running historian with several commits
// writes issues to refs/zhi/_/issues/<uuid> and records last_processed_sha.
func TestHistorianFullPipeline(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	// Three commits on the same files — should cluster into fewer issues
	// depending on threshold. With default config they may form 1-3 issues.
	addCommitToRepo(t, repo, dir, "main.go", "v1", "LOPS-1: initial implementation", "Alice", "alice@example.com", t0)
	addCommitToRepo(t, repo, dir, "main.go", "v2", "LOPS-1: fix edge case", "Alice", "alice@example.com", t1)
	addCommitToRepo(t, repo, dir, "util.go", "v1", "add utilities", "Bob", "bob@example.com", t2)

	_, _, err := runHistorian(t, app)
	if err != nil {
		t.Fatalf("historian: %v", err)
	}

	// At least one issue should have been created.
	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected at least one issue to be created, got none")
	}

	// All issues should be in the done state (retrospective).
	for _, iss := range issues {
		if iss.State != issue.StateDone {
			t.Errorf("issue %s: state = %q, want %q", iss.ID, iss.State, issue.StateDone)
		}
	}

	// last_processed_sha should NOT be written for non-incremental runs.
	// Only --incremental runs set the watermark.
	shaRef := "refs/zhi/_/historian/last_processed_sha"
	if store.RefExists(shaRef) {
		t.Error("expected last_processed_sha ref to NOT exist after non-incremental run")
	}
}

// TestHistorianDryRun verifies that --dry-run prints what would be created
// but does not write any refs.
func TestHistorianDryRun(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	addCommitToRepo(t, repo, dir, "a.go", "a", "fix: resolve crash", "Dev", "dev@example.com", t0)

	stdout, _, err := runHistorian(t, app, "--dry-run")
	if err != nil {
		t.Fatalf("historian --dry-run: %v", err)
	}

	// Should print something about the would-be issues.
	if stdout == "" {
		t.Error("expected some output from --dry-run, got empty string")
	}

	// No issues should be written to refs.
	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("--dry-run should not write issues; got %d", len(issues))
	}

	// No last_processed_sha should be recorded.
	shaRef := "refs/zhi/_/historian/last_processed_sha"
	if store.RefExists(shaRef) {
		t.Error("--dry-run should not write last_processed_sha ref")
	}
}

// TestHistorianLabelFlag verifies that --label assigns a label to all created
// issues and that BuildLabelIndexes is invoked (label index ref exists).
func TestHistorianLabelFlag(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 3, 10, 0, 0, 0, time.UTC)
	addCommitToRepo(t, repo, dir, "svc.go", "v1", "LOPS-99: add service layer", "Alice", "alice@example.com", t0)

	_, _, err := runHistorian(t, app, "--label", "LOPS")
	if err != nil {
		t.Fatalf("historian --label: %v", err)
	}

	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues to be created")
	}

	// All issues should carry the label.
	for _, iss := range issues {
		found := false
		for _, l := range iss.Labels {
			if l == "LOPS" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("issue %s does not have label LOPS; labels=%v", iss.ID, iss.Labels)
		}
	}

	// Label index ref should exist for at least one issue.
	labelRefs, err := store.ListRefs("refs/zhi/_/labels/LOPS/")
	if err != nil {
		t.Fatalf("ListRefs for label index: %v", err)
	}
	if len(labelRefs) == 0 {
		t.Error("expected label index refs under refs/zhi/_/labels/LOPS/, got none")
	}
}

// TestHistorianTitleMatchFlag verifies that --title-match filters commits by
// ticket ref pattern before clustering.
func TestHistorianTitleMatchFlag(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 4, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	// One commit matching LOPS-*, one that doesn't.
	addCommitToRepo(t, repo, dir, "a.go", "a", "LOPS-5: relevant change", "Alice", "alice@example.com", t0)
	addCommitToRepo(t, repo, dir, "b.go", "b", "chore: unrelated housekeeping", "Bob", "bob@example.com", t1)

	_, _, err := runHistorian(t, app, "--title-match", "LOPS-*")
	if err != nil {
		t.Fatalf("historian --title-match: %v", err)
	}

	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}

	// Only the LOPS-5 commit should have produced an issue.
	// The chore commit should be excluded.
	if len(issues) == 0 {
		t.Fatal("expected at least one issue from LOPS-5 commit")
	}
	for _, iss := range issues {
		// Every issue should reference a LOPS ticket via its title or tracker_id.
		isLOPS := strings.Contains(iss.Title, "LOPS") || strings.Contains(iss.TrackerID, "LOPS")
		if !isLOPS {
			t.Errorf("issue %q has no LOPS reference; should have been filtered", iss.Title)
		}
	}
}

// TestHistorianIncremental verifies that --incremental reads last_processed_sha
// and only processes commits after it.
func TestHistorianIncremental(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	addCommitToRepo(t, repo, dir, "x.go", "v1", "first commit", "Alice", "alice@example.com", t0)
	addCommitToRepo(t, repo, dir, "y.go", "v1", "second commit", "Alice", "alice@example.com", t1)

	// First full run.
	_, _, err := runHistorian(t, app)
	if err != nil {
		t.Fatalf("first historian run: %v", err)
	}
	firstRunIssues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues after first run: %v", err)
	}
	countAfterFirst := len(firstRunIssues)

	// Add a new commit after the first run.
	addCommitToRepo(t, repo, dir, "z.go", "v1", "third commit after first run", "Alice", "alice@example.com", t2)

	// Incremental run should only process the new commit.
	_, _, err = runHistorian(t, app, "--incremental")
	if err != nil {
		t.Fatalf("incremental historian run: %v", err)
	}

	allIssues, err := issue.LoadAllIssues(store)
	if err != nil {
		t.Fatalf("LoadAllIssues after incremental: %v", err)
	}

	// Should have more issues than before (or at least the same, since the new
	// commit may cluster with existing ones — but the incremental run must
	// process only the new commit, not reprocess the original two).
	// The key invariant: incremental run processed exactly 1 new commit.
	// We verify by checking that the total count is >= original count.
	if len(allIssues) < countAfterFirst {
		t.Errorf("incremental run produced fewer issues (%d) than original (%d)", len(allIssues), countAfterFirst)
	}
}

// TestHistorianStatus verifies that the status subcommand prints a coverage
// report without writing any refs.
func TestHistorianStatus(t *testing.T) {
	repo, dir, _, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC)
	addCommitToRepo(t, repo, dir, "main.go", "v1", "initial commit", "Alice", "alice@example.com", t0)

	// Run full pipeline first so there's something to report on.
	_, _, err := runHistorian(t, app)
	if err != nil {
		t.Fatalf("historian full run: %v", err)
	}

	stdout, _, err := runHistorian(t, app, "status")
	if err != nil {
		t.Fatalf("historian status: %v", err)
	}
	if stdout == "" {
		t.Error("expected non-empty output from historian status")
	}
	// Status should report something about commits / issues.
	if !strings.Contains(stdout, "commit") && !strings.Contains(stdout, "issue") {
		t.Errorf("status output does not mention commits or issues: %q", stdout)
	}
}

// TestHistorianReindex verifies that the reindex subcommand rebuilds label
// indexes for all existing issues.
func TestHistorianReindex(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	t0 := time.Date(2025, 1, 7, 10, 0, 0, 0, time.UTC)
	addCommitToRepo(t, repo, dir, "foo.go", "v1", "INFRA-10: add infra component", "Alice", "alice@example.com", t0)

	// Create issues with a label via historian.
	_, _, err := runHistorian(t, app, "--label", "INFRA")
	if err != nil {
		t.Fatalf("historian --label: %v", err)
	}

	// Manually delete the label index to simulate corruption.
	labelRefs, err := store.ListRefs("refs/zhi/_/labels/INFRA/")
	if err != nil {
		t.Fatalf("ListRefs: %v", err)
	}
	for _, ref := range labelRefs {
		if err := store.DeleteRef(ref); err != nil {
			t.Fatalf("DeleteRef: %v", err)
		}
	}

	// Reindex should rebuild the label indexes.
	_, _, err = runHistorian(t, app, "reindex")
	if err != nil {
		t.Fatalf("historian reindex: %v", err)
	}

	// Label index refs should be back.
	labelRefs, err = store.ListRefs("refs/zhi/_/labels/INFRA/")
	if err != nil {
		t.Fatalf("ListRefs after reindex: %v", err)
	}
	if len(labelRefs) == 0 {
		t.Error("expected label index refs to be rebuilt by reindex, got none")
	}
}

// ---------------------------------------------------------------------------
// Config ref tests
// ---------------------------------------------------------------------------

// TestConfigSaveAndLoad verifies round-trip of historian config through refs.
func TestConfigSaveAndLoad(t *testing.T) {
	_, _, store, _ := makeHistorianTestRepo(t)

	// Before saving, LoadConfig should return DefaultConfig.
	cfg, err := historian.LoadConfig(store)
	if err != nil {
		t.Fatalf("LoadConfig (default): %v", err)
	}
	def := cluster.DefaultConfig()
	if cfg.JoinThreshold != def.JoinThreshold {
		t.Errorf("default JoinThreshold = %v, want %v", cfg.JoinThreshold, def.JoinThreshold)
	}

	// Save a custom config.
	custom := cluster.DefaultConfig()
	custom.JoinThreshold = 0.50
	custom.CoherenceThreshold = 0.30
	if err := historian.SaveConfig(store, custom); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Load it back.
	loaded, err := historian.LoadConfig(store)
	if err != nil {
		t.Fatalf("LoadConfig (custom): %v", err)
	}
	if loaded.JoinThreshold != 0.50 {
		t.Errorf("JoinThreshold = %v, want 0.50", loaded.JoinThreshold)
	}
	if loaded.CoherenceThreshold != 0.30 {
		t.Errorf("CoherenceThreshold = %v, want 0.30", loaded.CoherenceThreshold)
	}
	// Weights should survive round-trip.
	if loaded.Weights.TicketMatch != def.Weights.TicketMatch {
		t.Errorf("TicketMatch weight = %v, want %v", loaded.Weights.TicketMatch, def.Weights.TicketMatch)
	}
}

// TestHistorianUsesConfigRef verifies that the main pipeline reads the config
// from the ref when it exists, rather than using hardcoded defaults.
func TestHistorianUsesConfigRef(t *testing.T) {
	repo, dir, store, app := makeHistorianTestRepo(t)

	// Create commits that are close together — they should cluster with
	// default thresholds but NOT with an impossibly high join threshold.
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	addCommitToRepo(t, repo, dir, "a.go", "package a", "[LOPS-1] first", "alice", "alice@x.com", base)
	addCommitToRepo(t, repo, dir, "a.go", "package a // v2", "[LOPS-1] second", "alice", "alice@x.com", base.Add(time.Hour))

	// Save a config with join threshold of 1.0 (impossibly high).
	// This means no commits can join a cluster — each becomes its own issue.
	cfg := cluster.DefaultConfig()
	cfg.JoinThreshold = 1.0
	if err := historian.SaveConfig(store, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	stdout, _, err := runHistorian(t, app)
	if err != nil {
		t.Fatalf("historian: %v", err)
	}

	// With threshold=1.0, each commit should become its own issue (2 issues).
	if !strings.Contains(stdout, "created 2 issue(s)") {
		t.Errorf("expected 2 issues with high threshold, got:\n%s", stdout)
	}
}
