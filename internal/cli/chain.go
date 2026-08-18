// ABOUTME: Top-level chain commands: list (show full chain), config (manage settings),
// ABOUTME: and next (alias for 'issue show HEAD' — what should I work on?).
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewListCommand creates the top-level 'list' command.
func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		// Takes no positional arguments; without NoArgs cobra discards them
		// silently, so `git zhi sync push` runs the default and exits 0.
		Args:  cobra.NoArgs,
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
		Use: "config [key] [value]",
		// A single positional is never meaningful: config takes a key and a
		// value, or nothing. Left to MaximumNArgs it would fall through to the
		// display path, so `config reindx` printed the config and exited 0
		// while the index went un-rebuilt.
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return fmt.Errorf("unknown command %q for %q; usage: %s <key> <value>",
					args[0], cmd.CommandPath(), cmd.CommandPath())
			}
			return cobra.MaximumNArgs(2)(cmd, args)
		},
		Short: "Manage git-zhi settings",
		RunE:  runChainConfig,
	}
	cmd.AddCommand(newConfigReindexCommand())
	return cmd
}

// NewNextCommand creates the top-level 'next' command.
func NewNextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "next",
		// Takes no positional arguments; without NoArgs cobra discards them
		// silently, so `git zhi sync push` runs the default and exits 0.
		Args:  cobra.NoArgs,
		Short: "Show the next issue to work on (alias for 'issue show HEAD')",
		RunE:  runChainNext,
	}
	cmd.Flags().String("actor", "", "per-worker identity for multi-agent resolution (e.g. agent:claude-code-1)")
	cmd.Flags().String("label", "", "filter ready set to issues with this label")
	return cmd
}
