// ABOUTME: Health computes churn-relative documentation staleness for docs with covers frontmatter.
// ABOUTME: Reports DriftNone/Low/High per doc, stability modulation, and coverage gap detection.
package docs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/goccy/go-yaml"
)

// DriftLevel classifies how stale a document is relative to code churn in the
// paths it covers.
type DriftLevel string

const (
	// DriftNone means no code commits have touched the covered paths since the
	// doc was last modified.
	DriftNone DriftLevel = "NONE"

	// DriftLow means a small number of commits have touched the covered paths
	// since the doc was last modified — within the stability-adjusted threshold.
	DriftLow DriftLevel = "LOW"

	// DriftHigh means the number of commits exceeds the stability-adjusted
	// threshold, indicating the doc is likely stale.
	DriftHigh DriftLevel = "HIGH"
)

// DocHealth summarises the staleness of one documentation file.
type DocHealth struct {
	// File is the slash-separated path to the doc relative to repoRoot.
	File string `json:"file"`

	// Covers lists the paths declared in the doc's covers frontmatter field.
	Covers []string `json:"covers"`

	// DocModified is the author timestamp of the most recent git commit that
	// touched the doc file.
	DocModified time.Time `json:"doc_modified"`

	// CodeChurn is the number of commits that touched any covered path after
	// DocModified.
	CodeChurn int `json:"code_churn"`

	// Drift classifies the churn relative to the doc's stability.
	Drift DriftLevel `json:"drift"`

	// Stability is the value from the doc's stability frontmatter field:
	// 0=deprecated, 1=experimental, 2=stable.
	Stability int `json:"stability"`
}

// HealthReport is the output of a Health run.
type HealthReport struct {
	// Documents holds one DocHealth entry per eligible doc.
	Documents []DocHealth `json:"documents"`

	// CoverageGaps lists internal/ packages (slash-separated, relative to
	// repoRoot) that have no corresponding file in docs/architecture/.
	CoverageGaps []string `json:"coverage_gaps"`

	// Summary is a human-readable one-line summary of the report.
	Summary string `json:"summary"`
}

// healthFrontmatter holds the fields Health reads from a doc's YAML frontmatter.
type healthFrontmatter struct {
	Covers    []string `yaml:"covers"`
	Stability *int     `yaml:"stability"`
}

// healthExemptPrefixes lists the doc subdirectories that are exempt from
// staleness checking. ADRs and postmortems are append-only records and should
// never be flagged for updates.
var healthExemptPrefixes = []string{
	"docs/decisions/",
	"docs/postmortems/",
}

// isHealthExempt returns true if the slash-separated path relative to repoRoot
// falls under an exempt directory.
func isHealthExempt(slashRel string) bool {
	for _, prefix := range healthExemptPrefixes {
		if strings.HasPrefix(slashRel, prefix) {
			return true
		}
	}
	return false
}

// lowThreshold returns the maximum churn count that is still classified as LOW
// for the given stability level.
//
//   - stability 0 (deprecated): not used (deprecated docs always return DriftNone)
//   - stability 1 (experimental): 2 commits
//   - stability 2 (stable): 5 commits
func lowThreshold(stability int) int {
	switch stability {
	case 1:
		return 2
	default: // 2 or any unknown value treated as stable
		return 5
	}
}

// classifyDrift returns the DriftLevel for the given stability and churn count.
func classifyDrift(stability, churn int) DriftLevel {
	if stability == 0 {
		// Deprecated docs never need updating.
		return DriftNone
	}
	if churn == 0 {
		return DriftNone
	}
	if churn <= lowThreshold(stability) {
		return DriftLow
	}
	return DriftHigh
}

