// ABOUTME: Implementation of the chain next command: shows the next issue to work on
// ABOUTME: by delegating to runIssueShow. Supports --actor for per-worker resolution.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
)

// runChainNext resolves the next issue for the given actor (or globally if no
// actor is specified) and delegates to runIssueShow for output formatting.
// Without --actor this is identical to v0.1 behavior: HEAD resolution via
// global WIP=1 semantics. --label restricts the candidate set to labeled issues.
func runChainNext(cmd *cobra.Command, args []string) error {
	actor, _ := cmd.Flags().GetString("actor")
	labelFilter, _ := cmd.Flags().GetString("label")

	// Without --actor and without --label, preserve v0.1 global behavior.
	if actor == "" && labelFilter == "" {
		return runIssueShow(cmd, []string{})
	}

	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}
	if len(allIssues) == 0 {
		return fmt.Errorf("no issues found")
	}

	// Build the graph from ALL issues so that blocking relationships across
	// label boundaries are respected.
	g := graph.New(allIssues)

	// When --label is set, compute the full ready set from the complete graph
	// and then filter to labeled issues only. This ensures that an unlabeled
	// blocker keeps a labeled issue out of the ready set.
	if labelFilter != "" {
		ready := g.ReadySet()
		var labeled []*issue.Issue
		for _, iss := range ready {
			for _, l := range iss.Labels {
				if l == labelFilter {
					labeled = append(labeled, iss)
					break
				}
			}
		}
		if len(labeled) == 0 {
			return fmt.Errorf("no issues found")
		}
		// Pass the full UUID of the first labeled ready issue.
		return runIssueShow(cmd, []string{labeled[0].ID.String()})
	}

	head, err := g.Head(actor)
	if err != nil {
		return fmt.Errorf("resolve HEAD for actor %q: %w", actor, err)
	}

	// Pass the full UUID string so runIssueShow resolves it via UUID prefix.
	return runIssueShow(cmd, []string{head.ID.String()})
}
