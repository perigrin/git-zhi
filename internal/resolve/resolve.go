// ABOUTME: Ref argument resolution for CLI commands. Resolves user input
// ABOUTME: (HEAD, tag, UUID prefix, title substring) to an entity ref path.
package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/storage"
)

// IsHead returns true if the input resolves to the HEAD reference.
// An empty ref argument means the caller passed no explicit target,
// which resolves to HEAD — the current in-progress issue or next on
// the critical chain.
func IsHead(input string) bool {
	return input == "" || input == "HEAD"
}

// ResolveRef resolves a user-supplied ref argument to a full issue ref path.
// HEAD (and empty string) resolve to the current in-progress issue, or the
// first pending issue by UUID sort order. Any other input is first tried as a
// tag name, then as a UUID prefix, then as a title substring.
func ResolveRef(store *storage.Store, input string) (string, error) {
	if IsHead(input) {
		return resolveHead(store)
	}
	// Try tag resolution before UUID prefix.
	if ref, err := resolveTag(store, input); err == nil {
		return ref, nil
	}
	// Try UUID prefix resolution before title substring.
	if ref, err := resolveUUIDPrefix(store, input); err == nil {
		return ref, nil
	}
	return resolveTitleSubstring(store, input)
}

// resolveTag reads the tag entity at refs/zhi/_/tags/<input> and returns
// the target ref path stored in tag.txt. Returns an error if the tag does
// not exist or points to a nonexistent ref.
func resolveTag(store *storage.Store, input string) (string, error) {
	tagRef := "refs/zhi/_/tags/" + input
	data, err := store.ReadEntity(tagRef, "tag.txt")
	if err != nil {
		return "", fmt.Errorf("tag %q not found: %w", input, err)
	}
	targetRef := strings.TrimSpace(string(data))
	if !store.RefExists(targetRef) {
		return "", fmt.Errorf("tag %q points to nonexistent ref %s", input, targetRef)
	}
	return targetRef, nil
}

// resolveHead returns the ref path for the critical chain leader:
//  1. If any issue is in-progress, return it (first by UUID sort if multiple).
//  2. Otherwise, return the first issue on the critical chain (longest DAG path).
//  3. If there is no critical chain, return the first pending issue by UUID sort.
//
// This delegates to graph.Head() which implements the full DAG scheduler logic.
func resolveHead(store *storage.Store) (string, error) {
	issues, err := issue.LoadAllIssues(store)
	if err != nil {
		return "", fmt.Errorf("load issues: %w", err)
	}
	if len(issues) == 0 {
		return "", fmt.Errorf("no issues found")
	}

	g := graph.New(issues)
	head, err := g.Head()
	if err != nil {
		return "", fmt.Errorf("resolve HEAD: %w", err)
	}
	return issue.RefPrefix + head.ID.String(), nil
}

// resolveUUIDPrefix scans all issue refs for ones whose UUID segment starts
// with the given prefix. It returns the matched ref if exactly one matches,
// an error if no refs match, and an error listing candidates if more than one
// match.
func resolveUUIDPrefix(store *storage.Store, prefix string) (string, error) {
	refs, err := store.ListRefs(issue.RefPrefix)
	if err != nil {
		return "", fmt.Errorf("list issue refs: %w", err)
	}

	var matches []string
	for _, ref := range refs {
		uuidSegment := strings.TrimPrefix(ref, issue.RefPrefix)
		if strings.HasPrefix(uuidSegment, prefix) {
			matches = append(matches, ref)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no issue found with prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("ambiguous prefix %q matches %d issues: %s", prefix, len(matches), strings.Join(matches, ", "))
	}
}

// resolveTitleSubstring scans all issues for those whose title contains input
// as a case-insensitive substring. Returns the ref if exactly one match,
// an error listing candidates if more than one match, and an error if none.
func resolveTitleSubstring(store *storage.Store, input string) (string, error) {
	allIssues, err := issue.LoadAllIssues(store)
	if err != nil {
		return "", fmt.Errorf("load issues for title search: %w", err)
	}

	lower := strings.ToLower(input)
	var matches []*issue.Issue
	for _, iss := range allIssues {
		if strings.Contains(strings.ToLower(iss.Title), lower) {
			matches = append(matches, iss)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no issue found matching %q", input)
	case 1:
		return issue.RefPrefix + matches[0].ID.String(), nil
	default:
		candidates := make([]string, len(matches))
		for i, iss := range matches {
			candidates[i] = iss.ID.String()[:8] + " " + iss.Title
		}
		sort.Strings(candidates)
		return "", fmt.Errorf("ambiguous title %q matches %d issues: %s", input, len(matches), strings.Join(candidates, "; "))
	}
}
