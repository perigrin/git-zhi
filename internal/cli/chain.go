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
	cmd.Flags().Bool("critical", false, "show critical chain only")
	cmd.Flags().Bool("ready", false, "show ready set with path overlap analysis")
	cmd.Flags().Bool("graph", false, "ASCII DAG visualization (not yet implemented)")
	return cmd
}

// NewConfigCommand creates the top-level 'config' command.
func NewConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config [key] [value]",
		Short: "Manage git-zhi settings",
		Args:  cobra.MaximumNArgs(2),
		RunE:  runChainConfig,
	}
}

// NewNextCommand creates the top-level 'next' command.
func NewNextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next issue to work on (alias for 'issue show HEAD')",
		RunE:  runChainNext,
	}
	cmd.Flags().String("actor", "", "per-worker identity for multi-agent resolution (e.g. agent:claude-code-1)")
	return cmd
}
