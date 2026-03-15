// ABOUTME: Git ref storage operations for chain entities (issues, milestones, config).
// ABOUTME: Wraps go-git to create blobs, trees, and commits on per-entity refs.
package storage

import (
	"fmt"
	"io"
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
	name := "git-chain"
	email := "git-chain@local"

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
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == startHash {
			return storer.ErrStop
		}
		count++
		return nil
	})
	if err != nil && err != storer.ErrStop {
		return 0, fmt.Errorf("walk commit log: %w", err)
	}
	return count, nil
}
