// ABOUTME: DORA metrics derived from issue state transitions.
// ABOUTME: Provides lead time, cycle time, rework rate, completion frequency, and aggregate Compute.
package dora

import (
	"time"

	"github.com/perigrin/git-zhi/internal/issue"
)

// DORAMetrics aggregates the four DORA delivery metrics for a set of issues.
type DORAMetrics struct {
	AvgLeadTime         time.Duration `json:"avg_lead_time_hours"`
	AvgCycleTime        time.Duration `json:"avg_cycle_time_hours"`
	ReworkRate          float64       `json:"rework_rate"`
	CompletionFrequency float64       `json:"completion_frequency_per_week"`
}

// firstTransition scans iss.Transitions for the first entry whose State
// matches the target and returns its Timestamp. The zero time and false
// are returned if no matching transition is found.
func firstTransition(iss *issue.Issue, state string) (time.Time, bool) {
	for _, tr := range iss.Transitions {
		if tr.State == state {
			return tr.Timestamp, true
		}
	}
	return time.Time{}, false
}

// hasTransition reports whether iss has any transition with the given state.
func hasTransition(iss *issue.Issue, state string) bool {
	_, ok := firstTransition(iss, state)
	return ok
}

// LeadTime returns the duration from iss.Created to the first done transition.
// Returns 0 if no done transition exists.
func LeadTime(iss *issue.Issue) time.Duration {
	doneAt, ok := firstTransition(iss, string(issue.StateDone))
	if !ok {
		return 0
	}
	return doneAt.Sub(iss.Created)
}

// CycleTime returns the duration from the first in-progress (start) transition
// to the first done transition. Returns 0 if either transition is missing.
func CycleTime(iss *issue.Issue) time.Duration {
	startAt, ok := firstTransition(iss, string(issue.StateInProgress))
	if !ok {
		return 0
	}
	doneAt, ok := firstTransition(iss, string(issue.StateDone))
	if !ok {
		return 0
	}
	return doneAt.Sub(startAt)
}

// ReworkRate returns the percentage of done issues that were reopened at least
// once. A done issue is counted as reworked if it has a reopened transition.
// Returns 0.0 if there are no done issues.
func ReworkRate(issues []*issue.Issue) float64 {
	var totalDone, reworked int
	for _, iss := range issues {
		if !hasTransition(iss, string(issue.StateDone)) {
			continue
		}
		totalDone++
		if hasTransition(iss, string(issue.StateReopened)) {
			reworked++
		}
	}
	if totalDone == 0 {
		return 0
	}
	return float64(reworked) / float64(totalDone) * 100.0
}

// CompletionFrequency returns the number of done issues completed per period.
// The span is defined as the time between the earliest issue.Created and the
// latest done transition across all done issues. Returns 0 if the span is zero
// or if there are no done issues.
func CompletionFrequency(issues []*issue.Issue, period time.Duration) float64 {
	var earliest time.Time
	var latest time.Time
	var doneCount int

	for _, iss := range issues {
		doneAt, ok := firstTransition(iss, string(issue.StateDone))
		if !ok {
			continue
		}
		doneCount++
		if earliest.IsZero() || iss.Created.Before(earliest) {
			earliest = iss.Created
		}
		if latest.IsZero() || doneAt.After(latest) {
			latest = doneAt
		}
	}

	if doneCount == 0 {
		return 0
	}
	span := latest.Sub(earliest)
	if span <= 0 {
		return 0
	}
	return float64(doneCount) / float64(span) * float64(period)
}

// Compute aggregates all four DORA metrics across a set of issues.
// Lead time and cycle time are averaged across done issues only.
func Compute(issues []*issue.Issue) DORAMetrics {
	var leadSum, cycleSum time.Duration
	var leadCount, cycleCount int

	for _, iss := range issues {
		lt := LeadTime(iss)
		if lt > 0 {
			leadSum += lt
			leadCount++
		}
		ct := CycleTime(iss)
		if ct > 0 {
			cycleSum += ct
			cycleCount++
		}
	}

	var avgLead, avgCycle time.Duration
	if leadCount > 0 {
		avgLead = leadSum / time.Duration(leadCount)
	}
	if cycleCount > 0 {
		avgCycle = cycleSum / time.Duration(cycleCount)
	}

	return DORAMetrics{
		AvgLeadTime:         avgLead,
		AvgCycleTime:        avgCycle,
		ReworkRate:          ReworkRate(issues),
		CompletionFrequency: CompletionFrequency(issues, 7*24*time.Hour),
	}
}
