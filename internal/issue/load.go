// ABOUTME: LoadAllIssues reads all issue refs from storage, parses them, and
// ABOUTME: populates the ID field from the ref path. Shared by CLI commands and graph.
package issue

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/perigrin/git-chain/internal/storage"
)

// LoadAllIssues reads all issue refs, parses each, and returns them sorted
// by UUID (creation order). Each issue's ID is derived from its ref path.
func LoadAllIssues(store *storage.Store) ([]*Issue, error) {
	refs, err := store.ListRefs(RefPrefix)
	if err != nil {
		return nil, fmt.Errorf("list issue refs: %w", err)
	}
	sort.Strings(refs)

	var issues []*Issue
	for _, ref := range refs {
		data, err := store.ReadEntity(ref, "issue.md")
		if err != nil {
			continue // skip unreadable issues
		}
		iss, err := Parse(data)
		if err != nil {
			continue // skip unparseable issues
		}
		uuidStr := strings.TrimPrefix(ref, RefPrefix)
		id, err := uuid.FromString(uuidStr)
		if err != nil {
			continue
		}
		iss.ID = id
		issues = append(issues, iss)
	}
	return issues, nil
}
