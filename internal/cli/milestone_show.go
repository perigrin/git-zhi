// ABOUTME: Implementation of the milestone show command: displays milestone
// ABOUTME: details with issue list, progress percentage, telemetry signals, and optional forecast.
package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/resolve"
	"github.com/perigrin/git-zhi/internal/telemetry"
)

// forecastPoint holds the estimated duration and context for a single worker count.
type forecastPoint struct {
	Workers     int     `json:"workers"`
	WeeksEst    float64 `json:"weeks_est"`
	CritIssues  int     `json:"critical_chain_issues"`
	ParallelTracks int  `json:"parallel_tracks"`
}

// workerCapacityEntry holds per-worker capacity information for the capacity summary.
type workerCapacityEntry struct {
	InProgress int `json:"in_progress"`
	Assigned   int `json:"assigned"`
}

// milestoneShowJSON is the JSON presentation of a milestone with its issues
// and computed telemetry.
type milestoneShowJSON struct {
	*milestone.Milestone
	Issues         []*issue.Issue                  `json:"issues"`
	TotalIssues    int                             `json:"total_issues"`
	DoneIssues     int                             `json:"done_issues"`
	Progress       int                             `json:"progress_pct"`
	Telemetry      *telemetry.Stats                `json:"telemetry"`
	Forecast       []forecastPoint                 `json:"forecast,omitempty"`
	WorkerCapacity map[string]*workerCapacityEntry `json:"worker_capacity,omitempty"`
}

// coordinationTaxPerPair is the fixed overhead fraction added per parallel pair.
const coordinationTaxPerPair = 0.10

// computeForecast estimates delivery time at each worker count from 1 to maxWorkers.
// The floor is always the sequential time of the critical chain (can't go faster
// than the longest path). For each additional worker beyond 1, the effective issue
// count is reduced by the ready set width (capped at remaining issues), with a
// 10% coordination tax per parallel pair.
//
// When speed is zero (no done issues yet), weeks are reported as 0 for all points.
func computeForecast(msIssues []*issue.Issue, stats *telemetry.Stats, maxWorkers int) []forecastPoint {
	if maxWorkers <= 0 {
		return nil
	}

	// Count remaining (non-done, non-cancelled) issues.
	var remaining []*issue.Issue
	for _, iss := range msIssues {
		if iss.State != issue.StateDone && iss.State != issue.StateCancelled {
			remaining = append(remaining, iss)
		}
	}
	remainingCount := len(remaining)

	// Build the graph for the remaining issues to get critical chain length and ready set.
	g := graph.New(remaining)
	critChain := g.CriticalChain()
	critLen := len(critChain)
	readySet := g.ReadySet()
	readyWidth := len(readySet)

	speed := stats.Speed // issues per week; 0 when no done issues exist

	// Sequential time is the baseline; parallel can only be faster down to the
	// floor set by the critical chain length.
	seqWeeks := 0.0
	if speed > 0 && remainingCount > 0 {
		seqWeeks = float64(remainingCount) / speed
	}
	critFloor := 0.0
	if speed > 0 && critLen > 0 {
		critFloor = float64(critLen) / speed
	}

	points := make([]forecastPoint, 0, maxWorkers)
	for w := 1; w <= maxWorkers; w++ {
		var weeksEst float64
		parallelTracks := 0

		if w == 1 || readyWidth <= 1 {
			// Single worker or no parallelism available: sequential time.
			weeksEst = seqWeeks
		} else {
			// Effective parallel workers capped at the ready set width.
			effectiveWorkers := w
			if effectiveWorkers > readyWidth {
				effectiveWorkers = readyWidth
			}
			parallelTracks = effectiveWorkers - 1

			// Coordination tax: 10% overhead per parallel pair.
			taxFactor := 1.0 + coordinationTaxPerPair*float64(parallelTracks)

			// Parallel speedup: divide sequential time by effective workers,
			// apply coordination tax, then bound below by critical chain floor.
			if speed > 0 {
				parallel := (seqWeeks / float64(effectiveWorkers)) * taxFactor
				weeksEst = math.Max(parallel, critFloor)
			}
		}

		points = append(points, forecastPoint{
			Workers:        w,
			WeeksEst:       math.Round(weeksEst*10) / 10, // round to 1 decimal
			CritIssues:     critLen,
			ParallelTracks: parallelTracks,
		})
	}

	return points
}

