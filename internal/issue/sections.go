// ABOUTME: ParseSections extracts structured sections from an issue's markdown body.
// ABOUTME: Recognizes Prerequisites, Context, and Acceptance Criteria headings; remaining text becomes Description.
package issue

import (
	"regexp"
	"strings"
)

// Checkbox represents a markdown task list item with its text and checked state.
type Checkbox struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// StructuredContext holds the key-value lists extracted from a Context section.
type StructuredContext struct {
	Paths       []string `json:"paths,omitempty"`
	Docs        []string `json:"docs,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Entrypoints []string `json:"entrypoints,omitempty"`
}

// Sections holds all parsed sections from an issue body.
type Sections struct {
	Prerequisites      []Checkbox         `json:"prerequisites,omitempty"`
	Context            *StructuredContext  `json:"context,omitempty"`
	AcceptanceCriteria []Checkbox         `json:"acceptance_criteria,omitempty"`
	Description        string             `json:"description,omitempty"`
}

var checkboxRe = regexp.MustCompile(`^- \[([xX ])\] (.+)$`)

// parseCheckboxes extracts checkbox items from a block of text.
// Lines matching `- [x]`, `- [X]`, or `- [ ]` become Checkbox values.
// Other lines are ignored.
func parseCheckboxes(text string) []Checkbox {
	var result []Checkbox
	for _, line := range strings.Split(text, "\n") {
		m := checkboxRe.FindStringSubmatch(strings.TrimRight(line, " \t"))
		if m == nil {
			continue
		}
		result = append(result, Checkbox{
			Text:    strings.TrimSpace(m[2]),
			Checked: m[1] == "x" || m[1] == "X",
		})
	}
	return result
}

// splitCSV trims and splits a comma-separated string into individual values,
// discarding empty entries.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			result = append(result, v)
		}
	}
	return result
}

// contextKeyRe matches lines like `- paths: value1, value2` in a Context section.
var contextKeyRe = regexp.MustCompile(`^- (paths|docs|commands|entrypoints):\s*(.*)$`)

// parseContext extracts structured key/value lists from a Context section.
// Lines matching known keys (paths, docs, commands, entrypoints) are parsed;
// other lines are collected as description text.
func parseContext(text string) (*StructuredContext, string) {
	ctx := &StructuredContext{}
	var descLines []string
	empty := true

	for _, line := range strings.Split(text, "\n") {
		m := contextKeyRe.FindStringSubmatch(strings.TrimRight(line, " \t"))
		if m == nil {
			descLines = append(descLines, line)
			continue
		}
		empty = false
		vals := splitCSV(m[2])
		switch m[1] {
		case "paths":
			ctx.Paths = append(ctx.Paths, vals...)
		case "docs":
			ctx.Docs = append(ctx.Docs, vals...)
		case "commands":
			ctx.Commands = append(ctx.Commands, vals...)
		case "entrypoints":
			ctx.Entrypoints = append(ctx.Entrypoints, vals...)
		}
	}

	if empty {
		return nil, strings.TrimSpace(strings.Join(descLines, "\n"))
	}
	return ctx, strings.TrimSpace(strings.Join(descLines, "\n"))
}

// ParseSections splits an issue body on `## ` headings, then routes each section
// to the appropriate parser. Unrecognized sections and non-key lines in Context
// are accumulated into Description. Malformed sections produce empty fields, not errors.
func ParseSections(body string) *Sections {
	s := &Sections{}
	if strings.TrimSpace(body) == "" {
		return s
	}

	// Split into sections on lines beginning with "## ".
	// The first segment (before any ## heading) is treated as description text.
	type namedSection struct {
		name string
		text string
	}

	var sections []namedSection
	var currentName string
	var currentLines []string

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			// Flush previous section.
			sections = append(sections, namedSection{
				name: currentName,
				text: strings.TrimSpace(strings.Join(currentLines, "\n")),
			})
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	// Flush final section.
	sections = append(sections, namedSection{
		name: currentName,
		text: strings.TrimSpace(strings.Join(currentLines, "\n")),
	})

	var descParts []string

	for _, sec := range sections {
		switch strings.ToLower(sec.name) {
		case "":
			// Text before any heading.
			if sec.text != "" {
				descParts = append(descParts, sec.text)
			}
		case "prerequisites":
			s.Prerequisites = parseCheckboxes(sec.text)
		case "context":
			ctx, extra := parseContext(sec.text)
			s.Context = ctx
			if extra != "" {
				descParts = append(descParts, extra)
			}
		case "acceptance criteria":
			s.AcceptanceCriteria = parseCheckboxes(sec.text)
		default:
			// Unrecognized section: treat its text as description.
			if sec.text != "" {
				descParts = append(descParts, sec.text)
			}
		}
	}

	s.Description = strings.TrimSpace(strings.Join(descParts, "\n\n"))
	return s
}
