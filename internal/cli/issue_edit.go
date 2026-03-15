// ABOUTME: Implementation of the 'issue edit' command, handling --state transitions,
// ABOUTME: dependency edges (block/unblock/before/after), milestone, and tag operations.
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

// knownEditFlags lists all flags that trigger edit behaviour. Used to detect
// the bare "no flags" case which opens $EDITOR (not yet implemented).
var knownEditFlags = []string{
	"state", "block", "unblock", "milestone", "tag", "untag",
	"before", "after", "split", "merge", "purge",
}

// runIssueEdit handles 'issue edit [ref] [flags]'.
func runIssueEdit(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	// Reject not-yet-implemented interactive flags early.
	if cmd.Flags().Changed("split") {
		return fmt.Errorf("--split: not yet implemented")
	}
	if cmd.Flags().Changed("merge") {
		return fmt.Errorf("--merge: not yet implemented")
	}
	if cmd.Flags().Changed("purge") {
		return fmt.Errorf("--purge: not yet implemented")
	}

	// If no known flag is set, this is the bare interactive edit ($EDITOR).
	anyFlagSet := false
	for _, name := range knownEditFlags {
		if cmd.Flags().Changed(name) {
			anyFlagSet = true
			break
		}
	}
	if !anyFlagSet {
		cmd.Println("edit without flags: not yet implemented (use $EDITOR)")
		return nil
	}

	// --state: handled separately because it needs RepoHEAD and outputs a
	// formatted result. Resolved early so we can share the ref path with the
	// other flags below.
	var refInput string
	if len(args) > 0 {
		refInput = args[0]
	}

	if cmd.Flags().Changed("state") {
		stateAction, _ := cmd.Flags().GetString("state")

		refPath, err := resolve.ResolveRef(app.Store, refInput)
		if err != nil {
			return fmt.Errorf("resolve ref: %w", err)
		}

		data, err := app.Store.ReadEntity(refPath, "issue.md")
		if err != nil {
			return fmt.Errorf("read issue: %w", err)
		}
		iss, err := issue.Parse(data)
		if err != nil {
			return fmt.Errorf("parse issue: %w", err)
		}

		uuidStr := strings.TrimPrefix(refPath, issue.RefPrefix)

		newState, err := issue.ValidateTransition(iss.State, stateAction)
		if err != nil {
			return err
		}

		currentHEAD, err := app.Store.RepoHEAD()
		if err != nil {
			return fmt.Errorf("get repo HEAD: %w", err)
		}

		switch stateAction {
		case "start", "resume":
			iss.Sessions = append(iss.Sessions, issue.Session{
				StartSHA: currentHEAD,
			})

		case "pause", "done":
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

		iss.State = newState
		iss.Updated = time.Now()

		out, err := issue.Marshal(iss)
		if err != nil {
			return fmt.Errorf("marshal issue: %w", err)
		}
		commitMsg := fmt.Sprintf("Edit issue %s: %s", uuidStr[:8], stateAction)
		if err := app.Store.WriteEntity(refPath, "issue.md", out, commitMsg); err != nil {
			return fmt.Errorf("write issue: %w", err)
		}

		parsedID, err := uuid.FromString(uuidStr)
		if err != nil {
			return fmt.Errorf("parse uuid: %w", err)
		}
		iss.ID = parsedID

		format, _ := cmd.Root().PersistentFlags().GetString("format")
		if format == "json" {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(iss)
		}

		return printEditResult(cmd, stateAction, uuidStr, iss, currentHEAD)
	}

	// For non-state flags: resolve and load the primary issue once, then apply
	// each flag's modification, writing back at the end.
	refPath, err := resolve.ResolveRef(app.Store, refInput)
	if err != nil {
		return fmt.Errorf("resolve ref: %w", err)
	}

	data, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return fmt.Errorf("read issue: %w", err)
	}
	iss, err := issue.Parse(data)
	if err != nil {
		return fmt.Errorf("parse issue: %w", err)
	}

	uuidStr := strings.TrimPrefix(refPath, issue.RefPrefix)

	// --milestone: update the milestone field.
	if cmd.Flags().Changed("milestone") {
		ms, _ := cmd.Flags().GetString("milestone")
		iss.Milestone = ms
	}

	// --block <target>: this issue blocks target.
	// Adds target's UUID to this.Blocks and this UUID to target.BlockedBy.
	if cmd.Flags().Changed("block") {
		targetInput, _ := cmd.Flags().GetString("block")
		if err := addBlockEdge(app, iss, uuidStr, targetInput, refPath); err != nil {
			return fmt.Errorf("--block: %w", err)
		}
	}

	// --unblock <target>: remove the block edge in both directions.
	if cmd.Flags().Changed("unblock") {
		targetInput, _ := cmd.Flags().GetString("unblock")
		if err := removeBlockEdge(app, iss, uuidStr, targetInput, refPath); err != nil {
			return fmt.Errorf("--unblock: %w", err)
		}
	}

	// --before <target>: this issue comes before target (same as --block).
	if cmd.Flags().Changed("before") {
		targetInput, _ := cmd.Flags().GetString("before")
		if err := addBlockEdge(app, iss, uuidStr, targetInput, refPath); err != nil {
			return fmt.Errorf("--before: %w", err)
		}
	}

	// --after <target>: this issue comes after target (target blocks this issue).
	if cmd.Flags().Changed("after") {
		targetInput, _ := cmd.Flags().GetString("after")
		if err := addBlockedByEdge(app, iss, uuidStr, targetInput, refPath); err != nil {
			return fmt.Errorf("--after: %w", err)
		}
	}

	// --tag <name>: create a tag ref pointing to this issue's ref path.
	if cmd.Flags().Changed("tag") {
		tagName, _ := cmd.Flags().GetString("tag")
		tagRef := "refs/chain/_/tags/" + tagName
		commitMsg := fmt.Sprintf("Tag: %s", tagName)
		if err := app.Store.WriteEntity(tagRef, "tag.txt", []byte(refPath), commitMsg); err != nil {
			return fmt.Errorf("write tag ref: %w", err)
		}
	}

	// --untag <name>: delete the tag ref.
	if cmd.Flags().Changed("untag") {
		tagName, _ := cmd.Flags().GetString("untag")
		tagRef := "refs/chain/_/tags/" + tagName
		if err := app.Store.DeleteRef(tagRef); err != nil {
			return fmt.Errorf("delete tag ref: %w", err)
		}
	}

	// Write the (potentially modified) primary issue back only if one of the
	// flags that modifies it directly was set.
	issueDirty := cmd.Flags().Changed("milestone") ||
		cmd.Flags().Changed("block") ||
		cmd.Flags().Changed("unblock") ||
		cmd.Flags().Changed("before") ||
		cmd.Flags().Changed("after")

	if issueDirty {
		iss.Updated = time.Now()
		out, err := issue.Marshal(iss)
		if err != nil {
			return fmt.Errorf("marshal issue: %w", err)
		}
		commitMsg := fmt.Sprintf("Edit issue %s", uuidStr[:8])
		if err := app.Store.WriteEntity(refPath, "issue.md", out, commitMsg); err != nil {
			return fmt.Errorf("write issue: %w", err)
		}
	}

	return nil
}

