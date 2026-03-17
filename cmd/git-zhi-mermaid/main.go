// ABOUTME: Entry point for the git-zhi-mermaid binary. Git discovers this as
// ABOUTME: a subcommand plugin invoked as 'git zhi mermaid'. Reads JSON, emits Mermaid text.
package main

import (
	"fmt"
	"os"

	"github.com/perigrin/git-zhi/internal/mermaid"
)

func main() {
	// git-zhi-mermaid is a pure formatter: no git repo access, no ref reads.
	// It reads a JSON array of issues from stdin or --file and emits Mermaid text.
	cmd := mermaid.NewMermaidCommand()

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi-mermaid: %s\n", err)
		os.Exit(1)
	}
}
