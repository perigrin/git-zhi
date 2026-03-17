// ABOUTME: Entry point for the git-zhi-historian binary. Git discovers this as
// ABOUTME: a subcommand plugin when the binary is on $PATH (invoked as 'git zhi historian').
package main

import (
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/historian"
	"github.com/perigrin/git-zhi/internal/storage"
)

func main() {
	cmd := historian.NewHistorianCommand()

	// Open the git repository from the current working directory and inject
	// the App into the command context so RunE can access the store and repo.
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-historian: get working directory: %s\n", err)
		os.Exit(1)
	}

	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-historian: open git repository: %s\n", err)
		os.Exit(1)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-historian: create store: %s\n", err)
		os.Exit(1)
	}

	app := &cli.App{Store: store, Repo: repo}
	cmd.SetContext(cli.WithApp(cmd.Context(), app))

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-historian: %s\n", err)
		os.Exit(1)
	}
}
