// ABOUTME: Git ref storage operations for chain entities (issues, milestones, config).
// ABOUTME: Wraps go-git to create blobs, trees, and commits on per-entity refs.
package storage

import git "github.com/go-git/go-git/v5"

// Store provides read/write access to chain entity refs in a git repository.
type Store struct {
	repo *git.Repository
}

// NewStore creates a Store backed by the given git repository.
func NewStore(repo *git.Repository) *Store {
	return &Store{repo: repo}
}
