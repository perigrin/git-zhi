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

// Dropped holds an AC checkbox item that contains backtick text but yielded no
// parenthesized verification command — a likely malformed command, such as the
// colon form "desc: `cmd`" instead of the convention "desc (`cmd`)". Surfacing
// these prevents a done issue's acceptance criteria from passing the gate
// without anything actually running.
type Dropped struct {
	// Text is the full AC checkbox item text, so the reader can see what to fix.
	Text string
	// Subsection is "positive" or "negative", matching the AC subsection the item came from.
	Subsection string
	// IssueID is the UUID of the issue this item belongs to.
	IssueID uuid.UUID
	// IssueTitle is the human-readable title of the issue.
	IssueTitle string
}

// parenBacktickRe matches a backtick-delimited command inside parentheses,
// typically the last parenthetical on an AC line. This avoids extracting
// inline code mentions like `.gitignore` that appear outside parentheses.
// The convention is: "description (`verification command`)"
var parenBacktickRe = regexp.MustCompile(`\(` + "`([^`]+)`" + `\)`)

// anyBacktickRe matches any backtick-delimited text. It distinguishes an AC
// item that carries backtick content (a possible malformed command) from one
// that is genuinely prose. Items with backtick text but no parenthesized
// command are reported as Dropped rather than silently skipped.
var anyBacktickRe = regexp.MustCompile("`[^`]+`")

// extractFromItems scans checkbox items and returns a Command for each item
// whose text contains a backtick-delimited string inside parentheses. An item
// that has backtick text but no parenthesized command is returned as Dropped.
// An item with no backtick text at all is genuine prose and is skipped. All
// returned values carry the given subsection tag, issueID, and issueTitle.
func extractFromItems(items []issue.Checkbox, subsection string, issueID uuid.UUID, issueTitle string) ([]Command, []Dropped) {
	var cmds []Command
	var dropped []Dropped
	for _, item := range items {
		m := parenBacktickRe.FindStringSubmatch(item.Text)
		if m != nil {
			cmds = append(cmds, Command{
				Text:       m[1],
				Subsection: subsection,
				IssueID:    issueID,
				IssueTitle: issueTitle,
			})
			continue
		}
		// No parenthesized command. If the item still contains backtick text, it
		// is a malformed command (e.g. the colon form), not prose — report it as
		// dropped so the gate does not pass it silently.
		if anyBacktickRe.MatchString(item.Text) {
			dropped = append(dropped, Dropped{
				Text:       item.Text,
				Subsection: subsection,
				IssueID:    issueID,
				IssueTitle: issueTitle,
			})
		}
	}
	return cmds, dropped
}

// ExtractCommands extracts all backtick-delimited commands from the
// PositiveScenarios and NegativeScenarios fields of sections, alongside any
// Dropped items — AC items that carry backtick text but produced no
// parenthesized command.
//
// When NegativeScenarios is empty (v0.1 flat AC format), PositiveScenarios
// already contains all items and they are all tagged as "positive".
//
// AC items that contain no backtick-delimited text are skipped silently.
func ExtractCommands(sections *issue.Sections, issueID uuid.UUID, issueTitle string) ([]Command, []Dropped) {
	posCmds, posDropped := extractFromItems(sections.PositiveScenarios, "positive", issueID, issueTitle)
	negCmds, negDropped := extractFromItems(sections.NegativeScenarios, "negative", issueID, issueTitle)

	var cmds []Command
	cmds = append(cmds, posCmds...)
	cmds = append(cmds, negCmds...)

	var dropped []Dropped
	dropped = append(dropped, posDropped...)
	dropped = append(dropped, negDropped...)

	return cmds, dropped
}
