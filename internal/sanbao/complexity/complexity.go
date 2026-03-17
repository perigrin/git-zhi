// ABOUTME: Language-agnostic code complexity metrics derived from git history and filesystem.
// ABOUTME: Provides FileChurn, FileSize, ChurnSizeHotspots, ChangeCoupling, and IndentationDepth.
package complexity

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"github.com/perigrin/git-zhi/internal/issue"
)

// ChurnSizeHotspot represents a file ranked by the product of its churn count
// and its line count. Higher scores indicate files that change frequently and
// are large — a common proxy for structural complexity risk.
type ChurnSizeHotspot struct {
	File  string `json:"file"`
	Churn int    `json:"churn"`
	Size  int    `json:"size"`
	Score int    `json:"score"` // churn * size
}

// FileChurn counts how many times each file was modified across all commits
// in the given session ranges. Each session's range is (StartSHA, EndSHA] —
// exclusive of StartSHA, inclusive of EndSHA. Sessions with an empty EndSHA
// are skipped (open/incomplete sessions).
func FileChurn(repo *git.Repository, sessions []issue.Session) map[string]int {
	churn := map[string]int{}
	for _, sess := range sessions {
		if sess.EndSHA == "" || sess.StartSHA == sess.EndSHA {
			continue
		}
		filesInRange(repo, sess.StartSHA, sess.EndSHA, func(path string) {
			churn[path]++
		})
	}
	return churn
}

// filesInRange calls fn for each file changed in any commit in the range
// (startSHA, endSHA]. The walk starts from endSHA and stops when startSHA
// is encountered, following the same convention used throughout the sanbao
// sub-packages.
func filesInRange(repo *git.Repository, startSHA, endSHA string, fn func(string)) {
	endHash := plumbing.NewHash(endSHA)
	startHash := plumbing.NewHash(startSHA)

	iter, err := repo.Log(&git.LogOptions{From: endHash})
	if err != nil {
		return
	}
	defer iter.Close()

	_ = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == startHash {
			return storer.ErrStop
		}
		paths, err := changedFilesInCommit(repo, c)
		if err != nil {
			return nil
		}
		for _, p := range paths {
			fn(p)
		}
		return nil
	})
}

// changedFilesInCommit returns the set of file paths modified in the given
// commit by diffing it against its first parent. Initial commits (no parent)
// return all files in the tree.
func changedFilesInCommit(repo *git.Repository, c *object.Commit) ([]string, error) {
	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parents().Next()
		if err != nil {
			return nil, err
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, err
		}
	}

	commitTree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := commitTree.Diff(parentTree)
	if err != nil {
		// Diff is (new, old); try reversed if first attempt fails.
		if parentTree != nil {
			changes, err = parentTree.Diff(commitTree)
		}
		if err != nil {
			return nil, err
		}
	}

	seen := map[string]struct{}{}
	for _, ch := range changes {
		if ch.From.Name != "" {
			seen[ch.From.Name] = struct{}{}
		}
		if ch.To.Name != "" {
			seen[ch.To.Name] = struct{}{}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	return paths, nil
}

// FileSize returns the number of lines in the file at filePath relative to
// repoRoot. Returns 0 if the file does not exist or cannot be read. A file
// with content but no newline at the end still counts its last line.
func FileSize(repoRoot string, filePath string) int {
	data, err := os.ReadFile(filepath.Join(repoRoot, filePath))
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	// Count newlines to get line count. If there is no trailing newline,
	// the last "line" still exists and must be counted.
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// ChurnSizeHotspots builds the ranked list of files from the churn map.
// For each file, it computes its current line count and the churn*size score.
// The result is sorted descending by Score. Files missing from disk get size 0.
func ChurnSizeHotspots(churn map[string]int, repoRoot string) []ChurnSizeHotspot {
	hotspots := make([]ChurnSizeHotspot, 0, len(churn))
	for file, c := range churn {
		size := FileSize(repoRoot, file)
		hotspots = append(hotspots, ChurnSizeHotspot{
			File:  file,
			Churn: c,
			Size:  size,
			Score: c * size,
		})
	}
	sort.Slice(hotspots, func(i, j int) bool {
		if hotspots[i].Score != hotspots[j].Score {
			return hotspots[i].Score > hotspots[j].Score
		}
		// Stable secondary sort by file name for deterministic output.
		return hotspots[i].File < hotspots[j].File
	})
	return hotspots
}

// ChangeCoupling builds a co-occurrence matrix: for each commit in the session
// ranges, it finds all files changed together and increments the pair's count.
// Only pairs with count > 1 are included in the result — single co-occurrences
// are not meaningful coupling signals. Keys are always sorted so [a,b] and
// [b,a] map to the same entry.
func ChangeCoupling(repo *git.Repository, sessions []issue.Session) map[[2]string]int {
	raw := map[[2]string]int{}

	for _, sess := range sessions {
		if sess.EndSHA == "" || sess.StartSHA == sess.EndSHA {
			continue
		}
		endHash := plumbing.NewHash(sess.EndSHA)
		startHash := plumbing.NewHash(sess.StartSHA)

		iter, err := repo.Log(&git.LogOptions{From: endHash})
		if err != nil {
			continue
		}

		_ = iter.ForEach(func(c *object.Commit) error {
			if c.Hash == startHash {
				return storer.ErrStop
			}
			files, err := changedFilesInCommit(repo, c)
			if err != nil || len(files) < 2 {
				return nil
			}
			// Sort for deterministic pair keys.
			sorted := make([]string, len(files))
			copy(sorted, files)
			sort.Strings(sorted)
			for i := 0; i < len(sorted); i++ {
				for j := i + 1; j < len(sorted); j++ {
					key := [2]string{sorted[i], sorted[j]}
					raw[key]++
				}
			}
			return nil
		})
		iter.Close()
	}

	// Filter to pairs with count > 1.
	result := map[[2]string]int{}
	for pair, count := range raw {
		if count > 1 {
			result[pair] = count
		}
	}
	return result
}

// IndentationDepth scans the file at filePath relative to repoRoot and
// computes the average and maximum indentation depth across all non-empty
// lines. Each leading tab counts as one level; every four consecutive leading
// spaces count as one level (integer division). Returns (0.0, 0) for missing,
// unreadable, or empty files.
func IndentationDepth(repoRoot string, filePath string) (avgDepth float64, maxDepth int) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filePath))
	if err != nil || len(data) == 0 {
		return 0.0, 0
	}

	lines := strings.Split(string(data), "\n")
	var totalDepth int
	var lineCount int

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		depth := leadingIndentDepth(line)
		totalDepth += depth
		if depth > maxDepth {
			maxDepth = depth
		}
		lineCount++
	}

	if lineCount == 0 {
		return 0.0, 0
	}
	return float64(totalDepth) / float64(lineCount), maxDepth
}

// leadingIndentDepth returns the indentation depth of a single line.
// Leading tabs each count as one level. Leading spaces count as one level
// per four spaces (integer division). Mixed indentation is not supported;
// the first non-whitespace character ends the count.
func leadingIndentDepth(line string) int {
	depth := 0
	spaces := 0
	for _, ch := range line {
		switch ch {
		case '\t':
			// Flush any accumulated spaces first.
			depth += spaces / 4
			spaces = 0
			depth++
		case ' ':
			spaces++
		default:
			depth += spaces / 4
			return depth
		}
	}
	depth += spaces / 4
	return depth
}
