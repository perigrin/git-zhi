// ABOUTME: Git ref storage operations for chain entities (issues, milestones, config).
// ABOUTME: Wraps go-git to create blobs, trees, and commits on per-entity refs.
package storage

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Store provides read/write access to chain entity refs in a git repository.
type Store struct {
	repo        *git.Repository
	authorName  string
	authorEmail string
}

// NewStore creates a Store backed by the given git repository, reading author
// info from git config if available.
func NewStore(repo *git.Repository) (*Store, error) {
	name := "git-zhi"
	email := "git-zhi@local"

	cfg, err := repo.Config()
	if err == nil {
		if cfg.User.Name != "" {
			name = cfg.User.Name
		}
		if cfg.User.Email != "" {
			email = cfg.User.Email
		}
	}

	return &Store{
		repo:        repo,
		authorName:  name,
		authorEmail: email,
	}, nil
}

// WriteEntity creates a blob from content, wraps it in a tree with filename,
// creates a commit (with parent if the ref already exists), and updates the ref.
func (s *Store) WriteEntity(refPath, filename string, content []byte, message string) error {
	st := s.repo.Storer

	// Create blob object.
	blob := &plumbing.MemoryObject{}
	blob.SetType(plumbing.BlobObject)
	blob.SetSize(int64(len(content)))
	if _, err := blob.Write(content); err != nil {
		return fmt.Errorf("write blob content: %w", err)
	}
	blobHash, err := st.SetEncodedObject(blob)
	if err != nil {
		return fmt.Errorf("store blob: %w", err)
	}

	// Create tree object with one entry for the file.
	tree := &object.Tree{
		Entries: []object.TreeEntry{
			{
				Name: filename,
				Mode: filemode.Regular,
				Hash: blobHash,
			},
		},
	}
	treeObj := &plumbing.MemoryObject{}
	if err := tree.Encode(treeObj); err != nil {
		return fmt.Errorf("encode tree: %w", err)
	}
	treeHash, err := st.SetEncodedObject(treeObj)
	if err != nil {
		return fmt.Errorf("store tree: %w", err)
	}

	// Build commit, attaching the existing tip as parent if the ref exists.
	sig := object.Signature{
		Name:  s.authorName,
		Email: s.authorEmail,
		When:  time.Now(),
	}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   message,
		TreeHash:  treeHash,
	}

	refName := plumbing.ReferenceName(refPath)
	existingRef, err := st.Reference(refName)
	if err == nil {
		commit.ParentHashes = []plumbing.Hash{existingRef.Hash()}
	}

	commitObj := &plumbing.MemoryObject{}
	if err := commit.Encode(commitObj); err != nil {
		return fmt.Errorf("encode commit: %w", err)
	}
	commitHash, err := st.SetEncodedObject(commitObj)
	if err != nil {
		return fmt.Errorf("store commit: %w", err)
	}

	// Update the ref to point to the new commit.
	ref := plumbing.NewHashReference(refName, commitHash)
	if err := st.SetReference(ref); err != nil {
		return fmt.Errorf("set reference %s: %w", refPath, err)
	}

	return nil
}

