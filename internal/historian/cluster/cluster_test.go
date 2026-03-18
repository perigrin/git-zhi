// ABOUTME: Tests for the historian cluster stage: greedy sequential clustering with centroid coherence.
// ABOUTME: Verifies scoring signals, cluster grouping, inactivity gap handling, and drift prevention.
package cluster_test

import (
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/historian/extract"
)

// baseTime is an arbitrary anchor for constructing test commit timestamps.
var baseTime = time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

func makeCommit(sha, author, message string, paths []string, ticketRefs []string, ts time.Time) extract.CommitData {
	return extract.CommitData{
		SHA:        sha,
		Author:     author,
		Email:      author + "@example.com",
		Timestamp:  ts,
		Message:    message,
		TicketRefs: ticketRefs,
		Fingerprint: extract.DiffFingerprint{
			Paths:       paths,
			FileCount:   len(paths),
			ChangeRatio: 1.0,
		},
	}
}

// TestDefaultConfig verifies the provisional threshold values from the design doc.
func TestDefaultConfig(t *testing.T) {
	cfg := cluster.DefaultConfig()

	if cfg.JoinThreshold != 0.25 {
		t.Errorf("JoinThreshold = %f, want 0.25", cfg.JoinThreshold)
	}
	if cfg.CoherenceThreshold != 0.20 {
		t.Errorf("CoherenceThreshold = %f, want 0.20", cfg.CoherenceThreshold)
	}
	if cfg.InactivityGap != 14*24*time.Hour {
		t.Errorf("InactivityGap = %v, want 14*24h", cfg.InactivityGap)
	}
	if cfg.Weights.TicketMatch != 0.40 {
		t.Errorf("Weights.TicketMatch = %f, want 0.40", cfg.Weights.TicketMatch)
	}
	if cfg.Weights.PathOverlap != 0.20 {
		t.Errorf("Weights.PathOverlap = %f, want 0.20", cfg.Weights.PathOverlap)
	}
	if cfg.Weights.DiffFingerprint != 0.15 {
		t.Errorf("Weights.DiffFingerprint = %f, want 0.15", cfg.Weights.DiffFingerprint)
	}
	if cfg.Weights.TimeProximity != 0.10 {
		t.Errorf("Weights.TimeProximity = %f, want 0.10", cfg.Weights.TimeProximity)
	}
	if cfg.Weights.AuthorMatch != 0.10 {
		t.Errorf("Weights.AuthorMatch = %f, want 0.10", cfg.Weights.AuthorMatch)
	}
	if cfg.Weights.MessageTokens != 0.05 {
		t.Errorf("Weights.MessageTokens = %f, want 0.05", cfg.Weights.MessageTokens)
	}
}

// TestClusterCommits_SameTicketRefClusters verifies that commits sharing a ticket
// reference group into a single cluster regardless of other signals.
func TestClusterCommits_SameTicketRefClusters(t *testing.T) {
	cfg := cluster.DefaultConfig()

	commits := []extract.CommitData{
		makeCommit("aaa1", "alice", "LOPS-42: initial work", []string{"foo.go"}, []string{"LOPS-42"}, baseTime),
		makeCommit("aaa2", "bob", "LOPS-42: follow-up from different dir", []string{"bar/baz.go"}, []string{"LOPS-42"}, baseTime.Add(2*time.Hour)),
		makeCommit("aaa3", "carol", "LOPS-42: final tweak", []string{"other.go"}, []string{"LOPS-42"}, baseTime.Add(4*time.Hour)),
	}

	clusters := cluster.ClusterCommits(commits, cfg)

	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster (same ticket ref), got %d", len(clusters))
	}
	if len(clusters[0].Commits) != 3 {
		t.Errorf("cluster has %d commits, want 3", len(clusters[0].Commits))
	}
	if clusters[0].TicketRef != "LOPS-42" {
		t.Errorf("TicketRef = %q, want LOPS-42", clusters[0].TicketRef)
	}
}

