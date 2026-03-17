// ABOUTME: Entry point for the git-zhi-verify binary. Git discovers this as
// ABOUTME: a subcommand plugin when the binary is on $PATH (invoked as 'git zhi verify').
package main

import (
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/storage"
	"github.com/perigrin/git-zhi/internal/verify"
)

func main() {
	cmd := verify.NewVerifyCommand()

	// Open the git repository from the current working directory and inject
	// the App into the command context so RunE can access the store.
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-verify: get working directory: %s\n", err)
		os.Exit(1)
	}

	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-verify: open git repository: %s\n", err)
		os.Exit(1)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-verify: create store: %s\n", err)
		os.Exit(1)
	}

	app := &cli.App{Store: store, Repo: repo}
	cmd.SetContext(cli.WithApp(cmd.Context(), app))

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-verify: %s\n", err)
		os.Exit(1)
	}
}
