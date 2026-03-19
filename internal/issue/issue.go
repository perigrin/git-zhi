// ABOUTME: Issue domain model for git-zhi. Defines the Issue struct, state
// ABOUTME: constants, and Session type, and Parse/Marshal/SplitBatch functions.
package issue

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/goccy/go-yaml"
	"github.com/gofrs/uuid/v5"
)

// State represents the lifecycle state of an issue.
type State string

const (
	StatePending    State = "pending"
	StateInProgress State = "in-progress"
	StateDone       State = "done"
	StateCancelled  State = "cancelled"
	StateReopened   State = "reopened"
)

// Urgency represents the scheduling priority of an issue within its milestone.
type Urgency string

const (
	UrgencyHigh   Urgency = "high"
	UrgencyNormal Urgency = "normal"
	UrgencyLow    Urgency = "low"
)

// RefPrefix is the git ref namespace under which all issues are stored.
const RefPrefix = "refs/zhi/_/issues/"

// Session records a measurement window: the commit range and count between
// start/resume and pause/done transitions. StartedAt and EndedAt are optional
// wall-clock timestamps used for time-in-chain computation; older sessions
// without timestamps remain valid.
type Session struct {
	StartSHA  string     `yaml:"start_sha" json:"start_sha"`
	EndSHA    string     `yaml:"end_sha" json:"end_sha"`
	Commits   int        `yaml:"commits" json:"commits"`
	StartedAt *time.Time `yaml:"started_at,omitempty" json:"started_at,omitempty"`
	EndedAt   *time.Time `yaml:"ended_at,omitempty" json:"ended_at,omitempty"`
}

// Transition records a single state-change event on an issue: what state was
// entered, which actor triggered it, and when it occurred. Used for lineage
// tracking, DORA metrics, and per-actor HEAD resolution.
type Transition struct {
	State     string    `yaml:"state" json:"state"`
	Actor     string    `yaml:"actor" json:"actor"`
	Timestamp time.Time `yaml:"timestamp" json:"timestamp"`
}

// Issue represents a node in the chain dependency graph.
type Issue struct {
	// ID is derived from the entity ref path (refs/zhi/_/issues/<uuid>),
	// not stored in YAML frontmatter. Set by the storage layer after reading.
	ID        uuid.UUID   `yaml:"-" json:"id"`
	Title     string      `yaml:"title" json:"title"`
	State     State       `yaml:"state" json:"state"`
	Urgency   Urgency     `yaml:"urgency,omitempty" json:"urgency"`
	Milestone string      `yaml:"milestone" json:"milestone"`
	BlockedBy []uuid.UUID `yaml:"blocked_by,omitempty" json:"blocked_by,omitempty"`
	Blocks    []uuid.UUID `yaml:"blocks,omitempty" json:"blocks,omitempty"`
	Created   time.Time   `yaml:"created" json:"created"`
	Updated   time.Time   `yaml:"updated" json:"updated"`
	Sessions      []Session    `yaml:"sessions,omitempty" json:"sessions,omitempty"`
	Transitions   []Transition `yaml:"transitions,omitempty" json:"transitions,omitempty"`
	// ObservedPaths records the file paths touched during sessions for this
	// issue. Populated by --state done from git diff --name-only. Used by
	// git-zhi-verify to prioritize re-verification and by the parallelizer
	// to detect path overlap between concurrent workers.
	ObservedPaths []string `yaml:"observed_paths,omitempty" json:"observed_paths,omitempty"`
	// Labels is a set of free-form tag strings for categorizing and filtering
	// issues (e.g. "LOPS", "microservices"). Introduced in v0.3.
	Labels []string `yaml:"labels,omitempty" json:"labels,omitempty"`
	// Assigned records the actor identity (e.g. "human:perigrin",
	// "agent:claude-code-1") responsible for working this issue. Empty string
	// means unassigned. Introduced in v0.3.
	Assigned string `yaml:"assigned,omitempty" json:"assigned,omitempty"`
	// Confidence records the historian's mapping reliability for this issue
	// on a 0.0-1.0 scale. Planned issues leave this at zero. Introduced in v0.3.
	Confidence float64 `yaml:"confidence,omitempty" json:"confidence,omitempty"`
	// Source records how the issue was constructed: tracker-match, ticket-ref,
	// cluster, single-commit, manual, or planned. Introduced in v0.3.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	// TrackerID records an external tracker reference (e.g. "jira:LOPS-142").
	// Introduced in v0.3.
	TrackerID string `yaml:"tracker_id,omitempty" json:"tracker_id,omitempty"`
	// LastSyncedAt records the timestamp of the last sync cycle for this issue.
	// Used by sync plugins for conflict resolution. Introduced in v0.3.
	LastSyncedAt *time.Time `yaml:"last_synced_at,omitempty" json:"last_synced_at,omitempty"`
	// Body is the raw markdown below the YAML frontmatter separator.
	// Handled separately from YAML marshaling. Included in JSON output
	// so --format json consumers get the full issue content.
	Body string `yaml:"-" json:"body,omitempty"`
	// After is an input-only field parsed from YAML frontmatter during
	// batch-add. It references another issue (by exact title or UUID ref)
	// that this issue depends on. Consumed during batch-add dependency
	// resolution, then cleared before persistence.
	After string `yaml:"after,omitempty" json:"-"`
	// Before is an input-only field parsed from YAML frontmatter during
	// batch-add. It references another issue that depends on this issue.
	// Same resolution rules as After. Cleared before persistence.
	Before string `yaml:"before,omitempty" json:"-"`
}

