// ABOUTME: Top-level chain commands: list (show full chain), config (manage settings),
// ABOUTME: and next (alias for 'issue show HEAD' — what should I work on?).
package cli

import "github.com/spf13/cobra"

// NewListCommand creates the top-level 'list' command.
func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show the full chain",
		RunE:  runChainList,
	}
	cmd.Flags().Bool("all", false, "include done and cancelled issues")
	cmd.Flags().String("milestone", "", "filter by milestone")
	cmd.Flags().String("label", "", "filter by label")
	cmd.Flags().Bool("critical", false, "show critical chain only")
	cmd.Flags().Bool("ready", false, "show ready set with path overlap analysis")
	cmd.Flags().Bool("graph", false, "ASCII DAG visualization (not yet implemented)")
	return cmd
}

// NewConfigCommand creates the top-level 'config' command group.
// With no subcommand and no args it displays the current config;
// with key+value args it sets a config key. 'config reindex' rebuilds
// label indexes.
func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config [key] [value]",
		Short: "Manage git-zhi settings",
		Args:  cobra.MaximumNArgs(2),
		RunE:  runChainConfig,
	}
	cmd.AddCommand(newConfigReindexCommand())
	return cmd
}

// NewNextCommand creates the top-level 'next' command.
func NewNextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next issue to work on (alias for 'issue show HEAD')",
		RunE:  runChainNext,
	}
	cmd.Flags().String("actor", "", "per-worker identity for multi-agent resolution (e.g. agent:claude-code-1)")
	cmd.Flags().String("label", "", "filter ready set to issues with this label")
	return cmd
}
