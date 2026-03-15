// ABOUTME: Tests for the App struct: context round-trip, lazy repo opening,
// ABOUTME: and EnsureInitialized creating default config and milestone.
package cli_test

import (
	"context"
	"os"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"

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

	// Verify the fetch refspec for refs/chain/* was added.
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
		if strings.Contains(spec.String(), "refs/chain/") {
			hasFetchSpec = true
		}
	}
	if !hasFetchSpec {
		t.Errorf("expected refs/chain/* fetch refspec to be configured, got: %v", remote.Fetch)
	}
}
