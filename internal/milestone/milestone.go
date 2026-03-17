// ABOUTME: Milestone domain model for git-zhi. Defines the Milestone struct
// ABOUTME: with frontmatter+markdown body format and YAML frontmatter serialization.
package milestone

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Milestone represents a delivery grouping of issues with an optional due date.
type Milestone struct {
	Name        string     `yaml:"name" json:"name"`
	Due         *time.Time `yaml:"due,omitempty" json:"due,omitempty"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Resolution  string     `yaml:"resolution,omitempty" json:"resolution,omitempty"`
	Created     time.Time  `yaml:"created" json:"created"`
	// State is the lifecycle state of the milestone. Valid values: "open", "completed".
	// Defaults to "open" for backward compatibility with v0.1 milestones.
	State string `yaml:"state,omitempty" json:"state,omitempty"`
	// Completed is set to the timestamp when the milestone transitions to
	// the "completed" state. Null for milestones that are still open.
	Completed *time.Time `yaml:"completed,omitempty" json:"completed,omitempty"`
	// Body is the raw markdown content below the YAML frontmatter separator.
	// Not stored in YAML; handled separately during Parse and Marshal.
	Body string `yaml:"-" json:"body,omitempty"`
}

// Parse reads a milestone from either frontmatter+body format (v0.2) or pure
// YAML (v0.1 backward compatibility). The format is detected by checking for a
// "---" separator: if a "---" line appears after the initial YAML content, the
// content before it is treated as frontmatter and the rest as the markdown body.
// If no such separator exists, the entire input is treated as pure YAML.
func Parse(raw []byte) (*Milestone, error) {
	text := string(raw)

	// Detect frontmatter+body format: a "---" line must appear as a separator
	// ending the YAML block. We look for a line that is exactly "---" (or "---"
	// with trailing whitespace) that is NOT at the very beginning of the file.
	// In v0.2 format the YAML block is not wrapped in "---" delimiters; instead
	// the content starts with YAML keys and a single "---" line separates the
	// YAML from the markdown body.
	lines := strings.Split(text, "\n")
	separatorIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIdx = i
			break
		}
	}

	var ms Milestone
	if separatorIdx >= 0 {
		// frontmatter+body format: YAML is above the separator, markdown below.
		frontmatterText := strings.Join(lines[:separatorIdx], "\n")
		bodyLines := lines[separatorIdx+1:]
		body := strings.TrimSpace(strings.Join(bodyLines, "\n"))

		if err := yaml.Unmarshal([]byte(frontmatterText), &ms); err != nil {
			return nil, fmt.Errorf("parse milestone frontmatter: %w", err)
		}
		ms.Body = body
	} else {
		// Pure YAML format (v0.1 backward compatibility): no separator found.
		if err := yaml.Unmarshal(raw, &ms); err != nil {
			return nil, fmt.Errorf("parse milestone yaml: %w", err)
		}
	}

	// Default state to "open" for backward compatibility with v0.1 milestones
	// that predate the state field.
	if ms.State == "" {
		ms.State = "open"
	}

	return &ms, nil
}

// MarshalMilestone serializes a Milestone to frontmatter+body format.
// The YAML fields are written first, followed by a "---" separator and
// the markdown body (if any). This is consistent with the issue format.
func MarshalMilestone(ms *Milestone) ([]byte, error) {
	frontmatterBytes, err := yaml.Marshal(ms)
	if err != nil {
		return nil, fmt.Errorf("marshal milestone: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(frontmatterBytes)
	buf.WriteString("---\n")
	if ms.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(ms.Body)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}
