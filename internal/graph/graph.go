// ABOUTME: DAG construction and operations for the issue dependency graph.
// ABOUTME: Enforces graph invariants, computes critical chain and ready set.
package graph

import (
	"fmt"
	"log"
	"sort"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/uuids"
)

// Graph represents the issue dependency DAG.
type Graph struct {
	issues   map[uuid.UUID]*issue.Issue
	forward  map[uuid.UUID][]uuid.UUID // issue -> issues it blocks (downstream)
	backward map[uuid.UUID][]uuid.UUID // issue -> issues that block it (upstream)
}

// Len returns the number of issues in the graph.
func (g *Graph) Len() int { return len(g.issues) }

// Forward returns the UUIDs of issues that the given issue blocks (downstream).
func (g *Graph) Forward(id uuid.UUID) []uuid.UUID {
	return g.forward[id]
}

// Backward returns the UUIDs of issues that block the given issue (upstream).
func (g *Graph) Backward(id uuid.UUID) []uuid.UUID {
	return g.backward[id]
}

// New constructs a Graph from a set of issues using the legacy interface.
// Dangling edges are silently skipped. Prefer Build for full error context.
func New(issues []*issue.Issue) *Graph {
	g, _ := Build(issues)
	return g
}

// Build constructs a Graph from a set of issues, populating adjacency lists
// from issue.Blocks and issue.BlockedBy. Dangling edge references (to UUIDs
// not present in the input) are logged and skipped; Build never errors on them
// to allow graceful degradation for concurrent edits.
func Build(issues []*issue.Issue) (*Graph, error) {
	g := &Graph{
		issues:   make(map[uuid.UUID]*issue.Issue, len(issues)),
		forward:  make(map[uuid.UUID][]uuid.UUID, len(issues)),
		backward: make(map[uuid.UUID][]uuid.UUID, len(issues)),
	}
	for _, iss := range issues {
		g.issues[iss.ID] = iss
		// Initialise adjacency lists so lookups never return nil.
		if g.forward[iss.ID] == nil {
			g.forward[iss.ID] = []uuid.UUID{}
		}
		if g.backward[iss.ID] == nil {
			g.backward[iss.ID] = []uuid.UUID{}
		}
	}

	// Build edges from Blocks declarations.
	for _, iss := range issues {
		for _, downID := range iss.Blocks {
			if _, ok := g.issues[downID]; !ok {
				log.Printf("graph: issue %s references nonexistent downstream %s (skipped)", iss.ID, downID)
				continue
			}
			g.forward[iss.ID] = append(g.forward[iss.ID], downID)
			g.backward[downID] = append(g.backward[downID], iss.ID)
		}
	}

	// Build edges from BlockedBy declarations that were not already captured
	// by Blocks (issues may declare either direction). Avoid duplicates.
	for _, iss := range issues {
		for _, upID := range iss.BlockedBy {
			if _, ok := g.issues[upID]; !ok {
				log.Printf("graph: issue %s references nonexistent upstream %s (skipped)", iss.ID, upID)
				continue
			}
			// Only add if not already present (Blocks may have added it).
			if !uuids.ContainsUUID(g.forward[upID], iss.ID) {
				g.forward[upID] = append(g.forward[upID], iss.ID)
				g.backward[iss.ID] = append(g.backward[iss.ID], upID)
			}
		}
	}

	return g, nil
}

// Validate checks the graph for cycles using iterative DFS.
// Returns an error describing the first cycle found, or nil.
func (g *Graph) Validate() error {
	// Colour: 0=white (unvisited), 1=grey (in stack), 2=black (done)
	colour := make(map[uuid.UUID]int, len(g.issues))
	parent := make(map[uuid.UUID]uuid.UUID, len(g.issues))

	var hasCycle bool
	var cycleNode uuid.UUID

	var dfs func(id uuid.UUID)
	dfs = func(id uuid.UUID) {
		if hasCycle {
			return
		}
		colour[id] = 1
		for _, next := range g.forward[id] {
			switch colour[next] {
			case 0:
				parent[next] = id
				dfs(next)
			case 1:
				hasCycle = true
				cycleNode = next
				return
			}
		}
		colour[id] = 2
	}

	for id := range g.issues {
		if colour[id] == 0 {
			dfs(id)
			if hasCycle {
				break
			}
		}
	}

	if hasCycle {
		return fmt.Errorf("graph: cycle detected involving issue %s", cycleNode)
	}
	return nil
}

