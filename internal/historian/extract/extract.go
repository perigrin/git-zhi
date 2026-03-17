// ABOUTME: Historian extract stage: parses git log into structured commit data.
// ABOUTME: Provides ExtractCommits, ExtractTicketRefs, and ComputeFingerprint.
package extract

import (
	"regexp"
	"sort"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DiffFingerprint summarises the shape of a commit's diff for similarity
// scoring in the cluster stage.
type DiffFingerprint struct {
	Paths       []string // sorted normalised file paths touched
	HunkCount   int      // number of change hunks
	Insertions  int      // total lines added
	Deletions   int      // total lines removed
	ChangeRatio float64  // insertions / (deletions + 1)
	Renames     int      // count of rename operations
	FileCount   int      // number of files touched
}

// CommitData holds the parsed information from a single git commit.
type CommitData struct {
	SHA         string
	Author      string
	Email       string
	Timestamp   time.Time
	Message     string
	TicketRefs  []string       // extracted ticket references sorted and deduped
	Fingerprint DiffFingerprint
}

// jiraTicketRE matches Jira-style ticket identifiers: one or more uppercase
// letters, a hyphen, then one or more digits (e.g. LOPS-142, AB-1).
var jiraTicketRE = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)

// githubRefRE matches GitHub-style issue references: a hash followed by digits
// (e.g. #42). The hash must not be immediately preceded by an alphanumeric
// character to avoid false positives inside URLs or hex strings.
var githubRefRE = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])(#\d+)`)

// ExtractTicketRefs parses commit message and returns a sorted, deduplicated
// slice of ticket references. Recognised patterns:
//   - Jira-style: one or more uppercase letters + hyphen + digits (e.g. LOPS-142)
//   - GitHub-style: # followed by digits (e.g. #42)
func ExtractTicketRefs(message string) []string {
	seen := make(map[string]struct{})

	for _, match := range jiraTicketRE.FindAllStringSubmatch(message, -1) {
		seen[match[1]] = struct{}{}
	}
	for _, match := range githubRefRE.FindAllStringSubmatch(message, -1) {
		seen[match[1]] = struct{}{}
	}

	result := make([]string, 0, len(seen))
	for ref := range seen {
		result = append(result, ref)
	}
	sort.Strings(result)
	return result
}

// ComputeFingerprint diffs commit against its first parent and builds a
// DiffFingerprint. For the root commit (no parent) the diff is computed
// against an empty tree. The patch stats (insertions, deletions, hunks) are
// accumulated across all changed files.
func ComputeFingerprint(commit *object.Commit, repo *git.Repository) DiffFingerprint {
	var fp DiffFingerprint

	commitTree, err := commit.Tree()
	if err != nil {
		return fp
	}

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parents().Next()
		if err != nil {
			return fp
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return fp
		}
	}

	// Diff parent tree → commit tree (parentTree may be nil for root commits).
	var changes object.Changes
	if parentTree == nil {
		// Root commit: diff empty tree against commit tree.
		changes, err = object.DiffTree(nil, commitTree)
	} else {
		changes, err = parentTree.Diff(commitTree)
	}
	if err != nil {
		return fp
	}

	seen := make(map[string]struct{})

	for _, change := range changes {
		// Determine the file path: prefer To (new name), fall back to From.
		path := change.To.Name
		if path == "" {
			path = change.From.Name
		}
		if path == "" {
			continue
		}

		// Detect renames: both From and To are non-empty and differ.
		if change.From.Name != "" && change.To.Name != "" && change.From.Name != change.To.Name {
			fp.Renames++
		}

		if _, exists := seen[path]; !exists {
			seen[path] = struct{}{}
			fp.Paths = append(fp.Paths, path)
		}

		// Get the patch for this change to count lines.
		patch, err := change.Patch()
		if err != nil {
			continue
		}
		stats := patch.Stats()
		fp.HunkCount += countHunks(patch)
		for _, s := range stats {
			fp.Insertions += s.Addition
			fp.Deletions += s.Deletion
		}
	}

	sort.Strings(fp.Paths)
	fp.FileCount = len(fp.Paths)
	fp.ChangeRatio = float64(fp.Insertions) / float64(fp.Deletions+1)

	return fp
}

// countHunks counts the total number of diff hunks across all file patches
// in the given Patch.
func countHunks(patch *object.Patch) int {
	count := 0
	for _, fp := range patch.FilePatches() {
		count += len(fp.Chunks())
	}
	return count
}

// ExtractCommits walks the repository from HEAD backward, collects commits,
// filters by the since cutoff when provided (inclusive: commits at exactly
// since are included), and returns them in chronological order (oldest first).
// If the repository has no commits (empty HEAD), an empty slice is returned
// without error.
//
// The full log is always walked rather than using early termination on
// timestamp comparison. Merge commits from feature branches have older author
// timestamps but appear late in the walk; early termination based on author
// timestamp would silently miss them.
func ExtractCommits(repo *git.Repository, since *time.Time) ([]CommitData, error) {
	head, err := repo.Head()
	if err != nil {
		// Empty repository — no commits yet.
		return []CommitData{}, nil
	}

	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var commits []CommitData
	err = iter.ForEach(func(c *object.Commit) error {
		ts := c.Author.When
		// Filter by since after collection so merge commits with older author
		// timestamps are not skipped when they appear late in the walk order.
		if since != nil && ts.Before(*since) {
			return nil
		}
		cd := CommitData{
			SHA:         c.Hash.String(),
			Author:      c.Author.Name,
			Email:       c.Author.Email,
			Timestamp:   ts,
			Message:     c.Message,
			TicketRefs:  ExtractTicketRefs(c.Message),
			Fingerprint: ComputeFingerprint(c, repo),
		}
		commits = append(commits, cd)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Log walks newest-first; reverse to produce chronological (oldest-first) order.
	for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
		commits[i], commits[j] = commits[j], commits[i]
	}

	return commits, nil
}
