// ABOUTME: VADER sentiment analysis applied to commit messages within issue session SHA ranges.
// ABOUTME: Provides per-commit scoring, per-issue trajectory, anomaly detection, and milestone aggregation.
package sentiment

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	vader "github.com/grassmudhorses/vader-go"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/issue"
)

// SentimentMetrics aggregates VADER sentiment data for a set of issues.
type SentimentMetrics struct {
	MeanSentiment float64  `json:"mean_sentiment"`
	Anomalies     []string `json:"anomalies"` // issue IDs with sustained negative sentiment
}

// AnalyzeCommitSentiment returns the VADER compound sentiment score for a
// single commit message. The compound score ranges from -1.0 (most negative)
// to +1.0 (most positive), with scores near 0.0 being neutral.
func AnalyzeCommitSentiment(message string) float64 {
	s := vader.GetSentiment(message)
	return s.Compound
}

// IssueTrajectory walks all commits in the issue's session SHA ranges and
// returns a sequence of compound sentiment scores — one per commit — in
// chronological order (oldest first). Open sessions (EndSHA == "") and
// sessions where StartSHA == EndSHA are skipped.
func IssueTrajectory(repo *git.Repository, iss *issue.Issue) []float64 {
	var scores []float64
	for _, sess := range iss.Sessions {
		if sess.EndSHA == "" || sess.StartSHA == sess.EndSHA {
			continue
		}
		sessScores := commitsInRange(repo, sess.StartSHA, sess.EndSHA)
		scores = append(scores, sessScores...)
	}
	return scores
}

// commitsInRange returns compound sentiment scores for each commit in the
// range (startSHA, endSHA] — i.e., exclusive of startSHA, inclusive of endSHA.
// Scores are ordered oldest-first (reversed from the log walk).
func commitsInRange(repo *git.Repository, startSHA, endSHA string) []float64 {
	endHash := plumbing.NewHash(endSHA)
	startHash := plumbing.NewHash(startSHA)

	iter, err := repo.Log(&git.LogOptions{From: endHash})
	if err != nil {
		return nil
	}
	defer iter.Close()

	var msgs []string
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == startHash {
			return storer.ErrStop
		}
		msgs = append(msgs, c.Message)
		return nil
	})
	if err != nil && err != storer.ErrStop {
		return nil
	}

	// msgs is newest-first; reverse to oldest-first for the trajectory.
	scores := make([]float64, len(msgs))
	for i, msg := range msgs {
		scores[len(msgs)-1-i] = AnalyzeCommitSentiment(msg)
	}
	return scores
}

// DetectAnomalies returns true if the trajectory contains compound < -0.1
// for 3 or more consecutive commits. This signals sustained negative sentiment
// that warrants attention.
func DetectAnomalies(trajectory []float64) bool {
	const threshold = -0.1
	const minRun = 3

	run := 0
	for _, score := range trajectory {
		if score <= threshold {
			run++
			if run >= minRun {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

// MilestoneSentiment returns the average compound score across all commits
// in all issues' sessions. Returns 0.0 if there are no scored commits.
func MilestoneSentiment(repo *git.Repository, issues []*issue.Issue) float64 {
	var sum float64
	var count int
	for _, iss := range issues {
		for _, score := range IssueTrajectory(repo, iss) {
			sum += score
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return sum / float64(count)
}

// Compute aggregates sentiment metrics for a set of issues. It computes the
// mean compound sentiment across all sessions and flags issue IDs that exhibit
// sustained negative sentiment (3+ consecutive commits below -0.1).
func Compute(repo *git.Repository, issues []*issue.Issue) SentimentMetrics {
	anomalies := []string{}
	var totalSum float64
	var totalCount int

	for _, iss := range issues {
		traj := IssueTrajectory(repo, iss)
		for _, score := range traj {
			totalSum += score
			totalCount++
		}
		if DetectAnomalies(traj) {
			anomalies = append(anomalies, iss.ID.String())
		}
	}

	var mean float64
	if totalCount > 0 {
		mean = totalSum / float64(totalCount)
	}

	return SentimentMetrics{
		MeanSentiment: mean,
		Anomalies:     anomalies,
	}
}
