// ABOUTME: The version command displays git-zhi version and build information.
// ABOUTME: Version, build time, and commit hash are set via ldflags at build time.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/version"
)

// NewVersionCommand creates the 'version' command.
func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use: "version",
		// Takes no positional arguments; without NoArgs cobra discards them
		// silently, so `git zhi sync push` runs the default and exits 0.
		Args:  cobra.NoArgs,
		Short: "Print git-zhi version and build information",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				fmt.Fprintf(cmd.OutOrStdout(), `{"version":"%s","build_time":"%s","commit":"%s"}`+"\n",
					version.Version, version.BuildTime, version.CommitHash)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), version.GetBuildInfo())
			return nil
		},
	}
}
