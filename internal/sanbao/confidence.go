// ABOUTME: Telemetry baseline quality assessment for a milestone.
// ABOUTME: Reports the composition of data driving forecasts by confidence tier.
package sanbao

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// ConfidenceReport holds the telemetry baseline quality breakdown.
type ConfidenceReport struct {
	Milestone          string `json:"milestone"`
	PlannedCount       int    `json:"planned_count"`
	TrackerMatchCount  int    `json:"tracker_match_count"`
	HighConfidenceCount int   `json:"high_confidence_count"`
	LowConfidenceCount int    `json:"low_confidence_count"`
	TotalCount         int    `json:"total_count"`
	Reliability        string `json:"reliability"`
}

// NewConfidenceCommand creates the "confidence" subcommand for sanbao.
func NewConfidenceCommand() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "confidence <milestone>",
		Short: "Assess telemetry baseline quality for a milestone",
		Long: `Reports the composition of the telemetry baseline: what percentage of
data driving forecasts comes from planned issues with real session data,
high-confidence retrospective issues, or low-confidence clusters.

Forecasts built on low-confidence data deserve less trust than the
fever chart implies. This command surfaces that rather than hiding it.`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			msName := args[0]

			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			// Verify the milestone exists.
			if _, err := milestone.LoadMilestone(app.Store, msName); err != nil {
				return fmt.Errorf("load milestone %q: %w", msName, err)
			}

			allIssues, err := issue.LoadAllIssues(app.Store)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			// Filter to milestone.
			var msIssues []*issue.Issue
			for _, iss := range allIssues {
				if iss.Milestone == msName {
					msIssues = append(msIssues, iss)
				}
			}

			rpt := computeConfidence(msName, msIssues)

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rpt)
			}

			return renderConfidenceHuman(cmd, rpt)
		},
	}

	cmd.Flags().StringVar(&format, "format", "", "output format (json)")
	return cmd
}

// computeConfidence classifies issues into confidence tiers.
func computeConfidence(msName string, issues []*issue.Issue) ConfidenceReport {
	rpt := ConfidenceReport{
		Milestone:  msName,
		TotalCount: len(issues),
	}

	for _, iss := range issues {
		switch {
		case iss.Source == "" || iss.Source == "planned" || iss.Source == "manual":
			// Planned or manually created issues have real session data.
			rpt.PlannedCount++
		case iss.Source == "tracker-match":
			// Tracker-matched retrospective issues have good signal.
			rpt.TrackerMatchCount++
		case iss.Confidence >= 0.7:
			// High-confidence cluster or ticket-ref.
			rpt.HighConfidenceCount++
		default:
			// Low-confidence cluster or single-commit.
			rpt.LowConfidenceCount++
		}
	}

	// Determine overall reliability.
	if rpt.TotalCount == 0 {
		rpt.Reliability = "NO DATA"
	} else {
		plannedPct := float64(rpt.PlannedCount) / float64(rpt.TotalCount)
		lowPct := float64(rpt.LowConfidenceCount) / float64(rpt.TotalCount)
		switch {
		case plannedPct >= 0.5:
			rpt.Reliability = "HIGH"
		case lowPct > 0.3:
			rpt.Reliability = "LOW"
		default:
			rpt.Reliability = "MODERATE"
		}
	}

	return rpt
}

// renderConfidenceHuman writes the confidence report in human-readable format.
func renderConfidenceHuman(cmd *cobra.Command, rpt ConfidenceReport) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Telemetry baseline quality: %s\n\n", rpt.Milestone)

	total := rpt.TotalCount
	if total == 0 {
		fmt.Fprintf(w, "  No issues found for milestone %s.\n", rpt.Milestone)
		return nil
	}

	pct := func(n int) float64 { return float64(n) / float64(total) * 100 }

	fmt.Fprintf(w, "  Planned issues (real sessions):     %3d issues  (%2.0f%%)  — high signal\n",
		rpt.PlannedCount, pct(rpt.PlannedCount))
	fmt.Fprintf(w, "  Retrospective (tracker-match):      %3d issues  (%2.0f%%)  — good signal\n",
		rpt.TrackerMatchCount, pct(rpt.TrackerMatchCount))
	fmt.Fprintf(w, "  Retrospective (cluster, conf >0.7): %3d issues  (%2.0f%%)  — moderate signal\n",
		rpt.HighConfidenceCount, pct(rpt.HighConfidenceCount))
	fmt.Fprintf(w, "  Retrospective (cluster, conf <0.7): %3d issues  (%2.0f%%)  — low signal\n",
		rpt.LowConfidenceCount, pct(rpt.LowConfidenceCount))

	fmt.Fprintf(w, "\nForecast reliability: %s\n", rpt.Reliability)

	if rpt.PlannedCount > 0 {
		fmt.Fprintf(w, "  %.0f%% of baseline from planned issues with real session data.\n",
			pct(rpt.PlannedCount))
	}
	if rpt.LowConfidenceCount > 0 {
		fmt.Fprintf(w, "  Warning: %d low-confidence retrospective issue(s) included.\n",
			rpt.LowConfidenceCount)
		fmt.Fprintf(w, "    Exclude with: git zhi sanbao report --min-confidence 0.7\n")
	}

	return nil
}
