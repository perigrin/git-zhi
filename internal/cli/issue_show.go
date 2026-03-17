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
	"github.com/perigrin/git-zhi/internal/resolve"
)

// IssueJSON is the presentation struct for JSON output of a single issue.
// It combines all Issue fields with parsed sections from the body.
type IssueJSON struct {
	ID                 uuid.UUID               `json:"id"`
	Title              string                  `json:"title"`
	State              issue.State             `json:"state"`
	Milestone          string                  `json:"milestone"`
	BlockedBy          []uuid.UUID             `json:"blocked_by,omitempty"`
	Blocks             []uuid.UUID             `json:"blocks,omitempty"`
	Created            time.Time               `json:"created"`
	Updated            time.Time               `json:"updated"`
	Sessions           []issue.Session         `json:"sessions,omitempty"`
	Prerequisites      []issue.Checkbox        `json:"prerequisites,omitempty"`
	Context            *issue.StructuredContext `json:"context,omitempty"`
	AcceptanceCriteria []issue.Checkbox        `json:"acceptance_criteria,omitempty"`
	PositiveScenarios  []issue.Checkbox        `json:"positive_scenarios,omitempty"`
	NegativeScenarios  []issue.Checkbox        `json:"negative_scenarios,omitempty"`
	Description        string                  `json:"description,omitempty"`
	Body               string                  `json:"body,omitempty"`
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
// milestone, created date, dependency titles, and raw body.
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

	return nil
}

// showJSON encodes the issue and its parsed sections as indented JSON.
func showJSON(cmd *cobra.Command, iss *issue.Issue) error {
	sections := issue.ParseSections(iss.Body)

	out := IssueJSON{
		ID:                 iss.ID,
		Title:              iss.Title,
		State:              iss.State,
		Milestone:          iss.Milestone,
		BlockedBy:          iss.BlockedBy,
		Blocks:             iss.Blocks,
		Created:            iss.Created,
		Updated:            iss.Updated,
		Sessions:           iss.Sessions,
		Prerequisites:      sections.Prerequisites,
		Context:            sections.Context,
		AcceptanceCriteria: sections.AcceptanceCriteria,
		PositiveScenarios:  sections.PositiveScenarios,
		NegativeScenarios:  sections.NegativeScenarios,
		Description:        sections.Description,
		Body:               iss.Body,
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
