// ABOUTME: Tests for binary replacer functionality.
// ABOUTME: Covers atomic replacement, dry-run, backup, validation, and installation method detection.

package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// createTestBinary creates a fake binary file of the given size in dir with the given name.
// The file is made executable.
func createTestBinary(t *testing.T, dir, name string, sizeBytes int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := make([]byte, sizeBytes)
	for i := range content {
		content[i] = 0xFF
	}
	if err := os.WriteFile(path, content, 0755); err != nil {
		t.Fatalf("createTestBinary: %v", err)
	}
	return path
}

// minBinarySize is the minimum acceptable binary size (1 MB).
const minBinarySize = 1024 * 1024

func TestReplaceBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("atomic replacement test not supported on Windows")
	}

	dir := t.TempDir()
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	newPath := createTestBinary(t, dir, "git-zhi-new", minBinarySize*2)

	replacer := NewBinaryReplacer()
	result, err := replacer.ReplaceBinary(&ReplacementOptions{
		CurrentPath: currentPath,
		NewPath:     newPath,
	})
	if err != nil {
		t.Fatalf("ReplaceBinary failed: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}

	// Verify the replacement happened by checking file size.
	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat current path after replacement: %v", err)
	}
	if info.Size() != int64(minBinarySize*2) {
		t.Errorf("expected size %d after replacement, got %d", minBinarySize*2, info.Size())
	}
}

func TestDryRun(t *testing.T) {
	dir := t.TempDir()
	originalSize := minBinarySize
	currentPath := createTestBinary(t, dir, "git-zhi", originalSize)
	newPath := createTestBinary(t, dir, "git-zhi-new", minBinarySize*2)

	replacer := NewBinaryReplacer()
	result, err := replacer.ReplaceBinary(&ReplacementOptions{
		CurrentPath: currentPath,
		NewPath:     newPath,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("ReplaceBinary (dry-run) failed: %v", err)
	}
	if !result.Success {
		t.Error("expected Success to be true for dry-run")
	}
	if !result.SimulatedOnly {
		t.Error("expected SimulatedOnly to be true for dry-run")
	}

	// Original file must be unchanged.
	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat current path after dry-run: %v", err)
	}
	if info.Size() != int64(originalSize) {
		t.Errorf("dry-run must not change current binary; expected size %d, got %d", originalSize, info.Size())
	}
}

func TestWithBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backup test not supported on Windows")
	}

	dir := t.TempDir()
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	newPath := createTestBinary(t, dir, "git-zhi-new", minBinarySize*2)

	bm := NewBackupManagerWithDir(dir)
	replacer := &BinaryReplacer{backupManager: bm}

	result, err := replacer.ReplaceBinary(&ReplacementOptions{
		CurrentPath:   currentPath,
		NewPath:       newPath,
		BackupEnabled: true,
	})
	if err != nil {
		t.Fatalf("ReplaceBinary with backup failed: %v", err)
	}
	if result.BackupPath == "" {
		t.Error("expected BackupPath to be set when BackupEnabled is true")
	}
	// Backup file must exist.
	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Errorf("backup file does not exist at %s", result.BackupPath)
	}
}

func TestValidateNewBinaryTooSmall(t *testing.T) {
	dir := t.TempDir()
	currentPath := createTestBinary(t, dir, "git-zhi", minBinarySize)
	// Create a binary that is far too small.
	tinyPath := createTestBinary(t, dir, "git-zhi-tiny", 512)

	replacer := NewBinaryReplacer()
	_, err := replacer.ReplaceBinary(&ReplacementOptions{
		CurrentPath:    currentPath,
		NewPath:        tinyPath,
		ValidateBinary: true,
	})
	if err == nil {
		t.Fatal("expected error for too-small binary, got nil")
	}
	if !strings.Contains(err.Error(), "too small") {
		t.Errorf("expected 'too small' in error, got: %v", err)
	}
}

func TestMissingCurrent(t *testing.T) {
	dir := t.TempDir()
	newPath := createTestBinary(t, dir, "git-zhi-new", minBinarySize)

	replacer := NewBinaryReplacer()
	_, err := replacer.ReplaceBinary(&ReplacementOptions{
		CurrentPath: filepath.Join(dir, "nonexistent"),
		NewPath:     newPath,
	})
	if err == nil {
		t.Fatal("expected error for missing current binary, got nil")
	}
}

func TestDetectInstallationMethodBinary(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "git-zhi")

	method, err := DetectInstallationMethod(binaryPath)
	if err != nil {
		t.Fatalf("DetectInstallationMethod failed: %v", err)
	}
	if method != InstallationBinary {
		t.Errorf("expected InstallationBinary for temp dir path, got %s", method)
	}
}

func TestCanSelfUpdate(t *testing.T) {
	tests := []struct {
		method   InstallationMethod
		expected bool
	}{
		{InstallationBinary, true},
		{InstallationHomebrew, false},
		{InstallationAPT, false},
		{InstallationYum, false},
		{InstallationDNF, false},
		{InstallationPacman, false},
		{InstallationSnap, false},
		{InstallationFlatpak, false},
		{InstallationChocolatey, false},
		{InstallationScoop, false},
		{InstallationWinget, false},
	}

	for _, tc := range tests {
		got := tc.method.CanSelfUpdate()
		if got != tc.expected {
			t.Errorf("%s.CanSelfUpdate() = %t, want %t", tc.method, got, tc.expected)
		}
	}
}

func TestGetUpdateInstructions(t *testing.T) {
	tests := []struct {
		method   InstallationMethod
		contains string
	}{
		{InstallationBinary, "git zhi update"},
		{InstallationHomebrew, "brew upgrade git-zhi"},
		{InstallationAPT, "apt"},
		{InstallationYum, "yum"},
		{InstallationDNF, "dnf"},
		{InstallationPacman, "pacman"},
		{InstallationSnap, "snap"},
		{InstallationFlatpak, "flatpak"},
		{InstallationChocolatey, "choco"},
		{InstallationScoop, "scoop"},
		{InstallationWinget, "winget"},
	}

	for _, tc := range tests {
		instructions := tc.method.GetUpdateInstructions()
		if instructions == "" {
			t.Errorf("%s.GetUpdateInstructions() returned empty string", tc.method)
			continue
		}
		if !strings.Contains(instructions, tc.contains) {
			t.Errorf("%s.GetUpdateInstructions() = %q, want it to contain %q",
				tc.method, instructions, tc.contains)
		}
	}
}
