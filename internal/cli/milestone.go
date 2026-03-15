// ABOUTME: Cobra command group for milestone subcommands (add, list, show, edit).
// ABOUTME: Each subcommand is a factory function returning a *cobra.Command.
package cli

import "github.com/spf13/cobra"

// NewMilestoneCommand creates the 'milestone' command group.
func NewMilestoneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "milestone",
		Short: "Manage milestones",
	}

	cmd.AddCommand(
		newMilestoneAddCommand(),
		newMilestoneListCommand(),
		newMilestoneShowCommand(),
		newMilestoneEditCommand(),
	)

	return cmd
}

func newMilestoneAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new milestone",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone add: not yet implemented")
			return nil
		},
	}
}

func newMilestoneListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all milestones",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone list: not yet implemented")
			return nil
		},
	}
}

func newMilestoneShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [ref]",
		Short: "Show milestone detail with issues and fever chart",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone show: not yet implemented")
			return nil
		},
	}
}

func newMilestoneEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [ref]",
		Short: "Modify a milestone",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone edit: not yet implemented")
			return nil
		},
	}
}
