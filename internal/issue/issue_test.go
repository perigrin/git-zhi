// ABOUTME: Tests for the Issue domain model: struct construction with UUIDv7
// ABOUTME: and verification of all four state constant string values.
package issue_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestNewIssue(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("failed to generate UUIDv7: %v", err)
	}

	iss := &issue.Issue{
		ID:        id,
		Title:     "Test issue",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   time.Now(),
		Updated:   time.Now(),
	}

	if iss.State != issue.StatePending {
		t.Fatalf("expected state %q, got %q", issue.StatePending, iss.State)
	}
}

func TestStateConstants(t *testing.T) {
	states := []issue.State{
		issue.StatePending,
		issue.StateInProgress,
		issue.StateDone,
		issue.StateCancelled,
	}
	expected := []string{"pending", "in-progress", "done", "cancelled"}

	for i, s := range states {
		if string(s) != expected[i] {
			t.Errorf("state %d: expected %q, got %q", i, expected[i], string(s))
		}
	}
}
