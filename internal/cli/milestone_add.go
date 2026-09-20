// ABOUTME: Implementation of the milestone add command: creates a new
// ABOUTME: milestone ref with optional due date, body and resolution command.
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

	if err := milestone.ValidateMilestoneName(name); err != nil {
		return err
	}

	refPath := milestone.RefPrefix + name
	if app.Store.RefExists(refPath) {
		return fmt.Errorf("milestone %q already exists", name)
	}

	ms := &milestone.Milestone{
		Name:    name,
		Created: time.Now(),
	}

	if n := stdinFlagCount(cmd, "body", "resolution"); n > 1 {
		return fmt.Errorf("only one flag can read stdin with '-'; %d asked for it", n)
	}

	// Both route through readTextArg so add and edit agree on the '-' stdin
	// sentinel. A new milestone has nothing to clear, so "none" is stored
	// literally here rather than treated as the clear sentinel.
	if cmd.Flags().Changed("body") {
		bodyValue, _ := cmd.Flags().GetString("body")
		body, ok, readErr := readTextArg(cmd, "body", bodyValue)
		if readErr != nil {
			return fmt.Errorf("--body: %w", readErr)
		}
		if ok {
			ms.Body = body
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: --body - read empty input; body left unset\n", name)
		}
	}
	if cmd.Flags().Changed("resolution") {
		resolutionValue, _ := cmd.Flags().GetString("resolution")
		resolution, ok, readErr := readTextArg(cmd, "resolution", resolutionValue)
		if readErr != nil {
			return fmt.Errorf("--resolution: %w", readErr)
		}
		if ok {
			ms.Resolution = resolution
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: --resolution - read empty input; resolution left unset\n", name)
		}
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