// Parse splits raw markdown into YAML frontmatter and body, then
// unmarshals the frontmatter into an Issue. The ID field is not in
// the frontmatter — the caller sets it from the ref path.
func Parse(raw []byte) (*Issue, error) {
	var iss Issue
	rest, err := frontmatter.Parse(bytes.NewReader(raw), &iss, frontmatter.NewFormat("---", "---", yaml.Unmarshal))
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	// frontmatter.Parse returns the body as []byte
	iss.Body = strings.TrimSpace(string(rest))
	// Default urgency to normal for backward compatibility with v0.1 issues
	// that predate the urgency field.
	if iss.Urgency == "" {
		iss.Urgency = UrgencyNormal
	}
	// Ensure Transitions is never nil for backward compatibility with v0.1
	// issues that predate this field. Callers can always append safely.
	if iss.Transitions == nil {
		iss.Transitions = []Transition{}
	}
	// Ensure ObservedPaths is never nil for backward compatibility with issues
	// that predate this field. Callers can always append safely.
	if iss.ObservedPaths == nil {
		iss.ObservedPaths = []string{}
	}
	// Ensure Labels is never nil for backward compatibility with v0.1/v0.2
	// issues that predate this field. Callers can always append safely.
	if iss.Labels == nil {
		iss.Labels = []string{}
	}
	return &iss, nil
}

// Marshal serializes an Issue back to YAML frontmatter + markdown body.
// Input-only fields (After, Before) are cleared before serialization
// to prevent them from being persisted to storage.
func Marshal(iss *Issue) ([]byte, error) {
	// Clear input-only fields before serialization.
	iss.After = ""
	iss.Before = ""
	frontmatterBytes, err := yaml.Marshal(iss)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(frontmatterBytes)
	buf.WriteString("---\n")
	if iss.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(iss.Body)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

// SplitBatch splits a multi-issue input (separated by ---) into
// individual issue blocks. Each block includes its own frontmatter.
// A --- in the body area starts a new issue only if the next non-empty
// line starts with "title:" (the one mandatory frontmatter field).
// Note: frontmatterSeen is not reset between blocks. This works because
// the closing --- of each new block hits the "else if inFrontmatter"
// branch, which correctly toggles inFrontmatter off.
func SplitBatch(raw []byte) [][]byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	text := string(raw)
	var blocks [][]byte
	lines := strings.Split(text, "\n")
	var current []string
	inFrontmatter := false
	frontmatterSeen := false

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" {
			if !frontmatterSeen {
				inFrontmatter = true
				frontmatterSeen = true
				current = append(current, lines[i])
			} else if inFrontmatter {
				inFrontmatter = false
				current = append(current, lines[i])
			} else {
				isNewIssue := false
				for j := i + 1; j < len(lines); j++ {
					nextTrimmed := strings.TrimSpace(lines[j])
					if nextTrimmed == "" {
						continue
					}
					if looksLikeIssueStart(nextTrimmed) {
						isNewIssue = true
					}
					break
				}
				if isNewIssue {
					blocks = append(blocks, []byte(strings.Join(current, "\n")))
					current = []string{lines[i]}
					inFrontmatter = true
				} else {
					current = append(current, lines[i])
				}
			}
		} else {
			current = append(current, lines[i])
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, []byte(strings.Join(current, "\n")))
	}
	return blocks
}

// looksLikeIssueStart returns true if the line starts with "title:",
// which is the one mandatory field in every issue's frontmatter.
// This is the most conservative heuristic for detecting a new issue
// boundary in batch input — it eliminates false positives from body
// content containing --- followed by prose with colons (e.g.,
// "status: active", "note: see above", URLs).
func looksLikeIssueStart(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "title:")
}
