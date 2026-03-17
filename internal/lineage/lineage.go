// ABOUTME: Lineage package maps git blame output back to issues through session SHA ranges.
// ABOUTME: BuildSessionIndex and ComputeLineage trace which issues authored each line of code.
package lineage

import (
	"sort"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
)

// LineageEntry records how many lines in a specific file trace back to a
// particular upstream issue via git blame and session SHA ranges.
type LineageEntry struct {
	UpstreamIssueID uuid.UUID
	Title           string
	FilePath        string
	LineCount       int
}

// BuildSessionIndex walks the commit history for each done issue's sessions and
// returns a map from commit SHA → issue UUID. Only done issues with sessions are
// indexed. The StartSHA of each session is exclusive (that commit is not
// attributed to this issue); the EndSHA is inclusive.
//
// If the repository is nil or a session range cannot be walked, that session is
// silently skipped.
func BuildSessionIndex(repo *git.Repository, issues []*issue.Issue) map[string]uuid.UUID {
	idx := make(map[string]uuid.UUID)
	if repo == nil {
		return idx
	}

	for _, iss := range issues {
		if iss.State != issue.StateDone {
			continue
		}
		for _, sess := range iss.Sessions {
			if sess.StartSHA == "" || sess.EndSHA == "" {
				continue
			}
			// StartSHA == EndSHA means an empty session with no commits.
			if sess.StartSHA == sess.EndSHA {
				continue
			}
			collectSessionSHAs(repo, sess.StartSHA, sess.EndSHA, iss.ID, idx)
		}
	}
	return idx
}

// collectSessionSHAs walks commits from endSHA (inclusive) back until startSHA
// (exclusive) and adds each commit SHA to idx mapped to issueID.
func collectSessionSHAs(repo *git.Repository, startSHA, endSHA string, issueID uuid.UUID, idx map[string]uuid.UUID) {
	endHash := plumbing.NewHash(endSHA)
	startHash := plumbing.NewHash(startSHA)

	iter, err := repo.Log(&git.LogOptions{From: endHash})
	if err != nil {
		return
	}
	defer iter.Close()

	_ = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == startHash {
			// StartSHA is exclusive — stop here without adding it.
			return storer.ErrStop
		}
		idx[c.Hash.String()] = issueID
		return nil
	})
}

// ComputeLineage runs git blame for each file in files, looks up each blame
// line's commit SHA in sessionIndex, and aggregates line counts per
// (issueID, filePath) pair. Files that cannot be blamed (missing, binary, etc.)
// are silently skipped. The returned slice is sorted by LineCount descending.
//
// issues is used only for title lookup; it may be nil, in which case Title
// fields in the result will be empty.
func ComputeLineage(repo *git.Repository, files []string, sessionIndex map[string]uuid.UUID, issues map[uuid.UUID]*issue.Issue) []LineageEntry {
	if repo == nil || len(files) == 0 || len(sessionIndex) == 0 {
		return []LineageEntry{}
	}

	// key: issueID + "|" + filePath → count
	type key struct {
		id       uuid.UUID
		filePath string
	}
	counts := make(map[key]int)

	// Resolve HEAD commit for blame.
	headRef, err := repo.Head()
	if err != nil {
		return []LineageEntry{}
	}
	headCommitObj, err := repo.Storer.EncodedObject(plumbing.CommitObject, headRef.Hash())
	if err != nil {
		return []LineageEntry{}
	}
	headCommit, err := object.DecodeCommit(repo.Storer, headCommitObj)
	if err != nil {
		return []LineageEntry{}
	}

	for _, filePath := range files {
		blameResult, err := git.Blame(headCommit, filePath)
		if err != nil {
			// File missing or unblameable — skip silently.
			continue
		}
		for _, line := range blameResult.Lines {
			sha := line.Hash.String()
			issueID, ok := sessionIndex[sha]
			if !ok {
				continue
			}
			counts[key{id: issueID, filePath: filePath}]++
		}
	}

	entries := make([]LineageEntry, 0, len(counts))
	for k, count := range counts {
		title := ""
		if issues != nil {
			if iss, ok := issues[k.id]; ok {
				title = iss.Title
			}
		}
		entries = append(entries, LineageEntry{
			UpstreamIssueID: k.id,
			Title:           title,
			FilePath:        k.filePath,
			LineCount:       count,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LineCount > entries[j].LineCount
	})

	return entries
}
