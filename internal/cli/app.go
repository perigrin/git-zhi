// ABOUTME: App struct holds shared dependencies (Store, Repo) for CLI commands.
// ABOUTME: Provides lazy repo opening and chain initialization.
package cli

import (
	"context"
	"fmt"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"

	"github.com/perigrin/git-chain/internal/config"
	"github.com/perigrin/git-chain/internal/milestone"
	"github.com/perigrin/git-chain/internal/storage"
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
		DetectDotGit: true,
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
// Writes default config and default milestone on first use. Checks both
// refs to handle partial init (e.g., config written but milestone failed).
// Also configures fetch refspecs for refs/chain/* when a remote exists.
func (a *App) EnsureInitialized() error {
	cfg := config.Default()
	configExists := a.Store.RefExists("refs/chain/_/config")
	milestoneExists := a.Store.RefExists("refs/chain/_/milestones/" + cfg.DefaultMilestone)
	if configExists && milestoneExists {
		return nil
	}

	if !configExists {
		cfgData, err := config.MarshalConfig(cfg)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		if err := a.Store.WriteEntity("refs/chain/_/config", "config.yaml", cfgData, "Initialize chain config"); err != nil {
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
		refPath := "refs/chain/_/milestones/" + cfg.DefaultMilestone
		if err := a.Store.WriteEntity(refPath, "milestone.yaml", msData, "Create default milestone: "+cfg.DefaultMilestone); err != nil {
			return fmt.Errorf("write milestone: %w", err)
		}
	}

	// Configure fetch refspecs for refs/chain/* when a remote exists.
	// This is best-effort — silently skipped if no remote or config fails.
	a.configureRemoteRefspecs()

	return nil
}

// configureRemoteRefspecs adds a fetch refspec for refs/chain/* to the origin
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

	fetchSpec := gitconfig.RefSpec("+refs/chain/*:refs/chain/*")
	// Note: go-git's RemoteConfig only exposes a Fetch field; there is no Push
	// field in the struct. Push refspecs (refs/chain/*:refs/chain/*) must be
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
