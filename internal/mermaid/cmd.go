// ABOUTME: Cobra command definitions for the git-zhi-mermaid binary.
// ABOUTME: Dispatches to RenderGantt or RenderDAG after reading JSON from stdin or --file.
package mermaid

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// readInput reads the JSON issue list from either a named file (if filePath is
// non-empty) or from stdin (the command's configured input reader).
func readInput(cmd *cobra.Command, filePath string) ([]IssueInput, error) {
	var r io.Reader
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open file %q: %w", filePath, err)
		}
		defer f.Close()
		r = f
	} else {
		r = cmd.InOrStdin()
	}

	var issues []IssueInput
	if err := json.NewDecoder(r).Decode(&issues); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return issues, nil
}

// NewMermaidCommand creates and returns the root Cobra command for the
// git-zhi-mermaid binary. It provides two subcommands: gantt and dag.
func NewMermaidCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "git-zhi-mermaid",
		Short: "Render Mermaid charts from git-zhi JSON output",
		Long: `git-zhi-mermaid reads a JSON array of issues (from stdin or --file)
and renders a Mermaid chart to stdout.

Two chart types are supported:
  gantt   — Gantt chart with resource lanes grouped by assigned worker
  dag     — Directed-acyclic-graph showing blocked_by relationships

Typical usage:
  git zhi list --format json | git zhi mermaid gantt
  git zhi list --format json | git zhi mermaid dag`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// gantt subcommand
	var ganttFile string
	var ganttTitle string
	ganttCmd := &cobra.Command{
		Use:   "gantt",
		Short: "Render a Gantt chart with resource lanes from JSON issue list",
		RunE: func(cmd *cobra.Command, args []string) error {
			issues, err := readInput(cmd, ganttFile)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), RenderGantt(issues, ganttTitle))
			return nil
		},
	}
	ganttCmd.Flags().StringVar(&ganttFile, "file", "", "read JSON from file instead of stdin")
	ganttCmd.Flags().StringVar(&ganttTitle, "title", "", "chart title (default: Chain)")

	// dag subcommand
	var dagFile string
	dagCmd := &cobra.Command{
		Use:   "dag",
		Short: "Render a DAG visualization from JSON issue list",
		RunE: func(cmd *cobra.Command, args []string) error {
			issues, err := readInput(cmd, dagFile)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), RenderDAG(issues))
			return nil
		},
	}
	dagCmd.Flags().StringVar(&dagFile, "file", "", "read JSON from file instead of stdin")

	root.AddCommand(ganttCmd)
	root.AddCommand(dagCmd)

	return root
}
