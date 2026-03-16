// ABOUTME: Entry point for the git-zhi binary. Git discovers this as a
// ABOUTME: subcommand when the binary is on $PATH (invoked as 'git zhi').
package main

import "github.com/perigrin/git-zhi/internal/cli"

func main() {
	cli.Execute()
}
