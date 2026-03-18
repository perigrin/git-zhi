// ABOUTME: Pure Mermaid chart rendering from IssueInput data.
// ABOUTME: Provides RenderGantt and RenderDAG — no git or ref access, JSON in, text out.
package mermaid

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// IssueInput is the data shape parsed from JSON produced by git zhi list or
// git zhi project show. All fields mirror the JSON output of those commands.
type IssueInput struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Assigned  string    `json:"assigned,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	BlockedBy []string  `json:"blocked_by,omitempty"`
}

// stateMarker maps an issue state to the Mermaid Gantt task marker prefix.
// done → ":done,", in-progress → ":active,", other states → no prefix.
func stateMarker(state string) string {
	switch state {
	case "done":
		return ":done, "
	case "in-progress":
		return ":active, "
	default:
		return ""
	}
}

// statusIcon maps an issue state to its display icon for DAG nodes.
func statusIcon(state string) string {
	switch state {
	case "done":
		return "✓"
	case "in-progress":
		return "🔵"
	case "cancelled":
		return "✗"
	default:
		return "◻️"
	}
}

// escapeMermaid sanitizes a string for safe embedding in Mermaid labels and
// task descriptions. Replaces characters that would break Mermaid syntax.
func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, "\"", "#quot;")
	s = strings.ReplaceAll(s, "<", "#lt;")
	s = strings.ReplaceAll(s, ">", "#gt;")
	return s
}

// nodeID returns the first 8 characters of an issue ID for use as a Mermaid
// node identifier.
func nodeID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// RenderGantt produces a Mermaid Gantt chart from the given issues.
//
// Issues are grouped into sections by their Assigned field. Issues with no
// assignment appear in an "Unassigned" section. Within each section, issues
// are ordered by Created timestamp. The title parameter overrides the chart
// title; if empty, "Chain" is used.
func RenderGantt(issues []IssueInput, title string) string {
	if title == "" {
		title = "Chain"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "gantt\n")
	fmt.Fprintf(&b, "    title %s\n", title)
	fmt.Fprintf(&b, "    dateFormat YYYY-MM-DD\n")

	if len(issues) == 0 {
		return b.String()
	}

	// Group issues by assigned worker, preserving insertion order of first
	// occurrence per section so output is deterministic.
	type sectionEntry struct {
		name   string
		issues []IssueInput
	}

	sectionMap := make(map[string]*sectionEntry)
	var sectionOrder []string

	for _, iss := range issues {
		name := iss.Assigned
		if name == "" {
			name = "Unassigned"
		}
		if _, exists := sectionMap[name]; !exists {
			sectionMap[name] = &sectionEntry{name: name}
			sectionOrder = append(sectionOrder, name)
		}
		sectionMap[name].issues = append(sectionMap[name].issues, iss)
	}

	// Sort issues within each section by Created timestamp.
	for _, name := range sectionOrder {
		sec := sectionMap[name]
		sort.Slice(sec.issues, func(i, j int) bool {
			return sec.issues[i].Created.Before(sec.issues[j].Created)
		})

		fmt.Fprintf(&b, "    section %s\n", name)
		for _, iss := range sec.issues {
			start := iss.Created.Format("2006-01-02")
			// Ensure a minimum 1-day duration so Mermaid renders a task bar
			// rather than a zero-width milestone marker.
			endTime := iss.Updated
			if !endTime.After(iss.Created) {
				endTime = iss.Created.AddDate(0, 0, 1)
			}
			end := endTime.Format("2006-01-02")
			marker := stateMarker(iss.State)
			fmt.Fprintf(&b, "    %s         :%s%s, %s\n", escapeMermaid(iss.Title), marker, start, end)
		}
	}

	return b.String()
}

// RenderDAG produces a Mermaid graph TD (top-down DAG) from the given issues.
//
// Each issue becomes a node with label "<short-id> <title> <icon>". Edges are
// generated from BlockedBy relationships: for each issue B that is blocked by
// A, an edge A --> B is emitted. Node IDs use the first 8 characters of the
// issue UUID.
func RenderDAG(issues []IssueInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "graph TD\n")

	if len(issues) == 0 {
		return b.String()
	}

	// Emit node definitions. Sort by ID for deterministic output.
	sorted := make([]IssueInput, len(issues))
	copy(sorted, issues)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	for _, iss := range sorted {
		nid := nodeID(iss.ID)
		icon := statusIcon(iss.State)
		fmt.Fprintf(&b, "    %s[\"%s %s %s\"]\n", nid, nid, escapeMermaid(iss.Title), icon)
	}

	// Emit edges from blocked_by relationships. For each blocked issue, emit
	// one edge per blocker: blocker --> blocked.
	for _, iss := range sorted {
		if len(iss.BlockedBy) == 0 {
			continue
		}
		blockedNID := nodeID(iss.ID)
		for _, blockerID := range iss.BlockedBy {
			blockerNID := nodeID(blockerID)
			fmt.Fprintf(&b, "    %s --> %s\n", blockerNID, blockedNID)
		}
	}

	return b.String()
}
