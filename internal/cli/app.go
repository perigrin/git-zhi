// ABOUTME: App struct holds shared dependencies (Store, Repo) for CLI commands.
// ABOUTME: Provides lazy repo opening and chain initialization.
package cli

import (
	"context"
	"fmt"
	"io"
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
	return &App{Store: store, Repo: repo}, nil
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
