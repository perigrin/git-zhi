// ABOUTME: PrioritizeIssues sorts done issues for regression-likelihood-first verification ordering.
// ABOUTME: Tier-1: path overlap; tier-2: lineage connections; tier-3: remaining in topoOrder.
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
// Tier 2 (requires lineageConnections != nil): issues whose IDs appear in
// lineageConnections but do not have path overlap. These have lineage
// connections to recently completed issues and are likely to surface related
// regressions. The caller computes the lineage set; this function only uses
// it for tier assignment.
//
// Tier 3 (called tier 2 when lineageConnections is nil): all remaining issues,
// ordered by their position in topoOrder (earlier position = verified first).
// Issues absent from topoOrder are appended after all in-topoOrder issues,
// sorted by UUID string for determinism.
//
// When lineageConnections is nil the function falls back to two-tier behavior
// (tier 1 then the former tier 2, now tier 3).
//
// The input slice is not modified; a new slice is returned.
func PrioritizeIssues(issues []*issue.Issue, recentChanges []string, topoOrder []uuid.UUID, lineageConnections map[uuid.UUID]bool) []*issue.Issue {
	if len(issues) == 0 {
		return []*issue.Issue{}
	}

	// Build a fast lookup for recent-changes paths.
	changedSet := make(map[string]struct{}, len(recentChanges))
	for _, p := range recentChanges {
		changedSet[p] = struct{}{}
	}

	// Build a position index from topoOrder for stable ordering within tiers.
	topoPos := make(map[uuid.UUID]int, len(topoOrder))
	for i, id := range topoOrder {
		topoPos[id] = i
	}
	// Sentinel: issues absent from topoOrder get a position beyond the last
	// real position, so they sort after all in-order issues.
	absentPos := len(topoOrder)

	// Partition into tier-1 (path overlap), tier-2 (lineage, when provided),
	// and tier-3 (remaining).
	var tier1, tier2, tier3 []*issue.Issue
	for _, iss := range issues {
		if hasOverlap(iss.ObservedPaths, changedSet) {
			// Path overlap takes precedence over lineage.
			tier1 = append(tier1, iss)
		} else if lineageConnections != nil && lineageConnections[iss.ID] {
			tier2 = append(tier2, iss)
		} else {
			tier3 = append(tier3, iss)
		}
	}

	// sortByTopo sorts a slice of issues by topoOrder position then UUID for
	// determinism.
	sortByTopo := func(slice []*issue.Issue) {
		sort.SliceStable(slice, func(i, j int) bool {
			pi := topoPosition(slice[i].ID, topoPos, absentPos)
			pj := topoPosition(slice[j].ID, topoPos, absentPos)
			if pi != pj {
				return pi < pj
			}
			return slice[i].ID.String() < slice[j].ID.String()
		})
	}

	sortByTopo(tier1)
	sortByTopo(tier2)
	sortByTopo(tier3)

	result := make([]*issue.Issue, 0, len(issues))
	result = append(result, tier1...)
	result = append(result, tier2...)
	result = append(result, tier3...)
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
