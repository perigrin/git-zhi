// ABOUTME: ExtractCommands pulls backtick-delimited shell commands from AC items in Sections.
// ABOUTME: Each extracted Command carries its text, subsection tag, and the parent issue identity.
package verify

import (
	"regexp"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
)

// Command holds a single shell command extracted from an acceptance criteria item.
type Command struct {
	// Text is the raw command string found between backticks.
	Text string
	// Subsection is "positive" or "negative", matching the AC subsection the item came from.
	Subsection string
	// IssueID is the UUID of the issue this command belongs to.
	IssueID uuid.UUID
	// IssueTitle is the human-readable title of the issue.
	IssueTitle string
}

// backtickRe matches the first backtick-delimited command in a string.
var backtickRe = regexp.MustCompile("`([^`]+)`")

// extractFromItems scans checkbox items and returns a Command for each item
// whose text contains at least one backtick-delimited string. Items without
// backtick commands are silently skipped. All returned commands carry the
// given subsection tag, issueID, and issueTitle.
func extractFromItems(items []issue.Checkbox, subsection string, issueID uuid.UUID, issueTitle string) []Command {
	var cmds []Command
	for _, item := range items {
		m := backtickRe.FindStringSubmatch(item.Text)
		if m == nil {
			continue
		}
		cmds = append(cmds, Command{
			Text:       m[1],
			Subsection: subsection,
			IssueID:    issueID,
			IssueTitle: issueTitle,
		})
	}
	return cmds
}

// ExtractCommands extracts all backtick-delimited commands from the
// PositiveScenarios and NegativeScenarios fields of sections.
//
// When NegativeScenarios is empty (v0.1 flat AC format), PositiveScenarios
// already contains all items and they are all tagged as "positive".
//
// AC items that contain no backtick-delimited text are skipped silently.
func ExtractCommands(sections *issue.Sections, issueID uuid.UUID, issueTitle string) []Command {
	var cmds []Command
	cmds = append(cmds, extractFromItems(sections.PositiveScenarios, "positive", issueID, issueTitle)...)
	cmds = append(cmds, extractFromItems(sections.NegativeScenarios, "negative", issueID, issueTitle)...)
	return cmds
}
