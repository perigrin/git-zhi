// ABOUTME: Issue domain model for git-chain. Defines the Issue struct, state
// ABOUTME: constants, and Session type. Parsing and marshaling added in issue 2.
package issue

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// State represents the lifecycle state of an issue.
type State string

const (
	StatePending    State = "pending"
	StateInProgress State = "in-progress"
	StateDone       State = "done"
	StateCancelled  State = "cancelled"
)

// Session records a measurement window: the commit range and count between
// start/resume and pause/done transitions.
type Session struct {
	StartSHA string `yaml:"start_sha"`
	EndSHA   string `yaml:"end_sha"`
	Commits  int    `yaml:"commits"`
}

// Issue represents a node in the chain dependency graph.
type Issue struct {
	ID        uuid.UUID   `yaml:"-"`
	Title     string      `yaml:"title"`
	State     State       `yaml:"state"`
	Milestone string      `yaml:"milestone"`
	BlockedBy []uuid.UUID `yaml:"blocked_by,omitempty"`
	Blocks    []uuid.UUID `yaml:"blocks,omitempty"`
	Created   time.Time   `yaml:"created"`
	Updated   time.Time   `yaml:"updated"`
	Sessions  []Session   `yaml:"sessions,omitempty"`
	Body      string      `yaml:"-"`
}
