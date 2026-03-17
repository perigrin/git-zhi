// ABOUTME: Sanbao report aggregator: collects DORA, SPACE, CALMS, sentiment,
// ABOUTME: complexity, and difficulty metrics for a milestone into a single Report.
package report

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/sanbao/calms"
	"github.com/perigrin/git-zhi/internal/sanbao/complexity"
	"github.com/perigrin/git-zhi/internal/sanbao/difficulty"
	"github.com/perigrin/git-zhi/internal/sanbao/dora"
	"github.com/perigrin/git-zhi/internal/sanbao/lsp"
	"github.com/perigrin/git-zhi/internal/sanbao/sentiment"
	"github.com/perigrin/git-zhi/internal/sanbao/space"
	"github.com/perigrin/git-zhi/internal/storage"
)

// Report aggregates all sanbao domain metrics for a milestone.
type Report struct {
	Milestone    string                     `json:"milestone"`
	DORA         dora.DORAMetrics           `json:"dora"`
	SPACE        space.SPACEMetrics         `json:"space"`
	CALMS        calms.CALMSIndicators      `json:"calms"`
	Sentiment    sentiment.SentimentMetrics `json:"sentiment"`
	Complexity   ComplexityReport           `json:"complexity"`
	Difficulties []DifficultyEntry          `json:"difficulties"`
}

// ComplexityReport holds the language-agnostic code complexity metrics for a milestone.
type ComplexityReport struct {
	Hotspots       []complexity.ChurnSizeHotspot `json:"hotspots"`
	ChangeCoupling []CouplingEntry               `json:"change_coupling"`
}

// CouplingEntry represents a co-changed file pair and how many times they
// changed together within the milestone's session ranges.
type CouplingEntry struct {
	FileA string `json:"file_a"`
	FileB string `json:"file_b"`
	Count int    `json:"count"`
}

// DifficultyEntry pairs an issue's identity with its computed difficulty score.
type DifficultyEntry struct {
	IssueID    string  `json:"issue_id"`
	IssueTitle string  `json:"issue_title"`
	Score      float64 `json:"score"`
}

// GenerateReport loads the named milestone and all of its done issues, then
// calls each sanbao sub-package to compute its metrics. The results are
// assembled into a Report and returned.
//
// The repo parameter is used for git-log–based computations (sentiment,
// complexity). A nil repo causes those sub-packages to return zero values
// gracefully, which is acceptable when a milestone has no done sessions or
// when operating outside a worktree.
//
// LSP diagnostics are best-effort: if no language server binary is configured
// or initialization fails, the LSP step is skipped with a warning.
func GenerateReport(repo *git.Repository, store *storage.Store, milestoneName string) (*Report, error) {
	// Validate the milestone exists.
	if _, err := milestone.LoadMilestone(store, milestoneName); err != nil {
		return nil, fmt.Errorf("load milestone %q: %w", milestoneName, err)
	}

	// Load all issues and filter to done issues in this milestone.
	allIssues, err := issue.LoadAllIssues(store)
	if err != nil {
		return nil, fmt.Errorf("load issues: %w", err)
	}

	var doneIssues []*issue.Issue
	for _, iss := range allIssues {
		if iss.Milestone == milestoneName && iss.State == issue.StateDone {
			doneIssues = append(doneIssues, iss)
		}
	}

	rpt := &Report{
		Milestone: milestoneName,
	}

	// --- DORA metrics ---
	rpt.DORA = dora.Compute(doneIssues)

	// --- SPACE metrics ---
	rpt.SPACE = space.Compute(doneIssues)

	// --- CALMS indicators ---
	rpt.CALMS = calms.Compute(doneIssues)

	// --- Sentiment metrics (best-effort; repo may be nil) ---
	if repo != nil {
		rpt.Sentiment = sentiment.Compute(repo, doneIssues)
	} else {
		rpt.Sentiment = sentiment.SentimentMetrics{Anomalies: []string{}}
	}

	// --- Complexity metrics (best-effort; repo may be nil) ---
	rpt.Complexity = computeComplexity(repo, doneIssues)

	// --- Per-issue difficulty scores ---
	rpt.Difficulties = computeDifficulties(repo, doneIssues)

	// --- LSP diagnostics (best-effort; skip if not configured or fails) ---
	runLSPDiagnostics(doneIssues)

	return rpt, nil
}

