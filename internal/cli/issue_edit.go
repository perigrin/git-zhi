// ABOUTME: Implementation of the 'issue edit' command, handling --state transitions,
// ABOUTME: dependency edges (block/unblock/before/after), --split, --merge, --purge, --batch, and tag operations.
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/actor"
	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/resolve"
	"github.com/perigrin/git-zhi/internal/uuids"
)

// knownEditFlags lists all flags that trigger edit behaviour. Used to detect
// the bare "no flags" case which opens $EDITOR (not yet implemented).
// Note: --yes is not included because it only confirms a destructive operation
// and does not independently trigger an edit.
var knownEditFlags = []string{
	"state", "block", "unblock", "milestone", "tag", "untag",
	"label", "unlabel",
	"assign", "unassign",
	"before", "after", "split", "merge", "purge", "batch",
}

// runIssueEdit handles 'issue edit [ref] [flags]'.
func runIssueEdit(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	// Determine the target ref input from args (shared across all branches).
	var refInput string
	if len(args) > 0 {
		refInput = args[0]
	}

	// --split, --merge, --purge are mutually exclusive with each other and with
	// --state and all other modification flags. Handle them first and return.
	destructiveCount := 0
	for _, name := range []string{"split", "merge", "purge"} {
		if cmd.Flags().Changed(name) {
			destructiveCount++
		}
	}
	if destructiveCount > 1 {
		return fmt.Errorf("--split, --merge, and --purge are mutually exclusive")
	}

	if cmd.Flags().Changed("split") {
		return runIssueEditSplit(cmd, app, refInput)
	}
	if cmd.Flags().Changed("merge") {
		return runIssueEditMerge(cmd, app, refInput)
	}
	if cmd.Flags().Changed("purge") {
		return runIssueEditPurge(cmd, app, refInput)
	}
	if cmd.Flags().Changed("batch") {
		return runIssueEditBatch(cmd, app)
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
	// formatted result. refInput was already set above.
	if cmd.Flags().Changed("state") {
		for _, name := range []string{"block", "unblock", "milestone", "tag", "untag", "label", "unlabel", "assign", "unassign", "before", "after"} {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("--%s cannot be combined with --state; run them as separate commands", name)
			}
		}

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

		// Capture wall-clock time once for this transition; reused across all
		// session fields so start/end timestamps are consistent within a single
		// state change.
		now := time.Now()

		switch stateAction {
		case "start", "resume":
			if findOpenSession(iss.Sessions) >= 0 {
				return fmt.Errorf("cannot %s: a measurement session is already open", stateAction)
			}
			iss.Sessions = append(iss.Sessions, issue.Session{
				StartSHA:  currentHEAD,
				StartedAt: &now,
			})

		case "pause", "done":
			openIdx := findOpenSession(iss.Sessions)
			if openIdx >= 0 {
				commitCount, countErr := app.Store.CountCommits(iss.Sessions[openIdx].StartSHA, currentHEAD)
				if countErr != nil {
					return fmt.Errorf("count commits: %w", countErr)
				}
				iss.Sessions[openIdx].EndSHA = currentHEAD
				iss.Sessions[openIdx].EndedAt = &now
				iss.Sessions[openIdx].Commits = commitCount
			}

			// On done, compute ObservedPaths via git diff --name-only spanning
			// the full history from the first session's StartSHA to the current
			// HEAD. This captures all files touched across all sessions.
			if stateAction == "done" && len(iss.Sessions) > 0 {
				firstStartSHA := iss.Sessions[0].StartSHA
				if firstStartSHA != "" {
					paths, diffErr := app.Store.DiffNameOnly(firstStartSHA, currentHEAD)
					if diffErr != nil {
						// Non-fatal: record empty paths rather than blocking the
						// state transition on a diff failure.
						iss.ObservedPaths = []string{}
					} else {
						iss.ObservedPaths = paths
					}
				}
			}

		case "cancel":
			openIdx := findOpenSession(iss.Sessions)
			if openIdx >= 0 {
				commitCount, countErr := app.Store.CountCommits(iss.Sessions[openIdx].StartSHA, currentHEAD)
				if countErr != nil {
					// If counting fails (e.g., after rebase), close with zero commits.
					iss.Sessions[openIdx].EndSHA = currentHEAD
					iss.Sessions[openIdx].EndedAt = &now
					iss.Sessions[openIdx].Commits = 0
				} else {
					iss.Sessions[openIdx].EndSHA = currentHEAD
					iss.Sessions[openIdx].EndedAt = &now
					iss.Sessions[openIdx].Commits = commitCount
				}
			}
		}

		// Record a Transition for every state change. Derive the actor from
		// the Store's git author config so lineage tracking knows who acted.
		authorName, authorEmail := app.Store.AuthorInfo()
		a := actor.DeriveActor(authorName, authorEmail)
		iss.Transitions = append(iss.Transitions, issue.Transition{
			State:     string(newState),
			Actor:     a.String(),
			Timestamp: now,
		})

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

	// --milestone: update the milestone field. Reject assignment to completed milestones.
	if cmd.Flags().Changed("milestone") {
		msName, _ := cmd.Flags().GetString("milestone")
		if msName != "" {
			ms, msErr := milestone.LoadMilestone(app.Store, msName)
			if msErr == nil && ms.State == "completed" {
				return fmt.Errorf("cannot assign issue to completed milestone %q", msName)
			}
		}
		iss.Milestone = msName
	}

	// --block <target>: this issue blocks target.
	// Adds target's UUID to this.Blocks and this UUID to target.BlockedBy.
	if cmd.Flags().Changed("block") {
		targetInput, _ := cmd.Flags().GetString("block")
		if err := addBlockEdge(app, iss, uuidStr, targetInput); err != nil {
			return fmt.Errorf("--block: %w", err)
		}
	}

	// --unblock <target>: remove the block edge in both directions.
	if cmd.Flags().Changed("unblock") {
		targetInput, _ := cmd.Flags().GetString("unblock")
		if err := removeBlockEdge(app, iss, uuidStr, targetInput); err != nil {
			return fmt.Errorf("--unblock: %w", err)
		}
	}

	// --before <target>: this issue comes before target (same as --block).
	if cmd.Flags().Changed("before") {
		targetInput, _ := cmd.Flags().GetString("before")
		if err := addBlockEdge(app, iss, uuidStr, targetInput); err != nil {
			return fmt.Errorf("--before: %w", err)
		}
	}

	// --after <target>: this issue comes after target (target blocks this issue).
	if cmd.Flags().Changed("after") {
		targetInput, _ := cmd.Flags().GetString("after")
		if err := addBlockedByEdge(app, iss, uuidStr, targetInput); err != nil {
			return fmt.Errorf("--after: %w", err)
		}
	}

	// --tag <name>: create a tag ref pointing to this issue's ref path.
	if cmd.Flags().Changed("tag") {
		tagName, _ := cmd.Flags().GetString("tag")
		tagRef := "refs/zhi/_/tags/" + tagName
		if app.Store.RefExists(tagRef) {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: tag %q already exists, overwriting\n", tagName)
		}
		commitMsg := fmt.Sprintf("Tag: %s", tagName)
		if err := app.Store.WriteEntity(tagRef, "tag.txt", []byte(refPath), commitMsg); err != nil {
			return fmt.Errorf("write tag ref: %w", err)
		}
	}

	// --untag <name>: delete the tag ref.
	if cmd.Flags().Changed("untag") {
		tagName, _ := cmd.Flags().GetString("untag")
		tagRef := "refs/zhi/_/tags/" + tagName
		if err := app.Store.DeleteRef(tagRef); err != nil {
			return fmt.Errorf("delete tag ref: %w", err)
		}
	}

	// --label <name>: append label to the issue's Labels slice (deduplicated).
	if cmd.Flags().Changed("label") {
		labelName, _ := cmd.Flags().GetString("label")
		found := false
		for _, l := range iss.Labels {
			if l == labelName {
				found = true
				break
			}
		}
		if !found {
			iss.Labels = append(iss.Labels, labelName)
		}
	}

	// --unlabel <name>: remove label from the issue's Labels slice.
	if cmd.Flags().Changed("unlabel") {
		labelName, _ := cmd.Flags().GetString("unlabel")
		filtered := iss.Labels[:0]
		for _, l := range iss.Labels {
			if l != labelName {
				filtered = append(filtered, l)
			}
		}
		iss.Labels = filtered
	}

	// --assign <worker>: set the Assigned field to the given worker identity.
	if cmd.Flags().Changed("assign") {
		worker, _ := cmd.Flags().GetString("assign")
		iss.Assigned = worker
	}

	// --unassign: clear the Assigned field.
	if cmd.Flags().Changed("unassign") {
		iss.Assigned = ""
	}

	// Write the (potentially modified) primary issue back only if one of the
	// flags that modifies it directly was set.
	issueDirty := cmd.Flags().Changed("milestone") ||
		cmd.Flags().Changed("block") ||
		cmd.Flags().Changed("unblock") ||
		cmd.Flags().Changed("before") ||
		cmd.Flags().Changed("after") ||
		cmd.Flags().Changed("label") ||
		cmd.Flags().Changed("unlabel") ||
		cmd.Flags().Changed("assign") ||
		cmd.Flags().Changed("unassign")

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
func addBlockEdge(app *App, iss *issue.Issue, issUUIDStr, targetInput string) error {
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

	if issUUID == targetUUID {
		return fmt.Errorf("cannot create dependency: issue cannot block itself")
	}

	// Check for cycles before persisting the edge.
	allIssues, loadErr := issue.LoadAllIssues(app.Store)
	if loadErr != nil {
		return fmt.Errorf("load issues for cycle check: %w", loadErr)
	}
	g := graph.New(allIssues)
	if g.HasCycle(issUUID, targetUUID) {
		return fmt.Errorf("adding edge %s -> %s would create a cycle", issUUIDStr[:8], targetUUIDStr[:8])
	}

	// Add target to this.Blocks (deduplicated).
	if !uuids.ContainsUUID(iss.Blocks, targetUUID) {
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
	if tIss.State == issue.StateDone || tIss.State == issue.StateCancelled {
		return fmt.Errorf("cannot add dependency: target issue %s is %s (done/cancelled issues cannot gain dependencies)", targetUUIDStr[:8], tIss.State)
	}
	if !uuids.ContainsUUID(tIss.BlockedBy, issUUID) {
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
func addBlockedByEdge(app *App, iss *issue.Issue, issUUIDStr, targetInput string) error {
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

	if issUUID == targetUUID {
		return fmt.Errorf("cannot create dependency: issue cannot block itself")
	}

	if iss.State == issue.StateDone || iss.State == issue.StateCancelled {
		return fmt.Errorf("cannot add dependency: issue %s is %s (done/cancelled issues cannot gain dependencies)", issUUIDStr[:8], iss.State)
	}

	// Check for cycles before persisting the edge. For --after, the edge is
	// targetUUID -> issUUID (target blocks this issue), so check HasCycle(targetUUID, issUUID).
	allIssues, loadErr := issue.LoadAllIssues(app.Store)
	if loadErr != nil {
		return fmt.Errorf("load issues for cycle check: %w", loadErr)
	}
	g := graph.New(allIssues)
	if g.HasCycle(targetUUID, issUUID) {
		return fmt.Errorf("adding edge %s -> %s would create a cycle", targetUUIDStr[:8], issUUIDStr[:8])
	}

	// Add target to this.BlockedBy (deduplicated).
	if !uuids.ContainsUUID(iss.BlockedBy, targetUUID) {
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
	if !uuids.ContainsUUID(tIss.Blocks, issUUID) {
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
func removeBlockEdge(app *App, iss *issue.Issue, issUUIDStr, targetInput string) error {
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

	// Remove target from this.Blocks and this.BlockedBy (direction-agnostic).
	iss.Blocks = uuids.RemoveUUID(iss.Blocks, targetUUID)
	iss.BlockedBy = uuids.RemoveUUID(iss.BlockedBy, targetUUID)

	// Load target and remove this from both directions on the target side.
	tData, err := app.Store.ReadEntity(targetRef, "issue.md")
	if err != nil {
		return fmt.Errorf("read target issue: %w", err)
	}
	tIss, err := issue.Parse(tData)
	if err != nil {
		return fmt.Errorf("parse target issue: %w", err)
	}
	tIss.BlockedBy = uuids.RemoveUUID(tIss.BlockedBy, issUUID)
	tIss.Blocks = uuids.RemoveUUID(tIss.Blocks, issUUID)
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

	case "reopen":
		fmt.Fprintf(cmd.OutOrStdout(), "Reopened %s: %s\n", shortID, iss.Title)
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

// runIssueEditSplit handles 'issue edit <ref> --split'. Reads replacement
// content from stdin using --- separators. Block 0 replaces the original issue
// (keeping its UUID). Blocks 1..N become new issues chained sequentially.
// The original issue's downstream dependencies (Blocks) are transferred to the
// last issue in the chain.
func runIssueEditSplit(cmd *cobra.Command, app *App, refInput string) error {
	refPath, err := resolve.ResolveRef(app.Store, refInput)
	if err != nil {
		return fmt.Errorf("resolve ref: %w", err)
	}

	data, err := app.Store.ReadEntity(refPath, "issue.md")
	if err != nil {
		return fmt.Errorf("read issue: %w", err)
	}
	origIss, err := issue.Parse(data)
	if err != nil {
		return fmt.Errorf("parse issue: %w", err)
	}
	uuidStr := strings.TrimPrefix(refPath, issue.RefPrefix)
	origUUID, err := uuid.FromString(uuidStr)
	if err != nil {
		return fmt.Errorf("parse uuid: %w", err)
	}
	origIss.ID = origUUID

	// Read stdin content.
	raw, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	blocks := issue.SplitBatch(raw)
	if len(blocks) == 0 {
		return fmt.Errorf("no issue content found on stdin")
	}

	// Parse all incoming blocks.
	type parsedBlock struct {
		iss *issue.Issue
	}
	parsed := make([]parsedBlock, len(blocks))
	for i, block := range blocks {
		b, parseErr := issue.Parse(block)
		if parseErr != nil {
			return fmt.Errorf("parse block %d: %w", i+1, parseErr)
		}
		parsed[i] = parsedBlock{iss: b}
	}

	// Preserve the original downstream deps — they transfer to the last issue.
	origDownstream := origIss.Blocks

	// Block 0 updates the original issue in place.
	now := time.Now()
	origIss.Title = parsed[0].iss.Title
	origIss.Body = parsed[0].iss.Body
	origIss.Updated = now
	origIss.Blocks = nil

	if len(blocks) == 1 {
		// Single block: content replacement only, no new issues.
		out, marshalErr := issue.Marshal(origIss)
		if marshalErr != nil {
			return fmt.Errorf("marshal issue: %w", marshalErr)
		}
		if writeErr := app.Store.WriteEntity(refPath, "issue.md", out, fmt.Sprintf("Split issue %s: replace content", uuidStr[:8])); writeErr != nil {
			return fmt.Errorf("write issue: %w", writeErr)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Split %s: replaced content\n", uuidStr[:8])
		return nil
	}

	// Blocks 1..N: create new issues with new UUIDs.
	newIssues := make([]*issue.Issue, len(blocks)-1)
	for i := 1; i < len(blocks); i++ {
		newID, idErr := uuid.NewV7()
		if idErr != nil {
			return fmt.Errorf("generate uuid for block %d: %w", i, idErr)
		}
		ni := parsed[i].iss
		ni.ID = newID
		if ni.State == "" {
			ni.State = issue.StatePending
		}
		ni.Milestone = origIss.Milestone
		ni.Created = now
		ni.Updated = now
		newIssues[i-1] = ni
	}

	// Wire the chain: origIss -> newIssues[0] -> newIssues[1] -> ... -> newIssues[N-1]
	// origIss blocks first new issue.
	origIss.Blocks = []uuid.UUID{newIssues[0].ID}
	newIssues[0].BlockedBy = append(newIssues[0].BlockedBy, origUUID)

	// Each new issue blocks the next.
	for i := 0; i < len(newIssues)-1; i++ {
		newIssues[i].Blocks = append(newIssues[i].Blocks, newIssues[i+1].ID)
		newIssues[i+1].BlockedBy = append(newIssues[i+1].BlockedBy, newIssues[i].ID)
	}

	// Transfer original downstream deps to the last new issue.
	lastNew := newIssues[len(newIssues)-1]
	for _, downUUID := range origDownstream {
		if !uuids.ContainsUUID(lastNew.Blocks, downUUID) {
			lastNew.Blocks = append(lastNew.Blocks, downUUID)
		}
	}

	// Update each downstream dep's BlockedBy to point to lastNew instead of orig.
	for _, downUUID := range origDownstream {
		downRef := issue.RefPrefix + downUUID.String()
		downData, readErr := app.Store.ReadEntity(downRef, "issue.md")
		if readErr != nil {
			continue // best-effort; skip unreadable deps
		}
		downIss, parseErr := issue.Parse(downData)
		if parseErr != nil {
			continue
		}
		downIss.BlockedBy = uuids.RemoveUUID(downIss.BlockedBy, origUUID)
		if !uuids.ContainsUUID(downIss.BlockedBy, lastNew.ID) {
			downIss.BlockedBy = append(downIss.BlockedBy, lastNew.ID)
		}
		downIss.Updated = now
		downOut, marshalErr := issue.Marshal(downIss)
		if marshalErr != nil {
			continue
		}
		_ = app.Store.WriteEntity(downRef, "issue.md", downOut,
			fmt.Sprintf("Edit issue %s: blocked_by transferred to %s", downUUID.String()[:8], lastNew.ID.String()[:8]))
	}

	// Write the updated original issue.
	origOut, err := issue.Marshal(origIss)
	if err != nil {
		return fmt.Errorf("marshal original issue: %w", err)
	}
	if err := app.Store.WriteEntity(refPath, "issue.md", origOut, fmt.Sprintf("Split issue %s", uuidStr[:8])); err != nil {
		return fmt.Errorf("write original issue: %w", err)
	}

	// Write each new issue.
	for _, ni := range newIssues {
		niOut, marshalErr := issue.Marshal(ni)
		if marshalErr != nil {
			return fmt.Errorf("marshal new issue: %w", marshalErr)
		}
		niRef := issue.RefPrefix + ni.ID.String()
		if writeErr := app.Store.WriteEntity(niRef, "issue.md", niOut, "Add issue (split): "+ni.Title); writeErr != nil {
			return fmt.Errorf("write new issue: %w", writeErr)
		}
	}

	// Human-readable output.
	fmt.Fprintf(cmd.OutOrStdout(), "Split %s into:\n", uuidStr[:8])
	fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", uuidStr[:8], origIss.Title)
	for i, ni := range newIssues {
		fmt.Fprintf(cmd.OutOrStdout(), "   \u2193\n")
		_ = i
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", ni.ID.String()[:8], ni.Title)
	}

	// Report transferred downstream deps.
	if len(origDownstream) > 0 {
		for _, downUUID := range origDownstream {
			fmt.Fprintf(cmd.OutOrStdout(), "\nDependencies transferred: %s now blocked by %s\n",
				downUUID.String()[:8], lastNew.ID.String()[:8])
		}
	}

	return nil
}

// runIssueEditMerge handles 'issue edit <ref> --merge <other-ref>'. Appends
// the merged issue's title and body, combines sessions, transfers dependency
// edges, and marks the merged issue as cancelled.
func runIssueEditMerge(cmd *cobra.Command, app *App, refInput string) error {
	mergeInput, _ := cmd.Flags().GetString("merge")

	// Resolve and load the primary (target) issue.
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
	issUUID, err := uuid.FromString(uuidStr)
	if err != nil {
		return fmt.Errorf("parse uuid: %w", err)
	}
	iss.ID = issUUID

	// Resolve and load the issue to be merged in.
	mergeRef, err := resolve.ResolveRef(app.Store, mergeInput)
	if err != nil {
		return fmt.Errorf("resolve merge ref: %w", err)
	}
	mergeData, err := app.Store.ReadEntity(mergeRef, "issue.md")
	if err != nil {
		return fmt.Errorf("read merge issue: %w", err)
	}
	mergeIss, err := issue.Parse(mergeData)
	if err != nil {
		return fmt.Errorf("parse merge issue: %w", err)
	}
	mergeUUIDStr := strings.TrimPrefix(mergeRef, issue.RefPrefix)
	mergeUUID, err := uuid.FromString(mergeUUIDStr)
	if err != nil {
		return fmt.Errorf("parse merge uuid: %w", err)
	}
	mergeIss.ID = mergeUUID

	if issUUID == mergeUUID {
		return fmt.Errorf("cannot merge an issue into itself")
	}

	now := time.Now()

	// Combine title and body.
	iss.Title = iss.Title + " + " + mergeIss.Title
	if mergeIss.Body != "" {
		if iss.Body != "" {
			iss.Body = iss.Body + "\n\n" + mergeIss.Body
		} else {
			iss.Body = mergeIss.Body
		}
	}

	// Combine sessions.
	iss.Sessions = append(iss.Sessions, mergeIss.Sessions...)

	// Remove the merge issue from iss.BlockedBy (if it was blocking iss).
	iss.BlockedBy = uuids.RemoveUUID(iss.BlockedBy, mergeUUID)

	// Transfer mergeIss.Blocks to iss.Blocks (avoid duplicates, skip self-refs).
	for _, u := range mergeIss.Blocks {
		if u == issUUID {
			continue // skip self-reference
		}
		if !uuids.ContainsUUID(iss.Blocks, u) {
			iss.Blocks = append(iss.Blocks, u)
		}
	}

	// Transfer mergeIss.BlockedBy to iss.BlockedBy (avoid duplicates, skip self-refs).
	for _, u := range mergeIss.BlockedBy {
		if u == issUUID {
			continue // skip self-reference
		}
		if !uuids.ContainsUUID(iss.BlockedBy, u) {
			iss.BlockedBy = append(iss.BlockedBy, u)
		}
	}

	// For each issue that mergeIss blocked, update their BlockedBy to point to iss.
	for _, downUUID := range mergeIss.Blocks {
		if downUUID == issUUID {
			continue
		}
		downRef := issue.RefPrefix + downUUID.String()
		downData, readErr := app.Store.ReadEntity(downRef, "issue.md")
		if readErr != nil {
			continue
		}
		downIss, parseErr := issue.Parse(downData)
		if parseErr != nil {
			continue
		}
		downIss.BlockedBy = uuids.RemoveUUID(downIss.BlockedBy, mergeUUID)
		if !uuids.ContainsUUID(downIss.BlockedBy, issUUID) {
			downIss.BlockedBy = append(downIss.BlockedBy, issUUID)
		}
		downIss.Updated = now
		downOut, marshalErr := issue.Marshal(downIss)
		if marshalErr != nil {
			continue
		}
		_ = app.Store.WriteEntity(downRef, "issue.md", downOut,
			fmt.Sprintf("Edit issue %s: blocked_by transferred from %s to %s",
				downUUID.String()[:8], mergeUUIDStr[:8], uuidStr[:8]))
	}

	// For each issue that blocked mergeIss, update their Blocks to point to iss.
	for _, upUUID := range mergeIss.BlockedBy {
		if upUUID == issUUID {
			continue
		}
		upRef := issue.RefPrefix + upUUID.String()
		upData, readErr := app.Store.ReadEntity(upRef, "issue.md")
		if readErr != nil {
			continue
		}
		upIss, parseErr := issue.Parse(upData)
		if parseErr != nil {
			continue
		}
		upIss.Blocks = uuids.RemoveUUID(upIss.Blocks, mergeUUID)
		if !uuids.ContainsUUID(upIss.Blocks, issUUID) {
			upIss.Blocks = append(upIss.Blocks, issUUID)
		}
		upIss.Updated = now
		upOut, marshalErr := issue.Marshal(upIss)
		if marshalErr != nil {
			continue
		}
		_ = app.Store.WriteEntity(upRef, "issue.md", upOut,
			fmt.Sprintf("Edit issue %s: blocks transferred from %s to %s",
				upUUID.String()[:8], mergeUUIDStr[:8], uuidStr[:8]))
	}

	iss.Updated = now

	// Write the updated primary issue.
	out, err := issue.Marshal(iss)
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	if err := app.Store.WriteEntity(refPath, "issue.md", out, fmt.Sprintf("Merge issue %s into %s", mergeUUIDStr[:8], uuidStr[:8])); err != nil {
		return fmt.Errorf("write issue: %w", err)
	}

	// Mark the merged issue as cancelled and clear its edges.
	mergeIss.State = issue.StateCancelled
	mergeIss.Blocks = nil
	mergeIss.BlockedBy = nil
	mergeIss.Updated = now
	mergeOut, err := issue.Marshal(mergeIss)
	if err != nil {
		return fmt.Errorf("marshal merge issue: %w", err)
	}
	if err := app.Store.WriteEntity(mergeRef, "issue.md", mergeOut, fmt.Sprintf("Cancel issue %s: merged into %s", mergeUUIDStr[:8], uuidStr[:8])); err != nil {
		return fmt.Errorf("write merge issue: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Merged %s into %s:\n", mergeUUIDStr[:8], uuidStr[:8])
	fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", uuidStr[:8], iss.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "\nDependencies from %s transferred to %s.\n", mergeUUIDStr[:8], uuidStr[:8])

	return nil
}

// runIssueEditPurge handles 'issue edit <ref> --purge'. Permanently deletes
// the issue ref and cleans up all dependency edges. Requires --yes flag.
func runIssueEditPurge(cmd *cobra.Command, app *App, refInput string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("--purge requires --yes flag for confirmation")
	}

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
	issUUID, err := uuid.FromString(uuidStr)
	if err != nil {
		return fmt.Errorf("parse uuid: %w", err)
	}

	now := time.Now()

	// Remove this issue from each upstream issue's Blocks list.
	for _, upUUID := range iss.BlockedBy {
		upRef := issue.RefPrefix + upUUID.String()
		upData, readErr := app.Store.ReadEntity(upRef, "issue.md")
		if readErr != nil {
			continue
		}
		upIss, parseErr := issue.Parse(upData)
		if parseErr != nil {
			continue
		}
		upIss.Blocks = uuids.RemoveUUID(upIss.Blocks, issUUID)
		upIss.Updated = now
		upOut, marshalErr := issue.Marshal(upIss)
		if marshalErr != nil {
			continue
		}
		_ = app.Store.WriteEntity(upRef, "issue.md", upOut,
			fmt.Sprintf("Edit issue %s: remove blocks %s (purged)", upUUID.String()[:8], uuidStr[:8]))
	}

	// Remove this issue from each downstream issue's BlockedBy list.
	for _, downUUID := range iss.Blocks {
		downRef := issue.RefPrefix + downUUID.String()
		downData, readErr := app.Store.ReadEntity(downRef, "issue.md")
		if readErr != nil {
			continue
		}
		downIss, parseErr := issue.Parse(downData)
		if parseErr != nil {
			continue
		}
		downIss.BlockedBy = uuids.RemoveUUID(downIss.BlockedBy, issUUID)
		downIss.Updated = now
		downOut, marshalErr := issue.Marshal(downIss)
		if marshalErr != nil {
			continue
		}
		_ = app.Store.WriteEntity(downRef, "issue.md", downOut,
			fmt.Sprintf("Edit issue %s: remove blocked_by %s (purged)", downUUID.String()[:8], uuidStr[:8]))
	}

	// Clean up any tags that point to this issue's ref.
	tagRefs, listErr := app.Store.ListRefs("refs/zhi/_/tags/")
	if listErr == nil {
		for _, tagRef := range tagRefs {
			tagData, readErr := app.Store.ReadEntity(tagRef, "tag.txt")
			if readErr != nil {
				continue
			}
			if strings.TrimSpace(string(tagData)) == refPath {
				_ = app.Store.DeleteRef(tagRef)
			}
		}
	}

	// Delete the issue ref.
	if err := app.Store.DeleteRef(refPath); err != nil {
		return fmt.Errorf("delete issue ref: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Purged %s: %s (permanently deleted)\n", uuidStr[:8], iss.Title)

	return nil
}

// batchOp is a single operation parsed from a --batch JSON line.
// Fields maps field names to their raw JSON values so each can be decoded
// individually without requiring all fields to be present at once.
type batchOp struct {
	IssueID string                     `json:"issue_id"`
	Fields  map[string]json.RawMessage `json:"fields"`
}

// runIssueEditBatch handles 'issue edit --batch'. Reads JSON-line operations
// from stdin, applies each field update to the named issue, and writes one
// result line per operation. Errors per operation are reported without aborting
// the remaining operations.
func runIssueEditBatch(cmd *cobra.Command, app *App) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNum++

		var op batchOp
		if err := json.Unmarshal([]byte(line), &op); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "%d error: parse JSON: %v\n", lineNum, err)
			continue
		}

		if err := applyBatchOp(app, op); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "%d error: %s: %v\n", lineNum, op.IssueID, err)
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%d ok: %s\n", lineNum, op.IssueID)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	return nil
}

// applyBatchOp resolves the issue identified by op.IssueID, applies all
// fields from op.Fields, and writes the updated issue back to storage.
// State changes use ValidateTransition and record a Transition entry.
func applyBatchOp(app *App, op batchOp) error {
	refPath, err := resolve.ResolveRef(app.Store, op.IssueID)
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

	dirty := false
	now := time.Now()

	// Apply each supported field.
	for fieldName, rawVal := range op.Fields {
		switch fieldName {
		case "title":
			var v string
			if err := json.Unmarshal(rawVal, &v); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			iss.Title = v
			dirty = true

		case "assigned":
			var v string
			if err := json.Unmarshal(rawVal, &v); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			iss.Assigned = v
			dirty = true

		case "urgency":
			var v string
			if err := json.Unmarshal(rawVal, &v); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			switch issue.Urgency(v) {
			case issue.UrgencyHigh, issue.UrgencyNormal, issue.UrgencyLow:
				// valid
			default:
				return fmt.Errorf("field %q: invalid urgency %q (must be high, normal, or low)", fieldName, v)
			}
			iss.Urgency = issue.Urgency(v)
			dirty = true

		case "labels":
			var v []string
			if err := json.Unmarshal(rawVal, &v); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			iss.Labels = v
			dirty = true

		case "tracker_id":
			var v string
			if err := json.Unmarshal(rawVal, &v); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			iss.TrackerID = v
			dirty = true

		case "last_synced_at":
			var v time.Time
			if err := json.Unmarshal(rawVal, &v); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			iss.LastSyncedAt = &v
			dirty = true

		case "state":
			var action string
			if err := json.Unmarshal(rawVal, &action); err != nil {
				return fmt.Errorf("field %q: %w", fieldName, err)
			}
			newState, transErr := issue.ValidateTransition(iss.State, action)
			if transErr != nil {
				return transErr
			}
			// Record the transition with actor identity derived from git config.
			authorName, authorEmail := app.Store.AuthorInfo()
			a := actor.DeriveActor(authorName, authorEmail)
			iss.Transitions = append(iss.Transitions, issue.Transition{
				State:     string(newState),
				Actor:     a.String(),
				Timestamp: now,
			})
			iss.State = newState
			dirty = true

		default:
			return fmt.Errorf("unsupported field %q in batch operation", fieldName)
		}
	}

	if !dirty {
		return nil
	}

	iss.Updated = now
	out, err := issue.Marshal(iss)
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	commitMsg := fmt.Sprintf("Batch edit issue %s", uuidStr[:8])
	if err := app.Store.WriteEntity(refPath, "issue.md", out, commitMsg); err != nil {
		return fmt.Errorf("write issue: %w", err)
	}
	return nil
}

