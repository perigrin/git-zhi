// ABOUTME: Check validates the structural integrity of a docs/ directory,
// ABOUTME: reporting unreachable files, dead links, ADR gaps, and invalid covers paths.
package docs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/goccy/go-yaml"
)

// CheckResult holds the output of a structural validation run.
type CheckResult struct {
	// UnreachableFiles lists paths (relative to repoRoot) of files in docs/
	// that are not linked from CONTRIBUTING.md directly or transitively.
	UnreachableFiles []string `json:"unreachable_files"`

	// DeadLinks lists "file: link" descriptions of relative markdown links
	// whose target does not exist on the filesystem.
	DeadLinks []string `json:"dead_links"`

	// ADRGaps lists the missing ADR numbers (e.g. [2, 4]) when
	// docs/decisions/ filenames are not sequential starting from 1.
	ADRGaps []int `json:"adr_gaps"`

	// InvalidCovers lists "file: path" descriptions of covers: frontmatter
	// entries that do not resolve to an existing file or directory.
	InvalidCovers []string `json:"invalid_covers"`

	// OK is true when all four categories are empty.
	OK bool `json:"ok"`
}

// markdownLinkRe matches inline markdown links of the form [text](target).
// It captures the link target (group 1).
var markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

// adrFilenameRe matches ADR files named like 0001-some-title.md.
var adrFilenameRe = regexp.MustCompile(`^(\d{4})-.*\.md$`)

// docFrontmatter is used to parse the covers field from a doc's YAML
// frontmatter. Only the fields we need are declared here.
type docFrontmatter struct {
	Covers []string `yaml:"covers"`
}

// Check validates the docs/ structure under repoRoot and returns a
// CheckResult. It returns a non-nil error only when it cannot perform the
// validation at all (e.g. cannot read CONTRIBUTING.md or walk docs/).
func Check(repoRoot string) (*CheckResult, error) {
	result := &CheckResult{}

	unreachable, err := checkReachability(repoRoot)
	if err != nil {
		return nil, err
	}
	result.UnreachableFiles = unreachable

	deadLinks, err := checkDeadLinks(repoRoot)
	if err != nil {
		return nil, err
	}
	result.DeadLinks = deadLinks

	adrGaps, err := checkADRGaps(repoRoot)
	if err != nil {
		return nil, err
	}
	result.ADRGaps = adrGaps

	invalidCovers, err := checkCovers(repoRoot)
	if err != nil {
		return nil, err
	}
	result.InvalidCovers = invalidCovers

	result.OK = len(result.UnreachableFiles) == 0 &&
		len(result.DeadLinks) == 0 &&
		len(result.ADRGaps) == 0 &&
		len(result.InvalidCovers) == 0

	return result, nil
}

// --------------------------------------------------------------------------
// Reachability check
// --------------------------------------------------------------------------

// reachabilityExemptPrefix lists docs/ subdirectories that are exempt from
// the reachability check. ADRs in docs/decisions/ form a sequential record
// indexed by number, not by explicit links from CONTRIBUTING.md.
// Postmortems in docs/postmortems/ follow the same convention.
var reachabilityExemptPrefixes = []string{
	"docs/decisions/",
	"docs/postmortems/",
}

// isExemptFromReachability returns true if the slash-separated relative path
// falls under a directory that is exempt from the reachability check.
func isExemptFromReachability(slashRel string) bool {
	for _, prefix := range reachabilityExemptPrefixes {
		if strings.HasPrefix(slashRel, prefix) {
			return true
		}
	}
	return false
}

// checkReachability returns the list of files inside docs/ (relative to
// repoRoot) that are not reachable by following markdown links starting
// from CONTRIBUTING.md. Files in docs/decisions/ and docs/postmortems/ are
// exempt — they are indexed by sequential numbering, not by explicit links.
func checkReachability(repoRoot string) ([]string, error) {
	docsDir := filepath.Join(repoRoot, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return nil, nil
	}

	// Collect every file under docs/ that is not exempt.
	all := map[string]struct{}{}
	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			slashRel := filepath.ToSlash(rel)
			if !isExemptFromReachability(slashRel) {
				all[slashRel] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk docs/: %w", err)
	}

	if len(all) == 0 {
		return nil, nil
	}

	// BFS from CONTRIBUTING.md collecting reachable paths.
	reachable, err := reachableFromContributing(repoRoot)
	if err != nil {
		return nil, err
	}

	var unreachable []string
	for path := range all {
		if _, ok := reachable[path]; !ok {
			unreachable = append(unreachable, path)
		}
	}
	sort.Strings(unreachable)
	return unreachable, nil
}

// reachableFromContributing performs a BFS from CONTRIBUTING.md and returns
// the set of doc paths (slash-separated, relative to repoRoot) that are
// reachable by following relative markdown links.
func reachableFromContributing(repoRoot string) (map[string]struct{}, error) {
	reachable := map[string]struct{}{}
	queue := []string{filepath.Join(repoRoot, "CONTRIBUTING.md")}
	visited := map[string]struct{}{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}

		raw, err := os.ReadFile(current)
		if err != nil {
			// If the file doesn't exist (dead link target), just skip.
			continue
		}

		for _, link := range extractRelativeLinks(raw) {
			// Resolve the link relative to the directory of the current file.
			target := filepath.Join(filepath.Dir(current), filepath.FromSlash(link))
			// Normalise to slash-separated relative path for the set.
			rel, err := filepath.Rel(repoRoot, target)
			if err != nil {
				continue
			}
			slashRel := filepath.ToSlash(rel)
			reachable[slashRel] = struct{}{}
			queue = append(queue, target)
		}
	}

	return reachable, nil
}

