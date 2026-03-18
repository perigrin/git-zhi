// ABOUTME: Historian cluster stage: groups commits into issue candidates using greedy sequential clustering.
// ABOUTME: Uses centroid coherence to prevent transitive drift; six weighted signals drive similarity scoring.
package cluster

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/perigrin/git-zhi/internal/historian/extract"
)

// SignalWeights holds per-signal contribution weights for the composite score.
// Weights should sum to 1.0 but are not enforced — a partial sum is valid for
// diagnostic purposes.
type SignalWeights struct {
	TicketMatch     float64 `yaml:"ticket_match" json:"ticket_match"`
	PathOverlap     float64 `yaml:"path_overlap" json:"path_overlap"`
	DiffFingerprint float64 `yaml:"diff_fingerprint" json:"diff_fingerprint"`
	TimeProximity   float64 `yaml:"time_proximity" json:"time_proximity"`
	AuthorMatch     float64 `yaml:"author_match" json:"author_match"`
	MessageTokens   float64 `yaml:"message_tokens" json:"message_tokens"`
}

// Config controls the clustering algorithm's thresholds and signal weights.
type Config struct {
	JoinThreshold      float64       `yaml:"join_threshold" json:"join_threshold"`
	CoherenceThreshold float64       `yaml:"coherence_threshold" json:"coherence_threshold"`
	InactivityGap      time.Duration `yaml:"inactivity_gap" json:"inactivity_gap"`
	Weights            SignalWeights `yaml:"weights" json:"weights"`
}

// DefaultConfig returns the provisional threshold and weight values from the
// v0.3 design document. Values are expected to be calibrated once real-world
// data is available in v0.3.1.
func DefaultConfig() Config {
	return Config{
		JoinThreshold:      0.35,
		CoherenceThreshold: 0.25,
		InactivityGap:      14 * 24 * time.Hour,
		Weights: SignalWeights{
			TicketMatch:     0.40,
			PathOverlap:     0.20,
			DiffFingerprint: 0.15,
			TimeProximity:   0.10,
			AuthorMatch:     0.10,
			MessageTokens:   0.05,
		},
	}
}

// Centroid aggregates the collective metrics of all commits in a cluster.
// It is updated incrementally as commits are added.
type Centroid struct {
	Paths          map[string]int          // file path → occurrence count
	Authors        map[string]int          // author name → occurrence count
	Tokens         map[string]int          // significant message word → occurrence count
	AvgFingerprint extract.DiffFingerprint // running average of diff shape metrics
	LastTimestamp  time.Time               // timestamp of the most recently added commit
}

// Cluster is a group of related commits that form a single issue candidate.
type Cluster struct {
	ID        string              // generated identifier (format: cluster-N)
	Commits   []extract.CommitData
	Centroid  Centroid
	TicketRef string // primary ticket reference, if any commit carried one
	Closed    bool   // true once the inactivity gap has been exceeded
}

// stopWords are filtered out when tokenising commit messages. They add noise
// without contributing to topic similarity.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "it": true, "this": true, "that": true, "not": true,
	"no": true, "be": true, "was": true, "are": true, "were": true,
	"has": true, "had": true, "have": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true,
}

// tokenise splits a commit message into significant lower-cased words,
// dropping stop words and single-character tokens.
func tokenise(message string) []string {
	// Split on whitespace and common punctuation.
	fields := strings.FieldsFunc(message, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' ||
			r == ',' || r == '.' || r == ':' || r == ';' ||
			r == '!' || r == '?' || r == '(' || r == ')' ||
			r == '[' || r == ']' || r == '{' || r == '}' ||
			r == '"' || r == '\''
	})

	var result []string
	for _, f := range fields {
		w := strings.ToLower(f)
		if len(w) <= 1 {
			continue
		}
		if stopWords[w] {
			continue
		}
		result = append(result, w)
	}
	return result
}

// jaccardStrings computes the Jaccard similarity index of two string sets
// (intersection / union). Returns 0.0 if both sets are empty.
func jaccardStrings(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	setA := make(map[string]bool, len(a))
	for _, v := range a {
		setA[v] = true
	}
	var intersect int
	setB := make(map[string]bool, len(b))
	for _, v := range b {
		setB[v] = true
		if setA[v] {
			intersect++
		}
	}
	union := len(setA) + len(setB) - intersect
	if union == 0 {
		return 0.0
	}
	return float64(intersect) / float64(union)
}

