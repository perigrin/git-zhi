// ABOUTME: Path overlap detection for parallelization analysis.
// ABOUTME: Determines whether two sets of file paths touch the same files or directories.
package graph

import (
	"path"
	"strings"
)

// PathsOverlap reports whether two path sets overlap at the file or directory
// level. Two path sets overlap if:
//   - Any file path appears in both sets (exact match), or
//   - Any two paths share the same parent directory.
//
// A path ending in "/" is treated as a directory and overlaps with any file
// whose parent directory matches that path (with the trailing slash stripped).
func PathsOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	// Build sets of (files, directories) for slice a.
	aFiles := make(map[string]bool, len(a))
	aDirs := make(map[string]bool, len(a))
	for _, p := range a {
		if strings.HasSuffix(p, "/") {
			aDirs[strings.TrimSuffix(p, "/")] = true
		} else {
			aFiles[p] = true
			aDirs[path.Dir(p)] = true
		}
	}

	// Check each path in b against a's files and directories.
	for _, p := range b {
		if strings.HasSuffix(p, "/") {
			dir := strings.TrimSuffix(p, "/")
			// A directory in b overlaps if a has any file inside that directory.
			if aDirs[dir] {
				return true
			}
		} else {
			// Exact file match.
			if aFiles[p] {
				return true
			}
			// Shared parent directory.
			if aDirs[path.Dir(p)] {
				return true
			}
		}
	}
	return false
}

// PathDirectories extracts unique parent directories from a set of file paths.
// For a file at "internal/parser/signature.go", the parent directory is
// "internal/parser". Root-level files (no "/" separator) yield an empty string.
// Paths ending in "/" are treated as directories themselves and returned as-is
// (without the trailing slash).
func PathDirectories(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		var dir string
		if strings.HasSuffix(p, "/") {
			dir = strings.TrimSuffix(p, "/")
		} else {
			dir = path.Dir(p)
			// path.Dir returns "." for a bare filename; normalise to empty string
			// to represent the repository root.
			if dir == "." {
				dir = ""
			}
		}
		if !seen[dir] {
			seen[dir] = true
			result = append(result, dir)
		}
	}
	return result
}
