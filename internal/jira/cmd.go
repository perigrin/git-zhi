// ABOUTME: Cobra command tree for the git-zhi-jira binary. Wires jira sync pull/push,
// ABOUTME: dry-run preview, conflict resolution, single-ticket fetch, and enrich subcommands.
package jira

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	jclient "github.com/perigrin/git-zhi/internal/jira/client"
	"github.com/perigrin/git-zhi/internal/jira/credentials"
	jirasync "github.com/perigrin/git-zhi/internal/jira/sync"
)

// defaultStateMapping is the fallback mapping from zhi state names to Jira
// status names. Callers may override via git config zhi.sync.jira.state.<name>.
var defaultStateMapping = jirasync.StateMapping{
	"done":        "Done",
	"in-progress": "In Progress",
	"pending":     "To Do",
	"cancelled":   "Won't Do",
}

// NewJiraCommand creates the top-level 'jira' Cobra command with all
// subcommands registered. The --snapshot-dir and --jira-url flags are
// persistent so every subcommand inherits them; they exist primarily to
// enable test injection without touching environment variables.
func NewJiraCommand() *cobra.Command {
	var snapshotDir string
	var jiraURL string

	root := &cobra.Command{
		Use:   "git-zhi-jira [ticket-key]",
		Short: "Jira Cloud sync and ticket import for git-zhi",
		// DisableFlagParsing is not needed; Args: cobra.ArbitraryArgs prevents
		// Cobra from treating the first positional argument as a subcommand name
		// when it does not match any registered subcommand.
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		// RunE handles the positional 'git zhi jira <ticket-key>' form.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runFetchTicket(cmd, args[0], jiraURL)
		},
	}

	root.PersistentFlags().StringVar(&snapshotDir, "snapshot-dir", "", "directory for sync snapshots (default: .git/zhi-sync)")
	root.PersistentFlags().StringVar(&jiraURL, "jira-url", "", "Jira base URL (overrides ZHI_JIRA_URL and git config)")

	// jira sync [pull|push|--dry-run]
	root.AddCommand(newSyncCommand(&snapshotDir, &jiraURL))
	// jira resolve <ticket> [--keep-zhi|--keep-tracker]
	root.AddCommand(newResolveCommand(&snapshotDir, &jiraURL))
	// jira enrich <milestone>
	root.AddCommand(newEnrichCommand(&snapshotDir, &jiraURL))

	return root
}

// ---------------------------------------------------------------------------
// Credential resolution helper
// ---------------------------------------------------------------------------

// resolveCredentials builds a Jira client from environment variables, git
// config, and the optional --jira-url flag override. The jiraURLOverride
// parameter is non-empty only when --jira-url is passed on the command line
// or in tests.
func resolveCredentials(cmd *cobra.Command, jiraURLOverride string) (*jclient.Client, error) {
	app := cli.GetApp(cmd.Context())

	// Read config values from git config as fallbacks.
	cfgToken, cfgEmail, cfgURL := "", "", ""
	if app != nil && app.Repo != nil {
		repoCfg, err := app.Repo.Config()
		if err == nil && repoCfg.Raw != nil {
			sec := repoCfg.Raw.Section("zhi")
			if sec != nil {
				sub := sec.Subsection("sync.jira")
				if sub != nil {
					cfgToken = sub.Option("token")
					cfgEmail = sub.Option("email")
					cfgURL = sub.Option("url")
				}
			}
		}
	}

	// --jira-url flag overrides everything for the URL.
	if jiraURLOverride != "" {
		cfgURL = jiraURLOverride
	}

	creds, err := credentials.Load(cfgToken, cfgEmail, cfgURL)
	if err != nil {
		return nil, err
	}

	return jclient.NewClient(creds.URL, creds.Email, creds.Token), nil
}

