// ABOUTME: Load helpers for milestones from storage. Reads and parses
// ABOUTME: milestone refs under refs/chain/_/milestones/.
package milestone

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/perigrin/git-chain/internal/storage"
)

// RefPrefix is the git ref namespace under which all milestones are stored.
const RefPrefix = "refs/chain/_/milestones/"

// UnmarshalMilestone deserializes a Milestone from YAML bytes.
func UnmarshalMilestone(data []byte) (*Milestone, error) {
	var ms Milestone
	if err := yaml.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("unmarshal milestone: %w", err)
	}
	return &ms, nil
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
