// ABOUTME: Top-level chain commands: list (show full chain), config (manage settings),
// ABOUTME: and next (alias for 'issue show HEAD' — what should I work on?).
package cli

import "github.com/spf13/cobra"

// NewListCommand creates the top-level 'list' command.
func NewListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the full chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("list: not yet implemented")
			return nil
		},
	}
}

// NewConfigCommand creates the top-level 'config' command.
func NewConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config [key] [value]",
		Short: "Manage git-chain settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("config: not yet implemented")
			return nil
		},
	}
}

// NewNextCommand creates the top-level 'next' command.
func NewNextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Show the next issue to work on (alias for 'issue show HEAD')",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("next: not yet implemented")
			return nil
		},
	}
}
