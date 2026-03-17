// ABOUTME: Implementation of the chain list command: shows all issues topo-sorted
// ABOUTME: and grouped by milestone, with --critical for the critical chain view and --ready for ready set.
package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// chainListJSON is the JSON output shape for chain list.
type chainListJSON struct {
	Issues            []*issue.Issue `json:"issues"`
	CriticalChain     []*issue.Issue `json:"critical_chain,omitempty"`
	CurrentConstraint string         `json:"current_constraint,omitempty"`
	ParallelWork      []string       `json:"parallel_work,omitempty"`
}

// readySetIssueJSON is the JSON representation of a single issue in the ready set,
// including its declared context paths.
type readySetIssueJSON struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Paths []string `json:"paths,omitempty"`
}

// readySetJSON is the JSON output shape for list --ready.
type readySetJSON struct {
	ReadySet        []readySetIssueJSON `json:"ready_set"`
	OverlapDetected bool                `json:"overlap_detected"`
}

// issueWithPaths pairs an issue with its declared context paths.
type issueWithPaths struct {
	iss   *issue.Issue
	paths []string
}

// issueContextPaths extracts the declared context.paths from an issue body.
// Returns nil if no Context section or no paths key is present.
func issueContextPaths(iss *issue.Issue) []string {
	sections := issue.ParseSections(iss.Body)
	if sections.Context == nil {
		return nil
	}
	return sections.Context.Paths
}

