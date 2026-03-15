// ABOUTME: DAG construction and operations for the issue dependency graph.
// ABOUTME: Enforces graph invariants, computes critical chain and ready set.
package graph

import (
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
)

// Graph represents the issue dependency DAG.
type Graph struct {
	issues map[uuid.UUID]*issue.Issue
}

// Len returns the number of issues in the graph.
func (g *Graph) Len() int { return len(g.issues) }

// New constructs a Graph from a set of issues.
func New(issues []*issue.Issue) *Graph {
	g := &Graph{
		issues: make(map[uuid.UUID]*issue.Issue, len(issues)),
	}
	for _, iss := range issues {
		g.issues[iss.ID] = iss
	}
	return g
}