// HasCycle reports whether adding an edge from→to would create a cycle.
// It performs a DFS from `to` following forward edges looking for `from`.
func (g *Graph) HasCycle(from, to uuid.UUID) bool {
	// DFS from `to` following forward edges; if we reach `from`, it's a cycle.
	visited := make(map[uuid.UUID]bool)
	stack := []uuid.UUID{to}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == from {
			return true
		}
		if visited[cur] {
			continue
		}
		visited[cur] = true
		for _, next := range g.forward[cur] {
			stack = append(stack, next)
		}
	}
	return false
}

// AddEdge adds a dependency edge: from blocks to.
// Validates that both issues exist, that `to` is not done/cancelled
// (invariant 3), and that adding the edge would not create a cycle.
func (g *Graph) AddEdge(from, to uuid.UUID) error {
	if _, ok := g.issues[from]; !ok {
		return fmt.Errorf("graph: issue %s not found", from)
	}
	toIss, ok := g.issues[to]
	if !ok {
		return fmt.Errorf("graph: issue %s not found", to)
	}
	if toIss.State == issue.StateDone || toIss.State == issue.StateCancelled {
		return fmt.Errorf("graph: cannot add dependency to %s issue %s (invariant 3)", toIss.State, to)
	}
	if g.HasCycle(from, to) {
		return fmt.Errorf("graph: adding edge %s->%s would create a cycle", from, to)
	}
	g.forward[from] = append(g.forward[from], to)
	g.backward[to] = append(g.backward[to], from)
	return nil
}

// RemoveEdge removes the dependency edge from→to if it exists.
func (g *Graph) RemoveEdge(from, to uuid.UUID) {
	g.forward[from] = uuids.RemoveUUID(g.forward[from], to)
	g.backward[to] = uuids.RemoveUUID(g.backward[to], from)
}

// TopologicalSort returns a topological ordering of non-done/non-cancelled
// issues using Kahn's algorithm. Dependencies appear before the issues that
// depend on them.
func (g *Graph) TopologicalSort() []*issue.Issue {
	// Collect active issues.
	active := g.activeIssues()

	// Build in-degree map restricted to active-to-active edges.
	inDeg := make(map[uuid.UUID]int, len(active))
	for _, iss := range active {
		inDeg[iss.ID] = 0
	}
	for _, iss := range active {
		for _, downID := range g.forward[iss.ID] {
			if _, ok := inDeg[downID]; ok {
				inDeg[downID]++
			}
		}
	}

	// Seed queue with zero-in-degree nodes, sorted for determinism.
	var queue []uuid.UUID
	for id, deg := range inDeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sortUUIDs(queue)

	var result []*issue.Issue
	for len(queue) > 0 {
		// Pop smallest UUID for determinism.
		sortUUIDs(queue)
		cur := queue[0]
		queue = queue[1:]
		result = append(result, g.issues[cur])

		neighbors := g.forward[cur]
		sorted := make([]uuid.UUID, len(neighbors))
		copy(sorted, neighbors)
		sortUUIDs(sorted)

		for _, downID := range sorted {
			if _, ok := inDeg[downID]; !ok {
				continue
			}
			inDeg[downID]--
			if inDeg[downID] == 0 {
				queue = append(queue, downID)
			}
		}
	}
	return result
}

