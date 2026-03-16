// ABOUTME: Implementation of the milestone add command: creates a new milestone
// ABOUTME: ref with optional due date, erroring on duplicates.
package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/milestone"
)

// runMilestoneAdd creates a new milestone ref. It errors if the milestone
// already exists. The --due flag accepts YYYY-MM-DD format.
func runMilestoneAdd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("milestone add requires exactly one argument: <name>")
	}
	name := args[0]

	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("initialize chain: %w", err)
	}

	refPath := milestone.RefPrefix + name
	if app.Store.RefExists(refPath) {
		return fmt.Errorf("milestone %q already exists", name)
	}

	ms := &milestone.Milestone{
		Name:    name,
		Created: time.Now(),
	}

	dueStr, _ := cmd.Flags().GetString("due")
	if dueStr != "" {
		due, err := time.Parse("2006-01-02", dueStr)
		if err != nil {
			return fmt.Errorf("invalid --due date %q: use YYYY-MM-DD format", dueStr)
		}
		ms.Due = &due
	}

	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		return fmt.Errorf("marshal milestone: %w", err)
	}
	if err := app.Store.WriteEntity(refPath, "milestone.yaml", data, "Create milestone: "+name); err != nil {
		return fmt.Errorf("write milestone: %w", err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(ms)
	}

	msg := fmt.Sprintf("Created milestone: %s", name)
	if ms.Due != nil {
		msg += fmt.Sprintf(" [due: %s]", ms.Due.Format("Jan 02 2006"))
	}
	fmt.Fprintln(cmd.OutOrStdout(), msg)
	return nil
}
