// ABOUTME: Cobra command group for issue subcommands (add, list, show, edit).
// ABOUTME: Each subcommand is a factory function returning a *cobra.Command.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	yaml "github.com/goccy/go-yaml"

	"github.com/perigrin/git-zhi/internal/config"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/resolve"
)

// NewIssueCommand creates the 'issue' command group.
func NewIssueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage issues in the chain",
		// NoArgs turns a mistyped subcommand into an error naming it,
		// instead of help text and a zero exit that a script reads as success.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newIssueAddCommand(),
		newIssueListCommand(),
		newIssueShowCommand(),
		newIssueEditCommand(),
	)

	return cmd
}

func newIssueAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create one or more new issues",
		RunE:  runIssueAdd,
	}

	cmd.Flags().String("milestone", "", "milestone to assign (overrides config default)")
	cmd.Flags().String("after", "", "issue ref that this issue depends on")
	cmd.Flags().String("before", "", "issue ref that depends on this issue")
	cmd.Flags().StringSlice("label", nil, "add a label to the new issue (repeatable)")
	cmd.Flags().String("body", "", "issue body text (used with positional title arg; overrides stdin)")
	cmd.Flags().String("body-file", "", "read the issue body from a file (mirrors git commit -F)")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")

	return cmd
}

func runIssueAdd(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("initialize chain: %w", err)
	}

	// Determine default milestone from config
	defaultMilestone := config.Default().DefaultMilestone
	cfgData, err := app.Store.ReadEntity("refs/zhi/_/config", "config.yaml")
	if err == nil {
		var cfg config.Config
		if yamlErr := yaml.Unmarshal(cfgData, &cfg); yamlErr == nil && cfg.DefaultMilestone != "" {
			defaultMilestone = cfg.DefaultMilestone
		}
	}

	// --milestone flag overrides config default
	if ms, _ := cmd.Flags().GetString("milestone"); ms != "" {
		defaultMilestone = ms
	}

	// --label may be repeated. Validate every name before anything is written,
	// so a bad label leaves no half-created issue behind.
	flagLabels, _ := cmd.Flags().GetStringSlice("label")
	for _, label := range flagLabels {
		if err := issue.ValidateLabelName(label); err != nil {
			return err
		}
	}

	// --after / --before supply a dependency for issues that do not name one
	// themselves, the same way --milestone yields to a frontmatter milestone.
	afterFlag, _ := cmd.Flags().GetString("after")
	beforeFlag, _ := cmd.Flags().GetString("before")

	// Determine input source: --body / --body-file take precedence over stdin.
	bodyFlag, _ := cmd.Flags().GetString("body")
	bodyFile, _ := cmd.Flags().GetString("body-file")
	if bodyFile != "" {
		raw, readErr := os.ReadFile(bodyFile)
		if readErr != nil {
			return fmt.Errorf("--body-file: %w", readErr)
		}
		bodyFlag = strings.TrimSpace(string(raw))
		if bodyFlag == "" {
			return fmt.Errorf("--body-file %s is empty", bodyFile)
		}
	}
	if bodyFlag != "" && len(args) == 0 {
		return fmt.Errorf("a title argument is required when the body comes from a flag: git zhi issue add \"Title\" --body-file <path>")
	}

	var blocks [][]byte
	if bodyFlag != "" && len(args) > 0 {
		// Build a synthetic frontmatter block from args (title) and --body flag.
		synthetic := fmt.Sprintf("---\ntitle: %q\n---\n\n%s\n", args[0], bodyFlag)
		blocks = [][]byte{[]byte(synthetic)}
	} else {
		// Read stdin
		raw, readErr := io.ReadAll(cmd.InOrStdin())
		if readErr != nil {
			return fmt.Errorf("read stdin: %w", readErr)
		}
		blocks = issue.SplitBatch(raw)
		if len(blocks) == 0 {
			return fmt.Errorf("no issue content found on stdin")
		}
	}

	now := time.Now()
	var created []*issue.Issue

	for i, block := range blocks {
		iss, parseErr := issue.Parse(block)
		if parseErr != nil {
			return fmt.Errorf("parse issue: %w", parseErr)
		}

		if strings.TrimSpace(iss.Title) == "" {
			return fmt.Errorf("issue %d: title is required", i+1)
		}

		// Generate UUIDv7 for stable, time-ordered identity
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return fmt.Errorf("generate uuid: %w", idErr)
		}
		iss.ID = id

		// Apply defaults
		if iss.State == "" {
			iss.State = issue.StatePending
		}
		if iss.Milestone == "" {
			iss.Milestone = defaultMilestone
		}
		if iss.After == "" {
			iss.After = afterFlag
		}
		if iss.Before == "" {
			iss.Before = beforeFlag
		}
		for _, label := range flagLabels {
			if !slices.Contains(iss.Labels, label) {
				iss.Labels = append(iss.Labels, label)
			}
		}
		iss.Created = now
		iss.Updated = now

		created = append(created, iss)
	}

	// Resolve explicit dependencies from After/Before frontmatter fields.
	// Build a title→issue index for intra-batch resolution.
	titleIndex := make(map[string]*issue.Issue)
	createdIDs := make(map[uuid.UUID]struct{}, len(created))
	for _, iss := range created {
		titleIndex[iss.Title] = iss
		createdIDs[iss.ID] = struct{}{}
	}

	// An After/Before target may be an issue that already exists rather than
	// one in this batch. Such a target gains a reverse edge and so must be
	// written back, and every reference to it has to reach the same in-memory
	// issue — resolveBatchDep reloads from storage on each call, so two new
	// issues pointing at one existing target would otherwise each hold a copy
	// carrying only their own edge, and persisting them would lose one.
	existingTargets := make(map[uuid.UUID]*issue.Issue)
	resolveDep := func(ref string) (*issue.Issue, error) {
		target, err := resolveBatchDep(ref, titleIndex, app)
		if err != nil {
			return nil, err
		}
		if _, inBatch := createdIDs[target.ID]; inBatch {
			return target, nil
		}
		if cached, ok := existingTargets[target.ID]; ok {
			return cached, nil
		}
		existingTargets[target.ID] = target
		return target, nil
	}

	for _, iss := range created {
		if iss.After != "" {
			target, resolveErr := resolveDep(iss.After)
			if resolveErr != nil {
				return fmt.Errorf("issue %q: after: %w", iss.Title, resolveErr)
			}
			target.Blocks = append(target.Blocks, iss.ID)
			iss.BlockedBy = append(iss.BlockedBy, target.ID)
		}
		if iss.Before != "" {
			target, resolveErr := resolveDep(iss.Before)
			if resolveErr != nil {
				return fmt.Errorf("issue %q: before: %w", iss.Title, resolveErr)
			}
			iss.Blocks = append(iss.Blocks, target.ID)
			target.BlockedBy = append(target.BlockedBy, iss.ID)
		}
	}

	// Capture dep annotations before Marshal clears After/Before fields.
	depAnnotations := make(map[uuid.UUID]string)
	for _, iss := range created {
		if iss.After != "" {
			depAnnotations[iss.ID] = fmt.Sprintf(" (after: %s)", iss.After)
		} else if iss.Before != "" {
			depAnnotations[iss.ID] = fmt.Sprintf(" (before: %s)", iss.Before)
		}
	}

	// A milestone ref exists only if an issue names it, or a human ran
	// `milestone add`. EnsureInitialized no longer creates one eagerly, so
	// the first issue naming a milestone with no ref creates it here.
	seenMilestones := make(map[string]struct{})
	for _, iss := range created {
		if iss.Milestone == "" {
			continue
		}
		if _, ok := seenMilestones[iss.Milestone]; ok {
			continue
		}
		seenMilestones[iss.Milestone] = struct{}{}

		refPath := milestone.RefPrefix + iss.Milestone
		if app.Store.RefExists(refPath) {
			continue
		}
		ms := &milestone.Milestone{
			Name:    iss.Milestone,
			Created: now,
		}
		msData, marshalErr := milestone.MarshalMilestone(ms)
		if marshalErr != nil {
			return fmt.Errorf("marshal milestone %s: %w", iss.Milestone, marshalErr)
		}
		if writeErr := app.Store.WriteEntity(refPath, "milestone.yaml", msData, "Create milestone: "+iss.Milestone); writeErr != nil {
			return fmt.Errorf("write milestone %s: %w", iss.Milestone, writeErr)
		}
	}

	// Persist each issue
	for _, iss := range created {
		data, marshalErr := issue.Marshal(iss)
		if marshalErr != nil {
			return fmt.Errorf("marshal issue %s: %w", iss.ID, marshalErr)
		}
		refPath := issue.RefPrefix + iss.ID.String()
		if writeErr := app.Store.WriteEntity(refPath, "issue.md", data, "Add issue: "+iss.Title); writeErr != nil {
			return fmt.Errorf("write issue %s: %w", iss.ID, writeErr)
		}
	}

	// Persist the pre-existing issues that gained an edge. Without this the
	// graph is half-wired: the new issue records its blocker, while the
	// blocker never records what it now blocks.
	for _, target := range existingTargets {
		data, marshalErr := issue.Marshal(target)
		if marshalErr != nil {
			return fmt.Errorf("marshal issue %s: %w", target.ID, marshalErr)
		}
		refPath := issue.RefPrefix + target.ID.String()
		if writeErr := app.Store.WriteEntity(refPath, "issue.md", data,
			"Add dependency edge: "+target.Title); writeErr != nil {
			return fmt.Errorf("write issue %s: %w", target.ID, writeErr)
		}
	}

	// Output
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(created)
	}

	// Human-readable output
	if len(created) == 1 {
		iss := created[0]
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s: %s\n", iss.ID.String()[:8], iss.Title)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Created %d issues:\n", len(created))
		for _, iss := range created {
			annotation := depAnnotations[iss.ID]
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s%s\n", iss.ID.String()[:8], iss.Title, annotation)
		}
	}

	return nil
}

