// ABOUTME: Tests for the historian enrich stage: cluster-to-issue conversion.
// ABOUTME: Verifies confidence scoring, session boundaries, actor derivation, and path union.
package enrich_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/historian/enrich"
	"github.com/perigrin/git-zhi/internal/historian/extract"
	"github.com/perigrin/git-zhi/internal/issue"
)

// baseTime is an arbitrary anchor for constructing test commit timestamps.
var baseTime = time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

// makeCommit builds a CommitData with the given fields for test use.
func makeCommit(sha, author, email, message string, paths []string, ticketRefs []string, ts time.Time) extract.CommitData {
	return extract.CommitData{
		SHA:        sha,
		Author:     author,
		Email:      email,
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

// makeCluster builds a Cluster with the given commits and ticket ref.
// It replicates what ClusterCommits produces, without needing to run
// the full clustering algorithm.
func makeCluster(id string, ticketRef string, commits []extract.CommitData) cluster.Cluster {
	c := cluster.Cluster{
		ID:        id,
		Commits:   commits,
		TicketRef: ticketRef,
		Centroid: cluster.Centroid{
			Paths:   make(map[string]int),
			Authors: make(map[string]int),
			Tokens:  make(map[string]int),
		},
	}
	for _, cd := range commits {
		cluster.UpdateCentroid(&c.Centroid, cd)
	}
	return c
}

// TestClusterToIssue_SingleCommitConfidence verifies that a single-commit cluster
// with no ticket ref gets confidence 0.30 and source "single-commit".
func TestClusterToIssue_SingleCommitConfidence(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("abc1234", "alice", "alice@example.com", "fix broken handler", []string{"handler.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-1", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Confidence != 0.30 {
		t.Errorf("Confidence = %f, want 0.30 for single-commit cluster", iss.Confidence)
	}
	if iss.Source != "single-commit" {
		t.Errorf("Source = %q, want single-commit", iss.Source)
	}
}

// TestClusterToIssue_MultiCommitClusterConfidence verifies that a multi-commit
// cluster with no ticket ref and >3 commits gets confidence 0.55 and source "cluster".
func TestClusterToIssue_MultiCommitClusterConfidence(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("c001", "alice", "alice@example.com", "refactor auth", []string{"auth.go"}, nil, baseTime),
		makeCommit("c002", "alice", "alice@example.com", "fix auth edge case", []string{"auth.go"}, nil, baseTime.Add(time.Hour)),
		makeCommit("c003", "alice", "alice@example.com", "add auth tests", []string{"auth_test.go"}, nil, baseTime.Add(2*time.Hour)),
		makeCommit("c004", "alice", "alice@example.com", "auth cleanup", []string{"auth.go"}, nil, baseTime.Add(3*time.Hour)),
	}
	c := makeCluster("cluster-2", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Confidence != 0.55 {
		t.Errorf("Confidence = %f, want 0.55 for heuristic cluster >3 commits", iss.Confidence)
	}
	if iss.Source != "cluster" {
		t.Errorf("Source = %q, want cluster", iss.Source)
	}
}

// TestClusterToIssue_TicketRefMultipleSignals verifies that a multi-commit cluster
// with a ticket ref gets confidence 0.90 and source "tracker-match".
func TestClusterToIssue_TicketRefMultipleSignals(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("d001", "alice", "alice@example.com", "LOPS-142: initial impl", []string{"service.go"}, []string{"LOPS-142"}, baseTime),
		makeCommit("d002", "alice", "alice@example.com", "LOPS-142: add tests", []string{"service_test.go"}, []string{"LOPS-142"}, baseTime.Add(time.Hour)),
		makeCommit("d003", "alice", "alice@example.com", "LOPS-142: review fixes", []string{"service.go"}, []string{"LOPS-142"}, baseTime.Add(2*time.Hour)),
	}
	c := makeCluster("cluster-3", "LOPS-142", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Confidence != 0.90 {
		t.Errorf("Confidence = %f, want 0.90 for tracker-match cluster", iss.Confidence)
	}
	if iss.Source != "tracker-match" {
		t.Errorf("Source = %q, want tracker-match", iss.Source)
	}
}

// TestClusterToIssue_TicketRefSingleCommit verifies that a single-commit cluster
// with a ticket ref gets confidence 0.75 and source "ticket-ref".
func TestClusterToIssue_TicketRefSingleCommit(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("e001", "alice", "alice@example.com", "LOPS-99: quick fix", []string{"fix.go"}, []string{"LOPS-99"}, baseTime),
	}
	c := makeCluster("cluster-4", "LOPS-99", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Confidence != 0.75 {
		t.Errorf("Confidence = %f, want 0.75 for single-commit with ticket ref", iss.Confidence)
	}
	if iss.Source != "ticket-ref" {
		t.Errorf("Source = %q, want ticket-ref", iss.Source)
	}
}

// TestClusterToIssue_SessionSHABoundaries verifies that session StartSHA and
// EndSHA match the first and last commit SHAs respectively.
func TestClusterToIssue_SessionSHABoundaries(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("sha-first", "alice", "alice@example.com", "first commit", []string{"a.go"}, nil, baseTime),
		makeCommit("sha-middle", "alice", "alice@example.com", "middle commit", []string{"b.go"}, nil, baseTime.Add(time.Hour)),
		makeCommit("sha-last", "alice", "alice@example.com", "last commit", []string{"c.go"}, nil, baseTime.Add(2*time.Hour)),
	}
	c := makeCluster("cluster-5", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(iss.Sessions))
	}
	sess := iss.Sessions[0]
	if sess.StartSHA != "sha-first" {
		t.Errorf("StartSHA = %q, want sha-first", sess.StartSHA)
	}
	if sess.EndSHA != "sha-last" {
		t.Errorf("EndSHA = %q, want sha-last", sess.EndSHA)
	}
	if sess.Commits != 3 {
		t.Errorf("Commits = %d, want 3", sess.Commits)
	}
}

// TestClusterToIssue_SessionTimestamps verifies that StartedAt and EndedAt
// match the first and last commit timestamps.
func TestClusterToIssue_SessionTimestamps(t *testing.T) {
	start := baseTime
	end := baseTime.Add(4 * time.Hour)
	commits := []extract.CommitData{
		makeCommit("ts001", "alice", "alice@example.com", "first", []string{"a.go"}, nil, start),
		makeCommit("ts002", "alice", "alice@example.com", "last", []string{"b.go"}, nil, end),
	}
	c := makeCluster("cluster-6", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if len(iss.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(iss.Sessions))
	}
	sess := iss.Sessions[0]
	if sess.StartedAt == nil {
		t.Fatal("StartedAt is nil")
	}
	if !sess.StartedAt.Equal(start) {
		t.Errorf("StartedAt = %v, want %v", sess.StartedAt, start)
	}
	if sess.EndedAt == nil {
		t.Fatal("EndedAt is nil")
	}
	if !sess.EndedAt.Equal(end) {
		t.Errorf("EndedAt = %v, want %v", sess.EndedAt, end)
	}
}

// TestClusterToIssue_ObservedPathsUnion verifies that ObservedPaths is the
// union of all file paths across all commits in the cluster.
func TestClusterToIssue_ObservedPathsUnion(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("p001", "alice", "alice@example.com", "work", []string{"a.go", "b.go"}, nil, baseTime),
		makeCommit("p002", "alice", "alice@example.com", "more work", []string{"b.go", "c.go"}, nil, baseTime.Add(time.Hour)),
		makeCommit("p003", "alice", "alice@example.com", "final", []string{"d.go"}, nil, baseTime.Add(2*time.Hour)),
	}
	c := makeCluster("cluster-7", "", commits)

	iss := enrich.ClusterToIssue(&c)

	expected := []string{"a.go", "b.go", "c.go", "d.go"}
	got := make([]string, len(iss.ObservedPaths))
	copy(got, iss.ObservedPaths)
	sort.Strings(got)

	if len(got) != len(expected) {
		t.Fatalf("ObservedPaths = %v (len %d), want %v (len %d)", got, len(got), expected, len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("ObservedPaths[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

// TestClusterToIssue_ActorFromMostFrequentAuthor verifies that the actor is
// derived from the author who appears most frequently across commits.
func TestClusterToIssue_ActorFromMostFrequentAuthor(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("a001", "alice", "alice@example.com", "alice work 1", []string{"a.go"}, nil, baseTime),
		makeCommit("a002", "bob", "bob@example.com", "bob work", []string{"b.go"}, nil, baseTime.Add(time.Hour)),
		makeCommit("a003", "alice", "alice@example.com", "alice work 2", []string{"a.go"}, nil, baseTime.Add(2*time.Hour)),
		makeCommit("a004", "alice", "alice@example.com", "alice work 3", []string{"a.go"}, nil, baseTime.Add(3*time.Hour)),
	}
	c := makeCluster("cluster-8", "", commits)

	iss := enrich.ClusterToIssue(&c)

	// alice appears 3 times, bob 1 time. The issue actor should be alice.
	// Actor is stored as "human:alice" in the transitions.
	if len(iss.Transitions) == 0 {
		t.Fatal("expected transitions to be populated")
	}
	foundAlice := false
	for _, tr := range iss.Transitions {
		if tr.Actor == "human:alice" {
			foundAlice = true
			break
		}
	}
	if !foundAlice {
		t.Errorf("expected transitions to reference actor human:alice, got %v", iss.Transitions)
	}
}

// TestClusterToIssue_TitleFromTicketRef verifies that when a ticket ref is
// present, the issue title is the ticket ref string.
func TestClusterToIssue_TitleFromTicketRef(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("t001", "alice", "alice@example.com", "LOPS-142: implement feature", []string{"feature.go"}, []string{"LOPS-142"}, baseTime),
	}
	c := makeCluster("cluster-9", "LOPS-142", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Title != "LOPS-142" {
		t.Errorf("Title = %q, want LOPS-142", iss.Title)
	}
}

// TestClusterToIssue_TitleFromFirstMessage verifies that when no ticket ref is
// present, the title comes from the first commit message (truncated to 80 chars).
func TestClusterToIssue_TitleFromFirstMessage(t *testing.T) {
	msg := "fix broken authentication handler in the login service"
	commits := []extract.CommitData{
		makeCommit("m001", "alice", "alice@example.com", msg, []string{"auth.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-10", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Title != msg {
		t.Errorf("Title = %q, want %q", iss.Title, msg)
	}
}

// TestClusterToIssue_TitleTruncatedTo80Chars verifies that a long first commit
// message is truncated to 80 characters.
func TestClusterToIssue_TitleTruncatedTo80Chars(t *testing.T) {
	longMsg := "this is a very long commit message that exceeds eighty characters and should be truncated by the enrich stage"
	commits := []extract.CommitData{
		makeCommit("m002", "alice", "alice@example.com", longMsg, []string{"x.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-11", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if len(iss.Title) > 80 {
		t.Errorf("Title length = %d, want <= 80", len(iss.Title))
	}
	if iss.Title != longMsg[:80] {
		t.Errorf("Title = %q, want %q", iss.Title, longMsg[:80])
	}
}

// TestClusterToIssue_TitleFirstLineOnly verifies that multi-line commit
// messages use only the subject line (first line) as the issue title.
func TestClusterToIssue_TitleFirstLineOnly(t *testing.T) {
	msg := "Fix authentication bug\n\nThis commit fixes the broken auth handler\nby validating tokens before checking permissions."
	commits := []extract.CommitData{
		makeCommit("m003", "alice", "alice@example.com", msg, []string{"auth.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-13", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if strings.Contains(iss.Title, "\n") {
		t.Errorf("Title contains newline: %q", iss.Title)
	}
	if iss.Title != "Fix authentication bug" {
		t.Errorf("Title = %q, want %q", iss.Title, "Fix authentication bug")
	}
}

// TestClusterToIssue_StateDone verifies that retrospective issues are always
// created in the done state.
func TestClusterToIssue_StateDone(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("s001", "alice", "alice@example.com", "some work", []string{"x.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-12", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.State != issue.StateDone {
		t.Errorf("State = %q, want done", iss.State)
	}
}

// TestClusterToIssue_Transitions verifies that transitions include in-progress
// at first commit time and done at last commit time.
func TestClusterToIssue_Transitions(t *testing.T) {
	start := baseTime
	end := baseTime.Add(3 * time.Hour)
	commits := []extract.CommitData{
		makeCommit("tr01", "alice", "alice@example.com", "start work", []string{"a.go"}, nil, start),
		makeCommit("tr02", "alice", "alice@example.com", "finish work", []string{"a.go"}, nil, end),
	}
	c := makeCluster("cluster-13", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if len(iss.Transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d: %v", len(iss.Transitions), iss.Transitions)
	}

	// First transition: in-progress at start time.
	if iss.Transitions[0].State != string(issue.StateInProgress) {
		t.Errorf("Transitions[0].State = %q, want in-progress", iss.Transitions[0].State)
	}
	if !iss.Transitions[0].Timestamp.Equal(start) {
		t.Errorf("Transitions[0].Timestamp = %v, want %v", iss.Transitions[0].Timestamp, start)
	}

	// Second transition: done at end time.
	if iss.Transitions[1].State != string(issue.StateDone) {
		t.Errorf("Transitions[1].State = %q, want done", iss.Transitions[1].State)
	}
	if !iss.Transitions[1].Timestamp.Equal(end) {
		t.Errorf("Transitions[1].Timestamp = %v, want %v", iss.Transitions[1].Timestamp, end)
	}
}

// TestClusterToIssue_TrackerID verifies that TrackerID is set from the ticket
// ref using the "unknown:<ref>" format when no tracker prefix is available.
func TestClusterToIssue_TrackerID(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("tid1", "alice", "alice@example.com", "LOPS-142: work", []string{"x.go"}, []string{"LOPS-142"}, baseTime),
	}
	c := makeCluster("cluster-14", "LOPS-142", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.TrackerID != "unknown:LOPS-142" {
		t.Errorf("TrackerID = %q, want unknown:LOPS-142", iss.TrackerID)
	}
}

// TestClusterToIssue_CreatedUpdatedTimestamps verifies that Created equals the
// first commit timestamp and Updated equals the last commit timestamp.
func TestClusterToIssue_CreatedUpdatedTimestamps(t *testing.T) {
	start := baseTime
	end := baseTime.Add(5 * time.Hour)
	commits := []extract.CommitData{
		makeCommit("cu01", "alice", "alice@example.com", "first", []string{"a.go"}, nil, start),
		makeCommit("cu02", "alice", "alice@example.com", "last", []string{"b.go"}, nil, end),
	}
	c := makeCluster("cluster-15", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if !iss.Created.Equal(start) {
		t.Errorf("Created = %v, want %v", iss.Created, start)
	}
	if !iss.Updated.Equal(end) {
		t.Errorf("Updated = %v, want %v", iss.Updated, end)
	}
}

// TestClusterToIssue_IDGenerated verifies that each call generates a non-zero
// UUIDv7 for the issue ID.
func TestClusterToIssue_IDGenerated(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("id01", "alice", "alice@example.com", "work", []string{"a.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-16", "", commits)

	iss1 := enrich.ClusterToIssue(&c)
	iss2 := enrich.ClusterToIssue(&c)

	if iss1.ID.IsNil() {
		t.Error("ID should not be nil UUID")
	}
	if iss2.ID.IsNil() {
		t.Error("ID should not be nil UUID")
	}
	if iss1.ID == iss2.ID {
		t.Error("two calls should generate distinct IDs")
	}
}

// TestClusterToIssue_LabelsEmpty verifies that Labels is initialized to an
// empty (non-nil) slice, not nil, for JSON serialization safety.
func TestClusterToIssue_LabelsEmpty(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("lb01", "alice", "alice@example.com", "work", []string{"a.go"}, nil, baseTime),
	}
	c := makeCluster("cluster-17", "", commits)

	iss := enrich.ClusterToIssue(&c)

	if iss.Labels == nil {
		t.Error("Labels should be an empty slice, not nil")
	}
	if len(iss.Labels) != 0 {
		t.Errorf("Labels = %v, want empty", iss.Labels)
	}
}

// TestEnrichClusters_SortedByCreated verifies that EnrichClusters returns
// issues sorted by Created timestamp (ascending).
func TestEnrichClusters_SortedByCreated(t *testing.T) {
	// Cluster A: commits starting at baseTime+5h
	clusterA := makeCluster("cluster-A", "", []extract.CommitData{
		makeCommit("a1", "alice", "alice@example.com", "later work", []string{"a.go"}, nil, baseTime.Add(5*time.Hour)),
	})
	// Cluster B: commits starting at baseTime (earlier)
	clusterB := makeCluster("cluster-B", "", []extract.CommitData{
		makeCommit("b1", "bob", "bob@example.com", "early work", []string{"b.go"}, nil, baseTime),
	})
	// Cluster C: commits starting at baseTime+2h
	clusterC := makeCluster("cluster-C", "", []extract.CommitData{
		makeCommit("c1", "carol", "carol@example.com", "middle work", []string{"c.go"}, nil, baseTime.Add(2*time.Hour)),
	})

	clusters := []cluster.Cluster{clusterA, clusterB, clusterC}
	issues := enrich.EnrichClusters(clusters)

	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}
	if !issues[0].Created.Equal(baseTime) {
		t.Errorf("issues[0].Created = %v, want %v (earliest)", issues[0].Created, baseTime)
	}
	if !issues[1].Created.Equal(baseTime.Add(2 * time.Hour)) {
		t.Errorf("issues[1].Created = %v, want %v", issues[1].Created, baseTime.Add(2*time.Hour))
	}
	if !issues[2].Created.Equal(baseTime.Add(5 * time.Hour)) {
		t.Errorf("issues[2].Created = %v, want %v (latest)", issues[2].Created, baseTime.Add(5*time.Hour))
	}
}

// TestEnrichClusters_EmptyInput returns an empty slice without panicking.
func TestEnrichClusters_EmptyInput(t *testing.T) {
	issues := enrich.EnrichClusters(nil)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for empty input, got %d", len(issues))
	}
}

// TestClusterToIssue_MultiCommitNoTicketFewCommits verifies that a 2-commit
// cluster with no ticket ref gets confidence 0.30 (single-commit threshold
// does not apply; but it has < 4 commits so it's still treated as single-commit
// by the design — actually: single-commit means exactly 1 commit. Let's verify
// the 2-commit case: no ticket ref and len < 4 should be treated as... re-reading
// the spec: "Heuristic clustering, >3 commits: 0.55 (cluster)" and
// "Single commit: 0.30 (single-commit)". A 2-commit no-ticket cluster has
// no tier specified between them. We treat len==1 as single-commit (0.30);
// len>3 as cluster (0.55); between 2-3 as single-commit as well per spec.
func TestClusterToIssue_TwoCommitNoTicket(t *testing.T) {
	commits := []extract.CommitData{
		makeCommit("x001", "alice", "alice@example.com", "work 1", []string{"a.go"}, nil, baseTime),
		makeCommit("x002", "alice", "alice@example.com", "work 2", []string{"b.go"}, nil, baseTime.Add(time.Hour)),
	}
	c := makeCluster("cluster-18", "", commits)

	iss := enrich.ClusterToIssue(&c)

	// 2 commits, no ticket ref: not enough to qualify as "cluster" (>3),
	// and not a single commit — but per the spec, only >3 gets 0.55.
	// Single commit = 0.30 applies to len==1.
	// For 2-3 commits with no ticket ref, we use 0.30 (single-commit) as
	// the spec does not define an intermediate tier.
	if iss.Confidence != 0.30 {
		t.Errorf("Confidence = %f, want 0.30 for 2-commit cluster without ticket ref", iss.Confidence)
	}
	if iss.Source != "single-commit" {
		t.Errorf("Source = %q, want single-commit for 2-commit cluster without ticket ref", iss.Source)
	}
}