// addBlockEdge makes `this` block `targetInput`: adds target's UUID to
// this.Blocks and this UUID to target.BlockedBy. Writes the target issue.
// The caller is responsible for writing the primary issue.
func addBlockEdge(app *App, iss *issue.Issue, issUUIDStr, targetInput, _ string) error {
	targetRef, err := resolve.ResolveRef(app.Store, targetInput)
	if err != nil {
		return fmt.Errorf("resolve target ref: %w", err)
	}
	targetUUIDStr := strings.TrimPrefix(targetRef, issue.RefPrefix)
	targetUUID, err := uuid.FromString(targetUUIDStr)
	if err != nil {
		return fmt.Errorf("parse target uuid: %w", err)
	}
	issUUID, err := uuid.FromString(issUUIDStr)
	if err != nil {
		return fmt.Errorf("parse issue uuid: %w", err)
	}

	// Add target to this.Blocks (deduplicated).
	if !containsUUID(iss.Blocks, targetUUID) {
		iss.Blocks = append(iss.Blocks, targetUUID)
	}

	// Load target and add this to target.BlockedBy.
	tData, err := app.Store.ReadEntity(targetRef, "issue.md")
	if err != nil {
		return fmt.Errorf("read target issue: %w", err)
	}
	tIss, err := issue.Parse(tData)
	if err != nil {
		return fmt.Errorf("parse target issue: %w", err)
	}
	if !containsUUID(tIss.BlockedBy, issUUID) {
		tIss.BlockedBy = append(tIss.BlockedBy, issUUID)
	}
	tIss.Updated = time.Now()
	tOut, err := issue.Marshal(tIss)
	if err != nil {
		return fmt.Errorf("marshal target issue: %w", err)
	}
	commitMsg := fmt.Sprintf("Edit issue %s: add blocked_by %s", targetUUIDStr[:8], issUUIDStr[:8])
	if err := app.Store.WriteEntity(targetRef, "issue.md", tOut, commitMsg); err != nil {
		return fmt.Errorf("write target issue: %w", err)
	}
	return nil
}