// CriticalChain returns the longest sequential path through the DAG
// considering only non-done, non-cancelled issues. It uses topological sort
// + dynamic programming to find the longest path, then reconstructs it.
func (g *Graph) CriticalChain() []*issue.Issue {
	sorted := g.TopologicalSort()
	if len(sorted) == 0 {
		return nil
	}

	// dp[id] = length of longest path ending at id (counting itself).
	dp := make(map[uuid.UUID]int, len(sorted))
	prev := make(map[uuid.UUID]uuid.UUID, len(sorted))
	nilID := uuid.UUID{}
	for _, iss := range sorted {
		prev[iss.ID] = nilID
	}

	activeSet := make(map[uuid.UUID]bool, len(sorted))
	for _, iss := range sorted {
		activeSet[iss.ID] = true
		dp[iss.ID] = 1
	}

	for _, iss := range sorted {
		for _, downID := range g.forward[iss.ID] {
			if !activeSet[downID] {
				continue
			}
			if dp[iss.ID]+1 > dp[downID] {
				dp[downID] = dp[iss.ID] + 1
				prev[downID] = iss.ID
			}
		}
	}

	// Find the node with the greatest dp value.
	var best uuid.UUID
	bestLen := 0
	for _, iss := range sorted {
		if dp[iss.ID] > bestLen {
			bestLen = dp[iss.ID]
			best = iss.ID
		}
	}
	if bestLen == 0 {
		return nil
	}

	// Reconstruct path by following prev pointers back to the root.
	var path []uuid.UUID
	cur := best
	for cur != nilID {
		path = append(path, cur)
		cur = prev[cur]
	}
	// Reverse to get root→leaf order.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	result := make([]*issue.Issue, len(path))
	for i, id := range path {
		result[i] = g.issues[id]
	}
	return result
}

// ReadySet returns pending issues where every blocker is done or cancelled.
func (g *Graph) ReadySet() []*issue.Issue {
	var ready []*issue.Issue
	for _, iss := range g.issues {
		if iss.State != issue.StatePending {
			continue
		}
		allDone := true
		for _, upID := range g.backward[iss.ID] {
			up, ok := g.issues[upID]
			if !ok {
				continue
			}
			if up.State != issue.StateDone && up.State != issue.StateCancelled {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, iss)
		}
	}
	sortIssues(ready)
	return ready
}

// Head returns the current "work item" for the chain:
//  1. If any issue is in-progress, return it (first by UUID sort if multiple).
//  2. Otherwise, return the first issue in the critical chain.
//  3. If no critical chain exists, return the first pending issue by UUID sort.
//  4. If no issues at all, return an error.
func (g *Graph) Head() (*issue.Issue, error) {
	if len(g.issues) == 0 {
		return nil, fmt.Errorf("graph: no issues")
	}

	// Step 1: in-progress issue.
	var inProgress []*issue.Issue
	for _, iss := range g.issues {
		if iss.State == issue.StateInProgress {
			inProgress = append(inProgress, iss)
		}
	}
	if len(inProgress) > 0 {
		sortIssues(inProgress)
		return inProgress[0], nil
	}

	// Step 2: critical chain leader.
	chain := g.CriticalChain()
	if len(chain) > 0 {
		return chain[0], nil
	}

	// Step 3: first pending by UUID sort.
	var pending []*issue.Issue
	for _, iss := range g.issues {
		if iss.State == issue.StatePending {
			pending = append(pending, iss)
		}
	}
	if len(pending) > 0 {
		sortIssues(pending)
		return pending[0], nil
	}

	return nil, fmt.Errorf("graph: no actionable issues")
}

