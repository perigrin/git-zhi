// ABOUTME: Implementation of the issue show command: resolves a ref, loads the issue,
// ABOUTME: and renders it in human-readable or JSON format with parsed sections.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/lineage"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/resolve"
)

// LineageEntryJSON is the JSON presentation of a single upstream lineage entry,
// recording which upstream issue authored lines in a context file.
type LineageEntryJSON struct {
	IssueID   uuid.UUID `json:"issue_id"`
	Title     string    `json:"title"`
	File      string    `json:"file"`
	LineCount int       `json:"line_count"`
}

// IssueJSON is the presentation struct for JSON output of a single issue.
// It combines all Issue fields with parsed sections from the body.
type IssueJSON struct {
	ID                 uuid.UUID               `json:"id"`
	Title              string                  `json:"title"`
	State              issue.State             `json:"state"`
	Urgency            issue.Urgency           `json:"urgency,omitempty"`
	Milestone          string                  `json:"milestone"`
	BlockedBy          []uuid.UUID             `json:"blocked_by,omitempty"`
	Blocks             []uuid.UUID             `json:"blocks,omitempty"`
	Created            time.Time               `json:"created"`
	Updated            time.Time               `json:"updated"`
	Sessions           []issue.Session         `json:"sessions,omitempty"`
	Transitions        []issue.Transition      `json:"transitions,omitempty"`
	ObservedPaths      []string                `json:"observed_paths,omitempty"`
	Labels             []string                `json:"labels,omitempty"`
	Assigned           string                  `json:"assigned,omitempty"`
	Confidence         float64                 `json:"confidence,omitempty"`
	Source             string                  `json:"source,omitempty"`
	TrackerID          string                  `json:"tracker_id,omitempty"`
	LastSyncedAt       *time.Time              `json:"last_synced_at,omitempty"`
	Prerequisites      []issue.Checkbox        `json:"prerequisites,omitempty"`
	Context            *issue.StructuredContext `json:"context,omitempty"`
	Steps              []string                `json:"steps,omitempty"`
	AcceptanceCriteria []issue.Checkbox        `json:"acceptance_criteria,omitempty"`
	PositiveScenarios  []issue.Checkbox        `json:"positive_scenarios,omitempty"`
	NegativeScenarios  []issue.Checkbox        `json:"negative_scenarios,omitempty"`
	Description        string                  `json:"description,omitempty"`
	Body               string                  `json:"body,omitempty"`
	// MilestoneContext is the markdown body of the parent milestone, providing
	// agents with the delivery context for this issue. Empty when no milestone
	// is assigned or when the milestone cannot be loaded.
	MilestoneContext string `json:"milestone_context,omitempty"`
	// Lineage lists upstream issues that authored lines in the context paths
	// of this issue. Populated from git blame + session SHA ranges. Empty when
	// there are no context paths or no blame matches.
	Lineage []LineageEntryJSON `json:"lineage,omitempty"`
}

// runIssueShow resolves the ref argument (or HEAD if absent), loads the issue,
// extracts the UUID from the ref path, and dispatches to the appropriate formatter.
func runIssueShow(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	input := ""
	if len(args) > 0 {
		input = args[0]
	}

	refPath, err := resolve.ResolveRef(app.Store, input)
	if err != nil {
		return fmt.Errorf("resolve ref: %w", err)
	}

	raw, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return fmt.Errorf("read issue: %w", err)
	}

	iss, err := issue.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse issue: %w", err)
	}

	// Extract UUID from the ref path — it is not stored in the frontmatter.
	uuidStr := strings.TrimPrefix(refPath, issue.RefPrefix)
	id, err := uuid.FromString(uuidStr)
	if err != nil {
		return fmt.Errorf("extract uuid from ref path %q: %w", refPath, err)
	}
	iss.ID = id

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		return showJSON(cmd, iss)
	}
	return showHuman(cmd, app, iss)
}

// showHuman prints a human-readable view of an issue including title, state,
// milestone, created date, dependency titles, raw body, and upstream lineage.
func showHuman(cmd *cobra.Command, app *App, iss *issue.Issue) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "%s  %s\n", iss.ID.String()[:8], iss.Title)
	fmt.Fprintf(w, "State:     %s\n", iss.State)
	if iss.Milestone != "" {
		fmt.Fprintf(w, "Milestone: %s\n", iss.Milestone)
	}
	fmt.Fprintf(w, "Created:   %s\n", iss.Created.Format("2006-01-02"))

	if len(iss.BlockedBy) > 0 {
		fmt.Fprintf(w, "Blocked by:\n")
		for _, depID := range iss.BlockedBy {
			title := resolveIssueTitle(app, depID)
			fmt.Fprintf(w, "  %s  %s\n", depID.String()[:8], title)
		}
	}

	if len(iss.Blocks) > 0 {
		fmt.Fprintf(w, "Blocks:\n")
		for _, depID := range iss.Blocks {
			title := resolveIssueTitle(app, depID)
			fmt.Fprintf(w, "  %s  %s\n", depID.String()[:8], title)
		}
	}

	if iss.Body != "" {
		fmt.Fprintf(w, "\n%s\n", iss.Body)
	}

	// Compute and display upstream lineage when context paths are declared.
	linEntries := computeLineageEntries(app, iss)
	if len(linEntries) > 0 {
		fmt.Fprintf(w, "\nUpstream lineage (code you'll be modifying):\n")
		for _, e := range linEntries {
			fmt.Fprintf(w, "  %s  %-28s → %d lines in %s\n",
				e.IssueID.String()[:8], e.Title, e.LineCount, e.File)
		}
	}

	return nil
}

