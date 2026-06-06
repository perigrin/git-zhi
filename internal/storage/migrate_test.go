// ABOUTME: Tests for migrating stranded worktree-local refs/zhi into the shared namespace.
// ABOUTME: Covers copy-of-missing, skip-of-existing, and the empty/no-op cases.

package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/storage"
)

// errBoom is a sentinel used to exercise setRef failure paths.
var errBoom = errors.New("boom")

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

func TestMigratePackedRefs_CopiesMissingSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	packedPath := filepath.Join(dir, "packed-refs")

	sha1 := "1111111111111111111111111111111111111111"
	sha2 := "2222222222222222222222222222222222222222"
	sha3 := "3333333333333333333333333333333333333333"
	sha4 := "4444444444444444444444444444444444444444"
	peeled := "5555555555555555555555555555555555555555"

	content := "" +
		"# pack-refs with: peeled fully-peeled sorted\n" +
		sha1 + " refs/zhi/_/issues/aaa\n" +
		sha2 + " refs/zhi/_/issues/bbb\n" +
		sha3 + " refs/zhi/_/milestones/v0.1\n" + // already exists -> skip
		"^" + peeled + "\n" + // peeled line for the milestone tag -> ignore
		sha4 + " refs/heads/unrelated\n" // not a zhi ref -> skip
	if err := os.WriteFile(packedPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	existing := map[string]bool{"refs/zhi/_/milestones/v0.1": true}
	written := map[string]string{}

	migrated, err := storage.MigratePackedRefs(packedPath, "refs/zhi",
		func(name string) bool { return existing[name] },
		func(name, sha string) error { written[name] = sha; return nil },
	)
	if err != nil {
		t.Fatalf("MigratePackedRefs: %v", err)
	}

	if migrated != 2 {
		t.Errorf("migrated = %d, want 2", migrated)
	}
	if written["refs/zhi/_/issues/aaa"] != sha1 {
		t.Errorf("aaa = %q, want %q", written["refs/zhi/_/issues/aaa"], sha1)
	}
	if written["refs/zhi/_/issues/bbb"] != sha2 {
		t.Errorf("bbb = %q, want %q", written["refs/zhi/_/issues/bbb"], sha2)
	}
	if _, ok := written["refs/zhi/_/milestones/v0.1"]; ok {
		t.Error("existing milestone v0.1 should have been skipped")
	}
	if _, ok := written["refs/heads/unrelated"]; ok {
		t.Error("non-zhi ref should not be migrated")
	}
}

func TestMigratePackedRefs_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	packedPath := filepath.Join(dir, "packed-refs")

	valid := "1111111111111111111111111111111111111111"
	content := "" +
		"\n" + // blank line
		"   \n" + // whitespace line
		"notasha refs/zhi/_/issues/garbage\n" + // bad SHA -> skip
		valid + "\n" + // no ref name -> skip
		valid + " refs/zhi/_/issues/good\n"
	if err := os.WriteFile(packedPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	written := map[string]string{}
	migrated, err := storage.MigratePackedRefs(packedPath, "refs/zhi",
		func(string) bool { return false },
		func(name, sha string) error { written[name] = sha; return nil },
	)
	if err != nil {
		t.Fatalf("MigratePackedRefs: %v", err)
	}
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1 (only the valid line)", migrated)
	}
	if written["refs/zhi/_/issues/good"] != valid {
		t.Errorf("good ref not migrated: %v", written)
	}
}

func TestMigratePackedRefs_SetRefErrorReturnsPartialCount(t *testing.T) {
	dir := t.TempDir()
	packedPath := filepath.Join(dir, "packed-refs")
	sha := "1111111111111111111111111111111111111111"
	content := sha + " refs/zhi/_/issues/first\n" +
		sha + " refs/zhi/_/issues/boom\n" +
		sha + " refs/zhi/_/issues/third\n"
	if err := os.WriteFile(packedPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	count := 0
	migrated, err := storage.MigratePackedRefs(packedPath, "refs/zhi",
		func(string) bool { return false },
		func(name, _ string) error {
			if name == "refs/zhi/_/issues/boom" {
				return errBoom
			}
			count++
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected error from setRef failure, got nil")
	}
	if !strings.Contains(err.Error(), "set ref refs/zhi/_/issues/boom") {
		t.Errorf("error should name the failing ref, got %v", err)
	}
	// One ref migrated before the failure; the function stops at the error.
	if migrated != 1 {
		t.Errorf("migrated = %d, want 1 (partial count before failure)", migrated)
	}
}

func TestMigratePackedRefs_NoFile(t *testing.T) {
	migrated, err := storage.MigratePackedRefs(filepath.Join(t.TempDir(), "packed-refs"), "refs/zhi",
		func(string) bool { return false },
		func(string, string) error { return nil },
	)
	if err != nil {
		t.Fatalf("MigratePackedRefs on missing file: %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0", migrated)
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
