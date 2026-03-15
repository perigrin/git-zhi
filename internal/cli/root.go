// ABOUTME: Cobra root command for git-chain. Wires up all subcommand groups
// ABOUTME: and persistent flags (--format). Entry point for CLI execution.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// NewRootCommand creates the top-level git-chain command with all subcommands.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "git-chain",
		Short: "A git-native task graph for developers and agents",
		Long: `git-chain manages development work as a dependency graph with
built-in telemetry. All state lives in git refs under refs/chain/.
No external services required.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("format", "", "output format (json)")

	root.AddCommand(
		NewIssueCommand(),
		NewMilestoneCommand(),
		NewListCommand(),
		NewConfigCommand(),
		NewNextCommand(),
	)

	return root
}

// Execute runs the root command. Called from main.
func Execute() {
	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
