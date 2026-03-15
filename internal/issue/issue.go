// ABOUTME: Issue domain model for git-chain. Defines the Issue struct, state
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
)

// RefPrefix is the git ref namespace under which all issues are stored.
const RefPrefix = "refs/chain/_/issues/"

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
	return &iss, nil
}

// Marshal serializes an Issue back to YAML frontmatter + markdown body.
func Marshal(iss *Issue) ([]byte, error) {
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