// Health computes documentation staleness for all eligible markdown files in
// docs/ under repoRoot. A file is eligible when it has a non-empty covers
// frontmatter field. Files in docs/decisions/ and docs/postmortems/ are
// exempt from staleness checks.
//
// Health also detects coverage gaps: internal/ packages (immediate
// sub-directories of repoRoot/internal/) that have no corresponding
// docs/architecture/ file.
//
// repo must be a go-git repository opened at repoRoot. If it is nil,
// git-dependent fields (DocModified, CodeChurn) are left at their zero
// values and drift defaults to DriftNone.
func Health(repoRoot string, repo *git.Repository) (*HealthReport, error) {
	report := &HealthReport{}

	docs, err := collectEligibleDocs(repoRoot)
	if err != nil {
		return nil, err
	}

	for _, d := range docs {
		health, err := computeDocHealth(repoRoot, d, repo)
		if err != nil {
			return nil, fmt.Errorf("compute health for %s: %w", d, err)
		}
		if health == nil {
			// No covers frontmatter — skip this doc.
			continue
		}
		report.Documents = append(report.Documents, *health)
	}

	gaps, err := detectCoverageGaps(repoRoot, report.Documents)
	if err != nil {
		return nil, err
	}
	report.CoverageGaps = gaps

	report.Summary = buildSummary(report)
	return report, nil
}

// collectEligibleDocs returns slash-separated paths (relative to repoRoot) of
// all markdown files in docs/ that are not exempt from staleness checks.
func collectEligibleDocs(repoRoot string) ([]string, error) {
	docsDir := filepath.Join(repoRoot, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var eligible []string
	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		slashRel := filepath.ToSlash(rel)
		if !isHealthExempt(slashRel) {
			eligible = append(eligible, slashRel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk docs/: %w", err)
	}
	sort.Strings(eligible)
	return eligible, nil
}

// computeDocHealth builds a DocHealth record for a single doc file. slashRel
// is the slash-separated path relative to repoRoot.
func computeDocHealth(repoRoot, slashRel string, repo *git.Repository) (*DocHealth, error) {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(slashRel))
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", slashRel, err)
	}

	var fm healthFrontmatter
	if _, parseErr := frontmatter.Parse(bytes.NewReader(raw), &fm, frontmatter.NewFormat("---", "---", yaml.Unmarshal)); parseErr != nil {
		// No valid frontmatter — skip (return nil with no error; caller skips nils)
		return nil, nil
	}

	if len(fm.Covers) == 0 {
		// No covers field — skip.
		return nil, nil
	}

	stability := 2 // default: stable
	if fm.Stability != nil {
		stability = *fm.Stability
	}

	health := &DocHealth{
		File:      slashRel,
		Covers:    fm.Covers,
		Stability: stability,
	}

	if repo == nil {
		health.Drift = DriftNone
		return health, nil
	}

	// Get last modified time of the doc from git log.
	docModified, err := lastModifiedTime(repo, slashRel)
	if err != nil {
		// Could not determine modification time — treat as very old (zero time)
		// which will cause all subsequent commits to be counted.
		docModified = time.Time{}
	}
	health.DocModified = docModified

	// Count commits touching covered paths after docModified.
	churn, err := countChurnSince(repo, fm.Covers, docModified)
	if err != nil {
		return nil, fmt.Errorf("count churn for %s: %w", slashRel, err)
	}
	health.CodeChurn = churn
	health.Drift = classifyDrift(stability, churn)

	return health, nil
}

// lastModifiedTime returns the author timestamp of the most recent commit
// that touched filePath (slash-separated relative path). Returns the zero
// time if the file has no git history.
func lastModifiedTime(repo *git.Repository, slashPath string) (time.Time, error) {
	iter, err := repo.Log(&git.LogOptions{
		All:        false,
		PathFilter: func(p string) bool { return p == slashPath },
	})
	if err != nil {
		return time.Time{}, err
	}
	defer iter.Close()

	var latest time.Time
	_ = iter.ForEach(func(c *object.Commit) error {
		if latest.IsZero() || c.Author.When.After(latest) {
			latest = c.Author.When
		}
		// The first commit from the log (most recent) is what we want.
		return fmt.Errorf("stop") // stop iteration after first commit
	})

	return latest, nil
}

