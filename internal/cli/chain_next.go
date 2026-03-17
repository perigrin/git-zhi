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
// global WIP=1 semantics.
func runChainNext(cmd *cobra.Command, args []string) error {
	actor, _ := cmd.Flags().GetString("actor")

	// Without --actor, preserve v0.1 global behavior by delegating directly.
	if actor == "" {
		return runIssueShow(cmd, []string{})
	}

	// With --actor, perform per-worker HEAD resolution and pass the resolved
	// UUID to runIssueShow so it uses the same formatting path.
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("no issues found")
	}

	g := graph.New(issues)
	head, err := g.Head(actor)
	if err != nil {
		return fmt.Errorf("resolve HEAD for actor %q: %w", actor, err)
	}

	// Pass the full UUID string so runIssueShow resolves it via UUID prefix.
	return runIssueShow(cmd, []string{head.ID.String()})
}
