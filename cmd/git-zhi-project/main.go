// ABOUTME: Entry point for the git-zhi-project binary. Git discovers this as
// ABOUTME: a subcommand plugin when the binary is on $PATH (invoked as 'git zhi project').
package main

import (
	"fmt"
	"os"

	"github.com/perigrin/git-zhi/internal/project"
)

func main() {
	cmd := project.NewProjectCommand()

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-project: %s\n", err)
		os.Exit(1)
	}
}