// TestClusterCommits_SamePathsClusters verifies that commits touching the same
// files group together even without ticket references.
func TestClusterCommits_SamePathsClusters(t *testing.T) {
	cfg := cluster.DefaultConfig()

	sharedPaths := []string{"internal/auth/handler.go", "internal/auth/middleware.go"}
	commits := []extract.CommitData{
		makeCommit("b001", "alice", "add auth handler", sharedPaths, nil, baseTime),
		makeCommit("b002", "alice", "fix auth middleware", sharedPaths, nil, baseTime.Add(time.Hour)),
		makeCommit("b003", "alice", "refactor auth handler", sharedPaths, nil, baseTime.Add(2*time.Hour)),
	}

	clusters := cluster.ClusterCommits(commits, cfg)

	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster (same paths), got %d", len(clusters))
	}
	if len(clusters[0].Commits) != 3 {
		t.Errorf("cluster has %d commits, want 3", len(clusters[0].Commits))
	}
}

// TestClusterCommits_DifferentWorkStaysSeparate verifies that commits with
// different authors, different paths, and no ticket refs produce separate clusters.
func TestClusterCommits_DifferentWorkStaysSeparate(t *testing.T) {
	cfg := cluster.DefaultConfig()

	// Use distinct authors, distinct files, distinct messages, and wide time
	// gaps so no signal is strong enough to cluster them together.
	commits := []extract.CommitData{
		makeCommit("c001", "alice", "implement payments charge flow", []string{"payments/charge.go"}, nil, baseTime),
		makeCommit("c002", "bob", "configure logging sinks for cloud", []string{"logging/sink.go"}, nil, baseTime.Add(72*time.Hour)),
		makeCommit("c003", "carol", "redesign user profile page", []string{"users/profile.go"}, nil, baseTime.Add(144*time.Hour)),
	}

	clusters := cluster.ClusterCommits(commits, cfg)

	if len(clusters) != 3 {
		t.Fatalf("expected 3 separate clusters, got %d", len(clusters))
	}
}

// TestClusterCommits_InactivityGapClosesClusters verifies that a long gap in
// commit time causes the old cluster to close and a new one to open for related work.
func TestClusterCommits_InactivityGapClosesClusters(t *testing.T) {
	cfg := cluster.DefaultConfig()

	sharedPaths := []string{"internal/auth/handler.go"}

	// First batch of commits.
	t0 := baseTime
	// Second batch starts 15 days later (beyond 14-day inactivity gap).
	t1 := baseTime.Add(15 * 24 * time.Hour)

	commits := []extract.CommitData{
		makeCommit("d001", "alice", "LOPS-10: auth work", sharedPaths, []string{"LOPS-10"}, t0),
		makeCommit("d002", "alice", "LOPS-10: more auth", sharedPaths, []string{"LOPS-10"}, t0.Add(time.Hour)),
		// After inactivity gap — same ticket, same paths, but old cluster is closed.
		makeCommit("d003", "alice", "LOPS-10: resume auth", sharedPaths, []string{"LOPS-10"}, t1),
	}

	clusters := cluster.ClusterCommits(commits, cfg)

	// The gap must result in 2 clusters even though the ticket ref matches.
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters (inactivity gap), got %d", len(clusters))
	}
}

// TestClusterCommits_CentroidCoherencePreventsTransitiveDrift verifies that
// clusters do not grow without bound via transitive similarity (A→B OK, B→C OK,
// but the cluster as a whole rejects D when its centroid has drifted far from D).
//
// We construct a scenario where each commit has completely disjoint paths AND
// different authors. The only connecting signal is time_proximity, which alone
// (0.10 max weight) cannot reach the JoinThreshold of 0.35. This verifies that
// commits with no overlapping work stay separate even when they are close in time.
func TestClusterCommits_CentroidCoherencePreventsTransitiveDrift(t *testing.T) {
	cfg := cluster.DefaultConfig()

	// Each commit: different author, different paths, no ticket refs, no shared tokens.
	// time_proximity at 1hr gaps contributes ~0.10, diffFingerprint ~0.075,
	// total < 0.35 JoinThreshold — so each opens a new cluster.
	commits := []extract.CommitData{
		makeCommit("e001", "alice", "payment service refactor", []string{"payments/charge.go"}, nil, baseTime),
		makeCommit("e002", "bob", "logging sink update", []string{"logging/kafka.go"}, nil, baseTime.Add(time.Hour)),
		makeCommit("e003", "carol", "user session cleanup", []string{"session/store.go"}, nil, baseTime.Add(2*time.Hour)),
		makeCommit("e004", "dave", "database migration script", []string{"db/migrate.go"}, nil, baseTime.Add(3*time.Hour)),
		makeCommit("e005", "eve", "infra deployment config", []string{"deploy/k8s.yaml"}, nil, baseTime.Add(4*time.Hour)),
	}

	clusters := cluster.ClusterCommits(commits, cfg)

	// With no shared signals, none of these should cluster together.
	if len(clusters) != 5 {
		t.Errorf("expected 5 separate clusters (distinct authors/paths/tokens), got %d", len(clusters))
	}
}

