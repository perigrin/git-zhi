// ABOUTME: Implementation of 'config reindex': rebuilds label indexes from all issues.
// ABOUTME: Reports how many labels and issues were indexed after the operation.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/issue"
)

// newConfigReindexCommand creates the 'config reindex' subcommand.
func newConfigReindexCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild label indexes from all issues",
		RunE:  runChainConfigReindex,
	}
}

// runChainConfigReindex loads all issues, builds label indexes, and reports
// the number of labels and issues indexed.
func runChainConfigReindex(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("ensure initialized: %w", err)
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	if err := issue.BuildLabelIndexes(app.Store, issues); err != nil {
		return fmt.Errorf("build label indexes: %w", err)
	}

	// Count how many distinct labels are used across all issues.
	labelSet := make(map[string]struct{})
	for _, iss := range issues {
		for _, label := range iss.Labels {
			labelSet[label] = struct{}{}
		}
	}

	// Count issues that carry at least one label.
	labeledCount := 0
	for _, iss := range issues {
		if len(iss.Labels) > 0 {
			labeledCount++
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"Rebuilt indexes for %d label(s) across %d issue(s)\n",
		len(labelSet), labeledCount,
	)
	return nil
}
