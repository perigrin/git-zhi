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
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new milestone",
		Args:  cobra.ExactArgs(1),
		RunE:  runMilestoneAdd,
	}
	cmd.Flags().String("due", "", "due date in YYYY-MM-DD format")
	return cmd
}

func newMilestoneListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all milestones with issue counts",
		RunE:  runMilestoneList,
	}
}

func newMilestoneShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show milestone detail with issues and progress",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runMilestoneShow,
	}
	cmd.Flags().Int("workers", 0, "show forecast for up to N parallel workers (0 = no forecast)")
	cmd.Flags().String("label", "", "scope issue list and telemetry to issues with this label")
	return cmd
}

func newMilestoneEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Modify a milestone",
		Args:  cobra.ExactArgs(1),
		RunE:  runMilestoneEdit,
	}
	cmd.Flags().String("due", "", "set due date (YYYY-MM-DD) or 'none' to clear")
	cmd.Flags().String("name", "", "rename the milestone")
	cmd.Flags().String("tag", "", "create a named tag pointing to this milestone")
	cmd.Flags().String("untag", "", "delete a named tag")
	cmd.Flags().Bool("resolve", false, "execute the milestone's resolution command")
	cmd.Flags().String("state", "", "transition milestone state (only 'complete' is supported)")
	return cmd
}