// --------------------------------------------------------------------------
// Dead link check
// --------------------------------------------------------------------------

// checkDeadLinks scans every markdown file in docs/ (and CONTRIBUTING.md) for
// relative links whose target does not exist on the filesystem. Returns
// descriptive strings of the form "file: link".
func checkDeadLinks(repoRoot string) ([]string, error) {
	files, err := allMarkdownFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	var dead []string
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, link := range extractRelativeLinks(raw) {
			target := filepath.Join(filepath.Dir(file), filepath.FromSlash(link))
			if _, err := os.Stat(target); os.IsNotExist(err) {
				rel, relErr := filepath.Rel(repoRoot, file)
				if relErr != nil {
					rel = file
				}
				dead = append(dead, fmt.Sprintf("%s: %s", filepath.ToSlash(rel), link))
			}
		}
	}
	sort.Strings(dead)
	return dead, nil
}

// extractRelativeLinks returns all relative link targets found in raw markdown
// content. External links (starting with http:// or https://) are excluded, as
// are fragment-only anchors (starting with #).
func extractRelativeLinks(raw []byte) []string {
	matches := markdownLinkRe.FindAllSubmatch(raw, -1)
	var links []string
	for _, m := range matches {
		target := string(m[2])
		// Skip external links.
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		// Skip anchor-only links.
		if strings.HasPrefix(target, "#") {
			continue
		}
		// Strip any fragment component from the path (e.g. page.md#section).
		if idx := strings.IndexByte(target, '#'); idx >= 0 {
			target = target[:idx]
		}
		if target == "" {
			continue
		}
		links = append(links, target)
	}
	return links
}

// --------------------------------------------------------------------------
// ADR gap check
// --------------------------------------------------------------------------

// checkADRGaps lists the ADR numbers that are missing from the sequential
// sequence expected in docs/decisions/. Files that do not match the ADR
// naming pattern (NNNN-title.md) are ignored.
func checkADRGaps(repoRoot string) ([]int, error) {
	decisionsDir := filepath.Join(repoRoot, "docs", "decisions")
	if _, err := os.Stat(decisionsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(decisionsDir)
	if err != nil {
		return nil, fmt.Errorf("read docs/decisions/: %w", err)
	}

	var numbers []int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := adrFilenameRe.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		numbers = append(numbers, n)
	}

	if len(numbers) == 0 {
		return nil, nil
	}

	sort.Ints(numbers)

	var gaps []int
	for i := 1; i <= numbers[len(numbers)-1]; i++ {
		found := false
		for _, n := range numbers {
			if n == i {
				found = true
				break
			}
		}
		if !found {
			gaps = append(gaps, i)
		}
	}
	return gaps, nil
}

// --------------------------------------------------------------------------
// Covers check
// --------------------------------------------------------------------------

// checkCovers scans all markdown files in docs/ for YAML frontmatter with a
// covers: field. Each path listed under covers must resolve to an existing
// file or directory relative to repoRoot. Returns descriptive strings of the
// form "file: path".
func checkCovers(repoRoot string) ([]string, error) {
	files, err := allMarkdownFilesInDocs(repoRoot)
	if err != nil {
		return nil, err
	}

	var invalid []string
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var fm docFrontmatter
		if _, parseErr := frontmatter.Parse(bytes.NewReader(raw), &fm, frontmatter.NewFormat("---", "---", yaml.Unmarshal)); parseErr != nil {
			// No valid frontmatter — skip.
			continue
		}

		for _, coverPath := range fm.Covers {
			target := filepath.Join(repoRoot, filepath.FromSlash(coverPath))
			if _, err := os.Stat(target); os.IsNotExist(err) {
				rel, relErr := filepath.Rel(repoRoot, file)
				if relErr != nil {
					rel = file
				}
				invalid = append(invalid, fmt.Sprintf("%s: %s", filepath.ToSlash(rel), coverPath))
			}
		}
	}
	sort.Strings(invalid)
	return invalid, nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// allMarkdownFiles returns the absolute paths of every .md file that Check
// should scan for dead links: CONTRIBUTING.md (if present) plus all .md
// files under docs/.
func allMarkdownFiles(repoRoot string) ([]string, error) {
	var files []string

	contributing := filepath.Join(repoRoot, "CONTRIBUTING.md")
	if _, err := os.Stat(contributing); err == nil {
		files = append(files, contributing)
	}

	docsFiles, err := allMarkdownFilesInDocs(repoRoot)
	if err != nil {
		return nil, err
	}
	files = append(files, docsFiles...)
	return files, nil
}

// allMarkdownFilesInDocs returns the absolute paths of every .md file under
// docs/, or nil if the docs/ directory does not exist.
func allMarkdownFilesInDocs(repoRoot string) ([]string, error) {
	docsDir := filepath.Join(repoRoot, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var files []string
	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk docs/: %w", err)
	}
	return files, nil
}