// ReadEntity resolves a ref to its latest commit, walks the tree, and returns
// the contents of the named file blob.
func (s *Store) ReadEntity(refPath, filename string) ([]byte, error) {
	st := s.repo.Storer

	refName := plumbing.ReferenceName(refPath)
	ref, err := st.Reference(refName)
	if err != nil {
		return nil, fmt.Errorf("resolve ref %s: %w", refPath, err)
	}

	commitObj, err := st.EncodedObject(plumbing.CommitObject, ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("get commit %s: %w", ref.Hash(), err)
	}
	commit, err := object.DecodeCommit(st, commitObj)
	if err != nil {
		return nil, fmt.Errorf("decode commit: %w", err)
	}

	treeObj, err := st.EncodedObject(plumbing.TreeObject, commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("get tree %s: %w", commit.TreeHash, err)
	}
	tree, err := object.DecodeTree(st, treeObj)
	if err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}

	var blobHash plumbing.Hash
	found := false
	for _, entry := range tree.Entries {
		if entry.Name == filename {
			blobHash = entry.Hash
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("file %q not found in tree at %s", filename, refPath)
	}

	blobObj, err := st.EncodedObject(plumbing.BlobObject, blobHash)
	if err != nil {
		return nil, fmt.Errorf("get blob %s: %w", blobHash, err)
	}
	reader, err := blobObj.Reader()
	if err != nil {
		return nil, fmt.Errorf("open blob reader: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}
	return data, nil
}

// ListRefs returns all ref names that start with the given prefix.
// Note: go-git's IterReferences walks ALL refs in the repo (branches, tags,
// remote tracking refs, chain refs). This is O(total refs) per call.
// Acceptable for v0.1 scale (tens to low hundreds of issues) but should be
// revisited if performance becomes a concern with large repos.
func (s *Store) ListRefs(prefix string) ([]string, error) {
	iter, err := s.repo.Storer.IterReferences()
	if err != nil {
		return nil, fmt.Errorf("iterate references: %w", err)
	}
	defer iter.Close()

	var refs []string
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if strings.HasPrefix(name, prefix) {
			refs = append(refs, name)
		}
		return nil
	})
	if err != nil && err != storer.ErrStop {
		return nil, fmt.Errorf("iterating references: %w", err)
	}
	return refs, nil
}

// DeleteRef removes a ref. Used for --untag.
func (s *Store) DeleteRef(refPath string) error {
	return s.repo.Storer.RemoveReference(plumbing.ReferenceName(refPath))
}

// SetRefToHash points refPath at an existing commit SHA without creating a new
// commit. Used to copy a ref pointer (e.g. during worktree-local migration);
// the referenced objects must already exist in the object store.
func (s *Store) SetRefToHash(refPath, sha string) error {
	ref := plumbing.NewHashReference(plumbing.ReferenceName(refPath), plumbing.NewHash(sha))
	return s.repo.Storer.SetReference(ref)
}

// MigrateStrandedWorktreeRefs copies any worktree-local refs under refPrefix
// into the shared namespace when they are not already present, covering both
// loose refs (refs/<prefix>/...) and entries in the worktree's own packed-refs
// file. Older git-zhi versions, lacking commondir support, wrote chain refs to
// the linked worktree's own refs storage; newer versions read the shared
// common-dir namespace and would otherwise not see them.
//
// worktreeGitDir is the per-worktree git directory (git rev-parse --git-dir);
// commonGitDir is the shared common dir (git rev-parse --git-common-dir). When
// they are equal, this is the main worktree and there is nothing to migrate.
// Returns the number of refs migrated.
func (s *Store) MigrateStrandedWorktreeRefs(worktreeGitDir, commonGitDir, refPrefix string) (int, error) {
	if worktreeGitDir == "" || commonGitDir == "" {
		return 0, nil
	}
	wtAbs, err := filepath.Abs(worktreeGitDir)
	if err != nil {
		return 0, fmt.Errorf("resolve worktree git dir: %w", err)
	}
	commonAbs, err := filepath.Abs(commonGitDir)
	if err != nil {
		return 0, fmt.Errorf("resolve common git dir: %w", err)
	}
	if wtAbs == commonAbs {
		// Main worktree: refs already live in the shared namespace.
		return 0, nil
	}

	// Loose refs under <worktree-git-dir>/refs/zhi.
	srcDir := filepath.Join(wtAbs, filepath.FromSlash(refPrefix))
	looseCount, err := MigrateLooseRefs(srcDir, refPrefix, s.RefExists, s.SetRefToHash)
	if err != nil {
		return looseCount, err
	}

	// Packed refs that a gc/pack-refs run may have moved into the worktree's
	// own packed-refs file. Loose entries already migrated above take
	// precedence; RefExists now reports them as present, so packed duplicates
	// are skipped.
	packedRefsPath := filepath.Join(wtAbs, "packed-refs")
	packedCount, err := MigratePackedRefs(packedRefsPath, refPrefix, s.RefExists, s.SetRefToHash)
	if err != nil {
		return looseCount + packedCount, err
	}

	return looseCount + packedCount, nil
}

