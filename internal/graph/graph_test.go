// ABOUTME: Tests for the Graph constructor: verifies an empty issue set
// ABOUTME: produces a valid non-nil graph.
package graph_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/graph"
	"github.com/perigrin/git-chain/internal/issue"
)

func TestNewGraph_Empty(t *testing.T) {
	g := graph.New([]*issue.Issue{})
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
}
