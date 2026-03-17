// ABOUTME: ParseSections extracts structured sections from an issue's markdown body.
// ABOUTME: Recognizes Prerequisites, Steps, Context, and Acceptance Criteria (with optional Positive/Negative subsections) headings; remaining text becomes Description.
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
	Prerequisites      []Checkbox        `json:"prerequisites,omitempty"`
	Context            *StructuredContext `json:"context,omitempty"`
	Steps              []string          `json:"steps,omitempty"`
	AcceptanceCriteria []Checkbox        `json:"acceptance_criteria,omitempty"`
	PositiveScenarios  []Checkbox        `json:"positive_scenarios,omitempty"`
	NegativeScenarios  []Checkbox        `json:"negative_scenarios,omitempty"`
	Description        string            `json:"description,omitempty"`
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

// parseSteps extracts the text of checkbox items from a Steps section.
// Both checked (`- [x]`) and unchecked (`- [ ]`) items are included; only
// their text is returned (the checked state is not tracked for steps).
func parseSteps(text string) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		m := checkboxRe.FindStringSubmatch(strings.TrimRight(line, " \t"))
		if m == nil {
			continue
		}
		result = append(result, strings.TrimSpace(m[2]))
	}
	return result
}

// parseACWithSubsections parses an Acceptance Criteria section that may contain
// ### Positive Scenarios and ### Negative Scenarios subsections. When no
// subsections are present (v0.1 flat format), all items go into PositiveScenarios.
// AcceptanceCriteria is always the union of positive and negative for backward
// compatibility with callers that only read that field.
func parseACWithSubsections(text string) (all, positive, negative []Checkbox) {
	// Check for ### subsection headings.
	hasSubsections := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "### ") {
			hasSubsections = true
			break
		}
	}

	if !hasSubsections {
		// Flat list: all items are positive scenarios.
		positive = parseCheckboxes(text)
		return positive, positive, nil
	}

	// Split on ### headings within the AC block.
	type subSection struct {
		name string
		text string
	}
	var subs []subSection
	var curName string
	var curLines []string

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "### ") {
			// Flush previous subsection.
			subs = append(subs, subSection{
				name: curName,
				text: strings.TrimSpace(strings.Join(curLines, "\n")),
			})
			curName = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			curLines = nil
		} else {
			curLines = append(curLines, line)
		}
	}
	// Flush final subsection.
	subs = append(subs, subSection{
		name: curName,
		text: strings.TrimSpace(strings.Join(curLines, "\n")),
	})

	for _, sub := range subs {
		switch strings.ToLower(sub.name) {
		case "positive scenarios":
			positive = append(positive, parseCheckboxes(sub.text)...)
		case "negative scenarios":
			negative = append(negative, parseCheckboxes(sub.text)...)
		}
	}

	// Union for backward compat.
	all = append(all, positive...)
	all = append(all, negative...)
	return all, positive, negative
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
		case "steps":
			s.Steps = parseSteps(sec.text)
		case "acceptance criteria":
			s.AcceptanceCriteria, s.PositiveScenarios, s.NegativeScenarios = parseACWithSubsections(sec.text)
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