// countChurnSince counts the number of commits that touch any of the given
// covers paths (slash-separated, relative to repoRoot) after sinceTime.
// Each commit is counted only once even if it touches multiple covered paths.
func countChurnSince(repo *git.Repository, coverPaths []string, sinceTime time.Time) (int, error) {
	if len(coverPaths) == 0 {
		return 0, nil
	}

	// Build a set of covered path prefixes for efficient matching.
	covered := make([]string, len(coverPaths))
	copy(covered, coverPaths)

	// We iterate all commits since sinceTime and check whether each commit
	// touched any of the covered paths. A commit is "after sinceTime" if
	// its author timestamp is strictly after sinceTime.
	//
	// go-git's Since option uses >=, so we add a nanosecond to sinceTime to
	// get strictly-after semantics (git stores timestamps at second precision,
	// so +1 second is safe for the docs use case where doc and code commits
	// have distinct timestamps).
	since := sinceTime.Add(time.Second)
	iter, err := repo.Log(&git.LogOptions{
		All:   false,
		Since: &since,
		PathFilter: func(p string) bool {
			return pathMatchesCovers(p, covered)
		},
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	count := 0
	_ = iter.ForEach(func(c *object.Commit) error {
		count++
		return nil
	})
	return count, nil
}

// pathMatchesCovers returns true if p (the path of a changed file in a commit)
// matches any of the covers paths. Matching is done at both the exact-file
// level and the directory-prefix level so that covering "internal/cli" matches
// any file under that directory.
func pathMatchesCovers(p string, covers []string) bool {
	for _, cover := range covers {
		if p == cover {
			return true
		}
		// Cover is a directory prefix if it is followed by "/" in p.
		if strings.HasPrefix(p, cover+"/") {
			return true
		}
	}
	return false
}

// detectCoverageGaps returns slash-separated paths of internal/ packages
// (immediate sub-directories of repoRoot/internal/) that have no corresponding
// file in docs/architecture/.
func detectCoverageGaps(repoRoot string, docs []DocHealth) ([]string, error) {
	internalDir := filepath.Join(repoRoot, "internal")
	if _, err := os.Stat(internalDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil, fmt.Errorf("read internal/: %w", err)
	}

	// Build the set of covered packages from existing docs.
	coveredPkgs := map[string]struct{}{}
	for _, d := range docs {
		for _, cp := range d.Covers {
			coveredPkgs[cp] = struct{}{}
		}
	}

	// Also scan docs/architecture/ for files and extract covers from frontmatter
	// to catch docs that cover a package but weren't included above (e.g. docs
	// whose covers don't use the exact package path).
	archDir := filepath.Join(repoRoot, "docs", "architecture")
	if _, err := os.Stat(archDir); err == nil {
		// Build a set of all covered paths from architecture docs.
		_ = filepath.Walk(archDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			var fm healthFrontmatter
			if _, parseErr := frontmatter.Parse(bytes.NewReader(raw), &fm, frontmatter.NewFormat("---", "---", yaml.Unmarshal)); parseErr == nil {
				for _, cp := range fm.Covers {
					coveredPkgs[cp] = struct{}{}
				}
			}
			return nil
		})
	}

	var gaps []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pkgPath := "internal/" + entry.Name()
		if _, ok := coveredPkgs[pkgPath]; !ok {
			gaps = append(gaps, pkgPath)
		}
	}
	sort.Strings(gaps)
	return gaps, nil
}

// buildSummary produces a human-readable one-line summary of the health report.
func buildSummary(report *HealthReport) string {
	total := len(report.Documents)
	highCount := 0
	lowCount := 0
	for _, d := range report.Documents {
		switch d.Drift {
		case DriftHigh:
			highCount++
		case DriftLow:
			lowCount++
		}
	}
	gapCount := len(report.CoverageGaps)

	if total == 0 && gapCount == 0 {
		return "no docs with covers frontmatter found"
	}

	parts := []string{fmt.Sprintf("%d docs checked", total)}
	if highCount > 0 {
		parts = append(parts, fmt.Sprintf("%d HIGH drift", highCount))
	}
	if lowCount > 0 {
		parts = append(parts, fmt.Sprintf("%d LOW drift", lowCount))
	}
	if gapCount > 0 {
		parts = append(parts, fmt.Sprintf("%d coverage gap(s)", gapCount))
	}
	if highCount == 0 && lowCount == 0 && gapCount == 0 {
		parts = append(parts, "all current")
	}
	return strings.Join(parts, ", ")
}
