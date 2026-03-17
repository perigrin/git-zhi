// ABOUTME: Telemetry computation for development signals (MPG, speed, buffer).
// ABOUTME: Stateless — derives all indicators from issue and milestone data.
package telemetry

import (
	"time"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// Status represents the fever chart health status of a milestone.
type Status string

const (
	StatusGreen  Status = "GREEN"
	StatusYellow Status = "YELLOW"
	StatusRed    Status = "RED"
)

// Stats holds the derived telemetry indicators for a milestone.
type Stats struct {
	MPG          float64 `yaml:"mpg" json:"mpg"`
	Speed        float64 `yaml:"speed" json:"speed"`
	BufferTotal  float64 `yaml:"buffer_total" json:"buffer_total"`
	BufferBurned float64 `yaml:"buffer_burned" json:"buffer_burned"`
	// TimeInChain is the ratio of active session duration to total calendar time
	// (sum of session durations / span from earliest StartedAt to latest EndedAt).
	// Only populated when sessions have StartedAt/EndedAt timestamps; zero otherwise.
	TimeInChain float64 `yaml:"time_in_chain" json:"time_in_chain"`
	// ShadowWork is the complement of TimeInChain: the fraction of calendar time
	// outside active measurement windows. Zero when TimeInChain is zero.
	ShadowWork float64 `yaml:"shadow_work" json:"shadow_work"`
	// ForecastAccuracy is the ratio predicted/actual from the most recent
	// ForecastEntry on the milestone. Values close to 1.0 indicate accurate
	// forecasts; >1.0 means overestimated (predicted longer than actual); <1.0
	// means underestimated. Zero when no ForecastHistory is present.
	ForecastAccuracy float64 `yaml:"forecast_accuracy" json:"forecast_accuracy"`
	FeverStatus      Status  `yaml:"fever_status" json:"fever_status"`
	// ReadySetWidth is the average number of simultaneously executable issues
	// at the time of measurement. Populated by the caller from graph state;
	// not computed inside Compute(). Zero when unset.
	ReadySetWidth float64 `yaml:"ready_set_width" json:"ready_set_width"`
	// PathOverlapCount is the number of parallel-eligible issue pairs that
	// share file-level paths, limiting safe concurrency. Populated by the
	// caller from graph state; not computed inside Compute(). Zero when unset.
	PathOverlapCount int `yaml:"path_overlap_count" json:"path_overlap_count"`
	// ParallelEff is the actual vs theoretical completion time ratio
	// (theoreticalWeeks / actualWeeks). Values close to 1.0 indicate good
	// parallelization. Populated by the caller via ComputeParallelEfficiency;
	// not computed inside Compute(). Zero when unset.
	ParallelEff float64 `yaml:"parallel_eff" json:"parallel_eff"`
}

// ComputeForecastAccuracy returns the ratio predicted/actual.
// Values close to 1.0 indicate accurate forecasts. Values > 1.0 mean the
// forecast overestimated (predicted longer than actual). Values < 1.0 mean
// the forecast underestimated. Returns 0 when actual is 0 to avoid division
// by zero.
func ComputeForecastAccuracy(predicted, actual float64) float64 {
	if actual == 0 {
		return 0
	}
	return predicted / actual
}

// ComputeParallelEfficiency returns the ratio theoreticalWeeks/actualWeeks.
// Values close to 1.0 indicate that parallelization is working well (actual
// completion time matches the theoretical minimum). Values below 1.0 indicate
// that actual work took longer than the theoretical parallel minimum. Returns 0
// when actualWeeks is 0 to avoid division by zero.
func ComputeParallelEfficiency(actualWeeks, theoreticalWeeks float64) float64 {
	if actualWeeks == 0 {
		return 0
	}
	return theoreticalWeeks / actualWeeks
}

// Compute derives telemetry indicators from the issues belonging to the given
// milestone. Returns a zero Stats if no milestone issues are found.
// All signals are derived statically from issue and session data — no
// external state is required.
func Compute(issues []*issue.Issue, ms *milestone.Milestone) *Stats {
	s := &Stats{}

	// Filter to issues in this milestone.
	var milestoneIssues []*issue.Issue
	for _, iss := range issues {
		if iss.Milestone == ms.Name {
			milestoneIssues = append(milestoneIssues, iss)
		}
	}
	if len(milestoneIssues) == 0 {
		return s
	}

	// Count done issues and total commits across all done issues.
	var doneCount int
	var totalCommits int
	for _, iss := range milestoneIssues {
		if iss.State == issue.StateDone {
			doneCount++
			for _, sess := range iss.Sessions {
				totalCommits += sess.Commits
			}
		}
	}

	// MPG: total commits on done issues divided by done issue count.
	if doneCount > 0 {
		s.MPG = float64(totalCommits) / float64(doneCount)
	}

	// Speed: done issues per week, measured from the earliest Updated time of
	// done issues (when work started producing results) to the latest Updated
	// time of done issues. Using done-issue timestamps gives a throughput
	// measure independent of when issues were originally created.
	var earliest, latest time.Time
	for _, iss := range milestoneIssues {
		if iss.State == issue.StateDone {
			if earliest.IsZero() || iss.Updated.Before(earliest) {
				earliest = iss.Updated
			}
			if latest.IsZero() || iss.Updated.After(latest) {
				latest = iss.Updated
			}
		}
	}
	if doneCount > 0 && !earliest.IsZero() && !latest.IsZero() {
		weeks := latest.Sub(earliest).Hours() / (24 * 7)
		if weeks > 0 {
			s.Speed = float64(doneCount) / weeks
		}
	}

	// BufferTotal: 50% of issue count.
	totalIssues := len(milestoneIssues)
	s.BufferTotal = float64(totalIssues) * 0.5

	// BufferBurned: for each done issue, the fraction of commits above MPG
	// represents burned buffer.
	if s.MPG > 0 {
		for _, iss := range milestoneIssues {
			if iss.State != issue.StateDone {
				continue
			}
			var issueCommits int
			for _, sess := range iss.Sessions {
				issueCommits += sess.Commits
			}
			if float64(issueCommits) > s.MPG {
				s.BufferBurned += (float64(issueCommits) - s.MPG) / s.MPG
			}
		}
	}

	// TimeInChain: ratio of active session time to total calendar time.
	// Sums durations of all sessions with both StartedAt and EndedAt set,
	// then divides by the span from the earliest StartedAt to the latest EndedAt.
	// Sessions without timestamps are skipped; TimeInChain remains zero if none
	// have timestamps.
	var totalSessionDuration time.Duration
	var earliestStart, latestEnd time.Time

	for _, iss := range milestoneIssues {
		for _, sess := range iss.Sessions {
			if sess.StartedAt == nil || sess.EndedAt == nil {
				continue
			}
			dur := sess.EndedAt.Sub(*sess.StartedAt)
			totalSessionDuration += dur
			if earliestStart.IsZero() || sess.StartedAt.Before(earliestStart) {
				earliestStart = *sess.StartedAt
			}
			if latestEnd.IsZero() || sess.EndedAt.After(latestEnd) {
				latestEnd = *sess.EndedAt
			}
		}
	}

	if !earliestStart.IsZero() && !latestEnd.IsZero() {
		calendarTime := latestEnd.Sub(earliestStart)
		if calendarTime > 0 {
			s.TimeInChain = totalSessionDuration.Seconds() / calendarTime.Seconds()
		}
	}

	if s.TimeInChain > 0 {
		s.ShadowWork = 1.0 - s.TimeInChain
	}

	// Fever chart: compare % buffer burned to % progress.
	// GREEN when buffer burn is less than progress; YELLOW when close;
	// RED when buffer burn substantially exceeds progress.
	progress := 0.0
	if totalIssues > 0 {
		progress = float64(doneCount) / float64(totalIssues) * 100
	}
	bufferPct := 0.0
	if s.BufferTotal > 0 {
		bufferPct = s.BufferBurned / s.BufferTotal * 100
	}

	if bufferPct < progress {
		s.FeverStatus = StatusGreen
	} else if bufferPct <= progress+20 {
		s.FeverStatus = StatusYellow
	} else {
		s.FeverStatus = StatusRed
	}

	// ForecastAccuracy: derived from the most recent ForecastEntry on the
	// milestone (if any). Zero when no history is recorded.
	if len(ms.ForecastHistory) > 0 {
		last := ms.ForecastHistory[len(ms.ForecastHistory)-1]
		s.ForecastAccuracy = ComputeForecastAccuracy(last.PredictedWeeks, last.ActualWeeks)
	}

	return s
}