// Cancel removes all edges involving the cancelled issue and reconnects the
// graph: each upstream dependency gains a direct edge to each downstream
// dependency. The caller is responsible for setting the issue's state to
// StateCancelled before or after calling Cancel.
func (g *Graph) Cancel(id uuid.UUID) error {
	if _, ok := g.issues[id]; !ok {
		return fmt.Errorf("graph: issue %s not found", id)
	}

	upstream := make([]uuid.UUID, len(g.backward[id]))
	copy(upstream, g.backward[id])
	downstream := make([]uuid.UUID, len(g.forward[id]))
	copy(downstream, g.forward[id])

	// Reconnect: each upstream now blocks each downstream directly.
	for _, upID := range upstream {
		for _, downID := range downstream {
			if !uuids.ContainsUUID(g.forward[upID], downID) {
				g.forward[upID] = append(g.forward[upID], downID)
				g.backward[downID] = append(g.backward[downID], upID)
			}
		}
	}

	// Remove all edges involving the cancelled issue.
	for _, upID := range upstream {
		g.forward[upID] = uuids.RemoveUUID(g.forward[upID], id)
	}
	for _, downID := range downstream {
		g.backward[downID] = uuids.RemoveUUID(g.backward[downID], id)
	}
	g.forward[id] = []uuid.UUID{}
	g.backward[id] = []uuid.UUID{}

	return nil
}

// ParallelAssignment partitions a ready set into worker groups for concurrent
// execution. Issues on the critical chain are prioritised over off-chain issues.
// Within each tier, issues are sorted by UUID for determinism.
//
// The greedy assignment loop accumulates a single "assigned paths" set across
// all groups. For each candidate issue (in priority order):
//   - If its paths do NOT overlap with any already-assigned paths, it is given
//     its own worker group and its paths are added to the accumulated set.
//   - If its paths DO overlap with already-assigned paths, it is deferred and
//     will be sequenced after the current batch.
//
// Assignment stops once workerCount groups are filled or the ready set is
// exhausted. Each inner slice in the result holds the issue(s) for one worker;
// issues in different groups are safe for concurrent execution.
func (g *Graph) ParallelAssignment(ready []uuid.UUID, workerCount int, pathsFn func(uuid.UUID) []string) [][]uuid.UUID {
	if len(ready) == 0 {
		return nil
	}

	// Determine which issues are on the critical chain.
	chain := g.CriticalChain()
	onChain := make(map[uuid.UUID]bool, len(chain))
	for _, iss := range chain {
		onChain[iss.ID] = true
	}

	// Sort ready set: critical chain issues first, then off-chain; UUID within
	// each tier for determinism.
	sorted := make([]uuid.UUID, len(ready))
	copy(sorted, ready)
	sort.Slice(sorted, func(i, j int) bool {
		iOnChain := onChain[sorted[i]]
		jOnChain := onChain[sorted[j]]
		if iOnChain != jOnChain {
			return iOnChain // on-chain sorts before off-chain
		}
		return sorted[i].String() < sorted[j].String()
	})

	// assignedPaths is the union of paths across all worker groups assigned so
	// far. A candidate may only be assigned if it does not overlap with this set
	// (ensuring all assigned issues are safe for parallel execution).
	var assignedPaths []string
	var groups [][]uuid.UUID

	for _, id := range sorted {
		if len(groups) >= workerCount {
			break
		}

		candidate := pathsFn(id)

		// If the candidate overlaps with any already-assigned issue's paths,
		// defer it to a subsequent batch.
		if PathsOverlap(assignedPaths, candidate) {
			continue
		}

		// No overlap: give this issue its own worker group and record its paths.
		groups = append(groups, []uuid.UUID{id})
		assignedPaths = append(assignedPaths, candidate...)
	}

	return groups
}

// activeIssues returns all issues that are not done or cancelled.
func (g *Graph) activeIssues() []*issue.Issue {
	var active []*issue.Issue
	for _, iss := range g.issues {
		if iss.State != issue.StateDone && iss.State != issue.StateCancelled {
			active = append(active, iss)
		}
	}
	return active
}

// sortUUIDs sorts a UUID slice lexicographically for deterministic output.
func sortUUIDs(ids []uuid.UUID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}

// sortIssues sorts issues by UUID string for deterministic output.
func sortIssues(issues []*issue.Issue) {
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].ID.String() < issues[j].ID.String()
	})
}
