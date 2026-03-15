// ABOUTME: Entry point for the git-chain binary. Git discovers this as a
// ABOUTME: subcommand when the binary is on $PATH (invoked as 'git chain').
package main

import "github.com/perigrin/git-chain/internal/cli"

func main() {
	cli.Execute()
}
