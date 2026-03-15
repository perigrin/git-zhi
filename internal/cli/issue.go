// ABOUTME: Cobra command group for issue subcommands (add, list, show, edit).
// ABOUTME: Each subcommand is a factory function returning a *cobra.Command.
package cli

import "github.com/spf13/cobra"

// NewIssueCommand creates the 'issue' command group.
func NewIssueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage issues in the chain",
	}

	cmd.AddCommand(
		newIssueAddCommand(),
		newIssueListCommand(),
		newIssueShowCommand(),
		newIssueEditCommand(),
	)

	return cmd
}

func newIssueAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Create one or more new issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue add: not yet implemented")
			return nil
		},
	}
}

func newIssueListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue list: not yet implemented")
			return nil
		},
	}
}

func newIssueShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [ref]",
		Short: "View an issue with full context",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue show: not yet implemented")
			return nil
		},
	}
}

func newIssueEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [ref]",
		Short: "Modify an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue edit: not yet implemented")
			return nil
		},
	}
}