// addBlockedByEdge makes `targetInput` block `this`: adds target's UUID to
// this.BlockedBy and this UUID to target.Blocks. Writes the target issue.
// The caller is responsible for writing the primary issue.
func addBlockedByEdge(app *App, iss *issue.Issue, issUUIDStr, targetInput, _ string) error {
	targetRef, err := resolve.ResolveRef(app.Store, targetInput)
	if err != nil {
		return fmt.Errorf("resolve target ref: %w", err)
	}
	targetUUIDStr := strings.TrimPrefix(targetRef, issue.RefPrefix)
	targetUUID, err := uuid.FromString(targetUUIDStr)
	if err != nil {
		return fmt.Errorf("parse target uuid: %w", err)
	}
	issUUID, err := uuid.FromString(issUUIDStr)
	if err != nil {
		return fmt.Errorf("parse issue uuid: %w", err)
	}

	// Add target to this.BlockedBy (deduplicated).
	if !containsUUID(iss.BlockedBy, targetUUID) {
		iss.BlockedBy = append(iss.BlockedBy, targetUUID)
	}

	// Load target and add this to target.Blocks.
	tData, err := app.Store.ReadEntity(targetRef, "issue.md")
	if err != nil {
		return fmt.Errorf("read target issue: %w", err)
	}
	tIss, err := issue.Parse(tData)
	if err != nil {
		return fmt.Errorf("parse target issue: %w", err)
	}
	if !containsUUID(tIss.Blocks, issUUID) {
		tIss.Blocks = append(tIss.Blocks, issUUID)
	}
	tIss.Updated = time.Now()
	tOut, err := issue.Marshal(tIss)
	if err != nil {
		return fmt.Errorf("marshal target issue: %w", err)
	}
	commitMsg := fmt.Sprintf("Edit issue %s: add blocks %s", targetUUIDStr[:8], issUUIDStr[:8])
	if err := app.Store.WriteEntity(targetRef, "issue.md", tOut, commitMsg); err != nil {
		return fmt.Errorf("write target issue: %w", err)
	}
	return nil
}

// removeBlockEdge removes the block edge between this and target in both
// directions. Writes the target issue; caller writes the primary issue.
func removeBlockEdge(app *App, iss *issue.Issue, issUUIDStr, targetInput, _ string) error {
	targetRef, err := resolve.ResolveRef(app.Store, targetInput)
	if err != nil {
		return fmt.Errorf("resolve target ref: %w", err)
	}
	targetUUIDStr := strings.TrimPrefix(targetRef, issue.RefPrefix)
	targetUUID, err := uuid.FromString(targetUUIDStr)
	if err != nil {
		return fmt.Errorf("parse target uuid: %w", err)
	}
	issUUID, err := uuid.FromString(issUUIDStr)
	if err != nil {
		return fmt.Errorf("parse issue uuid: %w", err)
	}

	// Remove target from this.Blocks.
	iss.Blocks = removeUUID(iss.Blocks, targetUUID)

	// Load target and remove this from target.BlockedBy.
	tData, err := app.Store.ReadEntity(targetRef, "issue.md")
	if err != nil {
		return fmt.Errorf("read target issue: %w", err)
	}
	tIss, err := issue.Parse(tData)
	if err != nil {
		return fmt.Errorf("parse target issue: %w", err)
	}
	tIss.BlockedBy = removeUUID(tIss.BlockedBy, issUUID)
	tIss.Updated = time.Now()
	tOut, err := issue.Marshal(tIss)
	if err != nil {
		return fmt.Errorf("marshal target issue: %w", err)
	}
	commitMsg := fmt.Sprintf("Edit issue %s: remove blocked_by %s", targetUUIDStr[:8], issUUIDStr[:8])
	if err := app.Store.WriteEntity(targetRef, "issue.md", tOut, commitMsg); err != nil {
		return fmt.Errorf("write target issue: %w", err)
	}
	return nil
}

// containsUUID reports whether the slice contains the given UUID.
func containsUUID(slice []uuid.UUID, target uuid.UUID) bool {
	for _, u := range slice {
		if u == target {
			return true
		}
	}
	return false
}

// removeUUID returns a new slice with all occurrences of target removed.
func removeUUID(slice []uuid.UUID, target uuid.UUID) []uuid.UUID {
	result := slice[:0:0]
	for _, u := range slice {
		if u != target {
			result = append(result, u)
		}
	}
	return result
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

