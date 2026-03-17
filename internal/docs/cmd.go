// ABOUTME: Cobra commands for the git-zhi-docs plugin: init, check, and health.
// ABOUTME: All three subcommands support --format json; check and health lazy-init docs/.
package docs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

// NewDocsCommand creates and returns the top-level docs Cobra command with
// three subcommands: init, check, and health.
//
// repoRoot is the absolute path to the repository's working directory.
// repo is the go-git Repository used by the health subcommand; it may be nil
// when running in environments without an initialised git repository.
func NewDocsCommand(repoRoot string, repo *git.Repository) *cobra.Command {
	var format string

	root := &cobra.Command{
		Use:   "git-zhi-docs",
		Short: "Manage documentation health for a git-zhi repository",
		// SilenceUsage prevents Cobra from printing usage text on every error.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&format, "format", "", "output format (json)")

	root.AddCommand(
		newInitCommand(repoRoot, &format),
		newCheckCommand(repoRoot, &format),
		newHealthCommand(repoRoot, repo, &format),
	)

	return root
}

// --------------------------------------------------------------------------
// docs init
// --------------------------------------------------------------------------

// initScaffoldItem records one item created or updated during init.
type initScaffoldItem struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "created" or "updated" or "exists"
}

// newInitCommand builds the `docs init` subcommand.
func newInitCommand(repoRoot string, format *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold the canonical docs/ directory structure",
		Long: `Create the docs/ directory structure and living documents.
Idempotent: existing files and directories are left untouched.
CONTRIBUTING.md is created if absent; a Short Links section is
appended if the file exists but lacks one.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := scaffoldWithReport(repoRoot)
			if err != nil {
				return err
			}

			if *format == "json" {
				type jsonOutput struct {
					Items []initScaffoldItem `json:"items"`
				}
				out := jsonOutput{Items: items}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			// Human output.
			fmt.Fprintln(cmd.OutOrStdout(), "Scaffolded docs/ directory structure:")
			for _, item := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s)\n", item.Path, item.Action)
			}
			return nil
		},
	}
}

// scaffoldWithReport calls Scaffold and returns a list of items describing
// what was created or updated. It detects pre-existing state by comparing
// filesystem state before and after Scaffold.
func scaffoldWithReport(repoRoot string) ([]initScaffoldItem, error) {
	// Snapshot existence of expected paths before scaffolding.
	type pathSpec struct {
		rel   string
		isDir bool
	}

	specs := make([]pathSpec, 0, len(canonicalDirs)+len(livingDocs)+1)
	for _, d := range canonicalDirs {
		specs = append(specs, pathSpec{rel: d + "/", isDir: true})
	}
	for _, doc := range livingDocs {
		specs = append(specs, pathSpec{rel: doc.path, isDir: false})
	}
	specs = append(specs, pathSpec{rel: "CONTRIBUTING.md", isDir: false})

	type snapshot struct {
		exists  bool
		isDir   bool
		hasLink bool // for CONTRIBUTING.md: already has Short Links section
	}

	before := make(map[string]snapshot, len(specs))
	for _, sp := range specs {
		abs := filepath.Join(repoRoot, filepath.FromSlash(strings.TrimSuffix(sp.rel, "/")))
		info, err := os.Stat(abs)
		s := snapshot{}
		if err == nil {
			s.exists = true
			s.isDir = info.IsDir()
		}
		if sp.rel == "CONTRIBUTING.md" && s.exists {
			raw, readErr := os.ReadFile(abs)
			if readErr == nil {
				s.hasLink = strings.Contains(string(raw), shortLinksHeading)
			}
		}
		before[sp.rel] = s
	}

	// Run Scaffold.
	if err := Scaffold(repoRoot); err != nil {
		return nil, err
	}

	// Build report by comparing state after Scaffold.
	var items []initScaffoldItem
	for _, sp := range specs {
		abs := filepath.Join(repoRoot, filepath.FromSlash(strings.TrimSuffix(sp.rel, "/")))
		_, statErr := os.Stat(abs)
		if statErr != nil {
			continue // Should not happen after a successful Scaffold.
		}

		prior := before[sp.rel]
		action := ""
		switch {
		case !prior.exists:
			action = "created"
		case sp.rel == "CONTRIBUTING.md" && !prior.hasLink:
			action = "updated"
		default:
			action = "exists"
		}
		items = append(items, initScaffoldItem{Path: sp.rel, Action: action})
	}

	return items, nil
}

// --------------------------------------------------------------------------
// docs check
// --------------------------------------------------------------------------

// newCheckCommand builds the `docs check` subcommand.
func newCheckCommand(repoRoot string, format *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the structural integrity of the docs/ directory",
		Long: `Check validates all docs/ files are reachable from CONTRIBUTING.md,
reports dead links, ADR numbering gaps, and invalid covers paths.

Exit code 1 when any issues are found.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Lazy init: create docs/ structure if it doesn't exist yet.
			docsDir := filepath.Join(repoRoot, "docs")
			if _, err := os.Stat(docsDir); os.IsNotExist(err) {
				if scaffoldErr := Scaffold(repoRoot); scaffoldErr != nil {
					return fmt.Errorf("lazy init docs/: %w", scaffoldErr)
				}
			}

			result, err := Check(repoRoot)
			if err != nil {
				return err
			}

			if *format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if encErr := enc.Encode(result); encErr != nil {
					return fmt.Errorf("encode JSON: %w", encErr)
				}
				if !result.OK {
					return fmt.Errorf("%d issue(s) found", countCheckIssues(result))
				}
				return nil
			}

			// Human-readable output.
			printCheckResult(cmd, result)

			if !result.OK {
				n := countCheckIssues(result)
				return fmt.Errorf("%d issue(s) found", n)
			}
			return nil
		},
	}
}