// jaccardCentroidPaths computes Jaccard similarity between the commit's file
// paths and the centroid's path keys.
func jaccardCentroidPaths(commitPaths []string, centroidPaths map[string]int) float64 {
	if len(commitPaths) == 0 && len(centroidPaths) == 0 {
		return 0.0
	}
	centroidKeys := make([]string, 0, len(centroidPaths))
	for k := range centroidPaths {
		centroidKeys = append(centroidKeys, k)
	}
	return jaccardStrings(commitPaths, centroidKeys)
}

// jaccardCentroidTokens computes Jaccard similarity between the commit's
// message tokens and the centroid's token keys.
func jaccardCentroidTokens(commitTokens []string, centroidTokens map[string]int) float64 {
	if len(commitTokens) == 0 && len(centroidTokens) == 0 {
		return 0.0
	}
	centroidKeys := make([]string, 0, len(centroidTokens))
	for k := range centroidTokens {
		centroidKeys = append(centroidKeys, k)
	}
	return jaccardStrings(commitTokens, centroidKeys)
}

// fingerprintSimilarity returns a [0, 1] similarity score between two diff
// fingerprints based on change_ratio and file_count proximity.
func fingerprintSimilarity(commit extract.DiffFingerprint, centroid extract.DiffFingerprint) float64 {
	// change_ratio similarity: 1 - |a - b| / (max(a, b) + 1)
	ratioDiff := math.Abs(commit.ChangeRatio - centroid.ChangeRatio)
	maxRatio := math.Max(commit.ChangeRatio, centroid.ChangeRatio) + 1
	ratioSim := 1.0 - ratioDiff/maxRatio

	// file_count similarity: 1 - |a - b| / (max(a, b) + 1)
	countDiff := math.Abs(float64(commit.FileCount - centroid.FileCount))
	maxCount := math.Max(float64(commit.FileCount), float64(centroid.FileCount)) + 1
	countSim := 1.0 - countDiff/maxCount

	return (ratioSim + countSim) / 2.0
}

// timeProximityScore returns a [0, 1] score: 1.0 when the gap is zero,
// decaying linearly to 0.0 at InactivityGap.
func timeProximityScore(commitTS, centroidTS time.Time, gap time.Duration) float64 {
	if centroidTS.IsZero() {
		return 1.0
	}
	diff := commitTS.Sub(centroidTS)
	if diff < 0 {
		diff = -diff
	}
	score := 1.0 - float64(diff)/float64(gap)
	if score < 0 {
		return 0.0
	}
	return score
}

// ScoreWithTicket computes the weighted similarity score between a commit and a
// cluster centroid. The ticketRef argument is the cluster's primary ticket
// reference (empty string if none). Exported so tests can drive individual
// signal contributions directly.
func ScoreWithTicket(commit extract.CommitData, centroid Centroid, ticketRef string, config Config) float64 {
	w := config.Weights

	// --- ticket_match ---
	var ticketScore float64
	if ticketRef != "" {
		for _, ref := range commit.TicketRefs {
			if ref == ticketRef {
				ticketScore = 1.0
				break
			}
		}
	}

	// --- path_overlap (Jaccard) ---
	pathScore := jaccardCentroidPaths(commit.Fingerprint.Paths, centroid.Paths)

	// --- diff_fingerprint ---
	fpScore := fingerprintSimilarity(commit.Fingerprint, centroid.AvgFingerprint)

	// --- time_proximity ---
	timeScore := timeProximityScore(commit.Timestamp, centroid.LastTimestamp, config.InactivityGap)

	// --- author_match ---
	var authorScore float64
	if _, ok := centroid.Authors[commit.Author]; ok {
		authorScore = 1.0
	}

	// --- message_tokens (Jaccard) ---
	tokens := tokenise(commit.Message)
	tokenScore := jaccardCentroidTokens(tokens, centroid.Tokens)

	return w.TicketMatch*ticketScore +
		w.PathOverlap*pathScore +
		w.DiffFingerprint*fpScore +
		w.TimeProximity*timeScore +
		w.AuthorMatch*authorScore +
		w.MessageTokens*tokenScore
}