// resolveSnapshotDir returns the effective snapshot directory: the --snapshot-dir
// flag if set, otherwise .git/zhi-sync in the repo root.
func resolveSnapshotDir(cmd *cobra.Command, snapDirFlag string) (string, error) {
	if snapDirFlag != "" {
		return snapDirFlag, nil
	}
	app := cli.GetApp(cmd.Context())
	if app != nil && app.Repo != nil {
		wt, err := app.Repo.Worktree()
		if err == nil {
			return filepath.Join(wt.Filesystem.Root(), ".git", "zhi-sync"), nil
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve snapshot dir: %w", err)
	}
	return filepath.Join(dir, ".git", "zhi-sync"), nil
}

// ---------------------------------------------------------------------------
// jira <ticket-key>
// ---------------------------------------------------------------------------

// runFetchTicket fetches a single Jira ticket by key and emits issue YAML to
// stdout, suitable for piping to 'git zhi issue add'.
func runFetchTicket(cmd *cobra.Command, key, jiraURLOverride string) error {
	client, err := resolveCredentials(cmd, jiraURLOverride)
	if err != nil {
		return err
	}

	ji, err := client.GetIssue(key)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", key, err)
	}

	now := time.Now()
	iss := &issue.Issue{
		Title:       ji.Summary,
		State:       issue.StatePending,
		Urgency:     mapJiraPriorityToUrgency(ji.Priority),
		Source:      "tracker-match",
		TrackerID:   "jira:" + ji.Key,
		Labels:      ji.Labels,
		Created:     now,
		Updated:     now,
		Transitions: []issue.Transition{},
		ObservedPaths: []string{},
	}
	if iss.Labels == nil {
		iss.Labels = []string{}
	}
	// Description becomes the Body (markdown section).
	iss.Body = ji.Description

	data, err := issue.Marshal(iss)
	if err != nil {
		return fmt.Errorf("marshal issue: %w", err)
	}
	_, err = cmd.OutOrStdout().Write(data)
	return err
}

// mapJiraPriorityToUrgency converts a Jira priority string to an issue.Urgency.
func mapJiraPriorityToUrgency(priority string) issue.Urgency {
	switch strings.ToLower(priority) {
	case "highest", "high":
		return issue.UrgencyHigh
	case "lowest", "low":
		return issue.UrgencyLow
	default:
		return issue.UrgencyNormal
	}
}

// ---------------------------------------------------------------------------
// jira sync
// ---------------------------------------------------------------------------

func newSyncCommand(snapshotDir *string, jiraURL *string) *cobra.Command {
	var dryRun bool

	syncCmd := &cobra.Command{
		Use:           "sync",
		Short:         "Bidirectional sync between Jira and git-zhi",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 'jira sync' alone (without pull/push) runs --dry-run preview if
			// that flag is set, or prints help otherwise.
			if dryRun {
				return runSyncDryRun(cmd, *snapshotDir, *jiraURL)
			}
			return cmd.Help()
		},
	}

	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview pull and push without executing")

	syncCmd.AddCommand(newSyncPullCommand(snapshotDir, jiraURL))
	syncCmd.AddCommand(newSyncPushCommand(snapshotDir, jiraURL))

	return syncCmd
}

func newSyncPullCommand(snapshotDir *string, jiraURL *string) *cobra.Command {
	return &cobra.Command{
		Use:           "pull",
		Short:         "Pull field changes from Jira into git-zhi (outputs batch edit JSON)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncPull(cmd, *snapshotDir, *jiraURL)
		},
	}
}

func newSyncPushCommand(snapshotDir *string, jiraURL *string) *cobra.Command {
	return &cobra.Command{
		Use:           "push",
		Short:         "Push zhi state changes to Jira",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncPush(cmd, *snapshotDir, *jiraURL)
		},
	}
}

// runSyncPull fetches Jira state for all issues with a tracker_id, computes
// the diff, and emits batch edit JSON lines to stdout.
func runSyncPull(cmd *cobra.Command, snapDirFlag, jiraURLOverride string) error {
	app := cli.GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("not in a git repository")
	}

	client, err := resolveCredentials(cmd, jiraURLOverride)
	if err != nil {
		return err
	}

	snapDir, err := resolveSnapshotDir(cmd, snapDirFlag)
	if err != nil {
		return err
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	result, err := jirasync.Pull(client, issues, snapDir)
	if err != nil {
		return fmt.Errorf("sync pull: %w", err)
	}

	data := jirasync.FormatBatchEdits(result.Pulled)
	if len(data) > 0 {
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}
	return nil
}

// runSyncPush sends zhi state changes to Jira for all issues with a tracker_id.
func runSyncPush(cmd *cobra.Command, snapDirFlag, jiraURLOverride string) error {
	app := cli.GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("not in a git repository")
	}

	client, err := resolveCredentials(cmd, jiraURLOverride)
	if err != nil {
		return err
	}

	snapDir, err := resolveSnapshotDir(cmd, snapDirFlag)
	if err != nil {
		return err
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	stateMapping := resolveStateMapping(cmd)

	result, err := jirasync.Push(client, issues, stateMapping, snapDir)
	if err != nil {
		return fmt.Errorf("sync push: %w", err)
	}

	for _, upd := range result.Pushed {
		fmt.Fprintf(cmd.OutOrStdout(), "pushed %s: %s → %s\n", upd.TrackerKey, upd.Field, upd.Value)
	}
	if len(result.Pushed) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "nothing to push")
	}
	return nil
}

