// ABOUTME: Telemetry computation for development signals (MPG, speed, buffer).
// ABOUTME: Stateless — derives all indicators from issue and milestone data.
package telemetry

import (
	"time"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/milestone"
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
	TimeInChain  float64 `yaml:"time_in_chain" json:"time_in_chain"`
	FeverStatus  Status  `yaml:"fever_status" json:"fever_status"`
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

	// Speed: done issues per week, using the span from the earliest issue
	// creation time to the latest done-issue updated time.
	var earliest, latest time.Time
	for _, iss := range milestoneIssues {
		if earliest.IsZero() || iss.Created.Before(earliest) {
			earliest = iss.Created
		}
		if iss.State == issue.StateDone && (latest.IsZero() || iss.Updated.After(latest)) {
			latest = iss.Updated
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

	// TimeInChain: fraction of issues done, as a simplified proxy for
	// active-session time vs calendar time.
	if totalIssues > 0 {
		s.TimeInChain = float64(doneCount) / float64(totalIssues)
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

	return s
}
