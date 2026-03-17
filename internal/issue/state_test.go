// ABOUTME: Tests for state machine validation: valid and invalid issue lifecycle transitions.
// ABOUTME: Covers all action strings (start, pause, resume, done, cancel, reopen) and error cases.
package issue_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

func TestValidateTransition_Start(t *testing.T) {
	next, err := issue.ValidateTransition(issue.StatePending, "start")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if next != issue.StateInProgress {
		t.Fatalf("expected %q, got %q", issue.StateInProgress, next)
	}
}

func TestValidateTransition_Pause(t *testing.T) {
	next, err := issue.ValidateTransition(issue.StateInProgress, "pause")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if next != issue.StateInProgress {
		t.Fatalf("expected %q, got %q", issue.StateInProgress, next)
	}
}

func TestValidateTransition_Resume(t *testing.T) {
	next, err := issue.ValidateTransition(issue.StateInProgress, "resume")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if next != issue.StateInProgress {
		t.Fatalf("expected %q, got %q", issue.StateInProgress, next)
	}
}

func TestValidateTransition_Done(t *testing.T) {
	next, err := issue.ValidateTransition(issue.StateInProgress, "done")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if next != issue.StateDone {
		t.Fatalf("expected %q, got %q", issue.StateDone, next)
	}
}

func TestValidateTransition_Cancel(t *testing.T) {
	for _, current := range []issue.State{issue.StatePending, issue.StateInProgress} {
		next, err := issue.ValidateTransition(current, "cancel")
		if err != nil {
			t.Fatalf("cancel from %q: expected no error, got: %v", current, err)
		}
		if next != issue.StateCancelled {
			t.Fatalf("cancel from %q: expected %q, got %q", current, issue.StateCancelled, next)
		}
	}
}

func TestValidateTransition_Invalid(t *testing.T) {
	cases := []struct {
		current issue.State
		action  string
	}{
		{issue.StateInProgress, "start"},
		{issue.StatePending, "done"},
		{issue.StateDone, "start"},
		{issue.StateDone, "cancel"},
		{issue.StateCancelled, "resume"},
	}

	for _, tc := range cases {
		_, err := issue.ValidateTransition(tc.current, tc.action)
		if err == nil {
			t.Fatalf("expected error for action %q from state %q, got nil", tc.action, tc.current)
		}
	}
}

// TestReopenedTransition covers the full reopened state lifecycle:
// done→reopened via "reopen", reopened→in-progress via "start",
// and rejected transitions from reopened (cancel, done directly).
func TestReopenedTransition(t *testing.T) {
	t.Run("done to reopened via reopen", func(t *testing.T) {
		next, err := issue.ValidateTransition(issue.StateDone, "reopen")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if next != issue.StateReopened {
			t.Fatalf("expected %q, got %q", issue.StateReopened, next)
		}
	})

	t.Run("reopened to in-progress via start", func(t *testing.T) {
		next, err := issue.ValidateTransition(issue.StateReopened, "start")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if next != issue.StateInProgress {
			t.Fatalf("expected %q, got %q", issue.StateInProgress, next)
		}
	})

	t.Run("invalid transitions from reopened", func(t *testing.T) {
		cases := []struct {
			action string
		}{
			{"cancel"},
			{"done"},
			{"pause"},
			{"resume"},
		}
		for _, tc := range cases {
			_, err := issue.ValidateTransition(issue.StateReopened, tc.action)
			if err == nil {
				t.Fatalf("expected error for action %q from state %q, got nil", tc.action, issue.StateReopened)
			}
		}
	})
}
