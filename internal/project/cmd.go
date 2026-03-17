// ABOUTME: Cobra command definitions for the git-zhi-project binary.
// ABOUTME: Provides 'project show' and 'project next' subcommands.
package project

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// NewProjectCommand creates and returns the root Cobra command for git-zhi-project.
// Subcommands: show, next.
func NewProjectCommand() *cobra.Command {
	var format string

	root := &cobra.Command{
		Use:   "git-zhi-project",
		Short: "Cross-repo project aggregation and CCPM buffer tracking",
		Long: `git-zhi-project reads a project definition YAML file describing multiple
git-zhi repos and computes cross-repo critical chain, CCPM buffers, and
per-worker recommendations.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&format, "format", "", "output format (json)")

	root.AddCommand(newShowCommand(&format))
	root.AddCommand(newNextCommand(&format))

	return root
}

// newShowCommand returns the 'show' subcommand.
func newShowCommand(format *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <file>",
		Short: "Show full project status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectFile := args[0]

			def, err := LoadProject(projectFile)
			if err != nil {
				return fmt.Errorf("load project: %w", err)
			}

			status, err := ComputeProjectStatus(def)
			if err != nil {
				return fmt.Errorf("compute project status: %w", err)
			}

			if *format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}

			return printProjectStatus(cmd, status)
		},
	}
}

// newNextCommand returns the 'next' subcommand.
func newNextCommand(format *string) *cobra.Command {
	var actor string

	cmd := &cobra.Command{
		Use:   "next <file>",
		Short: "Cross-repo next-issue recommendation for a specific worker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if actor == "" {
				return fmt.Errorf("--actor is required")
			}

			projectFile := args[0]
			def, err := LoadProject(projectFile)
			if err != nil {
				return fmt.Errorf("load project: %w", err)
			}

			repoPath, issueID, err := NextForActor(def, actor)
			if err != nil {
				return err
			}

			// Load the issue to get its title.
			title := ""
			for _, rd := range def.Repos {
				if rd.Path != repoPath {
					continue
				}
				rs, rsErr := openRepoState(rd)
				if rsErr == nil {
					for _, iss := range rs.issues {
						if iss.ID == issueID {
							title = iss.Title
							break
						}
					}
				}
				break
			}

			if *format == "json" {
				result := map[string]string{
					"repo":     repoPath,
					"issue_id": issueID.String(),
					"title":    title,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			shortID := issueID.String()
			if len(shortID) >= 8 {
				shortID = shortID[:8]
			}
			fmt.Fprintf(cmd.OutOrStdout(), "repo:  %s\n", repoPath)
			fmt.Fprintf(cmd.OutOrStdout(), "issue: %s  %s\n", shortID, title)
			return nil
		},
	}

	cmd.Flags().StringVar(&actor, "actor", "", "worker identity to find next issue for (required)")
	return cmd
}

// printProjectStatus renders project status in human-readable form.
func printProjectStatus(cmd *cobra.Command, status *ProjectStatus) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Project: %s\n", status.Name)
	fmt.Fprintf(w, "\nRepos:\n")
	for _, rs := range status.Repos {
		pct := int(rs.Progress * 100)
		fmt.Fprintf(w, "  %s  milestone: %s  progress: %d%%  fever: %s  issues: %d/%d\n",
			rs.Path, rs.Milestone, pct, rs.FeverChart, rs.IssuesDone, rs.IssuesTotal)
	}

	fmt.Fprintf(w, "\nProject Buffer:\n")
	fmt.Fprintf(w, "  size: %.1f days  consumed: %.2f days (%.0f%%)  status: %s\n",
		status.ProjectBuffer.SizeDays,
		status.ProjectBuffer.ConsumedDays,
		status.ProjectBuffer.ConsumedPct*100,
		status.ProjectBuffer.Status)

	if len(status.FeedingBuffers) > 0 {
		fmt.Fprintf(w, "\nFeeding Buffers:\n")
		for _, fb := range status.FeedingBuffers {
			fmt.Fprintf(w, "  %s -> %s  size: %.1f days  status: %s\n",
				fb.FromRepo, fb.ToRepo, fb.Buffer.SizeDays, fb.Buffer.Status)
		}
	}

	if len(status.ResourceBuffers) > 0 {
		fmt.Fprintf(w, "\nResource Buffers:\n")
		for _, rb := range status.ResourceBuffers {
			line := fmt.Sprintf("  %s: %s", rb.Worker, rb.Status)
			if rb.Detail != "" {
				line += fmt.Sprintf("  (%s)", rb.Detail)
			}
			fmt.Fprintln(w, line)
		}
	}

	if len(status.CriticalChain) > 0 {
		fmt.Fprintf(w, "\nCritical Chain (repo order):\n")
		for i, repo := range status.CriticalChain {
			fmt.Fprintf(w, "  %d. %s\n", i+1, repo)
		}
	}

	return nil
}
