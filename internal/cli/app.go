// ABOUTME: App struct holds shared dependencies (Store, Repo) for CLI commands.
// ABOUTME: Provides lazy repo opening and chain initialization.
package cli

import (
	"context"
	"fmt"
	"time"

	git "github.com/go-git/go-git/v5"

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
// Writes default config and default milestone on first use.
func (a *App) EnsureInitialized() error {
	if a.Store.RefExists("refs/chain/_/config") {
		return nil
	}

	cfg := config.Default()
	cfgData, err := config.MarshalConfig(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := a.Store.WriteEntity("refs/chain/_/config", "config.yaml", cfgData, "Initialize chain config"); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

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

	return nil
}
