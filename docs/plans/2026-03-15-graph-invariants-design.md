# Issue 7: Graph Invariants & Critical Chain — Design

## Scope

DAG construction from loaded issues, cycle detection, referential integrity enforcement, cancel reconnection, critical chain computation, Head() resolution. Retrofit invariant checks into issue edit.

## Graph Package Expansion

The `internal/graph/graph.go` stub has `New(issues)` and `Len()`. Expand with:

### Construction
- `Build(issues []*issue.Issue) (*Graph, error)` — replaces `New`. Constructs adjacency lists from BlockedBy/Blocks fields, validates referential integrity, returns error on invalid edges.

### Invariant Enforcement
1. **Acyclic**: `AddEdge(from, to) error` with DFS cycle detection
2. **Referential integrity**: `Build` rejects edges to nonexistent issues
3. **Done/cancelled cannot gain incoming deps**: checked in `AddEdge`
4. **Cancel reconnection**: `Cancel(id) error` — reconnects upstream to downstream
5. **Split inheritance**: deferred (--split not implemented yet)

### Queries
- `CriticalChain() []*issue.Issue` — longest path through non-done/non-cancelled issues via topo sort + DP
- `ReadySet() []*issue.Issue` — unblocked pending issues
- `Head() *issue.Issue` — in-progress issue, or next on critical chain with most downstream deps
- `TopologicalSort() []*issue.Issue` — for `zhi list` default view

### Integration
- Update `resolve.resolveHead` to use Graph.Head() when graph is available
- Add `LoadAllIssues(store) ([]*issue.Issue, error)` helper to reduce duplication

## Files

- Modify: `internal/graph/graph.go` — full implementation
- Modify: `internal/graph/graph_test.go` — comprehensive tests
- Create: `internal/issue/load.go` — LoadAllIssues helper
- Create: `internal/issue/load_test.go` — test for loader

## Testing

**Graph** (10+ tests):
- Build with valid DAG
- Build with dangling edge (referential integrity)
- AddEdge creating cycle (rejected)
- AddEdge to done issue (rejected)
- CriticalChain on linear chain
- CriticalChain on diamond DAG
- ReadySet returns unblocked pending
- Head returns in-progress
- Head returns critical chain leader when no in-progress
- TopologicalSort order
- Cancel reconnects graph

**Loader** (2 tests):
- LoadAllIssues returns all issues with IDs populated
- LoadAllIssues on empty repo returns empty slice
