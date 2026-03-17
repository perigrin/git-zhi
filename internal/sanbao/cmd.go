// ABOUTME: Cobra command definition for the git-zhi-sanbao binary.
// ABOUTME: Loads a milestone, generates a sanbao report, and formats it for humans or JSON consumers.
package sanbao

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/sanbao/complexity"
	"github.com/perigrin/git-zhi/internal/sanbao/report"
)

// domainNames is the set of valid values for the --domain flag.
var domainNames = map[string]bool{
	"dora":       true,
	"space":      true,
	"calms":      true,
	"sentiment":  true,
	"complexity": true,
}

// NewSanbaoCommand creates and returns the top-level sanbao Cobra command.
// It requires a milestone name as the sole positional argument.
func NewSanbaoCommand() *cobra.Command {
	var format string
	var domain string

	cmd := &cobra.Command{
		Use:   "git-zhi-sanbao <milestone>",
		Short: "Generate a sanbao (三宝) observability report for a milestone",
		Long: `git-zhi-sanbao loads the named milestone, computes DORA, SPACE, CALMS,
sentiment, complexity, and issue difficulty metrics, and renders a
human-readable or JSON report.

Use --domain to filter to a single metric domain.
Use --format json for machine-readable output.`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			milestoneName := args[0]

			// Validate domain flag if supplied.
			if domain != "" && !domainNames[strings.ToLower(domain)] {
				valid := make([]string, 0, len(domainNames))
				for k := range domainNames {
					valid = append(valid, k)
				}
				sort.Strings(valid)
				return fmt.Errorf("unknown domain %q; valid values: %s", domain, strings.Join(valid, ", "))
			}

			app := cli.GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("not in a git repository")
			}

			rpt, err := report.GenerateReport(app.Repo, app.Store, milestoneName)
			if err != nil {
				return err
			}

			if format == "json" {
				return renderJSON(cmd, rpt, domain)
			}
			return renderHuman(cmd, rpt, domain)
		},
	}

	cmd.Flags().StringVar(&format, "format", "", "output format (json)")
	cmd.Flags().StringVar(&domain, "domain", "", "filter to a single domain: dora, space, calms, sentiment, complexity")

	return cmd
}

