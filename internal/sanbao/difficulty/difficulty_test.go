// ABOUTME: Tests for the composite issue difficulty score computation.
// ABOUTME: Covers simple issues, hard issues, and edge cases like no sessions.
package difficulty_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/difficulty"
)

// t0 is a fixed base time used across tests for reproducibility.
var t0 = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

// makeIssue builds a minimal in-memory issue with the given sessions and transitions.
func makeIssue(sessions []issue.Session, transitions []issue.Transition) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	return &issue.Issue{
		ID:          id,
		Title:       "test issue",
		State:       issue.StateDone,
		Milestone:   "v0.1",
		Created:     t0,
		Updated:     t0,
		Sessions:    sessions,
		Transitions: transitions,
	}
}

func TestComputeDifficulty_SimpleIssue(t *testing.T) {
	// Simple issue: 1 commit, 1-hour cycle, no reopens, positive sentiment, 1 session.
	iss := makeIssue(
		[]issue.Session{
			{
				StartSHA: "aaa",
				EndSHA:   "bbb",
				Commits:  1,
			},
		},
		[]issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
		},
	)
	meanSentiment := 0.8 // positive

	score := difficulty.ComputeDifficulty(iss, meanSentiment)

	// A simple issue should produce a low difficulty score (well below 0.3).
	if score >= 0.3 {
		t.Fatalf("simple issue: expected score < 0.3, got %f", score)
	}
	if score < 0 || score > 1 {
		t.Fatalf("simple issue: score %f outside [0, 1] range", score)
	}
}

func TestComputeDifficulty_HardIssue(t *testing.T) {
	// Hard issue: 30+ commits, 8-day cycle (start to first done), 3 reopens,
	// negative sentiment, 10 sessions.
	//
	// CycleTime uses first in-progress → first done, so the first done
	// transition must be placed at the long cycle endpoint.
	sessions := make([]issue.Session, 10)
	for i := range sessions {
		sessions[i] = issue.Session{
			StartSHA: "aaa",
			EndSHA:   "bbb",
			Commits:  3, // 30 total across 10 sessions
		}
	}

	transitions := []issue.Transition{
		// Start (in-progress) at t0; first done at t0+8days → 8-day cycle (> 7-day cap).
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(8 * 24 * time.Hour)},
		{State: string(issue.StateReopened), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 1*time.Hour)},
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 2*time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 3*time.Hour)},
		{State: string(issue.StateReopened), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 4*time.Hour)},
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 5*time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 6*time.Hour)},
		{State: string(issue.StateReopened), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 7*time.Hour)},
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 8*time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(8*24*time.Hour + 9*time.Hour)},
	}

	iss := makeIssue(sessions, transitions)
	meanSentiment := -0.8 // very negative

	score := difficulty.ComputeDifficulty(iss, meanSentiment)

	// A hard issue should produce a high difficulty score (0.8 or above).
	if score < 0.8 {
		t.Fatalf("hard issue: expected score >= 0.8, got %f", score)
	}
	if score < 0 || score > 1 {
		t.Fatalf("hard issue: score %f outside [0, 1] range", score)
	}
}

func TestComputeDifficulty_NoSessions(t *testing.T) {
	// Issue with no sessions: all zero values except sentiment.
	iss := makeIssue([]issue.Session{}, []issue.Transition{})
	meanSentiment := 0.5 // mildly positive

	score := difficulty.ComputeDifficulty(iss, meanSentiment)

	// Zero sessions, zero commits, zero cycle time, zero reopens → low score.
	if score < 0 || score > 1 {
		t.Fatalf("no sessions: score %f outside [0, 1] range", score)
	}
	if score >= 0.3 {
		t.Fatalf("no sessions: expected score < 0.3 (zero-value issue), got %f", score)
	}
}

func TestComputeDifficulty_HighMPGScoresHigher(t *testing.T) {
	// Issue with high commit count should score higher than one with low commit count,
	// all else being equal (same cycle time, no reopens, neutral sentiment, 1 session).
	oneSession := []issue.Session{{StartSHA: "aaa", EndSHA: "bbb", Commits: 1}}
	manySession := []issue.Session{{StartSHA: "aaa", EndSHA: "bbb", Commits: 30}}

	transitions := []issue.Transition{
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
	}

	lowMPG := difficulty.ComputeDifficulty(makeIssue(oneSession, transitions), 0.0)
	highMPG := difficulty.ComputeDifficulty(makeIssue(manySession, transitions), 0.0)

	if highMPG <= lowMPG {
		t.Fatalf("high-MPG issue should score higher: lowMPG=%f highMPG=%f", lowMPG, highMPG)
	}
}

func TestComputeDifficulty_NegativeSentimentScoresHigher(t *testing.T) {
	// Issue with negative sentiment should score higher than same issue with positive sentiment.
	sessions := []issue.Session{{StartSHA: "aaa", EndSHA: "bbb", Commits: 5}}
	transitions := []issue.Transition{
		{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0.Add(1 * time.Hour)},
		{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(2 * time.Hour)},
	}

	positive := difficulty.ComputeDifficulty(makeIssue(sessions, transitions), 0.9)
	negative := difficulty.ComputeDifficulty(makeIssue(sessions, transitions), -0.9)

	if negative <= positive {
		t.Fatalf("negative sentiment should score higher: positive=%f negative=%f", positive, negative)
	}
}

func TestComputeDifficulty_Clamped(t *testing.T) {
	// Extreme inputs must not produce scores outside [0, 1].
	iss := makeIssue(
		[]issue.Session{{StartSHA: "aaa", EndSHA: "bbb", Commits: 9999}},
		[]issue.Transition{
			{State: string(issue.StateInProgress), Actor: "human:perigrin", Timestamp: t0},
			{State: string(issue.StateDone), Actor: "human:perigrin", Timestamp: t0.Add(365 * 24 * time.Hour)},
		},
	)

	score := difficulty.ComputeDifficulty(iss, -1.0)
	if score < 0 || score > 1 {
		t.Fatalf("extreme inputs: score %f outside [0, 1]", score)
	}
}
