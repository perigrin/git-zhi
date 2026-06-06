// ABOUTME: Tests for the App struct: context round-trip, lazy repo opening,
// ABOUTME: and EnsureInitialized creating default config and milestone.
package cli_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/storage"
)

func TestGetApp_NilOnBareContext(t *testing.T) {
	app := cli.GetApp(context.Background())
	if app != nil {
		t.Fatal("expected nil App from bare context")
	}
}

func TestWithApp_RoundTrip(t *testing.T) {
	original := &cli.App{}
	ctx := cli.WithApp(context.Background(), original)
	retrieved := cli.GetApp(ctx)
	if retrieved != original {
		t.Fatal("expected GetApp to return the same App that was set with WithApp")
	}
}

func TestEnsureInitialized_CreatesConfigAndMilestone(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}

	if !store.RefExists("refs/zhi/_/config") {
		t.Fatal("expected refs/zhi/_/config to exist after init")
	}
	if !store.RefExists("refs/zhi/_/milestones/v0.1") {
		t.Fatal("expected refs/zhi/_/milestones/v0.1 to exist after init")
	}

	// Second call should be a no-op
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("second EnsureInitialized failed: %v", err)
	}
}

func TestOpenRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	subdir := dir + "/sub"
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	app, err := cli.OpenRepo(subdir)
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil App")
	}
	if app.Store == nil {
		t.Fatal("expected non-nil Store")
	}
}

// runGit runs a git command (optionally with -C dir) and returns combined output, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s\n%s", args, err, out)
	}
	return string(out)
}

// runGitCommit makes a commit with deterministic identity.
func runGitCommit(t *testing.T, dir, message string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "commit", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s\n%s", err, out)
	}
}

func TestReportMigration(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, ""},
		{1, "notice: recovered 1 worktree-local ref into the shared refs/zhi namespace\n"},
		{13, "notice: recovered 13 worktree-local refs into the shared refs/zhi namespace\n"},
	}
	for _, tc := range tests {
		var buf strings.Builder
		app := &cli.App{MigratedRefCount: tc.count}
		app.ReportMigration(&buf)
		if buf.String() != tc.want {
			t.Errorf("count %d: got %q, want %q", tc.count, buf.String(), tc.want)
		}
	}
}

func TestOpenRepo_MigratesStrandedWorktreeRefs(t *testing.T) {
	// Main repo with an initial commit.
	mainDir := t.TempDir()
	runGit(t, "", "init", mainDir)
	testFile := filepath.Join(mainDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, mainDir, "add", "test.txt")
	runGitCommit(t, mainDir, "initial")
	headSHA := strings.TrimSpace(runGit(t, mainDir, "rev-parse", "HEAD"))

	// Add a linked worktree.
	wtDir := filepath.Join(t.TempDir(), "worktree")
	runGit(t, mainDir, "worktree", "add", wtDir, "-b", "feature/strand")

	// Resolve the worktree-local git dir and plant a stranded chain ref there,
	// exactly as an older git-zhi (without commondir support) would have.
	wtGitDir := strings.TrimSpace(runGit(t, wtDir, "rev-parse", "--git-dir"))
	if !filepath.IsAbs(wtGitDir) {
		wtGitDir = filepath.Join(wtDir, wtGitDir)
	}
	strandedRefPath := filepath.Join(wtGitDir, "refs", "zhi", "_", "issues", "019e9a26-aaaa-7000-8000-000000000001")
	if err := os.MkdirAll(filepath.Dir(strandedRefPath), 0o755); err != nil {
		t.Fatalf("mkdir stranded ref dir: %v", err)
	}
	if err := os.WriteFile(strandedRefPath, []byte(headSHA+"\n"), 0o644); err != nil {
		t.Fatalf("write stranded ref: %v", err)
	}

	// OpenRepo should detect and migrate the stranded ref into the shared namespace.
	app, err := cli.OpenRepo(wtDir)
	if err != nil {
		t.Fatalf("OpenRepo in worktree failed: %v", err)
	}

	if app.MigratedRefCount != 1 {
		t.Errorf("MigratedRefCount = %d, want 1", app.MigratedRefCount)
	}
	wantRef := "refs/zhi/_/issues/019e9a26-aaaa-7000-8000-000000000001"
	if !app.Store.RefExists(wantRef) {
		t.Errorf("expected migrated ref %s to be visible in shared namespace", wantRef)
	}

	// Re-opening must be idempotent: nothing left to migrate.
	app2, err := cli.OpenRepo(wtDir)
	if err != nil {
		t.Fatalf("second OpenRepo failed: %v", err)
	}
	if app2.MigratedRefCount != 0 {
		t.Errorf("second OpenRepo MigratedRefCount = %d, want 0 (idempotent)", app2.MigratedRefCount)
	}
}

