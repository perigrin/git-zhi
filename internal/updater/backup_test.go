// ABOUTME: Tests for the backup manager: creation, validation, listing, restore, and cleanup.
// ABOUTME: All tests use real on-disk files via t.TempDir() — no mocks.
package updater_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/updater"
)

// createTestBinary writes a minimal executable-sized file at path and returns the path.
func createTestBinary(t *testing.T, dir, name string) string {
	t.Helper()
	// Must be at least 1024 bytes to pass basicBackupValidation.
	content := make([]byte, 2048)
	for i := range content {
		content[i] = byte(i % 256)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0755); err != nil {
		t.Fatalf("createTestBinary: %v", err)
	}
	return path
}

func TestCreateBackup(t *testing.T) {
	dir := t.TempDir()
	binaryPath := createTestBinary(t, dir, "git-zhi")
	backupDir := filepath.Join(dir, "backups")

	bm := updater.NewBackupManagerWithDir(backupDir)
	backupPath, err := bm.CreateBackup(binaryPath)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Backup file must exist.
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file not found at %s: %v", backupPath, err)
	}

	// Metadata file must exist alongside the backup.
	metaPath := backupPath + ".meta"
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("metadata file not found at %s: %v", metaPath, err)
	}
}

func TestValidateBackup(t *testing.T) {
	dir := t.TempDir()
	binaryPath := createTestBinary(t, dir, "git-zhi")
	backupDir := filepath.Join(dir, "backups")

	bm := updater.NewBackupManagerWithDir(backupDir)
	backupPath, err := bm.CreateBackup(binaryPath)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	if err := bm.ValidateBackup(backupPath); err != nil {
		t.Errorf("ValidateBackup on fresh backup: %v", err)
	}
}

func TestValidateBackup_Missing(t *testing.T) {
	dir := t.TempDir()
	bm := updater.NewBackupManagerWithDir(dir)

	err := bm.ValidateBackup(filepath.Join(dir, "nonexistent.backup"))
	if err == nil {
		t.Error("expected error for missing backup, got nil")
	}
}

func TestListBackups(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	bm := updater.NewBackupManagerWithDir(backupDir)

	bin1 := createTestBinary(t, dir, "git-zhi-a")
	bin2 := createTestBinary(t, dir, "git-zhi-b")

	if _, err := bm.CreateBackup(bin1); err != nil {
		t.Fatalf("CreateBackup bin1: %v", err)
	}
	// Small sleep so timestamps differ and filenames don't collide.
	time.Sleep(time.Second)
	if _, err := bm.CreateBackup(bin2); err != nil {
		t.Fatalf("CreateBackup bin2: %v", err)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Errorf("expected 2 backups, got %d", len(backups))
	}
}

func TestRestoreFromBackup(t *testing.T) {
	dir := t.TempDir()
	binaryPath := createTestBinary(t, dir, "git-zhi")
	original, _ := os.ReadFile(binaryPath)

	backupDir := filepath.Join(dir, "backups")
	bm := updater.NewBackupManagerWithDir(backupDir)

	backupPath, err := bm.CreateBackup(binaryPath)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Overwrite the original to simulate a broken update.
	if err := os.WriteFile(binaryPath, []byte("broken"), 0755); err != nil {
		t.Fatalf("overwriting binary: %v", err)
	}

	if err := bm.RestoreFromBackup(backupPath, binaryPath); err != nil {
		t.Fatalf("RestoreFromBackup: %v", err)
	}

	restored, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(restored) != string(original) {
		t.Error("restored content does not match original")
	}
}

func TestCleanupOldBackups(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	bm := updater.NewBackupManagerWithDir(backupDir)

	bin := createTestBinary(t, dir, "git-zhi")
	backupPath, err := bm.CreateBackup(bin)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Back-date the backup file so it appears old.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(backupPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	// Also back-date the metadata file.
	if err := os.Chtimes(backupPath+".meta", oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes meta: %v", err)
	}

	// Adjust the CreatedAt in the metadata by removing the meta file and
	// relying on the mod-time fallback in ListBackups.
	if err := os.Remove(backupPath + ".meta"); err != nil {
		t.Fatalf("removing meta: %v", err)
	}

	if err := bm.CleanupOldBackups(24 * time.Hour); err != nil {
		t.Fatalf("CleanupOldBackups: %v", err)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after cleanup: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups after cleanup, got %d", len(backups))
	}
}

func TestListBackups_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	bm := updater.NewBackupManagerWithDir(filepath.Join(dir, "backups"))

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups on empty/missing dir: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups for empty dir, got %d", len(backups))
	}
}
