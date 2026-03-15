// ABOUTME: App struct holds shared dependencies (Store, Repo) for CLI commands.
// ABOUTME: Attached to Cobra command context so subcommands can retrieve it.
package cli

import (
	"context"

	git "github.com/go-git/go-git/v5"

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
