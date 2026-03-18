// ABOUTME: The 'setup' subcommand creates companion symlinks for the unified binary.
// ABOUTME: Runs after install or update to ensure git discovers all plugin commands.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/dispatch"
)

// NewSetupCommand returns the "setup" subcommand that creates symlinks for
// all companion binary names in the same directory as the running binary.
func NewSetupCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Create companion symlinks for git plugin discovery",
		Long: `Creates symlinks (or hardlinks on Windows) for all git-zhi companion
commands in the same directory as the git-zhi binary. This enables git
to discover them as subcommands (e.g. 'git zhi historian').

Run this after installing or updating git-zhi. Safe to re-run — existing
correct links are left alone.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			created, err := dispatch.Setup()
			if err != nil {
				return err
			}
			if len(created) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "setup: all companion links already exist")
				return nil
			}
			for _, path := range created {
				fmt.Fprintf(cmd.OutOrStdout(), "  created: %s\n", path)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "setup: created %d companion link(s)\n", len(created))
			return nil
		},
	}
}
