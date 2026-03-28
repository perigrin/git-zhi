// ABOUTME: Implementation of the git zhi status command: shows current HEAD
// ABOUTME: issue, session info, milestone, and ready set count at a glance.
package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
)

// NewStatusCommand creates the top-level "status" subcommand.
func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current work state at a glance",
		Long: `Displays the current HEAD issue (if any), its state, milestone, session
info, and how many issues are ready to start.`,
		RunE: runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")

	if len(allIssues) == 0 {
		if format == "json" {
			return writeJSON(cmd, statusJSON{Message: "No issues in chain"})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "No issues in chain.")
		return nil
	}

	g := graph.New(allIssues)
	readySet := g.ReadySet()
	readyCount := len(readySet)

	head, headErr := g.Head("")

	if format == "json" {
		return printStatusJSON(cmd, head, headErr, app, readyCount, g)
	}

	return printStatusText(cmd, head, headErr, app, readyCount, g)
}

// statusJSON is the JSON output structure for git zhi status.
type statusJSON struct {
	Message    string      `json:"message,omitempty"`
	Head       string      `json:"head,omitempty"`
	Title      string      `json:"title,omitempty"`
	State      string      `json:"state,omitempty"`
	Milestone  string      `json:"milestone,omitempty"`
	Session    *sessionJSON `json:"session,omitempty"`
	ReadyCount int         `json:"ready_count"`
	Next       string      `json:"next,omitempty"`
}

type sessionJSON struct {
	StartedAt *time.Time `json:"started_at,omitempty"`
	Commits   int        `json:"commits"`
}

func printStatusJSON(cmd *cobra.Command, head *issue.Issue, headErr error, app *App, readyCount int, g *graph.Graph) error {
	out := statusJSON{ReadyCount: readyCount}

	if headErr != nil || head == nil {
		out.Message = "No issue in progress"
		ready := g.ReadySet()
		if len(ready) > 0 {
			out.Next = ready[0].ID.String()[:8]
		}
	} else {
		out.Head = head.ID.String()[:8]
		out.Title = head.Title
		out.State = string(head.State)
		out.Milestone = head.Milestone
		out.Session = openSessionInfo(head, app)
	}

	return writeJSON(cmd, out)
}

func writeJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printStatusText(cmd *cobra.Command, head *issue.Issue, headErr error, app *App, readyCount int, g *graph.Graph) error {
	w := cmd.OutOrStdout()

	if headErr != nil || head == nil || head.State != issue.StateInProgress {
		fmt.Fprintln(w, "No issue in progress.")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Ready: %d issues unblocked\n", readyCount)
		ready := g.ReadySet()
		if len(ready) > 0 {
			fmt.Fprintf(w, "Next:  %s %q\n", ready[0].ID.String()[:8], ready[0].Title)
		}
		return nil
	}

	fmt.Fprintf(w, "On issue %s %q\n", head.ID.String()[:8], head.Title)
	fmt.Fprintf(w, "  State:     %s\n", head.State)
	fmt.Fprintf(w, "  Milestone: %s\n", head.Milestone)

	si := openSessionInfo(head, app)
	if si != nil {
		if si.StartedAt != nil {
			fmt.Fprintf(w, "  Session:   started %s, %d commits\n",
				si.StartedAt.Format(time.RFC3339), si.Commits)
		} else {
			fmt.Fprintf(w, "  Session:   %d commits\n", si.Commits)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Ready: %d issues unblocked\n", readyCount)

	return nil
}

// openSessionInfo returns session details for the currently open session
// (no EndSHA), computing the live commit count from the session's StartSHA
// to the current repo HEAD.
func openSessionInfo(iss *issue.Issue, app *App) *sessionJSON {
	for i := len(iss.Sessions) - 1; i >= 0; i-- {
		sess := iss.Sessions[i]
		if sess.EndSHA == "" {
			// Open session — compute live commit count.
			commits := 0
			if sess.StartSHA != "" {
				if currentHEAD, err := app.Store.RepoHEAD(); err == nil {
					commits, _ = app.Store.CountCommits(sess.StartSHA, currentHEAD)
				}
			}
			return &sessionJSON{
				StartedAt: sess.StartedAt,
				Commits:   commits,
			}
		}
	}
	return nil
}
