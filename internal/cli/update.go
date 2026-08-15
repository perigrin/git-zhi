// ABOUTME: The update command points at the install script rather than
// ABOUTME: replacing the running binary in-process.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/version"
)

// installCommand is the canonical one-liner that installs or upgrades git-zhi.
// It matches the usage documented at the top of install.sh.
const installCommand = "curl -fsSL https://raw.githubusercontent.com/perigrin/git-zhi/pu/install.sh | sh"

// NewUpdateCommand creates the 'update' command. git-zhi is distributed as a
// release tarball and installed by install.sh, so updating is the same
// operation as installing — we print the command rather than reimplementing
// download, replacement, and rollback in the binary itself.
func NewUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use: "update",
		// Takes no positional arguments; without NoArgs cobra discards them
		// silently, so `git zhi sync push` runs the default and exits 0.
		Args:  cobra.NoArgs,
		Short: "Show how to update git-zhi to the latest release",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "git-zhi %s\n\n", version.Version)
			fmt.Fprintf(out, "To update to the latest release, run:\n\n    %s\n\n", installCommand)
			fmt.Fprintln(out, "Releases: https://github.com/perigrin/git-zhi/releases")
			return nil
		},
	}
}
