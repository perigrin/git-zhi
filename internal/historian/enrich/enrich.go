// ABOUTME: Historian enrich stage: converts commit clusters into retrospective Issue structs.
// ABOUTME: Assigns confidence scores, constructs sessions, derives actor from commit authors.
package enrich

import (
	"sort"
	"strings"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/actor"
	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/issue"
)

// ClusterToIssue converts a cluster into a retrospective Issue struct ready
// for storage. The issue is always in the done state (completed work).
func ClusterToIssue(c *cluster.Cluster) *issue.Issue {
	if len(c.Commits) == 0 {
		return nil
	}

	first := c.Commits[0]
	last := c.Commits[len(c.Commits)-1]

	// --- Title ---
	title := titleFromCluster(c)

	// --- Confidence and Source ---
	confidence, source := confidenceAndSource(c)

	// --- Actor from most frequent author ---
	a := mostFrequentActor(c)

	// --- TrackerID ---
	trackerID := ""
	if c.TicketRef != "" {
		trackerID = "unknown:" + c.TicketRef
	}

	// --- ObservedPaths: union of all commit fingerprint paths ---
	observedPaths := collectPaths(c)

	// --- Session: one per cluster ---
	startedAt := first.Timestamp
	endedAt := last.Timestamp
	sess := issue.Session{
		StartSHA:  first.SHA,
		EndSHA:    last.SHA,
		Commits:   len(c.Commits),
		StartedAt: &startedAt,
		EndedAt:   &endedAt,
	}

	// --- Transitions: in-progress at first commit, done at last commit ---
	actorStr := a.String()
	transitions := []issue.Transition{
		{
			State:     string(issue.StateInProgress),
			Actor:     actorStr,
			Timestamp: first.Timestamp,
		},
		{
			State:     string(issue.StateDone),
			Actor:     actorStr,
			Timestamp: last.Timestamp,
		},
	}

	// --- ID: new UUIDv7 ---
	id, err := uuid.NewV7()
	if err != nil {
		// Fall back to v4 if v7 fails (should not happen in practice).
		id, _ = uuid.NewV4()
	}

	iss := &issue.Issue{
		ID:            id,
		Title:         title,
		State:         issue.StateDone,
		Created:       first.Timestamp,
		Updated:       last.Timestamp,
		Sessions:      []issue.Session{sess},
		Transitions:   transitions,
		ObservedPaths: observedPaths,
		Labels:        []string{},
		Confidence:    confidence,
		Source:        source,
		TrackerID:     trackerID,
	}

	return iss
}

// EnrichClusters converts all clusters into issues and returns them sorted by
// Created timestamp (ascending, oldest first).
func EnrichClusters(clusters []cluster.Cluster) []*issue.Issue {
	if len(clusters) == 0 {
		return []*issue.Issue{}
	}

	issues := make([]*issue.Issue, 0, len(clusters))
	for i := range clusters {
		iss := ClusterToIssue(&clusters[i])
		if iss != nil {
			issues = append(issues, iss)
		}
	}

	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Created.Before(issues[j].Created)
	})

	return issues
}

// titleFromCluster derives an issue title from the cluster. When a ticket ref
// is available it is used directly. Otherwise the first commit message is used,
// trimmed of leading/trailing whitespace and truncated to 80 characters.
func titleFromCluster(c *cluster.Cluster) string {
	if c.TicketRef != "" {
		return c.TicketRef
	}
	msg := strings.TrimSpace(c.Commits[0].Message)
	if len(msg) > 80 {
		return msg[:80]
	}
	return msg
}

// confidenceAndSource determines the confidence score and source label for the
// cluster according to the four-tier classification:
//
//   - tracker-match (0.90): has a ticket ref AND more than one commit
//   - ticket-ref (0.75): has a ticket ref but only one commit
//   - cluster (0.55): no ticket ref, more than three commits (heuristic clustering)
//   - single-commit (0.30): no ticket ref, one to three commits
func confidenceAndSource(c *cluster.Cluster) (float64, string) {
	hasTicket := c.TicketRef != ""
	count := len(c.Commits)

	switch {
	case hasTicket && count > 1:
		return 0.90, "tracker-match"
	case hasTicket:
		return 0.75, "ticket-ref"
	case count > 3:
		return 0.55, "cluster"
	default:
		return 0.30, "single-commit"
	}
}

// mostFrequentActor returns the Actor derived from the author who appears most
// often across all commits in the cluster. In case of a tie, the first author
// (by commit order) wins. Uses the email from the centroid's most frequent author.
func mostFrequentActor(c *cluster.Cluster) actor.Actor {
	// Build frequency map from commits (centroid.Authors holds counts by name).
	type authorEntry struct {
		name  string
		email string
		count int
	}
	freq := make(map[string]*authorEntry)
	for _, cd := range c.Commits {
		if e, ok := freq[cd.Author]; ok {
			e.count++
		} else {
			freq[cd.Author] = &authorEntry{name: cd.Author, email: cd.Email, count: 1}
		}
	}

	// Find the author with the highest count; break ties by first appearance.
	var best *authorEntry
	for _, cd := range c.Commits {
		e := freq[cd.Author]
		if best == nil || e.count > best.count {
			best = e
		}
	}

	if best == nil {
		return actor.Actor{Type: actor.TypeHuman, ID: "unknown"}
	}
	return actor.DeriveActor(best.name, best.email)
}

// collectPaths returns the union of all file paths touched across all commits
// in the cluster. The result is sorted for determinism.
func collectPaths(c *cluster.Cluster) []string {
	seen := make(map[string]struct{})
	for _, cd := range c.Commits {
		for _, p := range cd.Fingerprint.Paths {
			seen[p] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

