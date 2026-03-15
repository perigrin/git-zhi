// ABOUTME: Implementation of the 'issue edit' command, handling --state transitions
// ABOUTME: and measurement session bookmarks (start/pause/resume/done/cancel).
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-chain/internal/issue"
	"github.com/perigrin/git-chain/internal/resolve"
)

// runIssueEdit handles 'issue edit [ref] --state <action>'.
func runIssueEdit(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	// Only --state is implemented; other edit flags are future work (issue 6).
	if !cmd.Flags().Changed("state") {
		cmd.Println("edit without --state: not yet implemented")
		return nil
	}

	stateAction, _ := cmd.Flags().GetString("state")

	// Resolve the ref argument (required when --state is set).
	var refInput string
	if len(args) > 0 {
		refInput = args[0]
	}
	refPath, err := resolve.ResolveRef(app.Store, refInput)
	if err != nil {
		return fmt.Errorf("resolve ref: %w", err)
	}

	// Read and parse the issue.
	data, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return fmt.Errorf("read issue: %w", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		return fmt.Errorf("parse issue: %w", err)
	}

	// Extract the UUID from the ref path for output and storage.
	uuidStr := strings.TrimPrefix(refPath, issue.RefPrefix)

	// Validate the transition and derive the new state.
	newState, err := issue.ValidateTransition(iss.State, stateAction)
	if err != nil {
		return err
	}

	// Get the current repo HEAD SHA for measurement bookmarks.
	currentHEAD, err := app.Store.RepoHEAD()
	if err != nil {
		return fmt.Errorf("get repo HEAD: %w", err)
	}

	// Apply measurement bookmark logic based on the action.
	switch stateAction {
	case "start", "resume":
		// Open a new measurement session.
		iss.Sessions = append(iss.Sessions, issue.Session{
			StartSHA: currentHEAD,
		})

	case "pause", "done":
		// Close the last open session (the one with an empty EndSHA).
		openIdx := findOpenSession(iss.Sessions)
		if openIdx >= 0 {
			commitCount, countErr := app.Store.CountCommits(iss.Sessions[openIdx].StartSHA, currentHEAD)
			if countErr != nil {
				return fmt.Errorf("count commits: %w", countErr)
			}
			iss.Sessions[openIdx].EndSHA = currentHEAD
			iss.Sessions[openIdx].Commits = commitCount
		}

	case "cancel":
		// Cancel does not record measurement bookmarks.
	}

	// Update state and timestamp.
	iss.State = newState
	iss.Updated = time.Now()

	// Marshal and persist.
	out, err := issue.Marshal(iss)
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	commitMsg := fmt.Sprintf("Edit issue %s: %s", uuidStr[:8], stateAction)
	if err := app.Store.WriteEntity(refPath, "issue.md", out, commitMsg); err != nil {
		return fmt.Errorf("write issue: %w", err)
	}

	// Attach the ID derived from the ref path (not stored in YAML frontmatter).
	parsedID, err := uuid.FromString(uuidStr)
	if err != nil {
		return fmt.Errorf("parse uuid: %w", err)
	}
	iss.ID = parsedID

	// Output.
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(iss)
	}

	return printEditResult(cmd, stateAction, uuidStr, iss, currentHEAD)
}

// findOpenSession returns the index of the last session whose EndSHA is empty,
// or -1 if no open session exists.
func findOpenSession(sessions []issue.Session) int {
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].EndSHA == "" {
			return i
		}
	}
	return -1
}

// printEditResult writes the human-readable output for a state transition.
func printEditResult(cmd *cobra.Command, action, uuidStr string, iss *issue.Issue, currentHEAD string) error {
	shortID := uuidStr[:8]
	shortHEAD := shortSHA(currentHEAD)

	switch action {
	case "start":
		fmt.Fprintf(cmd.OutOrStdout(), "Started %s: %s\n", shortID, iss.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "  Recorded HEAD: %s\n", shortHEAD)

	case "resume":
		fmt.Fprintf(cmd.OutOrStdout(), "Resumed %s: %s\n", shortID, iss.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "  Recorded HEAD: %s\n", shortHEAD)

	case "pause":
		fmt.Fprintf(cmd.OutOrStdout(), "Paused %s: %s\n", shortID, iss.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "  Recorded HEAD: %s\n", shortHEAD)
		// Report the window for the session we just closed.
		closedIdx := findLastClosedSession(iss.Sessions)
		if closedIdx >= 0 {
			s := iss.Sessions[closedIdx]
			fmt.Fprintf(cmd.OutOrStdout(), "  Window: %s..%s (%d commits)\n",
				shortSHA(s.StartSHA), shortSHA(s.EndSHA), s.Commits)
		}

	case "done":
		fmt.Fprintf(cmd.OutOrStdout(), "Completed %s: %s\n", shortID, iss.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "  Recorded HEAD: %s\n", shortHEAD)
		// Report the window for the session we just closed.
		closedIdx := findLastClosedSession(iss.Sessions)
		if closedIdx >= 0 {
			s := iss.Sessions[closedIdx]
			fmt.Fprintf(cmd.OutOrStdout(), "  Window: %s..%s (%d commits)\n",
				shortSHA(s.StartSHA), shortSHA(s.EndSHA), s.Commits)
		}
		total := totalCommits(iss.Sessions)
		windows := len(iss.Sessions)
		fmt.Fprintf(cmd.OutOrStdout(), "  Total: %d commits across %d measurement window(s)\n", total, windows)

	case "cancel":
		fmt.Fprintf(cmd.OutOrStdout(), "Cancelled %s: %s\n", shortID, iss.Title)
	}

	return nil
}

// findLastClosedSession returns the index of the last session that has a non-empty EndSHA.
func findLastClosedSession(sessions []issue.Session) int {
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].EndSHA != "" {
			return i
		}
	}
	return -1
}

// totalCommits sums the Commits field across all sessions.
func totalCommits(sessions []issue.Session) int {
	total := 0
	for _, s := range sessions {
		total += s.Commits
	}
	return total
}

// shortSHA returns the first 7 characters of a SHA string, safe on short strings.
func shortSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