// runSyncDryRun prints a preview of what pull and push would do without making
// any HTTP calls or writing snapshots.
func runSyncDryRun(cmd *cobra.Command, snapDirFlag, jiraURLOverride string) error {
	app := cli.GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("not in a git repository")
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	stateMapping := resolveStateMapping(cmd)

	// Preview pull: list issues with tracker IDs.
	fmt.Fprintln(cmd.OutOrStdout(), "[dry-run] pull: would fetch Jira state for:")
	trackerCount := 0
	for _, iss := range issues {
		if iss.TrackerID != "" && strings.HasPrefix(iss.TrackerID, "jira:") {
			key := strings.TrimPrefix(iss.TrackerID, "jira:")
			fmt.Fprintf(cmd.OutOrStdout(), "  %s ← %s\n", iss.ID.String()[:8], key)
			trackerCount++
		}
	}
	if trackerCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  (no issues with Jira tracker_id)")
	}

	// Preview push: list issues whose state has a mapping.
	fmt.Fprintln(cmd.OutOrStdout(), "[dry-run] push: would transition in Jira:")
	pushCount := 0
	for _, iss := range issues {
		if iss.TrackerID == "" || !strings.HasPrefix(iss.TrackerID, "jira:") {
			continue
		}
		if target, ok := stateMapping[string(iss.State)]; ok {
			key := strings.TrimPrefix(iss.TrackerID, "jira:")
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: state → %s\n", key, target)
			pushCount++
		}
	}
	if pushCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  (nothing to push)")
	}
	return nil
}

// resolveStateMapping builds the state mapping from git config with defaults.
func resolveStateMapping(cmd *cobra.Command) jirasync.StateMapping {
	mapping := make(jirasync.StateMapping)
	// Start with defaults.
	for k, v := range defaultStateMapping {
		mapping[k] = v
	}
	// Overlay git config overrides.
	app := cli.GetApp(cmd.Context())
	if app != nil && app.Repo != nil {
		repoCfg, err := app.Repo.Config()
		if err == nil && repoCfg.Raw != nil {
			sec := repoCfg.Raw.Section("zhi")
			if sec != nil {
				sub := sec.Subsection("sync.jira.state")
				if sub != nil {
					for _, opt := range sub.Options {
						mapping[opt.Key] = opt.Value
					}
				}
			}
		}
	}
	return mapping
}

// ---------------------------------------------------------------------------
// jira resolve
// ---------------------------------------------------------------------------

func newResolveCommand(snapshotDir *string, jiraURL *string) *cobra.Command {
	var keepZhi bool
	var keepTracker bool

	resolveCmd := &cobra.Command{
		Use:           "resolve <ticket>",
		Short:         "Resolve a sync conflict by choosing which side wins",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !keepZhi && !keepTracker {
				return fmt.Errorf("must specify either --keep-zhi or --keep-tracker")
			}
			if keepZhi && keepTracker {
				return fmt.Errorf("--keep-zhi and --keep-tracker are mutually exclusive")
			}
			return runResolve(cmd, args[0], keepZhi, *snapshotDir, *jiraURL)
		},
	}

	resolveCmd.Flags().BoolVar(&keepZhi, "keep-zhi", false, "resolve conflict by keeping the git-zhi value")
	resolveCmd.Flags().BoolVar(&keepTracker, "keep-tracker", false, "resolve conflict by taking the Jira value")

	return resolveCmd
}

