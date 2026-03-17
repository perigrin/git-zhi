// ABOUTME: Composite issue difficulty score computation from multiple weighted factors.
// ABOUTME: Normalizes MPG, cycle time, reopen count, mean sentiment, and session count to a 0-1 score.
package difficulty

import (
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/dora"
)

// Factor caps define the maximum value for each factor before the normalized
// score saturates at 1.0.
const (
	capMPG        = 30.0      // commits per issue; 30+ = max difficulty
	capCycleHours = 7 * 24.0  // 7 days in hours; 7+ days = max difficulty
	capReopens    = 3.0       // reopen count; 3+ = max difficulty
	capSessions   = 10.0      // session count; 10+ = max difficulty
)

// Weights for the five factors; must sum to 1.0.
const (
	weightMPG       = 0.25
	weightCycleTime = 0.25
	weightReopens   = 0.20
	weightSentiment = 0.15
	weightSessions  = 0.15
)

// countReopens returns the number of "reopened" transitions recorded for iss.
func countReopens(iss *issue.Issue) int {
	n := 0
	for _, tr := range iss.Transitions {
		if tr.State == string(issue.StateReopened) {
			n++
		}
	}
	return n
}

// totalCommits sums the commit counts across all sessions for iss.
func totalCommits(iss *issue.Issue) int {
	total := 0
	for _, sess := range iss.Sessions {
		total += sess.Commits
	}
	return total
}

// clamp restricts v to [0, 1].
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// normalize maps a raw value to [0, 1] by dividing by the cap, then clamping.
func normalize(raw, cap float64) float64 {
	if cap <= 0 {
		return 0
	}
	return clamp(raw / cap)
}

// ComputeDifficulty returns a composite difficulty score normalized to [0, 1]
// for the given issue. meanSentiment is the pre-computed average VADER compound
// score across all commits in the issue's sessions (range: [-1, 1]); the caller
// is responsible for computing this from the repository. Higher scores mean the
// issue was harder to complete.
//
// Factors and weight direction:
//   - MPG (commits per issue):  higher = harder  (weight 0.25)
//   - Cycle time:               longer = harder  (weight 0.25)
//   - Reopen count:             more = harder    (weight 0.20)
//   - Mean sentiment:           lower = harder   (weight 0.15)
//   - Session count:            more = harder    (weight 0.15)
func ComputeDifficulty(iss *issue.Issue, meanSentiment float64) float64 {
	// MPG: cap at 30 commits.
	mpgNorm := normalize(float64(totalCommits(iss)), capMPG)

	// Cycle time: cap at 7 days (168 hours).
	cycleNorm := normalize(dora.CycleTime(iss).Hours(), capCycleHours)

	// Reopen count: cap at 3.
	reopenNorm := normalize(float64(countReopens(iss)), capReopens)

	// Sentiment: map [-1, 1] → [1, 0] so most negative compound maps to 1.0.
	// Formula: (1 - compound) / 2 gives 1.0 when compound=-1, 0.0 when compound=1.
	sentimentNorm := clamp((1.0 - meanSentiment) / 2.0)

	// Session count: cap at 10.
	sessionNorm := normalize(float64(len(iss.Sessions)), capSessions)

	// Weighted average of all five factors.
	score := weightMPG*mpgNorm +
		weightCycleTime*cycleNorm +
		weightReopens*reopenNorm +
		weightSentiment*sentimentNorm +
		weightSessions*sessionNorm

	return clamp(score)
}
