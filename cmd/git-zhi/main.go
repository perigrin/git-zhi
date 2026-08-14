// ABOUTME: Entry point for the git-zhi binary. Wires the plugin packages in as
// ABOUTME: subcommands; they import internal/cli, so they cannot self-register.
package main

import (
	"os"

	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/docs"
	"github.com/perigrin/git-zhi/internal/historian"
	"github.com/perigrin/git-zhi/internal/jira"
	"github.com/perigrin/git-zhi/internal/mermaid"
	"github.com/perigrin/git-zhi/internal/project"
	"github.com/perigrin/git-zhi/internal/sanbao"
	"github.com/perigrin/git-zhi/internal/verify"
)

func main() {
	cli.Execute(
		historian.NewHistorianCommand(),
		jira.NewJiraCommand(),
		verify.NewVerifyCommand(),
		sanbao.NewSanbaoCommand(),
		mermaid.NewMermaidCommand(),
		project.NewProjectCommand(),
		docsCommand(),
	)
}

// docsCommand builds the docs subcommand. Unlike the other plugins, docs needs
// the working-tree root and a go-git handle at construction time for its churn
// computation, and tolerates both being absent outside a repository.
func docsCommand() *cobra.Command {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	var repo *git.Repository
	r, openErr := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if openErr == nil {
		repo = r
		if wt, wtErr := repo.Worktree(); wtErr == nil {
			dir = wt.Filesystem.Root()
		}
	}
	return docs.NewDocsCommand(dir, repo)
}
