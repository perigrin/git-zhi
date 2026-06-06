// ABOUTME: Tests for migrating stranded worktree-local refs/zhi into the shared namespace.
// ABOUTME: Covers copy-of-missing, skip-of-existing, and the empty/no-op cases.

package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/perigrin/git-zhi/internal/storage"
)

// writeLooseRef writes a loose ref file (a 40/64-char SHA + newline) under dir.
func writeLooseRef(t *testing.T, refsDir, refSubPath, sha string) {
	t.Helper()
	full := filepath.Join(refsDir, filepath.FromSlash(refSubPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(sha+"\n"), 0o644); err != nil {
		t.Fatalf("write loose ref: %v", err)
	}
}

func TestMigrateLooseRefs_CopiesMissingSkipsExisting(t *testing.T) {
	srcDir := t.TempDir() // stands in for .git/worktrees/<name>/refs/zhi

	sha1 := "1111111111111111111111111111111111111111"
	sha2 := "2222222222222222222222222222222222222222"
	sha3 := "3333333333333333333333333333333333333333"

	// Source (worktree-local) holds three refs.
	writeLooseRef(t, srcDir, "_/issues/aaa", sha1)
	writeLooseRef(t, srcDir, "_/issues/bbb", sha2)
	writeLooseRef(t, srcDir, "_/milestones/v0.1", sha3)

	// Shared namespace already has the milestone — it must be skipped.
	existing := map[string]bool{
		"refs/zhi/_/milestones/v0.1": true,
	}
	var written = map[string]string{}

	setRef := func(name, sha string) error {
		written[name] = sha
		return nil
	}
	refExists := func(name string) bool {
		return existing[name]
	}

	migrated, err := storage.MigrateLooseRefs(srcDir, "refs/zhi", refExists, setRef)
	if err != nil {
		t.Fatalf("MigrateLooseRefs: %v", err)
	}

	if migrated != 2 {
		t.Errorf("migrated = %d, want 2", migrated)
	}
	if written["refs/zhi/_/issues/aaa"] != sha1 {
		t.Errorf("issue aaa = %q, want %q", written["refs/zhi/_/issues/aaa"], sha1)
	}
	if written["refs/zhi/_/issues/bbb"] != sha2 {
		t.Errorf("issue bbb = %q, want %q", written["refs/zhi/_/issues/bbb"], sha2)
	}
	if _, ok := written["refs/zhi/_/milestones/v0.1"]; ok {
		t.Error("existing milestone v0.1 should have been skipped, not written")
	}
}

func TestMigrateLooseRefs_SkipsNonHashContent(t *testing.T) {
	srcDir := t.TempDir()

	validSHA := "1111111111111111111111111111111111111111"
	// A genuine, migratable ref.
	writeLooseRef(t, srcDir, "_/issues/good", validSHA)
	// A symbolic ref — must be skipped.
	writeLooseRef(t, srcDir, "_/issues/symbolic", "ref: refs/zhi/_/issues/good")
	// Garbage / short content — must be skipped, not zero-padded into a bogus ref.
	writeLooseRef(t, srcDir, "_/issues/garbage", "not-a-sha")
	// A leftover lock file — must be skipped.
	writeLooseRef(t, srcDir, "_/issues/good.lock", validSHA)

	written := map[string]string{}
	migrated, err := storage.MigrateLooseRefs(srcDir, "refs/zhi",
		func(string) bool { return false },
		func(name, sha string) error { written[name] = sha; return nil },
	)
	if err != nil {
		t.Fatalf("MigrateLooseRefs: %v", err)
	}
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1 (only the valid hash ref)", migrated)
	}
	if written["refs/zhi/_/issues/good"] != validSHA {
		t.Errorf("valid ref not migrated: %v", written)
	}
	for _, bad := range []string{"refs/zhi/_/issues/symbolic", "refs/zhi/_/issues/garbage", "refs/zhi/_/issues/good.lock"} {
		if _, ok := written[bad]; ok {
			t.Errorf("non-hash entry %q should have been skipped, got %q", bad, written[bad])
		}
	}
}

func TestMigrateLooseRefs_NoSourceDir(t *testing.T) {
	// A non-existent source directory is a no-op, not an error.
	migrated, err := storage.MigrateLooseRefs(filepath.Join(t.TempDir(), "missing"), "refs/zhi",
		func(string) bool { return false },
		func(string, string) error { return nil },
	)
	if err != nil {
		t.Fatalf("MigrateLooseRefs on missing dir: %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0", migrated)
	}
}
