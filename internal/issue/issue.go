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
	StartSHA string `yaml:"start_sha" json:"start_sha"`
	EndSHA   string `yaml:"end_sha" json:"end_sha"`
	Commits  int    `yaml:"commits" json:"commits"`
}

// Issue represents a node in the chain dependency graph.
type Issue struct {
	// ID is derived from the entity ref path (refs/chain/_/issues/<uuid>),
	// not stored in YAML frontmatter. Set by the storage layer after reading.
	ID        uuid.UUID   `yaml:"-" json:"id"`
	Title     string      `yaml:"title" json:"title"`
	State     State       `yaml:"state" json:"state"`
	Milestone string      `yaml:"milestone" json:"milestone"`
	BlockedBy []uuid.UUID `yaml:"blocked_by,omitempty" json:"blocked_by,omitempty"`
	Blocks    []uuid.UUID `yaml:"blocks,omitempty" json:"blocks,omitempty"`
	Created   time.Time   `yaml:"created" json:"created"`
	Updated   time.Time   `yaml:"updated" json:"updated"`
	Sessions  []Session   `yaml:"sessions,omitempty" json:"sessions,omitempty"`
	// Body is the raw markdown below the YAML frontmatter separator.
	// Handled separately from YAML marshaling. Included in JSON output
	// so --format json consumers get the full issue content.
	Body string `yaml:"-" json:"body,omitempty"`
}
