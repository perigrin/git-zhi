// ABOUTME: SPACE framework metrics derived from issue data.
// ABOUTME: Provides agent autonomy rate, review cycle count, session count, and aggregate Compute.
package space

import (
	"strings"

	"github.com/perigrin/git-zhi/internal/actor"
	"github.com/perigrin/git-zhi/internal/issue"
)

// SPACEMetrics aggregates the SPACE framework metrics computable from issue data.
// Dimensions requiring external signals (e.g., satisfaction surveys) are omitted.
type SPACEMetrics struct {
	AgentAutonomyRate float64 `json:"agent_autonomy_rate"`
	AvgReviewCycles   float64 `json:"avg_review_cycles"`
	AvgSessionCount   float64 `json:"avg_session_count"`
}

// AgentAutonomyRate returns the percentage of done issues where ALL
// state-change transitions were triggered by agents (no human interventions).
// Returns 0 if there are no done issues.
func AgentAutonomyRate(issues []*issue.Issue) float64 {
	var totalDone, agentAutonomous int
	for _, iss := range issues {
		if !hasDoneTransition(iss) {
			continue
		}
		totalDone++
		if allTransitionsAgent(iss) {
			agentAutonomous++
		}
	}
	if totalDone == 0 {
		return 0
	}
	return float64(agentAutonomous) / float64(totalDone) * 100.0
}

// ReviewCycleCount returns the number of done→reopened cycles on an issue.
// Each "reopened" transition represents one review cycle.
func ReviewCycleCount(iss *issue.Issue) int {
	count := 0
	for _, tr := range iss.Transitions {
		if tr.State == string(issue.StateReopened) {
			count++
		}
	}
	return count
}

// SessionCountPerIssue returns the number of work sessions recorded on an issue.
func SessionCountPerIssue(iss *issue.Issue) int {
	return len(iss.Sessions)
}

// Compute aggregates all SPACE metrics across a set of issues.
// Review cycles and session counts are averaged across done issues only,
// since non-done issues may have incomplete data.
func Compute(issues []*issue.Issue) SPACEMetrics {
	var reviewSum, sessionSum int
	var doneCount int

	for _, iss := range issues {
		if !hasDoneTransition(iss) {
			continue
		}
		doneCount++
		reviewSum += ReviewCycleCount(iss)
		sessionSum += SessionCountPerIssue(iss)
	}

	m := SPACEMetrics{
		AgentAutonomyRate: AgentAutonomyRate(issues),
	}
	if doneCount > 0 {
		m.AvgReviewCycles = float64(reviewSum) / float64(doneCount)
		m.AvgSessionCount = float64(sessionSum) / float64(doneCount)
	}
	return m
}

// hasDoneTransition reports whether the issue has at least one done transition.
func hasDoneTransition(iss *issue.Issue) bool {
	for _, tr := range iss.Transitions {
		if tr.State == string(issue.StateDone) {
			return true
		}
	}
	return false
}

// allTransitionsAgent reports whether every transition on an issue was
// triggered by an agent. An issue with no transitions is not considered
// agent-autonomous because there is no evidence of agent involvement.
func allTransitionsAgent(iss *issue.Issue) bool {
	if len(iss.Transitions) == 0 {
		return false
	}
	for _, tr := range iss.Transitions {
		a := actor.ParseActor(tr.Actor)
		if !strings.EqualFold(a.Type, actor.TypeAgent) {
			return false
		}
	}
	return true
}
