// ABOUTME: Entry point for the git-zhi-jira binary. Git discovers this as
// ABOUTME: a subcommand plugin when the binary is on $PATH (invoked as 'git zhi jira').
package main

import (
	"context"
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	jira "github.com/perigrin/git-zhi/internal/jira"
	"github.com/perigrin/git-zhi/internal/storage"
)

func main() {
	cmd := jira.NewJiraCommand()

	// Open the git repository from the current working directory and inject
	// the App into the command context so RunE can access the store and repo.
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-jira: get working directory: %s\n", err)
		os.Exit(1)
	}

	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-jira: open git repository: %s\n", err)
		os.Exit(1)
	}

	store, err := storage.NewStore(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-jira: create store: %s\n", err)
		os.Exit(1)
	}

	app := &cli.App{Store: store, Repo: repo}
	cmd.SetContext(cli.WithApp(context.Background(), app))

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-jira: %s\n", err)
		os.Exit(1)
	}
}
