package storage_test

import (
	"testing"

	git "github.com/go-git/go-git/v5"
	gitstorage "github.com/go-git/go-git/v5/storage/memory"

	"github.com/perigrin/git-chain/internal/storage"
)

// This test uses a bare in-memory repo for constructor validation only.
// Write-path tests (issue 2) must use a non-bare repo with a filesystem
// worktree to match real usage via git.PlainOpen.
func TestNewStore(t *testing.T) {
	repo, err := git.Init(gitstorage.NewStorage(), nil)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	store := storage.NewStore(repo)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}
