// ABOUTME: State machine validation for issue lifecycle transitions.
// ABOUTME: Enforces which --state transitions are valid from each current state.
package issue

import (
	"fmt"
	"strings"
)

// transition describes a valid action: the required current state and the resulting state.
type transition struct {
	required State
	next     State
}

// transitions maps action strings to their valid transitions.
// pause and resume keep the state as in-progress; they only affect measurement sessions.
// reopen moves a done issue back into the reopened holding state before work resumes.
var transitions = map[string][]transition{
	"start": {
		{required: StatePending, next: StateInProgress},
		{required: StateReopened, next: StateInProgress},
	},
	"pause":  {{required: StateInProgress, next: StateInProgress}},
	"resume": {{required: StateInProgress, next: StateInProgress}},
	"done":   {{required: StateInProgress, next: StateDone}},
	"cancel": {
		{required: StatePending, next: StateCancelled},
		{required: StateInProgress, next: StateCancelled},
	},
	"reopen": {{required: StateDone, next: StateReopened}},
}

// ValidateTransition checks whether the given action is valid for the current
// state and returns the resulting state. Returns an error if the action is
// unknown or the current state does not satisfy the action's precondition.
func ValidateTransition(current State, action string) (State, error) {
	valid, ok := transitions[action]
	if !ok {
		return "", fmt.Errorf("unknown action %q", action)
	}

	for _, t := range valid {
		if current == t.required {
			return t.next, nil
		}
	}

	var expected []string
	for _, t := range valid {
		expected = append(expected, string(t.required))
	}
	return "", fmt.Errorf("cannot %s: issue is %s, expected %s", action, current, strings.Join(expected, " or "))
}