// renderJSON encodes the report (or a filtered sub-object) to JSON on cmd's
// output writer. When domain is non-empty only the corresponding sub-object
// is emitted alongside the "milestone" key for context.
func renderJSON(cmd *cobra.Command, rpt *report.Report, domain string) error {
	var payload interface{}

	if domain == "" {
		payload = rpt
	} else {
		switch strings.ToLower(domain) {
		case "dora":
			payload = map[string]interface{}{
				"milestone": rpt.Milestone,
				"dora":      rpt.DORA,
			}
		case "space":
			payload = map[string]interface{}{
				"milestone": rpt.Milestone,
				"space":     rpt.SPACE,
			}
		case "calms":
			payload = map[string]interface{}{
				"milestone": rpt.Milestone,
				"calms":     rpt.CALMS,
			}
		case "sentiment":
			payload = map[string]interface{}{
				"milestone": rpt.Milestone,
				"sentiment": rpt.Sentiment,
			}
		case "complexity":
			payload = map[string]interface{}{
				"milestone":  rpt.Milestone,
				"complexity": rpt.Complexity,
			}
		default:
			payload = rpt
		}
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

// renderHuman writes the report in a human-readable format to cmd's output.
// When domain is non-empty only the corresponding section is rendered.
func renderHuman(cmd *cobra.Command, rpt *report.Report, domain string) error {
	w := cmd.OutOrStdout()
	d := strings.ToLower(domain)

	showAll := d == ""

	if showAll || d == "dora" {
		fmt.Fprintf(w, "DORA:\n")
		fmt.Fprintf(w, "  Lead time:   %s\n", formatDuration(rpt.DORA.AvgLeadTime))
		fmt.Fprintf(w, "  Cycle time:  %s\n", formatDuration(rpt.DORA.AvgCycleTime))
		fmt.Fprintf(w, "  Rework rate: %.0f%%\n", rpt.DORA.ReworkRate)
		fmt.Fprintf(w, "  Frequency:   %.1f issues/week\n", rpt.DORA.CompletionFrequency)
		if showAll {
			fmt.Fprintln(w)
		}
	}

	if showAll || d == "space" {
		fmt.Fprintf(w, "Efficiency:\n")
		fmt.Fprintf(w, "  Agent autonomy:  %.0f%%\n", rpt.SPACE.AgentAutonomyRate)
		fmt.Fprintf(w, "  Avg review cycles: %.1f\n", rpt.SPACE.AvgReviewCycles)
		fmt.Fprintf(w, "  Avg sessions/issue: %.1f\n", rpt.SPACE.AvgSessionCount)
		if showAll {
			fmt.Fprintln(w)
		}
	}

	if showAll || d == "calms" {
		fmt.Fprintf(w, "CALMS:\n")
		fmt.Fprintf(w, "  Verification coverage: %.0f%%\n", rpt.CALMS.VerificationCoverage)
		fmt.Fprintf(w, "  WIP violations:        %d\n", rpt.CALMS.WIPViolations)
		fmt.Fprintf(w, "  Scope cut frequency:   %.0f%%\n", rpt.CALMS.ScopeCutFrequency)
		fmt.Fprintf(w, "  Metric completeness:   %.0f%%\n", rpt.CALMS.MetricCompleteness)
		if showAll {
			fmt.Fprintln(w)
		}
	}

	if showAll || d == "sentiment" {
		fmt.Fprintf(w, "Signals:\n")
		for _, issID := range rpt.Sentiment.Anomalies {
			// Truncate UUID to 8 chars for display, matching the rest of the CLI.
			short := issID
			if len(short) >= 8 {
				short = short[:8]
			}
			fmt.Fprintf(w, "  ! %s sustained negative sentiment\n", short)
		}
		fmt.Fprintf(w, "  Milestone mean sentiment: %.2f\n", rpt.Sentiment.MeanSentiment)
		if showAll {
			fmt.Fprintln(w)
		}
	}

	if showAll || d == "complexity" {
		fmt.Fprintf(w, "Complexity:\n")
		if len(rpt.Complexity.Hotspots) > 0 {
			fmt.Fprintf(w, "  Hotspots (churn x size):\n")
			for _, h := range topN(rpt.Complexity.Hotspots, 5) {
				fmt.Fprintf(w, "    %s  (%d changes, %d lines)\n", h.File, h.Churn, h.Size)
			}
		} else {
			fmt.Fprintf(w, "  Hotspots: none\n")
		}
		if len(rpt.Complexity.ChangeCoupling) > 0 {
			fmt.Fprintf(w, "  Change coupling:\n")
			for _, ce := range rpt.Complexity.ChangeCoupling {
				fmt.Fprintf(w, "    %s <-> %s (%d)\n", ce.FileA, ce.FileB, ce.Count)
			}
		}
		if showAll {
			fmt.Fprintln(w)
		}
	}

	return nil
}

// formatDuration renders a time.Duration in a concise human-readable form.
// Durations over 24 hours are shown as days; otherwise as hours with one
// decimal place.
func formatDuration(d interface{ Hours() float64 }) string {
	h := d.Hours()
	if h == 0 {
		return "n/a"
	}
	days := h / 24.0
	if days >= 1 {
		return fmt.Sprintf("%.1f days avg", days)
	}
	return fmt.Sprintf("%.1f hrs avg", h)
}

// topN returns up to n hotspots from a slice already sorted descending by score.
func topN(hotspots []complexity.ChurnSizeHotspot, n int) []complexity.ChurnSizeHotspot {
	if len(hotspots) <= n {
		return hotspots
	}
	return hotspots[:n]
}
