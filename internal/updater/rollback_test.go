// ABOUTME: Tests for rollback manager: PerformRollback, AutoRollback, error cases, status, and post-rollback validation.
// ABOUTME: All tests use real on-disk files via t.TempDir() — no mocks.

package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRollbackManager creates a RollbackManager backed by a temp backup directory.
func newTestRollbackManager(t *testing.T, backupDir string) *RollbackManager {
	t.Helper()
	return NewRollbackManagerWithDir(backupDir)
}

func TestPerformRollback(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	rm := newTestRollbackManager(t, backupDir)

	// Create a "current" binary and a backup.
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	backupPath := createTestBinary(t, dir, "git-zhi-backup", minBinarySize*2)

	opts := &RollbackOptions{
		TargetPath: currentPath,
		BackupPath: backupPath,
	}

	result, err := rm.PerformRollback(opts)
	if err != nil {
		t.Fatalf("PerformRollback: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.BackupPath != backupPath {
		t.Errorf("expected BackupPath %q, got %q", backupPath, result.BackupPath)
	}

	// The current binary should now have the size of the backup.
	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat after rollback: %v", err)
	}
	if info.Size() != int64(minBinarySize*2) {
		t.Errorf("expected size %d after rollback, got %d", minBinarySize*2, info.Size())
	}
}

func TestPerformRollback_DryRun(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	rm := newTestRollbackManager(t, backupDir)

	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	backupPath := createTestBinary(t, dir, "git-zhi-backup", minBinarySize*2)
	originalSize := minBinarySize

	opts := &RollbackOptions{
		TargetPath: currentPath,
		BackupPath: backupPath,
		DryRun:     true,
	}

	result, err := rm.PerformRollback(opts)
	if err != nil {
		t.Fatalf("PerformRollback dry-run: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true for dry-run")
	}
	if !result.SimulatedOnly {
		t.Error("expected SimulatedOnly to be true for dry-run")
	}

	// Current binary must be unchanged.
	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat after dry-run rollback: %v", err)
	}
	if info.Size() != int64(originalSize) {
		t.Errorf("dry-run must not change current binary; expected size %d, got %d",
			originalSize, info.Size())
	}
}

func TestPerformRollback_WithValidation(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	bm := NewBackupManagerWithDir(backupDir)

	// Create a binary and back it up via BackupManager so metadata exists.
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	storedBackupPath, err := bm.CreateBackup(currentPath)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	rm := NewRollbackManagerWithDir(backupDir)

	opts := &RollbackOptions{
		TargetPath:     currentPath,
		BackupPath:     storedBackupPath,
		ValidateBackup: true,
	}

	result, err := rm.PerformRollback(opts)
	if err != nil {
		t.Fatalf("PerformRollback with validation: %v", err)
	}
	if !result.ValidationPassed {
		t.Error("expected ValidationPassed to be true")
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
}

func TestPerformRollback_NilOptions(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)

	_, err := rm.PerformRollback(nil)
	if err == nil {
		t.Fatal("expected error for nil options, got nil")
	}
}

func TestPerformRollback_EmptyTargetPath(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)
	backupPath := createTestBinary(t, dir, "git-zhi-backup", minBinarySize)

	_, err := rm.PerformRollback(&RollbackOptions{
		BackupPath: backupPath,
	})
	if err == nil {
		t.Fatal("expected error for empty target path, got nil")
	}
}

func TestAutoRollback(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	bm := NewBackupManagerWithDir(backupDir)

	// Create and back up a binary so there is something to roll back to.
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	_, err := bm.CreateBackup(currentPath)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	rm := NewRollbackManagerWithDir(backupDir)
	if err := rm.AutoRollback(currentPath); err != nil {
		t.Fatalf("AutoRollback: %v", err)
	}
}

func TestAutoRollback_NoBackups(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	rm := NewRollbackManagerWithDir(backupDir)
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)

	err := rm.AutoRollback(currentPath)
	if err == nil {
		t.Fatal("expected error when no backups are available, got nil")
	}
	if !strings.Contains(err.Error(), "no backups") {
		t.Errorf("expected 'no backups' in error, got: %v", err)
	}
}

func TestGetRollbackStatus_WithBackups(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	bm := NewBackupManagerWithDir(backupDir)

	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	if _, err := bm.CreateBackup(currentPath); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	rm := NewRollbackManagerWithDir(backupDir)
	status, err := rm.GetRollbackStatus(currentPath)
	if err != nil {
		t.Fatalf("GetRollbackStatus: %v", err)
	}

	if !status.TargetExists {
		t.Error("expected TargetExists to be true")
	}
	if !status.RollbackPossible {
		t.Error("expected RollbackPossible to be true")
	}
	if status.AvailableBackups != 1 {
		t.Errorf("expected 1 available backup, got %d", status.AvailableBackups)
	}
	if status.LatestBackupPath == "" {
		t.Error("expected LatestBackupPath to be set")
	}
}

func TestGetRollbackStatus_NoBackups(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)

	rm := NewRollbackManagerWithDir(backupDir)
	status, err := rm.GetRollbackStatus(currentPath)
	if err != nil {
		t.Fatalf("GetRollbackStatus: %v", err)
	}

	if status.RollbackPossible {
		t.Error("expected RollbackPossible to be false when no backups exist")
	}
	if status.AvailableBackups != 0 {
		t.Errorf("expected 0 available backups, got %d", status.AvailableBackups)
	}
}

func TestGetRollbackStatus_MissingTarget(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	rm := NewRollbackManagerWithDir(backupDir)

	status, err := rm.GetRollbackStatus(filepath.Join(dir, "nonexistent"))
	if err != nil {
		t.Fatalf("GetRollbackStatus: %v", err)
	}

	if status.TargetExists {
		t.Error("expected TargetExists to be false for missing binary")
	}
	if !status.RollbackNeeded {
		t.Error("expected RollbackNeeded to be true when target is missing")
	}
}

func TestValidateRollbackTarget(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)

	binaryPath := createTestBinary(t, dir, "git-zhi", minBinarySize)

	if err := rm.ValidateRollbackTarget(binaryPath); err != nil {
		t.Errorf("ValidateRollbackTarget on writable dir: %v", err)
	}
}

func TestValidateRollbackTarget_NonexistentDir(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)

	err := rm.ValidateRollbackTarget(filepath.Join(dir, "missing", "git-zhi"))
	if err == nil {
		t.Fatal("expected error for nonexistent target directory, got nil")
	}
}

func TestValidatePostRollback(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)

	binaryPath := createTestBinary(t, dir, "git-zhi", minBinarySize)

	if err := rm.ValidatePostRollback(binaryPath); err != nil {
		t.Errorf("ValidatePostRollback on valid executable: %v", err)
	}
}

func TestValidatePostRollback_MissingBinary(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)

	err := rm.ValidatePostRollback(filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestValidatePostRollback_NotExecutable(t *testing.T) {
	dir := t.TempDir()
	rm := newTestRollbackManager(t, dir)

	path := filepath.Join(dir, "git-zhi-noexec")
	if err := os.WriteFile(path, make([]byte, minBinarySize), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := rm.ValidatePostRollback(path)
	if err == nil {
		t.Fatal("expected error for non-executable binary, got nil")
	}
}

func TestFindAvailableBackups(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	bm := NewBackupManagerWithDir(backupDir)

	bin := createTestBinary(t, dir, "git-zhi", minBinarySize)
	if _, err := bm.CreateBackup(bin); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	rm := NewRollbackManagerWithDir(backupDir)
	backups, err := rm.FindAvailableBackups()
	if err != nil {
		t.Fatalf("FindAvailableBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}
}