// resolveBatchDep resolves a dependency reference from an After or Before
// field. Resolution order: exact title match in the batch, then existing
// issue ref. Errors on duplicate title match or no match.
func resolveBatchDep(ref string, titleIndex map[string]*issue.Issue, app *App) (*issue.Issue, error) {
	// 1. Exact title match in batch
	if target, ok := titleIndex[ref]; ok {
		return target, nil
	}

	// 2. Check for duplicate titles (substring match could be ambiguous,
	// but we only do exact match — duplicates mean two issues have the
	// same title in the batch).
	// Already handled by map: last-write wins. But if the caller wants
	// duplicate detection, they should check before calling us. For now,
	// exact match is sufficient — the PRD says "exact title match."

	// 3. Try resolving as an existing issue ref (UUID prefix). Collect every
	// match rather than taking the first: the 8-char prefix is the top 32 bits
	// of a UUIDv7 millisecond timestamp, so issues created within about a
	// minute of each other share it. Returning the first would wire a
	// dependency edge to an arbitrary issue, which is worse than refusing.
	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return nil, fmt.Errorf("load existing issues: %w", err)
	}
	var matches []*issue.Issue
	for _, existing := range allIssues {
		if strings.HasPrefix(existing.ID.String(), ref) {
			matches = append(matches, existing)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no issue found matching %q (not in batch, not an existing ref)", ref)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID.String()
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("%w: prefix %q matches %d issues: %s",
			resolve.ErrAmbiguous, ref, len(matches), strings.Join(ids, ", "))
	}
}

func newIssueListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		Long: `List issues in the chain.

By default only pending and in-progress issues are shown; pass --all (or
--state all) to include done, cancelled and reopened ones. This is why a
completed milestone lists nothing while milestone show still reports its
issues.

--ready narrows the list to work that can be picked up now: pending or
reopened issues whose blockers are all done or cancelled. It spans every
milestone unless --milestone scopes it, and readiness is computed over the
whole graph, so a blocker excluded by a filter still counts.

JSON output (--format json) names dependency edges:

  blocks      issues this one blocks (forward edges)
  blocked_by  issues that block this one (reverse edges)

Both are arrays of full 36-character UUIDs, and both are omitted when empty —
so an issue with no blockers has no blocked_by key at all.`,
		RunE: runIssueList,
	}
	cmd.Flags().Bool("all", false, "include done and cancelled issues")
	cmd.Flags().String("milestone", "", "filter by milestone")
	cmd.Flags().String("state", "", "filter by state (pending, in-progress, done, cancelled, reopened)")
	cmd.Flags().String("label", "", "filter by label")
	cmd.Flags().String("assigned", "", "filter by assigned worker")
	cmd.Flags().Bool("ready", false, "only issues whose blockers are all done or cancelled")
	return cmd
}

func newIssueShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [ref]",
		Short: "View an issue with full context",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runIssueShow,
	}
}

func newIssueEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <ref>",
		Short: "Modify an issue",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runIssueEdit,
	}
	cmd.Flags().String("state", "", "transition state: start, pause, resume, done, cancel")
	cmd.Flags().String("title", "", "rename the issue in place, keeping its id and graph edges")
	cmd.Flags().String("block", "", "add forward dependency: this issue blocks <ref>")
	cmd.Flags().String("unblock", "", "remove forward dependency")
	cmd.Flags().String("milestone", "", "move to different milestone")
	cmd.Flags().String("tag", "", "add a human-readable tag")
	cmd.Flags().String("untag", "", "remove a tag")
	cmd.Flags().String("label", "", "add a label to this issue")
	cmd.Flags().String("unlabel", "", "remove a label from this issue")
	cmd.Flags().String("assign", "", "assign this issue to a worker (e.g. agent:claude-code-1)")
	cmd.Flags().Bool("unassign", false, "clear the assignment on this issue")
	cmd.Flags().String("before", "", "position before another issue (this issue blocks <ref>)")
	cmd.Flags().String("after", "", "position after another issue (<ref> blocks this issue)")
	cmd.Flags().Bool("split", false, "split into multiple issues (reads replacement content from stdin)")
	cmd.Flags().String("merge", "", "merge another issue into this one")
	cmd.Flags().Bool("purge", false, "permanently delete this issue (requires --yes)")
	cmd.Flags().Bool("yes", false, "confirm destructive operations (required for --purge)")
	cmd.Flags().Bool("batch", false, "read a stream of JSON edit operations from stdin and apply them in bulk")
	cmd.Flags().String("body", "", "replace the issue body: inline text, '-' to read from stdin, or '' to open $EDITOR")
	cmd.Flags().String("body-file", "", "replace the issue body with a file's contents (mirrors git commit -F)")
	cmd.Flags().Bool("force", false, "override safety checks (e.g. allow --state done with zero commits)")
	cmd.MarkFlagsMutuallyExclusive("split", "merge", "purge")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")
	return cmd
}