// showJSON encodes the issue and its parsed sections as indented JSON.
// The parent milestone's body is embedded in the milestone_context field so
// that agents have delivery context without a separate round-trip. Upstream
// lineage is computed from context paths and embedded for agent consumption.
func showJSON(cmd *cobra.Command, iss *issue.Issue) error {
	app := GetApp(cmd.Context())
	sections := issue.ParseSections(iss.Body)

	var milestoneContext string
	if iss.Milestone != "" {
		if app != nil {
			ms, err := milestone.LoadMilestone(app.Store, iss.Milestone)
			if err == nil {
				milestoneContext = ms.Body
			}
			// err != nil means milestone is missing or unreadable; leave empty.
		}
	}

	linEntries := computeLineageEntries(app, iss)

	out := IssueJSON{
		ID:                 iss.ID,
		Title:              iss.Title,
		State:              iss.State,
		Urgency:            iss.Urgency,
		Milestone:          iss.Milestone,
		BlockedBy:          iss.BlockedBy,
		Blocks:             iss.Blocks,
		Created:            iss.Created,
		Updated:            iss.Updated,
		Sessions:           iss.Sessions,
		Transitions:        iss.Transitions,
		ObservedPaths:      iss.ObservedPaths,
		Labels:             iss.Labels,
		Assigned:           iss.Assigned,
		Confidence:         iss.Confidence,
		Source:             iss.Source,
		TrackerID:          iss.TrackerID,
		LastSyncedAt:       iss.LastSyncedAt,
		Prerequisites:      sections.Prerequisites,
		Context:            sections.Context,
		Steps:              sections.Steps,
		AcceptanceCriteria: sections.AcceptanceCriteria,
		PositiveScenarios:  sections.PositiveScenarios,
		NegativeScenarios:  sections.NegativeScenarios,
		Description:        sections.Description,
		Body:               iss.Body,
		MilestoneContext:   milestoneContext,
		Lineage:            linEntries,
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// resolveIssueTitle loads an issue by UUID and returns its title.
// Returns "(unknown)" if the issue cannot be read or parsed.
func resolveIssueTitle(app *App, id uuid.UUID) string {
	refPath := issue.RefPrefix + id.String()
	raw, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return "(unknown)"
	}
	iss, err := issue.Parse(raw)
	if err != nil {
		return "(unknown)"
	}
	return iss.Title
}

// computeLineageEntries parses the issue body for context paths, loads all done
// issues, builds a session SHA index, and runs git blame to find upstream
// lineage. Returns nil when there are no context paths or when lineage
// computation is not possible (e.g., no git repo in app). Errors during
// computation are silently swallowed — lineage is advisory.
func computeLineageEntries(app *App, iss *issue.Issue) []LineageEntryJSON {
	if app == nil || app.Repo == nil {
		return nil
	}

	sections := issue.ParseSections(iss.Body)
	if sections.Context == nil || len(sections.Context.Paths) == 0 {
		return nil
	}
	paths := sections.Context.Paths

	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return nil
	}

	// Build issue map for title lookup, filtering out the current issue to
	// avoid self-references in lineage output.
	issueMap := make(map[uuid.UUID]*issue.Issue, len(allIssues))
	for _, candidate := range allIssues {
		if candidate.ID == iss.ID {
			continue
		}
		issueMap[candidate.ID] = candidate
	}

	// Build session index from done issues (excluding self).
	doneIssues := make([]*issue.Issue, 0, len(allIssues))
	for _, candidate := range allIssues {
		if candidate.ID != iss.ID && candidate.State == issue.StateDone {
			doneIssues = append(doneIssues, candidate)
		}
	}

	sessionIndex := lineage.BuildSessionIndex(app.Repo, doneIssues)
	if len(sessionIndex) == 0 {
		return nil
	}

	entries := lineage.ComputeLineage(app.Repo, paths, sessionIndex, issueMap)
	if len(entries) == 0 {
		return nil
	}

	result := make([]LineageEntryJSON, 0, len(entries))
	for _, e := range entries {
		result = append(result, LineageEntryJSON{
			IssueID:   e.UpstreamIssueID,
			Title:     e.Title,
			File:      e.FilePath,
			LineCount: e.LineCount,
		})
	}
	return result
}
