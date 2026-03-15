// ABOUTME: Tests for the App struct: context round-trip, lazy repo opening,
// ABOUTME: and EnsureInitialized creating default config and milestone.
package cli_test

import (
	"context"
	"os"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-chain/internal/cli"
	"github.com/perigrin/git-chain/internal/storage"
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

	if !store.RefExists("refs/chain/_/config") {
		t.Fatal("expected refs/chain/_/config to exist after init")
	}
	if !store.RefExists("refs/chain/_/milestones/v0.1") {
		t.Fatal("expected refs/chain/_/milestones/v0.1 to exist after init")
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