// computeComplexity gathers file churn and change coupling across all done
// issues' sessions and assembles a ComplexityReport. Returns a report with
// non-nil empty slices when repo is nil or no sessions produce results.
func computeComplexity(repo *git.Repository, issues []*issue.Issue) ComplexityReport {
	cr := ComplexityReport{
		Hotspots:       []complexity.ChurnSizeHotspot{},
		ChangeCoupling: []CouplingEntry{},
	}
	if repo == nil {
		return cr
	}

	// Collect all sessions across all done issues.
	var allSessions []issue.Session
	for _, iss := range issues {
		allSessions = append(allSessions, iss.Sessions...)
	}

	if len(allSessions) == 0 {
		return cr
	}

	// Determine the repo root for file size lookups.
	repoRoot := repoRootPath(repo)

	// File churn map.
	churn := complexity.FileChurn(repo, allSessions)
	if len(churn) > 0 {
		cr.Hotspots = complexity.ChurnSizeHotspots(churn, repoRoot)
	}

	// Change coupling.
	coupling := complexity.ChangeCoupling(repo, allSessions)
	for pair, count := range coupling {
		cr.ChangeCoupling = append(cr.ChangeCoupling, CouplingEntry{
			FileA: pair[0],
			FileB: pair[1],
			Count: count,
		})
	}

	return cr
}

// computeDifficulties computes a DifficultyEntry for each done issue using
// per-issue mean sentiment from the repo (when available).
func computeDifficulties(repo *git.Repository, issues []*issue.Issue) []DifficultyEntry {
	entries := make([]DifficultyEntry, 0, len(issues))
	for _, iss := range issues {
		var meanSentiment float64
		if repo != nil {
			traj := sentiment.IssueTrajectory(repo, iss)
			if len(traj) > 0 {
				var sum float64
				for _, s := range traj {
					sum += s
				}
				meanSentiment = sum / float64(len(traj))
			}
		}
		score := difficulty.ComputeDifficulty(iss, meanSentiment)
		entries = append(entries, DifficultyEntry{
			IssueID:    iss.ID.String(),
			IssueTitle: iss.Title,
			Score:      score,
		})
	}
	return entries
}

// runLSPDiagnostics attempts to start a language server and collect diagnostics
// for observed paths across all done issues. This is purely best-effort: if the
// server binary is not on $PATH or initialization fails, a warning is logged and
// the function returns. Diagnostics are not currently incorporated into the
// Report struct — they are printed to stderr so operators can see them.
func runLSPDiagnostics(issues []*issue.Issue) {
	// Discover the first language server binary we recognise.
	serverCmd := detectLSPBinary()
	if serverCmd == "" {
		// No LSP server available — skip silently.
		return
	}

	// Collect unique observed paths across all done issues.
	seen := map[string]struct{}{}
	var paths []string
	for _, iss := range issues {
		for _, p := range iss.ObservedPaths {
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				paths = append(paths, p)
			}
		}
	}
	if len(paths) == 0 {
		return
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return
	}

	client, err := lsp.NewClient(serverCmd, repoRoot)
	if err != nil {
		log.Printf("sanbao: LSP init skipped: %v", err)
		return
	}
	defer client.Shutdown() //nolint:errcheck

	for _, p := range paths {
		absPath := repoRoot + "/" + p
		diags, err := client.Diagnostics(absPath)
		if err != nil {
			continue
		}
		for _, d := range diags {
			log.Printf("sanbao: LSP %s %s:%d: %s", d.Severity, d.File, d.Line, d.Message)
		}
	}
}

// detectLSPBinary returns the name of a language server binary if one is
// available on $PATH, or an empty string if none is found. Only gopls is
// currently probed.
func detectLSPBinary() string {
	candidates := []string{"gopls"}
	for _, c := range candidates {
		if _, err := findBinary(c); err == nil {
			return c
		}
	}
	return ""
}

// findBinary reports whether the named binary is on $PATH.
func findBinary(name string) (string, error) {
	return lookPath(name)
}

// lookPath is a thin wrapper around exec.LookPath, enabling the binary
// detection logic to be compiled without conditional imports.
func lookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// repoRootPath returns the working-tree root for file size lookups. Falls back
// to "." when the repo has no attached worktree (e.g., bare repos in tests).
func repoRootPath(repo *git.Repository) string {
	if repo == nil {
		return "."
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "."
	}
	return wt.Filesystem.Root()
}
