// ABOUTME: Label index management for fast --label filtering without full issue scans.
// ABOUTME: BuildLabelIndexes writes refs under refs/zhi/<label>/; LoadLabelIndex reads them back.
package issue

import (
	"fmt"
	"strings"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/storage"
)

// labelIndexPrefix is the root namespace for all label index refs.
// Each label gets a sub-namespace: refs/zhi/<label>/<issue-uuid>.
const labelIndexPrefix = "refs/zhi/"

// labelIndexRefPrefix returns the ref namespace for a specific label.
func labelIndexRefPrefix(label string) string {
	return labelIndexPrefix + label + "/"
}

// BuildLabelIndexes clears and rebuilds label index refs for all provided issues.
// For each issue-label pair, a lightweight marker ref is written at
// refs/zhi/<label>/<issue-uuid> containing the full issue ref path.
// Running BuildLabelIndexes twice with the same input is idempotent.
// Returns an error if any label name is invalid (contains "/" or equals "_").
func BuildLabelIndexes(store *storage.Store, issues []*Issue) error {
	// Collect the complete set of labels used by this issue list.
	labelsInUse := make(map[string]struct{})
	for _, iss := range issues {
		for _, label := range iss.Labels {
			// Reject labels that would collide with the core refs/zhi/_/ namespace
			// or produce malformed ref paths.
			if label == "_" {
				return fmt.Errorf("invalid label name %q: label '_' would collide with the core refs/zhi/_/ namespace", label)
			}
			if strings.Contains(label, "/") {
				return fmt.Errorf("invalid label name %q: label names must not contain '/'", label)
			}
			labelsInUse[label] = struct{}{}
		}
	}

	// Clear all existing index refs for labels that appear in this build.
	// This ensures stale entries (from removed labels) are cleaned out.
	for label := range labelsInUse {
		prefix := labelIndexRefPrefix(label)
		existing, err := store.ListRefs(prefix)
		if err != nil {
			return fmt.Errorf("list label index refs for %q: %w", label, err)
		}
		for _, ref := range existing {
			if err := store.DeleteRef(ref); err != nil {
				return fmt.Errorf("delete stale index ref %q: %w", ref, err)
			}
		}
	}

	// Also clear index refs for labels that existed previously but are no
	// longer in the current issue set. Discover them by scanning the full
	// refs/zhi/ namespace and removing anything under a label sub-path that
	// is not an issue ref or config/milestone path.
	allZhiRefs, err := store.ListRefs(labelIndexPrefix)
	if err != nil {
		return fmt.Errorf("list all zhi refs: %w", err)
	}
	for _, ref := range allZhiRefs {
		// Skip known non-index ref prefixes.
		if strings.HasPrefix(ref, "refs/zhi/_/") {
			continue
		}
		// This is a label index ref. Extract the label from the ref path.
		// Pattern: refs/zhi/<label>/<uuid>
		trimmed := strings.TrimPrefix(ref, labelIndexPrefix)
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) != 2 {
			continue
		}
		label := parts[0]
		// If the label is not in the current issue set, delete this ref.
		if _, present := labelsInUse[label]; !present {
			if err := store.DeleteRef(ref); err != nil {
				return fmt.Errorf("delete orphaned index ref %q: %w", ref, err)
			}
		}
	}

	// Write fresh index entries for every issue-label pair.
	for _, iss := range issues {
		for _, label := range iss.Labels {
			indexRef := labelIndexRefPrefix(label) + iss.ID.String()
			issueRef := RefPrefix + iss.ID.String()
			content := []byte(issueRef)
			if err := store.WriteEntity(indexRef, "ref", content, "index: "+label+" -> "+iss.ID.String()[:8]); err != nil {
				return fmt.Errorf("write label index ref %q: %w", indexRef, err)
			}
		}
	}

	return nil
}

// LoadLabelIndex returns the UUIDs of all issues indexed under the given label.
// Returns an empty (non-nil) slice when no issues carry that label.
func LoadLabelIndex(store *storage.Store, label string) ([]uuid.UUID, error) {
	prefix := labelIndexRefPrefix(label)
	refs, err := store.ListRefs(prefix)
	if err != nil {
		return nil, fmt.Errorf("list label index refs for %q: %w", label, err)
	}

	ids := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		uuidStr := strings.TrimPrefix(ref, prefix)
		id, err := uuid.FromString(uuidStr)
		if err != nil {
			// Skip malformed index entries rather than failing the whole load.
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}