// UpdateCentroid updates c in-place to incorporate the new commit's data.
// It accumulates path and author frequency counts, merges message tokens,
// updates the LastTimestamp if the commit is newer, and recomputes the running
// average diff fingerprint.
func UpdateCentroid(c *Centroid, commit extract.CommitData) {
	// Paths.
	for _, p := range commit.Fingerprint.Paths {
		c.Paths[p]++
	}

	// Author.
	c.Authors[commit.Author]++

	// Message tokens.
	for _, tok := range tokenise(commit.Message) {
		c.Tokens[tok]++
	}

	// Timestamp: always advance to the latest seen.
	if commit.Timestamp.After(c.LastTimestamp) {
		c.LastTimestamp = commit.Timestamp
	}

	// Running average of diff fingerprint.
	// We approximate by keeping a simple average across all commits in the
	// centroid. The count is recovered from the total author occurrences.
	totalCount := 0
	for _, v := range c.Authors {
		totalCount += v
	}
	// Recompute after the author count was already incremented above.
	if totalCount <= 1 {
		c.AvgFingerprint = commit.Fingerprint
	} else {
		prev := float64(totalCount - 1)
		curr := float64(totalCount)
		c.AvgFingerprint.ChangeRatio = (c.AvgFingerprint.ChangeRatio*prev + commit.Fingerprint.ChangeRatio) / curr
		c.AvgFingerprint.FileCount = int(math.Round((float64(c.AvgFingerprint.FileCount)*prev + float64(commit.Fingerprint.FileCount)) / curr))
		c.AvgFingerprint.HunkCount = int(math.Round((float64(c.AvgFingerprint.HunkCount)*prev + float64(commit.Fingerprint.HunkCount)) / curr))
		c.AvgFingerprint.Insertions = int(math.Round((float64(c.AvgFingerprint.Insertions)*prev + float64(commit.Fingerprint.Insertions)) / curr))
		c.AvgFingerprint.Deletions = int(math.Round((float64(c.AvgFingerprint.Deletions)*prev + float64(commit.Fingerprint.Deletions)) / curr))
	}
}

// ClusterCommits groups commits into issue candidates using greedy sequential
// clustering with centroid coherence. Commits must arrive in chronological
// order (oldest first), as produced by extract.ExtractCommits.
//
// Algorithm:
//  1. For each commit, score it against all open (non-closed) cluster centroids.
//  2. If the best score >= JoinThreshold AND score >= CoherenceThreshold: add
//     to that cluster and update its centroid.
//  3. If no cluster qualifies: open a new cluster for this commit.
//  4. After processing each commit: close any cluster whose LastTimestamp is
//     more than InactivityGap before the current commit's timestamp.
//
// Returns all clusters (open and closed) in creation order.
func ClusterCommits(commits []extract.CommitData, config Config) []Cluster {
	if len(commits) == 0 {
		return nil
	}

	var clusters []Cluster
	nextID := 1

	for _, commit := range commits {
		// --- Step 1: close clusters that have exceeded the inactivity gap ---
		// This must happen before scoring so that closed clusters are not
		// available for the current commit to join.
		for i := range clusters {
			if clusters[i].Closed {
				continue
			}
			if commit.Timestamp.Sub(clusters[i].Centroid.LastTimestamp) > config.InactivityGap {
				clusters[i].Closed = true
			}
		}

		// --- Step 2: score commit against all open clusters ---
		bestIdx := -1
		bestScore := -1.0

		for i := range clusters {
			if clusters[i].Closed {
				continue
			}
			score := ScoreWithTicket(commit, clusters[i].Centroid, clusters[i].TicketRef, config)
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}

		// --- Step 3: join or open ---
		if bestIdx >= 0 &&
			bestScore >= config.JoinThreshold &&
			bestScore >= config.CoherenceThreshold {
			// Add commit to the best matching cluster.
			clusters[bestIdx].Commits = append(clusters[bestIdx].Commits, commit)
			UpdateCentroid(&clusters[bestIdx].Centroid, commit)
			// Propagate ticket ref if the cluster doesn't have one yet.
			if clusters[bestIdx].TicketRef == "" && len(commit.TicketRefs) > 0 {
				clusters[bestIdx].TicketRef = commit.TicketRefs[0]
			}
		} else {
			// Open a new cluster.
			ticketRef := ""
			if len(commit.TicketRefs) > 0 {
				ticketRef = commit.TicketRefs[0]
			}
			centroid := Centroid{
				Paths:   make(map[string]int),
				Authors: make(map[string]int),
				Tokens:  make(map[string]int),
			}
			UpdateCentroid(&centroid, commit)

			clusters = append(clusters, Cluster{
				ID:        fmt.Sprintf("cluster-%d", nextID),
				Commits:   []extract.CommitData{commit},
				Centroid:  centroid,
				TicketRef: ticketRef,
			})
			nextID++
		}
	}

	return clusters
}