// computeWorkerCapacity builds a per-worker capacity map from the milestone's
// issues. For each issue:
//   - If in-progress, the last transition actor is credited with an in-progress count.
//   - If pending and assigned, the assignee is credited with an assigned count.
//
// Workers with no in-progress issues but at least one assigned issue are idle.
// Returns nil when no workers are found (no in-progress or assigned issues).
func computeWorkerCapacity(msIssues []*issue.Issue) map[string]*workerCapacityEntry {
	capacity := make(map[string]*workerCapacityEntry)

	for _, iss := range msIssues {
		if iss.State == issue.StateInProgress {
			actor := ""
			if len(iss.Transitions) > 0 {
				actor = iss.Transitions[len(iss.Transitions)-1].Actor
			}
			if actor != "" {
				if capacity[actor] == nil {
					capacity[actor] = &workerCapacityEntry{}
				}
				capacity[actor].InProgress++
			}
		}
		if iss.Assigned != "" && iss.State == issue.StatePending {
			if capacity[iss.Assigned] == nil {
				capacity[iss.Assigned] = &workerCapacityEntry{}
			}
			capacity[iss.Assigned].Assigned++
		}
	}

	if len(capacity) == 0 {
		return nil
	}
	return capacity
}

// runMilestoneShow loads a milestone by name (or the current milestone if no
// arg is provided) and displays its details with the associated issue list
// and computed telemetry signals.
func runMilestoneShow(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	milestones, err := milestone.LoadAllMilestones(app.Store)
	if err != nil {
		return fmt.Errorf("load milestones: %w", err)
	}

	if len(milestones) == 0 {
		return fmt.Errorf("no milestones found")
	}

	// Load all issues once; used for both current-milestone detection and filtering.
	allIssues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}

	// If no name given, resolve via the HEAD issue's milestone. Fall back to
	// scanning for the first milestone with pending/in-progress issues, then
	// to the first milestone in the list.
	var ms *milestone.Milestone
	if name == "" {
		// Attempt to derive the milestone from the current HEAD issue.
		headRef, headErr := resolve.ResolveRef(app.Store, "")
		if headErr == nil {
			headData, _ := app.Store.ReadEntity(headRef, "issue.md")
			if headData != nil {
				headIss, _ := issue.Parse(headData)
				if headIss != nil && headIss.Milestone != "" {
					name = headIss.Milestone
				}
			}
		}
		// Fall back to first milestone with pending/in-progress issues.
		if name == "" {
			found := findCurrentMilestone(milestones, allIssues)
			if found != nil {
				ms = found
			} else {
				// Last resort: the first milestone in the list.
				ms = milestones[0]
			}
		}
	}
	// If name was resolved (either from HEAD or provided by the caller), load it.
	if ms == nil {
		loaded, err := milestone.LoadMilestone(app.Store, name)
		if err != nil {
			return fmt.Errorf("milestone %q not found: %w", name, err)
		}
		ms = loaded
	}

	// Read --label flag to optionally scope the issue list and telemetry.
	labelFilter, _ := cmd.Flags().GetString("label")

	var msIssues []*issue.Issue
	doneCount := 0
	for _, iss := range allIssues {
		if iss.Milestone != ms.Name {
			continue
		}
		// Apply --label filter when set.
		if labelFilter != "" {
			found := false
			for _, l := range iss.Labels {
				if l == labelFilter {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		msIssues = append(msIssues, iss)
		if iss.State == issue.StateDone || iss.State == issue.StateCancelled {
			doneCount++
		}
	}

	progressPct := 0
	if len(msIssues) > 0 {
		progressPct = (doneCount * 100) / len(msIssues)
	}

	// Compute telemetry from milestone issues.
	stats := telemetry.Compute(allIssues, ms)

	// Compute forecast if --workers flag was provided.
	maxWorkers, _ := cmd.Flags().GetInt("workers")
	var forecast []forecastPoint
	if maxWorkers > 0 {
		forecast = computeForecast(msIssues, stats, maxWorkers)
	}

	// Compute per-worker capacity summary from in-progress and assigned issues.
	workerCap := computeWorkerCapacity(msIssues)

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		out := &milestoneShowJSON{
			Milestone:      ms,
			Issues:         msIssues,
			TotalIssues:    len(msIssues),
			DoneIssues:     doneCount,
			Progress:       progressPct,
			Telemetry:      stats,
			Forecast:       forecast,
			WorkerCapacity: workerCap,
		}
		if out.Issues == nil {
			out.Issues = []*issue.Issue{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	return showMilestoneHuman(cmd, ms, msIssues, doneCount, progressPct, stats, forecast, workerCap)
}

// showMilestoneHuman renders the milestone detail view in human-readable format,
// including telemetry signals, an optional forecast, and a worker capacity summary.
func showMilestoneHuman(cmd *cobra.Command, ms *milestone.Milestone, issues []*issue.Issue, doneCount, progressPct int, stats *telemetry.Stats, forecast []forecastPoint, workerCap map[string]*workerCapacityEntry) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Milestone: %s\n", ms.Name)
	if ms.Due != nil {
		fmt.Fprintf(w, "Due:       %s\n", ms.Due.Format("Jan 02 2006"))
	}
	fmt.Fprintf(w, "Progress:  %d/%d done (%d%%)\n", doneCount, len(issues), progressPct)

	// Telemetry section.
	if stats != nil && stats.FeverStatus != "" {
		fmt.Fprintf(w, "MPG:       %.1f commits/issue\n", stats.MPG)
		fmt.Fprintf(w, "Speed:     %.2f issues/week\n", stats.Speed)
		fmt.Fprintf(w, "Buffer:    %.1f total, %.1f burned\n", stats.BufferTotal, stats.BufferBurned)
		fmt.Fprintf(w, "Fever:     %s\n", stats.FeverStatus)
		if stats.TimeInChain > 0 {
			fmt.Fprintf(w, "  Time-in-chain: %.0f%%\n", stats.TimeInChain*100)
			fmt.Fprintf(w, "  Shadow work:   %.0f%%\n", stats.ShadowWork*100)
		}
	}

	// Forecast section (only present when --workers was given).
	if len(forecast) > 0 {
		fmt.Fprintln(w, "\nForecast:")
		for _, pt := range forecast {
			workerLabel := "worker"
			if pt.Workers > 1 {
				workerLabel = "workers"
			}
			parallelNote := ""
			if pt.ParallelTracks > 0 {
				parallelNote = fmt.Sprintf(" (%d off-chain parallel track", pt.ParallelTracks)
				if pt.ParallelTracks > 1 {
					parallelNote += "s"
				}
				parallelNote += ")"
			} else if pt.Workers > 1 {
				parallelNote = " (diminishing returns)"
			}
			fmt.Fprintf(w, "  At %d %s:  ~%.1f weeks (critical chain: %d issues)%s\n",
				pt.Workers, workerLabel, pt.WeeksEst, pt.CritIssues, parallelNote)
		}
	}

	// Worker capacity section (only present when workers have in-progress or assigned issues).
	if len(workerCap) > 0 {
		fmt.Fprintln(w, "\nWorker capacity:")
		// Sort worker names for deterministic output.
		workers := make([]string, 0, len(workerCap))
		for name := range workerCap {
			workers = append(workers, name)
		}
		sort.Strings(workers)
		for _, name := range workers {
			entry := workerCap[name]
			if entry.InProgress == 0 {
				fmt.Fprintf(w, "  %s: idle, %d assigned\n", name, entry.Assigned)
			} else {
				fmt.Fprintf(w, "  %s: %d in-progress, %d assigned\n", name, entry.InProgress, entry.Assigned)
			}
		}
	}

	if len(issues) > 0 {
		fmt.Fprintln(w, "\nIssues:")
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
	}

	return nil
}

// findCurrentMilestone returns the first milestone (by sort order) that has at
// least one pending or in-progress issue.
func findCurrentMilestone(milestones []*milestone.Milestone, issues []*issue.Issue) *milestone.Milestone {
	for _, ms := range milestones {
		for _, iss := range issues {
			if iss.Milestone == ms.Name &&
				(iss.State == issue.StatePending || iss.State == issue.StateInProgress) {
				return ms
			}
		}
	}
	return nil
}
