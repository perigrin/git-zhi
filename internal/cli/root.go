// ABOUTME: Cobra root command for git-chain. Wires up all subcommand groups
// ABOUTME: and persistent flags (--format). Entry point for CLI execution.
package cli

import (
	"fmt"
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
		// SilenceUsage prevents Cobra from dumping usage on every error.
		// SilenceErrors prevents Cobra from printing errors (we do it in Execute).
		// Subcommands that want to show usage on bad arguments must call
		// cmd.Usage() explicitly before returning their error.
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
		fmt.Fprintf(os.Stderr, "git-chain: %s\n", err)
		os.Exit(1)
	}
}