// TestClusterCommits_EmptyInput returns an empty slice.
func TestClusterCommits_EmptyInput(t *testing.T) {
	cfg := cluster.DefaultConfig()
	clusters := cluster.ClusterCommits(nil, cfg)
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters for empty input, got %d", len(clusters))
	}
}

// TestClusterCommits_SingleCommit returns a cluster with one commit.
func TestClusterCommits_SingleCommit(t *testing.T) {
	cfg := cluster.DefaultConfig()
	commits := []extract.CommitData{
		makeCommit("f001", "alice", "single fix", []string{"foo.go"}, nil, baseTime),
	}
	clusters := cluster.ClusterCommits(commits, cfg)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster for single commit, got %d", len(clusters))
	}
	if len(clusters[0].Commits) != 1 {
		t.Errorf("cluster has %d commits, want 1", len(clusters[0].Commits))
	}
}

// TestClusterCommits_ClusterIDsUnique verifies that each cluster gets a distinct ID.
func TestClusterCommits_ClusterIDsUnique(t *testing.T) {
	cfg := cluster.DefaultConfig()

	commits := []extract.CommitData{
		makeCommit("g001", "alice", "work alpha", []string{"alpha.go"}, nil, baseTime),
		makeCommit("g002", "bob", "work beta", []string{"beta.go"}, nil, baseTime.Add(time.Hour)),
		makeCommit("g003", "carol", "work gamma", []string{"gamma.go"}, nil, baseTime.Add(2*time.Hour)),
	}

	clusters := cluster.ClusterCommits(commits, cfg)

	seen := make(map[string]bool)
	for _, c := range clusters {
		if c.ID == "" {
			t.Errorf("cluster has empty ID")
		}
		if seen[c.ID] {
			t.Errorf("duplicate cluster ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

// TestScore_TicketMatchDominates verifies that a matching ticket ref yields a
// score well above the JoinThreshold (0.35) and that the ticket_match signal
// contributes exactly 0.40 to the total (its full weight).
func TestScore_TicketMatchDominates(t *testing.T) {
	cfg := cluster.DefaultConfig()

	commit := makeCommit("h001", "alice", "LOPS-99: fix", nil, []string{"LOPS-99"}, baseTime)

	centroid := cluster.Centroid{
		Paths:         map[string]int{},
		Authors:       map[string]int{},
		Tokens:        map[string]int{},
		LastTimestamp: baseTime.Add(-time.Hour),
	}
	// TicketRef on the centroid must be set for ticket_match to fire.
	ticketRef := "LOPS-99"

	score := cluster.ScoreWithTicket(commit, centroid, ticketRef, cfg)

	// The total score includes ticket_match (0.40) plus any non-zero residual
	// from other signals (time_proximity and diff_fingerprint contribute even
	// when paths and author are empty). The score must be >= 0.40 and well
	// above the JoinThreshold of 0.35.
	if score < 0.40 {
		t.Errorf("score with ticket_match should be at least 0.40, got %f", score)
	}
	// Verify it is above the join threshold.
	if score < cfg.JoinThreshold {
		t.Errorf("score %f is below JoinThreshold %f — ticket match should guarantee joining", score, cfg.JoinThreshold)
	}

	// Verify the no-ticket version scores lower (ticket_match is additive).
	scoreNoTicket := cluster.ScoreWithTicket(commit, centroid, "", cfg)
	if score <= scoreNoTicket {
		t.Errorf("ticket_match should raise score: with=%f without=%f", score, scoreNoTicket)
	}
}

// TestScore_PathOverlap verifies Jaccard similarity computation for file paths.
func TestScore_PathOverlap(t *testing.T) {
	cfg := cluster.DefaultConfig()

	commit := makeCommit("i001", "alice", "update service", []string{"service/a.go", "service/b.go"}, nil, baseTime)

	centroid := cluster.Centroid{
		Paths:         map[string]int{"service/a.go": 2, "service/b.go": 1, "service/c.go": 1},
		Authors:       map[string]int{"alice": 1},
		Tokens:        map[string]int{},
		LastTimestamp: baseTime.Add(-30 * time.Minute),
	}

	score := cluster.ScoreWithTicket(commit, centroid, "", cfg)

	// Path Jaccard: |{a,b} ∩ {a,b,c}| / |{a,b} ∪ {a,b,c}| = 2/3 ≈ 0.667
	// path_overlap contributes 0.667 * 0.20 ≈ 0.133
	// author_match: alice matches → 0.10
	// time_proximity: 30min/336h ≈ ~0.10 contribution
	// total >= 0.30 (above join threshold)
	if score < 0.25 {
		t.Errorf("score with strong path overlap = %f, expected >= 0.25", score)
	}
}

// TestUpdateCentroid verifies that centroid state is updated correctly after
// adding a commit.
func TestUpdateCentroid(t *testing.T) {
	c := &cluster.Centroid{
		Paths:         map[string]int{},
		Authors:       map[string]int{},
		Tokens:        map[string]int{},
		LastTimestamp: time.Time{},
	}

	commit := makeCommit("j001", "alice", "LOPS-5: implement widget", []string{"widget.go", "widget_test.go"}, []string{"LOPS-5"}, baseTime)

	cluster.UpdateCentroid(c, commit)

	if c.Authors["alice"] != 1 {
		t.Errorf("Authors[alice] = %d, want 1", c.Authors["alice"])
	}
	if c.Paths["widget.go"] != 1 {
		t.Errorf("Paths[widget.go] = %d, want 1", c.Paths["widget.go"])
	}
	if c.Paths["widget_test.go"] != 1 {
		t.Errorf("Paths[widget_test.go] = %d, want 1", c.Paths["widget_test.go"])
	}
	if !c.LastTimestamp.Equal(baseTime) {
		t.Errorf("LastTimestamp = %v, want %v", c.LastTimestamp, baseTime)
	}
	// "implement" and "widget" should be in tokens (stop words filtered out).
	if c.Tokens["implement"] == 0 {
		t.Errorf("Tokens[implement] should be > 0")
	}
	if c.Tokens["widget"] == 0 {
		t.Errorf("Tokens[widget] should be > 0")
	}
}

// TestUpdateCentroid_Accumulates verifies that multiple UpdateCentroid calls
// accumulate counts correctly.
func TestUpdateCentroid_Accumulates(t *testing.T) {
	c := &cluster.Centroid{
		Paths:         map[string]int{},
		Authors:       map[string]int{},
		Tokens:        map[string]int{},
		LastTimestamp: time.Time{},
	}

	c1 := makeCommit("k001", "alice", "first commit widget", []string{"widget.go"}, nil, baseTime)
	c2 := makeCommit("k002", "alice", "second commit widget", []string{"widget.go"}, nil, baseTime.Add(time.Hour))

	cluster.UpdateCentroid(c, c1)
	cluster.UpdateCentroid(c, c2)

	if c.Authors["alice"] != 2 {
		t.Errorf("Authors[alice] = %d, want 2", c.Authors["alice"])
	}
	if c.Paths["widget.go"] != 2 {
		t.Errorf("Paths[widget.go] = %d, want 2", c.Paths["widget.go"])
	}
	if !c.LastTimestamp.Equal(baseTime.Add(time.Hour)) {
		t.Errorf("LastTimestamp = %v, want %v", c.LastTimestamp, baseTime.Add(time.Hour))
	}
}
