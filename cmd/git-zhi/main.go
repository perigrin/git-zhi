// ABOUTME: Unified entry point for all git-zhi binaries. Uses busybox-style
// ABOUTME: argv[0] dispatch — symlinks like git-zhi-historian route to the right command.
package main

import (
	"context"
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/dispatch"
	"github.com/perigrin/git-zhi/internal/docs"
	"github.com/perigrin/git-zhi/internal/historian"
	jira "github.com/perigrin/git-zhi/internal/jira"
	"github.com/perigrin/git-zhi/internal/mermaid"
	"github.com/perigrin/git-zhi/internal/project"
	"github.com/perigrin/git-zhi/internal/sanbao"
	"github.com/perigrin/git-zhi/internal/verify"
)

func main() {
	name := dispatch.ResolveName(os.Args[0])

	switch name {
	case "git-zhi-historian":
		runWithRepo(historian.NewHistorianCommand(), name)
	case "git-zhi-jira":
		runWithRepo(jira.NewJiraCommand(), name)
	case "git-zhi-verify":
		runWithRepo(verify.NewVerifyCommand(), name)
	case "git-zhi-sanbao":
		runWithRepo(sanbao.NewSanbaoCommand(), name)
	case "git-zhi-mermaid":
		runSimple(mermaid.NewMermaidCommand(), name)
	case "git-zhi-project":
		runSimple(project.NewProjectCommand(), name)
	case "git-zhi-docs":
		runDocs()
	default:
		// Core git-zhi command tree — repo opening handled by PersistentPreRunE.
		cli.Execute()
	}
}

// runWithRepo opens the git repository, injects an App into the command
// context, and executes the command.
func runWithRepo(cmd *cobra.Command, name string) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: get working directory: %s\n", name, err)
		os.Exit(1)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", name, err)
		os.Exit(1)
	}
	cmd.SetContext(cli.WithApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", name, err)
		os.Exit(1)
	}
}

// runSimple executes a command that needs no git repo context.
func runSimple(cmd *cobra.Command, name string) {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", name, err)
		os.Exit(1)
	}
}

// runDocs handles the docs command, which needs the worktree root path and an
// optional git.Repository for churn computation.
func runDocs() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-docs: get working directory: %s\n", err)
		os.Exit(1)
	}
	var repo *git.Repository
	r, openErr := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if openErr == nil {
		repo = r
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