// runChainList loads all issues, builds the dependency graph, applies filters,
// and renders the result grouped by milestone (default) or as critical chain.
func runChainList(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("ensure initialized: %w", err)
	}

	if cmd.Flags().Changed("graph") {
		return fmt.Errorf("--graph: not yet implemented")
	}

	includeAll, _ := cmd.Flags().GetBool("all")
	milestoneFilter, _ := cmd.Flags().GetString("milestone")
	showCritical, _ := cmd.Flags().GetBool("critical")
	showReady, _ := cmd.Flags().GetBool("ready")

	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	// Apply filters.
	var filtered []*issue.Issue
	for _, iss := range allIssues {
		if !includeAll && iss.State != issue.StatePending && iss.State != issue.StateInProgress {
			continue
		}
		if milestoneFilter != "" && iss.Milestone != milestoneFilter {
			continue
		}
		filtered = append(filtered, iss)
	}

	g, _ := graph.Build(filtered)

	format, _ := cmd.Root().PersistentFlags().GetString("format")

	if showReady {
		return runChainListReady(cmd, g, filtered, format)
	}

	if showCritical {
		chain := g.CriticalChain()

		// Compute parallel work: ready issues not already on the critical chain.
		chainSet := make(map[uuid.UUID]bool)
		for _, iss := range chain {
			chainSet[iss.ID] = true
		}
		ready := g.ReadySet()
		var parallel []*issue.Issue
		for _, iss := range ready {
			if !chainSet[iss.ID] {
				parallel = append(parallel, iss)
			}
		}

		if format == "json" {
			out := chainListJSON{
				Issues:        filtered,
				CriticalChain: chain,
			}
			if len(chain) > 0 {
				out.CurrentConstraint = chain[0].ID.String()[:8]
			}
			if len(parallel) > 0 {
				ids := make([]string, len(parallel))
				for i, iss := range parallel {
					ids[i] = iss.ID.String()[:8]
				}
				out.ParallelWork = ids
			}
			if out.Issues == nil {
				out.Issues = []*issue.Issue{}
			}
			if out.CriticalChain == nil {
				out.CriticalChain = []*issue.Issue{}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}
		return chainListCriticalHuman(cmd, chain, parallel)
	}

	sorted := g.TopologicalSort()

	if format == "json" {
		out := chainListJSON{Issues: sorted}
		if out.Issues == nil {
			out.Issues = []*issue.Issue{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	return chainListGroupedHuman(cmd, app, sorted, milestoneFilter)
}

// chainListGroupedHuman renders the issue list grouped by milestone in topo order.
func chainListGroupedHuman(cmd *cobra.Command, app *App, issues []*issue.Issue, milestoneFilter string) error {
	w := cmd.OutOrStdout()

	// Build milestone map: name -> issues (preserving topo order within each group).
	groups := make(map[string][]*issue.Issue)
	for _, iss := range issues {
		groups[iss.Milestone] = append(groups[iss.Milestone], iss)
	}

	// Load milestones to get due dates, sorted by name.
	milestones, err := milestone.LoadAllMilestones(app.Store)
	if err != nil {
		// Graceful degradation: proceed without due dates.
		milestones = nil
	}

	// Collect milestone names to display. Start with known milestones in order,
	// then append any milestone names seen on issues but not in the loaded list.
	seen := make(map[string]bool)
	var orderedNames []string
	for _, ms := range milestones {
		if milestoneFilter != "" && ms.Name != milestoneFilter {
			continue
		}
		if len(groups[ms.Name]) == 0 {
			continue
		}
		seen[ms.Name] = true
		orderedNames = append(orderedNames, ms.Name)
	}
	// Add any milestone names from issues not present in the loaded milestones list.
	for _, iss := range issues {
		if !seen[iss.Milestone] {
			seen[iss.Milestone] = true
			orderedNames = append(orderedNames, iss.Milestone)
		}
	}
	sort.Strings(orderedNames)

	// Build a quick lookup for milestone due dates.
	msDue := make(map[string]string)
	for _, ms := range milestones {
		if ms.Due != nil {
			msDue[ms.Name] = ms.Due.Format("Jan 02")
		}
	}

	for _, name := range orderedNames {
		group := groups[name]
		if len(group) == 0 {
			continue
		}

		// Milestone header.
		duePart := "no due date"
		if d, ok := msDue[name]; ok {
			duePart = "due: " + d
		}
		fmt.Fprintf(w, "%s [%s]\n\n", name, duePart)

		for _, iss := range group {
			icon := stateIcon[iss.State]
			if icon == "" {
				icon = "?"
			}
			fmt.Fprintf(w, "  %s  %-40s %s %s\n",
				iss.ID.String()[:8],
				iss.Title,
				icon,
				string(iss.State),
			)
		}
		fmt.Fprintln(w)
	}

	return nil
}

// runChainListReady handles the --ready flag: computes the ready set, performs
// path overlap analysis, and renders output in human or JSON format.
func runChainListReady(cmd *cobra.Command, g *graph.Graph, filtered []*issue.Issue, format string) error {
	ready := g.ReadySet()

	// Gather paths for each ready issue.
	readyWithPaths := make([]issueWithPaths, 0, len(ready))
	for _, iss := range ready {
		paths := issueContextPaths(iss)
		readyWithPaths = append(readyWithPaths, issueWithPaths{iss: iss, paths: paths})
	}

	// Detect overlap: accumulate all paths and check each new issue against the set.
	overlapDetected := false
	var accumulatedPaths []string
	for _, item := range readyWithPaths {
		if graph.PathsOverlap(accumulatedPaths, item.paths) {
			overlapDetected = true
			break
		}
		accumulatedPaths = append(accumulatedPaths, item.paths...)
	}

	if format == "json" {
		items := make([]readySetIssueJSON, len(readyWithPaths))
		for i, item := range readyWithPaths {
			items[i] = readySetIssueJSON{
				ID:    item.iss.ID.String()[:8],
				Title: item.iss.Title,
				Paths: item.paths,
			}
		}
		if items == nil {
			items = []readySetIssueJSON{}
		}
		out := readySetJSON{
			ReadySet:        items,
			OverlapDetected: overlapDetected,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	return chainListReadyHuman(cmd, readyWithPaths, overlapDetected)
}

// chainListReadyHuman renders the ready set with path analysis in human-readable format.
func chainListReadyHuman(cmd *cobra.Command, items []issueWithPaths, overlapDetected bool) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Ready set: %d issue", len(items))
	if len(items) != 1 {
		fmt.Fprintf(w, "s")
	}
	fmt.Fprintf(w, " (no blocking dependencies)\n\n")

	for _, item := range items {
		fmt.Fprintf(w, "  %s  %s", item.iss.ID.String()[:8], item.iss.Title)
		if len(item.paths) > 0 {
			fmt.Fprintf(w, "    paths: %s", strings.Join(item.paths, ", "))
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w)
	if overlapDetected {
		fmt.Fprintf(w, "  Path overlap detected. Some issues share files or directories.\n")
	} else {
		fmt.Fprintf(w, "  No path overlap detected. All %d safe for parallel execution.\n", len(items))
	}

	return nil
}

// chainListCriticalHuman renders the critical chain with arrow separators between issues,
// followed by the current constraint and any parallel work available.
func chainListCriticalHuman(cmd *cobra.Command, chain []*issue.Issue, parallel []*issue.Issue) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Critical Chain (%d issues)\n\n", len(chain))

	for i, iss := range chain {
		icon := stateIcon[iss.State]
		if icon == "" {
			icon = "?"
		}
		fmt.Fprintf(w, "  %s  %-40s %s %s\n",
			iss.ID.String()[:8],
			iss.Title,
			icon,
			string(iss.State),
		)
		if i < len(chain)-1 {
			fmt.Fprintln(w, "   ↓")
		}
	}

	if len(chain) > 0 {
		fmt.Fprintf(w, "\nCurrent constraint: %s\n", chain[0].ID.String()[:8])
	}
	if len(parallel) > 0 {
		ids := make([]string, len(parallel))
		for i, iss := range parallel {
			ids[i] = iss.ID.String()[:8]
		}
		fmt.Fprintf(w, "Parallel work available: %s\n", strings.Join(ids, ", "))
	}

	return nil
}