// runResolve resolves a conflict for the given tracker key. When keepZhi is
// true, the current zhi value wins and we emit a batch edit that writes it
// back (re-anchoring the snapshot). When keepZhi is false, we fetch the
// current Jira value and emit a batch edit to apply it to zhi.
func runResolve(cmd *cobra.Command, trackerKey string, keepZhi bool, snapDirFlag, jiraURLOverride string) error {
	app := cli.GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("not in a git repository")
	}

	snapDir, err := resolveSnapshotDir(cmd, snapDirFlag)
	if err != nil {
		return err
	}

	// Find the issue with this tracker key.
	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	fullKey := "jira:" + trackerKey
	var target *issue.Issue
	for _, iss := range issues {
		if iss.TrackerID == fullKey {
			target = iss
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no issue found with tracker_id %q", fullKey)
	}

	if keepZhi {
		// Emit a batch edit that writes the zhi value back for each tracked field,
		// effectively re-anchoring the snapshot to the current zhi state.
		updates := []jirasync.PullUpdate{
			{IssueID: target.ID.String(), Field: "state", OldValue: "", NewValue: string(target.State)},
		}
		data := jirasync.FormatBatchEdits(updates)
		if len(data) > 0 {
			_, err = cmd.OutOrStdout().Write(data)
		}
		// Update the snapshot so subsequent syncs don't re-flag as conflict.
		snapshot := map[string]string{
			"state":    string(target.State),
			"urgency":  string(target.Urgency),
			"assigned": target.Assigned,
			"labels":   strings.Join(target.Labels, ","),
		}
		_ = jirasync.SaveSnapshot(snapDir, target.ID.String(), snapshot)
		return err
	}

	// --keep-tracker: fetch Jira value and emit batch edit to apply it.
	client, err := resolveCredentials(cmd, jiraURLOverride)
	if err != nil {
		return err
	}

	ji, err := client.GetIssue(trackerKey)
	if err != nil {
		return fmt.Errorf("fetch %s from Jira: %w", trackerKey, err)
	}

	jiraState := mapJiraStatusToZhi(ji.Status)

	updates := []jirasync.PullUpdate{
		{IssueID: target.ID.String(), Field: "state", OldValue: string(target.State), NewValue: jiraState},
	}
	data := jirasync.FormatBatchEdits(updates)
	if len(data) > 0 {
		_, err = cmd.OutOrStdout().Write(data)
	}
	// Update snapshot.
	snapshot := map[string]string{
		"state":    jiraState,
		"urgency":  string(target.Urgency),
		"assigned": target.Assigned,
		"labels":   strings.Join(target.Labels, ","),
	}
	_ = jirasync.SaveSnapshot(snapDir, target.ID.String(), snapshot)
	return err
}

// mapJiraStatusToZhi converts a Jira status name to a zhi state string.
// This duplicates the mapping in sync.go intentionally — the cmd package
// must not import sync internals for this small conversion.
func mapJiraStatusToZhi(status string) string {
	switch strings.ToLower(status) {
	case "to do", "open", "backlog", "new":
		return "pending"
	case "in progress", "in review":
		return "in-progress"
	case "done", "closed", "resolved":
		return "done"
	case "cancelled", "won't do", "wont do":
		return "cancelled"
	default:
		return strings.ToLower(status)
	}
}

// ---------------------------------------------------------------------------
// jira enrich
// ---------------------------------------------------------------------------

func newEnrichCommand(snapshotDir *string, jiraURL *string) *cobra.Command {
	return &cobra.Command{
		Use:           "enrich <milestone>",
		Short:         "Fetch Jira metadata for all issues in milestone and emit batch edits",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnrich(cmd, args[0], *snapshotDir, *jiraURL)
		},
	}
}

// runEnrich fetches Jira metadata for every issue in the milestone that has a
// tracker_id and emits batch edit JSON to update the title and description.
func runEnrich(cmd *cobra.Command, milestoneName, snapDirFlag, jiraURLOverride string) error {
	app := cli.GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("not in a git repository")
	}

	client, err := resolveCredentials(cmd, jiraURLOverride)
	if err != nil {
		return err
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	var updates []jirasync.PullUpdate
	for _, iss := range issues {
		if iss.Milestone != milestoneName {
			continue
		}
		if iss.TrackerID == "" || !strings.HasPrefix(iss.TrackerID, "jira:") {
			continue
		}
		key := strings.TrimPrefix(iss.TrackerID, "jira:")

		ji, err := client.GetIssue(key)
		if err != nil {
			// Log and continue — enrichment is best-effort.
			fmt.Fprintf(cmd.ErrOrStderr(), "warn: fetch %s: %v\n", key, err)
			continue
		}

		if ji.Summary != "" && ji.Summary != iss.Title {
			updates = append(updates, jirasync.PullUpdate{
				IssueID:  iss.ID.String(),
				Field:    "title",
				OldValue: iss.Title,
				NewValue: ji.Summary,
			})
		}
	}

	data := jirasync.FormatBatchEdits(updates)
	if len(data) > 0 {
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}
	return nil
}
