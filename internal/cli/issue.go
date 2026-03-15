// ABOUTME: Cobra command group for issue subcommands (add, list, show, edit).
// ABOUTME: Each subcommand is a factory function returning a *cobra.Command.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	yaml "github.com/goccy/go-yaml"

	"github.com/perigrin/git-chain/internal/config"
	"github.com/perigrin/git-chain/internal/issue"
)

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
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create one or more new issues",
		RunE:  runIssueAdd,
	}

	cmd.Flags().String("milestone", "", "milestone to assign (overrides config default)")
	cmd.Flags().String("after", "", "issue ref that this issue depends on (not yet implemented)")
	cmd.Flags().String("before", "", "issue ref that depends on this issue (not yet implemented)")

	return cmd
}

func runIssueAdd(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("initialize chain: %w", err)
	}

	// Guard --after and --before: not yet implemented
	if cmd.Flags().Changed("after") {
		return fmt.Errorf("--after: not yet implemented")
	}
	if cmd.Flags().Changed("before") {
		return fmt.Errorf("--before: not yet implemented")
	}

	// Determine default milestone from config
	defaultMilestone := "v0.1"
	cfgData, err := app.Store.ReadEntity("refs/chain/_/config", "config.yaml")
	if err == nil {
		var cfg config.Config
		if yamlErr := yaml.Unmarshal(cfgData, &cfg); yamlErr == nil && cfg.DefaultMilestone != "" {
			defaultMilestone = cfg.DefaultMilestone
		}
	}

	// --milestone flag overrides config default
	if ms, _ := cmd.Flags().GetString("milestone"); ms != "" {
		defaultMilestone = ms
	}

	// Read stdin
	raw, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	blocks := issue.SplitBatch(raw)
	if len(blocks) == 0 {
		return fmt.Errorf("no issue content found on stdin")
	}

	now := time.Now()
	var created []*issue.Issue

	for _, block := range blocks {
		iss, parseErr := issue.Parse(block)
		if parseErr != nil {
			return fmt.Errorf("parse issue: %w", parseErr)
		}

		// Generate UUIDv7 for stable, time-ordered identity
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return fmt.Errorf("generate uuid: %w", idErr)
		}
		iss.ID = id

		// Apply defaults
		if iss.State == "" {
			iss.State = issue.StatePending
		}
		if iss.Milestone == "" {
			iss.Milestone = defaultMilestone
		}
		iss.Created = now
		iss.Updated = now

		created = append(created, iss)
	}

	// Wire sequential dependencies for batch: issue[i] blocks issue[i+1]
	for i := 0; i < len(created)-1; i++ {
		created[i].Blocks = append(created[i].Blocks, created[i+1].ID)
		created[i+1].BlockedBy = append(created[i+1].BlockedBy, created[i].ID)
	}

	// Persist each issue
	for _, iss := range created {
		data, marshalErr := issue.Marshal(iss)
		if marshalErr != nil {
			return fmt.Errorf("marshal issue %s: %w", iss.ID, marshalErr)
		}
		refPath := "refs/chain/_/issues/" + iss.ID.String()
		if writeErr := app.Store.WriteEntity(refPath, "issue.md", data, "Add issue: "+iss.Title); writeErr != nil {
			return fmt.Errorf("write issue %s: %w", iss.ID, writeErr)
		}
	}

	// Output
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(created)
	}

	// Human-readable output
	if len(created) == 1 {
		iss := created[0]
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s: %s\n", iss.ID.String()[:8], iss.Title)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Created %d issues (chained sequentially):\n", len(created))
		for i, iss := range created {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", iss.ID.String()[:8], iss.Title)
			if i < len(created)-1 {
				fmt.Fprintf(cmd.OutOrStdout(), "   \u2193\n")
			}
		}
	}

	return nil
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
