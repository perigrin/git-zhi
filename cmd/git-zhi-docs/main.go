// ABOUTME: Entry point for the git-zhi-docs binary. Git discovers this as
// ABOUTME: a subcommand plugin when the binary is on $PATH (invoked as 'git zhi docs').
package main

import (
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/docs"
)

func main() {
	// Resolve the repository working directory. The docs commands operate on
	// the filesystem, so we need the worktree root rather than a git Store.
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-docs: get working directory: %s\n", err)
		os.Exit(1)
	}

	// Open the git repository so Health can compute churn via git log.
	// A nil repo is acceptable — Health degrades gracefully with zero drift.
	var repo *git.Repository
	r, openErr := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if openErr == nil {
		repo = r
		// Resolve the actual worktree root (may differ from cwd when inside a subdir).
		wt, wtErr := repo.Worktree()
		if wtErr == nil {
			dir = wt.Filesystem.Root()
		}
	}

	cmd := docs.NewDocsCommand(dir, repo)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-docs: %s\n", err)
		os.Exit(1)
	}
}
