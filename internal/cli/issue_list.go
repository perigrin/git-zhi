// ABOUTME: Implementation of the issue list command: loads all issues, applies
// ABOUTME: filters (--all, --milestone, --state), and renders in human or JSON format.
package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-chain/internal/issue"
)

// stateIcon maps issue states to single-character display icons.
var stateIcon = map[issue.State]string{
	issue.StateInProgress: "●",
	issue.StatePending:    "○",
	issue.StateDone:       "✓",
	issue.StateCancelled:  "✗",
}

// runIssueList loads all issues from storage, applies the active filters, and
// renders the result in human-readable table format or as a JSON array.
func runIssueList(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	includeAll, _ := cmd.Flags().GetBool("all")
	milestoneFilter, _ := cmd.Flags().GetString("milestone")
	stateFilter, _ := cmd.Flags().GetString("state")

	refs, err := app.Store.ListRefs("refs/chain/_/issues/")
	if err != nil {
		return fmt.Errorf("list issue refs: %w", err)
	}

	// Sort lexicographically — UUIDv7 encodes creation time, so this
	// gives chronological order without loading issue content.
	sort.Strings(refs)

	var issues []*issue.Issue

	for _, refPath := range refs {
		raw, err := app.Store.ReadEntity(refPath, "issue.md")
		if err != nil {
			return fmt.Errorf("read issue at %s: %w", refPath, err)
		}

		iss, err := issue.Parse(raw)
		if err != nil {
			return fmt.Errorf("parse issue at %s: %w", refPath, err)
		}

		// Extract UUID from the ref path — not stored in frontmatter.
		uuidStr := strings.TrimPrefix(refPath, "refs/chain/_/issues/")
		id, err := uuid.FromString(uuidStr)
		if err != nil {
			return fmt.Errorf("extract uuid from ref path %q: %w", refPath, err)
		}
		iss.ID = id

		// Apply --state filter first (overrides --all for state selection).
		if stateFilter != "" {
			if string(iss.State) != stateFilter {
				continue
			}
		} else if !includeAll {
			// Default: show only active states.
			if iss.State != issue.StatePending && iss.State != issue.StateInProgress {
				continue
			}
		}

		// Apply --milestone filter.
		if milestoneFilter != "" && iss.Milestone != milestoneFilter {
			continue
		}

		issues = append(issues, iss)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if issues == nil {
			issues = []*issue.Issue{}
		}
		return enc.Encode(issues)
	}

	return listHuman(cmd, issues)
}

// listHuman renders the issue list as a human-readable table with state icons.
func listHuman(cmd *cobra.Command, issues []*issue.Issue) error {
	w := cmd.OutOrStdout()
	for _, iss := range issues {
		icon := stateIcon[iss.State]
		if icon == "" {
			icon = "?"
		}
		fmt.Fprintf(w, "  %s  %-40s %s %s\n",
			iss.ID.String()[:8],
			iss.Title,
			icon,
			string(iss.State),
		)
	}
	return nil
}
