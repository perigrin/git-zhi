// ABOUTME: CALMS DevOps framework indicators derived from issue data.
// ABOUTME: Provides verification coverage, WIP compliance, scope cut frequency, and metric completeness.
package calms

import (
	"sort"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/verify"
)

// CALMSIndicators aggregates the CALMS DevOps framework indicators for a set of issues.
type CALMSIndicators struct {
	VerificationCoverage float64 `json:"verification_coverage"`
	WIPViolations        int     `json:"wip_violations"`
	ScopeCutFrequency    float64 `json:"scope_cut_frequency"`
	MetricCompleteness   float64 `json:"metric_completeness"`
}

// VerificationCoverage returns the percentage of issues that have at least one
// executable backtick command in their acceptance criteria. An issue without
// a body or without any backtick commands in its AC items counts as not covered.
// Returns 0 if the slice is empty.
func VerificationCoverage(issues []*issue.Issue) float64 {
	if len(issues) == 0 {
		return 0
	}
	var covered int
	for _, iss := range issues {
		sections := issue.ParseSections(iss.Body)
		cmds := verify.ExtractCommands(sections, uuid.UUID{}, "")
		if len(cmds) > 0 {
			covered++
		}
	}
	return float64(covered) / float64(len(issues)) * 100.0
}

// wipInterval represents the open period when an issue was in-progress.
// End is the zero time if the issue is still in-progress.
type wipInterval struct {
	start time.Time
	end   time.Time // zero means still open
}

// buildWIPIntervals extracts all in-progress intervals from an issue's
// transitions. Each time the issue transitions to in-progress, a new interval
// starts; the interval closes when the next non-in-progress transition occurs
// (done, cancelled, or reopened). Issues that enter in-progress multiple times
// (e.g., after a pause-resume) produce multiple intervals.
func buildWIPIntervals(iss *issue.Issue) []wipInterval {
	var intervals []wipInterval
	var openStart time.Time

	for _, tr := range iss.Transitions {
		switch tr.State {
		case string(issue.StateInProgress):
			openStart = tr.Timestamp
		case string(issue.StateDone), string(issue.StateCancelled), string(issue.StateReopened):
			if !openStart.IsZero() {
				intervals = append(intervals, wipInterval{start: openStart, end: tr.Timestamp})
				openStart = time.Time{}
			}
		}
	}
	// If the issue is still in-progress at the end of its transitions, the
	// interval remains open (end stays zero).
	if !openStart.IsZero() {
		intervals = append(intervals, wipInterval{start: openStart})
	}
	return intervals
}

// isOpenAt reports whether the interval includes the given moment. An interval
// with a zero end is treated as still open (extends to the future).
func isOpenAt(iv wipInterval, t time.Time) bool {
	if t.Before(iv.start) {
		return false
	}
	if iv.end.IsZero() {
		return true
	}
	return t.Before(iv.end)
}

// WIPCompliance counts instances where more than one issue was in-progress
// simultaneously. Each transition to in-progress is examined; if another issue
// was already open at that moment, a violation is recorded.
// Returns 0 if no violations are found.
func WIPCompliance(issues []*issue.Issue) int {
	// Collect all in-progress intervals per issue.
	type issueIntervals struct {
		intervals []wipInterval
	}
	allIntervals := make([]issueIntervals, len(issues))
	for i, iss := range issues {
		allIntervals[i] = issueIntervals{intervals: buildWIPIntervals(iss)}
	}

	// Collect all in-progress start events sorted by timestamp.
	type startEvent struct {
		issueIdx int
		t        time.Time
	}
	var events []startEvent
	for i, ii := range allIntervals {
		for _, iv := range ii.intervals {
			events = append(events, startEvent{issueIdx: i, t: iv.start})
		}
	}
	sort.Slice(events, func(a, b int) bool {
		return events[a].t.Before(events[b].t)
	})

	violations := 0
	for _, ev := range events {
		// Count how many OTHER issues are already open at this start time
		// (strictly before, so the new issue itself does not count).
		openCount := 0
		for j, ii := range allIntervals {
			if j == ev.issueIdx {
				continue
			}
			for _, iv := range ii.intervals {
				// Another issue's interval is "already open" at ev.t if ev.t
				// is strictly inside [iv.start, iv.end). We exclude ev.t == iv.start
				// so that two issues starting at the exact same instant each only
				// see the other as a single violation event rather than doubling.
				if iv.start.Before(ev.t) && isOpenAt(iv, ev.t) {
					openCount++
					break // count each OTHER issue at most once per event
				}
			}
		}
		if openCount > 0 {
			violations++
		}
	}
	return violations
}

// ScopeCutFrequency returns the percentage of issues that ended up cancelled.
// Returns 0 if the slice is empty.
func ScopeCutFrequency(issues []*issue.Issue) float64 {
	if len(issues) == 0 {
		return 0
	}
	var cancelled int
	for _, iss := range issues {
		if iss.State == issue.StateCancelled {
			cancelled++
		}
	}
	return float64(cancelled) / float64(len(issues)) * 100.0
}

// MetricCompleteness returns the percentage of issues that have all three
// completeness signals: at least one session, at least one executable AC
// command, and at least one context path. Issues missing any of the three
// are not considered metrically complete.
// Returns 0 if the slice is empty.
func MetricCompleteness(issues []*issue.Issue) float64 {
	if len(issues) == 0 {
		return 0
	}
	var complete int
	for _, iss := range issues {
		if !isMetricallyComplete(iss) {
			continue
		}
		complete++
	}
	return float64(complete) / float64(len(issues)) * 100.0
}

// isMetricallyComplete reports whether an issue has all three completeness
// signals: sessions, executable AC commands, and context paths.
func isMetricallyComplete(iss *issue.Issue) bool {
	if len(iss.Sessions) == 0 {
		return false
	}
	sections := issue.ParseSections(iss.Body)
	cmds := verify.ExtractCommands(sections, uuid.UUID{}, "")
	if len(cmds) == 0 {
		return false
	}
	if sections.Context == nil || len(sections.Context.Paths) == 0 {
		return false
	}
	return true
}

// Compute aggregates all four CALMS indicators across a set of issues.
func Compute(issues []*issue.Issue) CALMSIndicators {
	return CALMSIndicators{
		VerificationCoverage: VerificationCoverage(issues),
		WIPViolations:        WIPCompliance(issues),
		ScopeCutFrequency:    ScopeCutFrequency(issues),
		MetricCompleteness:   MetricCompleteness(issues),
	}
}