// countCheckIssues returns the total number of distinct issues in a CheckResult.
func countCheckIssues(r *CheckResult) int {
	return len(r.UnreachableFiles) + len(r.DeadLinks) + len(r.ADRGaps) + len(r.InvalidCovers)
}

// printCheckResult writes a human-readable check report to the command output.
func printCheckResult(cmd *cobra.Command, r *CheckResult) {
	w := cmd.OutOrStdout()

	if len(r.UnreachableFiles) == 0 {
		fmt.Fprintln(w, "✓ All files reachable from CONTRIBUTING.md")
	} else {
		fmt.Fprintf(w, "✗ %d unreachable file(s):\n", len(r.UnreachableFiles))
		for _, f := range r.UnreachableFiles {
			fmt.Fprintf(w, "  %s\n", f)
		}
	}

	if len(r.DeadLinks) == 0 {
		fmt.Fprintln(w, "✓ No dead links")
	} else {
		fmt.Fprintf(w, "✗ %d dead link(s):\n", len(r.DeadLinks))
		for _, dl := range r.DeadLinks {
			fmt.Fprintf(w, "  %s\n", dl)
		}
	}

	if len(r.ADRGaps) == 0 {
		fmt.Fprintln(w, "✓ ADR numbering sequential")
	} else {
		fmt.Fprintf(w, "✗ ADR numbering gaps: %v\n", r.ADRGaps)
	}

	if len(r.InvalidCovers) == 0 {
		// Only print this line if there are docs with covers declarations to check.
		// Printing it when there are no docs avoids confusing "no covers to check"
		// output. Always print for consistency with the spec.
		fmt.Fprintln(w, "✓ All covers paths valid")
	} else {
		fmt.Fprintf(w, "⚠ %d invalid covers path(s):\n", len(r.InvalidCovers))
		for _, ic := range r.InvalidCovers {
			fmt.Fprintf(w, "  %s\n", ic)
		}
	}

	n := countCheckIssues(r)
	if n == 0 {
		fmt.Fprintln(w, "\n0 issues found")
	} else {
		fmt.Fprintf(w, "\n%d issue(s) found\n", n)
	}
}

// --------------------------------------------------------------------------
// docs health
// --------------------------------------------------------------------------

// newHealthCommand builds the `docs health` subcommand.
func newHealthCommand(repoRoot string, repo *git.Repository, format *string) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Report documentation drift relative to code churn",
		Long: `Health checks each document with a covers frontmatter field for
drift: how many commits have touched the covered paths since the
doc was last modified. Stability metadata modulates the threshold.

ADRs and postmortems are exempt from staleness checks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Lazy init: create docs/ structure if it doesn't exist yet.
			docsDir := filepath.Join(repoRoot, "docs")
			if _, err := os.Stat(docsDir); os.IsNotExist(err) {
				if scaffoldErr := Scaffold(repoRoot); scaffoldErr != nil {
					return fmt.Errorf("lazy init docs/: %w", scaffoldErr)
				}
			}

			report, err := Health(repoRoot, repo)
			if err != nil {
				return err
			}

			if *format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}

			// Human-readable output.
			printHealthReport(cmd, report)
			return nil
		},
	}
}

// printHealthReport writes a human-readable health report to the command output.
func printHealthReport(cmd *cobra.Command, report *HealthReport) {
	w := cmd.OutOrStdout()

	for _, d := range report.Documents {
		fmt.Fprintf(w, "%s\n", d.File)
		fmt.Fprintf(w, "  Covers: %s\n", strings.Join(d.Covers, ", "))
		if !d.DocModified.IsZero() {
			fmt.Fprintf(w, "  Doc modified: %s | Code churn since: %d commit(s)\n",
				d.DocModified.Format(time.DateOnly), d.CodeChurn)
		} else {
			fmt.Fprintf(w, "  Code churn: %d commit(s)\n", d.CodeChurn)
		}
		fmt.Fprintf(w, "  Drift: %s\n", d.Drift)
		fmt.Fprintln(w)
	}

	if len(report.CoverageGaps) > 0 {
		fmt.Fprintln(w, "Coverage gaps (no architecture doc):")
		for _, g := range report.CoverageGaps {
			fmt.Fprintf(w, "  %s\n", g)
		}
		fmt.Fprintln(w)
	}

	highCount, lowCount := 0, 0
	for _, d := range report.Documents {
		switch d.Drift {
		case DriftHigh:
			highCount++
		case DriftLow:
			lowCount++
		}
	}
	gapCount := len(report.CoverageGaps)
	fmt.Fprintf(w, "Summary: %d high drift, %d low drift, %d coverage gap(s)\n",
		highCount, lowCount, gapCount)
}
