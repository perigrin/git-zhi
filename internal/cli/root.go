// ABOUTME: Cobra root command for git-zhi. Wires up all subcommand groups
// ABOUTME: and persistent flags (--format). Entry point for CLI execution.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewRootCommand creates the top-level git-zhi command with all subcommands.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "git-zhi",
		Short: "A git-native task graph for developers and agents",
		Long: `git-zhi manages development work as a dependency graph with
built-in telemetry. All state lives in git refs under refs/zhi/.
No external services required.`,
		// SilenceUsage prevents Cobra from dumping usage on every error.
		// SilenceErrors prevents Cobra from printing errors (we do it in Execute).
		// Subcommands that want to show usage on bad arguments must call
		// cmd.Usage() explicitly before returning their error.
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// If an App was already injected (e.g., by test harness), keep it.
			if GetApp(cmd.Context()) != nil {
				return nil
			}
			// Skip repo opening for help requests
			if cmd.Name() == "help" || cmd.Flags().Changed("help") {
				return nil
			}
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			app, err := OpenRepo(dir)
			if err != nil {
				// Not in a git repo — commands will fail when they try to use the store,
				// but --help and other non-repo commands should still work.
				return nil
			}
			app.ReportMigration(cmd.ErrOrStderr())
			cmd.SetContext(WithApp(cmd.Context(), app))
			return nil
		},
	}

	root.PersistentFlags().String("format", "", "output format (json)")

	root.AddCommand(
		NewIssueCommand(),
		NewMilestoneCommand(),
		NewListCommand(),
		NewConfigCommand(),
		NewNextCommand(),
		NewVersionCommand(),
		NewUpdateCommand(),
		NewSetupCommand(),
		NewStatusCommand(),
	)

	// Discover and register external git-zhi-* subcommands
	for _, extCmd := range DiscoverExternalCommands() {
		root.AddCommand(extCmd)
	}

	return root
}

// Execute runs the root command. Called from main.
func Execute() {
	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "git-zhi: %s\n", err)
		os.Exit(1)
	}
}
