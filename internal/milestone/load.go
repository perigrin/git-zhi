// ABOUTME: Load helpers for milestones from storage. Reads and parses
// ABOUTME: milestone refs under refs/zhi/_/milestones/.
package milestone

import (
	"fmt"
	"sort"
	"strings"

	"github.com/perigrin/git-zhi/internal/storage"
)

// RefPrefix is the git ref namespace under which all milestones are stored.
const RefPrefix = "refs/zhi/_/milestones/"

// UnmarshalMilestone deserializes a Milestone from either frontmatter+body
// format (v0.2) or pure YAML (v0.1). Delegates to Parse for format detection.
func UnmarshalMilestone(data []byte) (*Milestone, error) {
	ms, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal milestone: %w", err)
	}
	return ms, nil
}

// LoadMilestone reads the milestone named by name from storage.
func LoadMilestone(store *storage.Store, name string) (*Milestone, error) {
	ref := RefPrefix + name
	data, err := store.ReadEntity(ref, "milestone.yaml")
	if err != nil {
		return nil, fmt.Errorf("read milestone %q: %w", name, err)
	}
	ms, err := UnmarshalMilestone(data)
	if err != nil {
		return nil, err
	}
	// Ensure the Name field reflects the ref path name, not just what's in YAML.
	if ms.Name == "" {
		ms.Name = name
	}
	return ms, nil
}

// LoadAllMilestones reads all milestone refs and returns them sorted by name.
func LoadAllMilestones(store *storage.Store) ([]*Milestone, error) {
	refs, err := store.ListRefs(RefPrefix)
	if err != nil {
		return nil, fmt.Errorf("list milestone refs: %w", err)
	}
	sort.Strings(refs)

	var milestones []*Milestone
	for _, ref := range refs {
		name := strings.TrimPrefix(ref, RefPrefix)
		ms, err := LoadMilestone(store, name)
		if err != nil {
			continue // skip unreadable milestones
		}
		milestones = append(milestones, ms)
	}
	return milestones, nil
}
