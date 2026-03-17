// ABOUTME: PrioritizeIssues sorts done issues for regression-likelihood-first verification ordering.
// ABOUTME: Tier-1 issues have ObservedPaths overlapping recentChanges; tier-2 falls back to topoOrder.
package verify

import (
	"sort"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
)

// PrioritizeIssues returns issues sorted for regression-risk-first verification.
//
// Tier 1: issues whose ObservedPaths contain at least one path also present in
// recentChanges. These are most likely to have regressed due to recent work.
//
// Tier 2: all remaining issues, ordered by their position in topoOrder
// (earlier position = verified first). Issues absent from topoOrder are
// appended after all in-topoOrder issues, sorted by UUID string for determinism.
//
// The input slice is not modified; a new slice is returned.
func PrioritizeIssues(issues []*issue.Issue, recentChanges []string, topoOrder []uuid.UUID) []*issue.Issue {
	if len(issues) == 0 {
		return []*issue.Issue{}
	}

	// Build a fast lookup for recent-changes paths.
	changedSet := make(map[string]struct{}, len(recentChanges))
	for _, p := range recentChanges {
		changedSet[p] = struct{}{}
	}

	// Build a position index from topoOrder for stable tier-2 ordering.
	topoPos := make(map[uuid.UUID]int, len(topoOrder))
	for i, id := range topoOrder {
		topoPos[id] = i
	}
	// Sentinel: issues absent from topoOrder get a position beyond the last
	// real position, so they sort after all in-order issues.
	absentPos := len(topoOrder)

	// Partition into tier-1 (overlapping) and tier-2 (no overlap).
	var tier1, tier2 []*issue.Issue
	for _, iss := range issues {
		if hasOverlap(iss.ObservedPaths, changedSet) {
			tier1 = append(tier1, iss)
		} else {
			tier2 = append(tier2, iss)
		}
	}

	// Sort tier-1 by topoOrder position (ascending), then UUID for determinism.
	sort.SliceStable(tier1, func(i, j int) bool {
		pi := topoPosition(tier1[i].ID, topoPos, absentPos)
		pj := topoPosition(tier1[j].ID, topoPos, absentPos)
		if pi != pj {
			return pi < pj
		}
		return tier1[i].ID.String() < tier1[j].ID.String()
	})

	// Sort tier-2 by topoOrder position (ascending), then UUID for determinism.
	sort.SliceStable(tier2, func(i, j int) bool {
		pi := topoPosition(tier2[i].ID, topoPos, absentPos)
		pj := topoPosition(tier2[j].ID, topoPos, absentPos)
		if pi != pj {
			return pi < pj
		}
		return tier2[i].ID.String() < tier2[j].ID.String()
	})

	result := make([]*issue.Issue, 0, len(issues))
	result = append(result, tier1...)
	result = append(result, tier2...)
	return result
}

// hasOverlap returns true if any path in observed appears in the changedSet.
func hasOverlap(observed []string, changedSet map[string]struct{}) bool {
	for _, p := range observed {
		if _, ok := changedSet[p]; ok {
			return true
		}
	}
	return false
}

// topoPosition returns the position of id in the topoPos map, or absentPos
// when the id is not present (placing it at the end of the tier).
func topoPosition(id uuid.UUID, topoPos map[uuid.UUID]int, absentPos int) int {
	if pos, ok := topoPos[id]; ok {
		return pos
	}
	return absentPos
}
