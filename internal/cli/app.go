// ABOUTME: App struct holds shared dependencies (Store, Repo) for CLI commands.
// ABOUTME: Provides lazy repo opening and chain initialization.
package cli

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"

	"github.com/perigrin/git-zhi/internal/config"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/storage"
)

type contextKey string

const appKey contextKey = "app"

// App holds shared dependencies for all CLI commands.
type App struct {
	Store *storage.Store
	Repo  *git.Repository

	// MigratedRefCount is the number of stranded worktree-local refs/zhi refs
	// that OpenRepo migrated into the shared namespace. Zero in the common case.
	MigratedRefCount int

	// MigrationErr records a non-fatal failure during stranded-ref migration.
	// Migration is best-effort and never blocks a command, but a failure is
	// surfaced so a still-invisible chain is not mistaken for "nothing to do".
	MigrationErr error
}

// worktreeDir returns the absolute path to the repo's working tree root, used
// when shelling out to git (e.g. by sync).
func (a *App) worktreeDir() (string, error) {
	wt, err := a.Repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("resolve worktree: %w", err)
	}
	return wt.Filesystem.Root(), nil
}

// ReportMigration writes a one-line notice to w when OpenRepo recovered
// stranded worktree-local refs, so the user knows their previously-invisible
// chain has been restored. Writes nothing when no refs were migrated.
func (a *App) ReportMigration(w io.Writer) {
	if a == nil {
		return
	}
	if a.MigrationErr != nil {
		fmt.Fprintf(w, "warning: worktree-local ref migration failed: %v\n", a.MigrationErr)
	}
	if a.MigratedRefCount == 0 {
		return
	}
	suffix := "ref"
	if a.MigratedRefCount != 1 {
		suffix = "refs"
	}
	fmt.Fprintf(w, "notice: recovered %d worktree-local %s into the shared refs/zhi namespace\n", a.MigratedRefCount, suffix)
}

// WithApp attaches an App to a command's context.
func WithApp(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, appKey, app)
}

// GetApp retrieves the App from a command's context.
func GetApp(ctx context.Context) *App {
	app, _ := ctx.Value(appKey).(*App)
	return app
}

// OpenRepo opens the git repository at or above the given directory
// and returns an App with a Store backed by that repo.
func OpenRepo(dir string) (*App, error) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open git repository: %w", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	app := &App{Store: store, Repo: repo}

	// Recover chain refs that older versions stranded in a linked worktree's
	// own refs directory (they predate commondir support). Best-effort: a
	// failure here must not block normal commands, but it is recorded so the
	// user is not left with a still-invisible chain and no diagnostic.
	migrated, mErr := migrateStrandedRefs(dir, store)
	app.MigratedRefCount = migrated
	app.MigrationErr = mErr

	return app, nil
}

// migrateStrandedRefs resolves the per-worktree and common git directories for
// dir and asks the store to copy any stranded worktree-local refs/zhi pointers
// into the shared namespace. Returns the number migrated.
func migrateStrandedRefs(dir string, store *storage.Store) (int, error) {
	gitDir, err := gitRevParse(dir, "--git-dir")
	if err != nil {
		return 0, err
	}
	commonDir, err := gitRevParse(dir, "--git-common-dir")
	if err != nil {
		return 0, err
	}
	return store.MigrateStrandedWorktreeRefs(gitDir, commonDir, "refs/zhi")
}

// gitRevParse runs `git -C dir rev-parse <flag>` and returns the trimmed,
// absolute path. Relative results are resolved against dir, matching how git
// reports per-worktree paths.
func gitRevParse(dir, flag string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", flag)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", flag, err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return path, nil
}

// EnsureInitialized checks for chain state and creates it if missing.
// Writes default config and default milestone on first use. Any output
// (e.g., push refspec guidance) is discarded. Use EnsureInitializedWithOutput
// when init output should be shown to the user.
func (a *App) EnsureInitialized() error {
	return a.EnsureInitializedWithOutput(io.Discard)
}

// EnsureInitializedWithOutput is like EnsureInitialized but writes any user-
// facing notes (e.g., push refspec guidance) to w. On first init with a
// remote, it prints a note about configuring the push refspec since go-git's
// API only exposes the fetch refspec field on RemoteConfig.
func (a *App) EnsureInitializedWithOutput(w io.Writer) error {
	cfg := config.Default()
	configExists := a.Store.RefExists("refs/zhi/_/config")
	milestoneExists := a.Store.RefExists("refs/zhi/_/milestones/" + cfg.DefaultMilestone)
	if configExists && milestoneExists {
		return nil
	}

	if !configExists {
		cfgData, err := config.MarshalConfig(cfg)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		if err := a.Store.WriteEntity("refs/zhi/_/config", "config.yaml", cfgData, "Initialize chain config"); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	if !milestoneExists {
		ms := &milestone.Milestone{
			Name:    cfg.DefaultMilestone,
			Created: time.Now(),
		}
		msData, err := milestone.MarshalMilestone(ms)
		if err != nil {
			return fmt.Errorf("marshal milestone: %w", err)
		}
		refPath := "refs/zhi/_/milestones/" + cfg.DefaultMilestone
		if err := a.Store.WriteEntity(refPath, "milestone.yaml", msData, "Create default milestone: "+cfg.DefaultMilestone); err != nil {
			return fmt.Errorf("write milestone: %w", err)
		}
	}

	// Configure fetch refspecs for refs/zhi/* when a remote exists.
	// This is best-effort — silently skipped if no remote or config fails.
	a.configureRemoteRefspecs()

	// Print push refspec guidance when a remote is present. go-git's
	// RemoteConfig struct only exposes the Fetch field, so push refspecs
	// must be configured manually. One git-config command is all it takes.
	repoCfg, err := a.Repo.Config()
	if err == nil {
		if _, hasOrigin := repoCfg.Remotes["origin"]; hasOrigin {
			fmt.Fprintln(w, "Chain state initialized.")
			fmt.Fprintln(w, `Note: run 'git config --add remote.origin.push "refs/zhi/*:refs/zhi/*"' to enable push sync.`)
		}
	}

	return nil
}

// configureRemoteRefspecs adds a fetch refspec for refs/zhi/* to the origin
// remote if it exists and the refspec is not already present. Errors are
// silently ignored because sync configuration is best-effort at init time.
func (a *App) configureRemoteRefspecs() {
	repoCfg, err := a.Repo.Config()
	if err != nil {
		return
	}
	remote, ok := repoCfg.Remotes["origin"]
	if !ok {
		return
	}

	fetchSpec := gitconfig.RefSpec("+refs/zhi/*:refs/zhi/*")
	// Note: go-git's RemoteConfig only exposes a Fetch field; there is no Push
	// field in the struct. Push refspecs (refs/zhi/*:refs/zhi/*) must be
	// configured manually in .git/config until go-git adds Push support.

	hasFetch := false
	for _, spec := range remote.Fetch {
		if spec == fetchSpec {
			hasFetch = true
		}
	}

	if !hasFetch {
		remote.Fetch = append(remote.Fetch, fetchSpec)
		// SetConfig persists the updated remote configuration.
		_ = a.Repo.SetConfig(repoCfg)
	}
}
