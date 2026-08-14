// ABOUTME: Tests for path overlap detection used in parallelization analysis.
// ABOUTME: Covers exact file match, shared directory, no overlap, empty inputs, and directory-only paths.
package graph_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/graph"
)

// TestPathsOverlap_SharedDirectory verifies that two files in the same directory
// are detected as overlapping.
func TestPathsOverlap_SharedDirectory(t *testing.T) {
	a := []string{"internal/parser/signature.go"}
	b := []string{"internal/parser/errors.go"}
	if !graph.PathsOverlap(a, b) {
		t.Error("expected overlap for files in the same directory")
	}
}

// TestPathsOverlap_ExactFileMatch verifies that an exact file path match
// is detected as overlapping.
func TestPathsOverlap_ExactFileMatch(t *testing.T) {
	a := []string{"internal/parser/signature.go"}
	b := []string{"internal/parser/signature.go"}
	if !graph.PathsOverlap(a, b) {
		t.Error("expected overlap for exact file match")
	}
}

// TestPathsOverlap_NoOverlap verifies that files in different directories
// are not detected as overlapping.
func TestPathsOverlap_NoOverlap(t *testing.T) {
	a := []string{"internal/parser/signature.go"}
	b := []string{"internal/config/config.go"}
	if graph.PathsOverlap(a, b) {
		t.Error("expected no overlap for files in different directories")
	}
}

// TestPathsOverlap_EmptyInputs verifies that empty path sets never overlap.
func TestPathsOverlap_EmptyInputs(t *testing.T) {
	if graph.PathsOverlap(nil, nil) {
		t.Error("expected no overlap for nil inputs")
	}
	if graph.PathsOverlap([]string{}, []string{}) {
		t.Error("expected no overlap for empty inputs")
	}
	if graph.PathsOverlap([]string{"internal/parser/signature.go"}, nil) {
		t.Error("expected no overlap when second set is nil")
	}
	if graph.PathsOverlap(nil, []string{"internal/parser/signature.go"}) {
		t.Error("expected no overlap when first set is nil")
	}
}

// TestPathsOverlap_DirectoryOnlyPath verifies that a trailing-slash directory
// path overlaps with files inside that directory.
func TestPathsOverlap_DirectoryOnlyPath(t *testing.T) {
	a := []string{"internal/parser/"}
	b := []string{"internal/parser/signature.go"}
	if !graph.PathsOverlap(a, b) {
		t.Error("expected overlap: directory-only path should match files within it")
	}
}

// TestPathsOverlap_MultiplePathsPartialOverlap verifies that overlap is detected
// when only some paths in two sets share a directory.
func TestPathsOverlap_MultiplePathsPartialOverlap(t *testing.T) {
	a := []string{"internal/parser/signature.go", "internal/config/config.go"}
	b := []string{"cmd/main.go", "internal/config/other.go"}
	if !graph.PathsOverlap(a, b) {
		t.Error("expected overlap: internal/config/ is shared across sets")
	}
}