// RefExists reports whether the given ref path resolves to a valid reference.
func (s *Store) RefExists(refPath string) bool {
	_, err := s.repo.Storer.Reference(plumbing.ReferenceName(refPath))
	return err == nil
}

// RepoHEAD returns the current HEAD commit SHA of the working repository.
func (s *Store) RepoHEAD() (string, error) {
	ref, err := s.repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD: %w", err)
	}
	return ref.Hash().String(), nil
}

// DiffNameOnly returns the file paths changed between fromSHA (exclusive) and
// toSHA (inclusive), equivalent to `git diff --name-only fromSHA..toSHA`.
// Returns an empty slice when fromSHA == toSHA.
func (s *Store) DiffNameOnly(fromSHA, toSHA string) ([]string, error) {
	if fromSHA == toSHA {
		return []string{}, nil
	}

	fromHash := plumbing.NewHash(fromSHA)
	toHash := plumbing.NewHash(toSHA)

	fromCommitObj, err := s.repo.Storer.EncodedObject(plumbing.CommitObject, fromHash)
	if err != nil {
		return nil, fmt.Errorf("get from-commit %s: %w", fromSHA, err)
	}
	fromCommit, err := object.DecodeCommit(s.repo.Storer, fromCommitObj)
	if err != nil {
		return nil, fmt.Errorf("decode from-commit: %w", err)
	}

	toCommitObj, err := s.repo.Storer.EncodedObject(plumbing.CommitObject, toHash)
	if err != nil {
		return nil, fmt.Errorf("get to-commit %s: %w", toSHA, err)
	}
	toCommit, err := object.DecodeCommit(s.repo.Storer, toCommitObj)
	if err != nil {
		return nil, fmt.Errorf("decode to-commit: %w", err)
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get from-tree: %w", err)
	}

	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get to-tree: %w", err)
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return nil, fmt.Errorf("diff trees: %w", err)
	}

	seen := make(map[string]struct{})
	var paths []string
	for _, change := range changes {
		// Use the To path when available (file added or modified); fall back
		// to From path for deleted files.
		name := change.To.Name
		if name == "" {
			name = change.From.Name
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			paths = append(paths, name)
		}
	}
	return paths, nil
}

// AuthorInfo returns the git author name and email configured for this Store.
// These values come from the repository's git config at store creation time.
func (s *Store) AuthorInfo() (name, email string) {
	return s.authorName, s.authorEmail
}

// CountCommits counts the number of commits between startSHA (exclusive) and
// endSHA (inclusive) by walking the commit log backward from endSHA.
// Returns 0 if startSHA == endSHA.
func (s *Store) CountCommits(startSHA, endSHA string) (int, error) {
	if startSHA == endSHA {
		return 0, nil
	}

	endHash := plumbing.NewHash(endSHA)
	startHash := plumbing.NewHash(startSHA)

	iter, err := s.repo.Log(&git.LogOptions{From: endHash})
	if err != nil {
		return 0, fmt.Errorf("get commit log from %s: %w", endSHA, err)
	}
	defer iter.Close()

	count := 0
	found := false
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == startHash {
			found = true
			return storer.ErrStop
		}
		count++
		return nil
	})
	if err != nil && err != storer.ErrStop {
		return 0, fmt.Errorf("walk commit log: %w", err)
	}
	if !found {
		return 0, fmt.Errorf("start SHA %s not found in ancestry of %s", startSHA, endSHA)
	}
	return count, nil
}
