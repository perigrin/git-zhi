// ABOUTME: Ref argument resolution for CLI commands. Resolves user input
// ABOUTME: (HEAD, tag, UUID prefix, title substring) to an entity ref path.
package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/storage"
)

const issueRefPrefix = "refs/chain/_/issues/"

// IsHead returns true if the input resolves to the HEAD reference.
// An empty ref argument means the caller passed no explicit target,
// which resolves to HEAD — the current in-progress issue or next on
// the critical chain.
func IsHead(input string) bool {
	return input == "" || input == "HEAD"
}

// ResolveRef resolves a user-supplied ref argument to a full issue ref path.
// HEAD (and empty string) resolve to the current in-progress issue, or the
// first pending issue by UUID sort order. Any other input is treated as a
// UUID prefix and matched against all issue refs.
func ResolveRef(store *storage.Store, input string) (string, error) {
	if IsHead(input) {
		return resolveHead(store)
	}
	return resolveUUIDPrefix(store, input)
}

// resolveHead returns the ref path for the first in-progress issue, or the
// first pending issue sorted lexicographically by UUID. UUIDv7 sorts by
// creation time, so lexicographic order is chronological order.
func resolveHead(store *storage.Store) (string, error) {
	refs, err := store.ListRefs(issueRefPrefix)
	if err != nil {
		return "", fmt.Errorf("list issue refs: %w", err)
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("no issues found")
	}

	sort.Strings(refs)

	var firstPending string
	for _, ref := range refs {
		raw, err := store.ReadEntity(ref, "issue.md")
		if err != nil {
			return "", fmt.Errorf("read issue at %s: %w", ref, err)
		}
		iss, err := issue.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse issue at %s: %w", ref, err)
		}
		if iss.State == issue.StateInProgress {
			return ref, nil
		}
		if iss.State == issue.StatePending && firstPending == "" {
			firstPending = ref
		}
	}

	if firstPending != "" {
		return firstPending, nil
	}
	return "", fmt.Errorf("no in-progress or pending issues found")
}

// resolveUUIDPrefix scans all issue refs for ones whose UUID segment starts
// with the given prefix. It returns the matched ref if exactly one matches,
// an error if no refs match, and an error listing candidates if more than one
// match.
func resolveUUIDPrefix(store *storage.Store, prefix string) (string, error) {
	refs, err := store.ListRefs(issueRefPrefix)
	if err != nil {
		return "", fmt.Errorf("list issue refs: %w", err)
	}

	var matches []string
	for _, ref := range refs {
		uuidSegment := strings.TrimPrefix(ref, issueRefPrefix)
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