func TestOpenRepo_WorktreeHEADResolution(t *testing.T) {
	// Create main repo with a commit so HEAD exists.
	mainDir := t.TempDir()
	cmd := exec.Command("git", "init", mainDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s\n%s", err, out)
	}

	// Create an initial commit on the default branch.
	testFile := filepath.Join(mainDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd = exec.Command("git", "-C", mainDir, "add", "test.txt")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", mainDir, "commit", "-m", "initial")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s\n%s", err, out)
	}

	// Create a feature branch and add a worktree for it.
	wtDir := filepath.Join(t.TempDir(), "worktree")
	cmd = exec.Command("git", "-C", mainDir, "worktree", "add", wtDir, "-b", "feature/test-branch")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %s\n%s", err, out)
	}

	// Open the worktree with OpenRepo — this is the code under test.
	app, err := cli.OpenRepo(wtDir)
	if err != nil {
		t.Fatalf("OpenRepo in worktree failed: %v", err)
	}

	// RepoHEAD must resolve the branch ref that lives in the main repo's
	// refs/heads/, not in the worktree-local refs directory.
	head, err := app.Store.RepoHEAD()
	if err != nil {
		t.Fatalf("RepoHEAD in worktree failed: %v", err)
	}
	if head == "" {
		t.Fatal("expected non-empty HEAD SHA in worktree")
	}

	// Verify HEAD matches the commit we made.
	cmd = exec.Command("git", "-C", wtDir, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %s\n%s", err, out)
	}
	expected := strings.TrimSpace(string(out))
	if head != expected {
		t.Fatalf("RepoHEAD returned %q, expected %q", head, expected)
	}
}

// TestEnsureInitialized_PrintsPushRefspecNote verifies that the first-time init
// prints a note about configuring the push refspec when a remote exists.
func TestEnsureInitialized_PrintsPushRefspecNote(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Add an "origin" remote so EnsureInitialized detects it.
	remoteDir := t.TempDir()
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteDir},
	})
	if err != nil {
		t.Fatalf("failed to create remote: %v", err)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}

	var out strings.Builder
	if err := app.EnsureInitializedWithOutput(&out); err != nil {
		t.Fatalf("EnsureInitializedWithOutput failed: %v", err)
	}

	note := out.String()
	if !strings.Contains(note, "refs/zhi/*:refs/zhi/*") {
		t.Errorf("expected push refspec note in output, got:\n%s", note)
	}
	if !strings.Contains(note, "remote.origin.push") {
		t.Errorf("expected 'remote.origin.push' in note, got:\n%s", note)
	}
}

func TestEnsureInitialized_WithRemote(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Add an "origin" remote so EnsureInitialized can configure refspecs.
	remoteDir := t.TempDir()
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteDir},
	})
	if err != nil {
		t.Fatalf("failed to create remote: %v", err)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	app := &cli.App{Store: store, Repo: repo}

	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}

	// Verify the fetch refspec for refs/zhi/* was added.
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("failed to read repo config: %v", err)
	}
	remote, ok := cfg.Remotes["origin"]
	if !ok {
		t.Fatal("expected origin remote to exist")
	}

	hasFetchSpec := false
	for _, spec := range remote.Fetch {
		if strings.Contains(spec.String(), "refs/zhi/") {
			hasFetchSpec = true
		}
	}
	if !hasFetchSpec {
		t.Errorf("expected refs/zhi/* fetch refspec to be configured, got: %v", remote.Fetch)
	}
}
