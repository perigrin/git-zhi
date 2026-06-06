// ABOUTME: Tests that --dry-run reports the planned update without side effects.
// ABOUTME: Verifies the dry-run result is built from version info, not a download.

package updater

import (
	"strings"
	"testing"
	"time"
)

func TestBuildDryRunResult(t *testing.T) {
	start := time.Now().Add(-time.Second)
	result := buildDryRunResult("0.3.1", "0.3.8", start)

	if !result.Success {
		t.Error("dry-run result should be marked successful")
	}
	if !result.DryRun {
		t.Error("dry-run result should have DryRun set")
	}
	if result.UpdatePerformed {
		t.Error("dry-run must not report an update as performed")
	}
	if result.PreviousVersion != "0.3.1" {
		t.Errorf("PreviousVersion = %q, want %q", result.PreviousVersion, "0.3.1")
	}
	if result.NewVersion != "0.3.8" {
		t.Errorf("NewVersion = %q, want %q", result.NewVersion, "0.3.8")
	}
	if !strings.Contains(result.Message, "0.3.1") || !strings.Contains(result.Message, "0.3.8") {
		t.Errorf("message should mention both versions, got %q", result.Message)
	}
	if result.BackupCreated {
		t.Error("dry-run must not create a backup")
	}
	if result.Duration <= 0 {
		t.Error("dry-run should record a non-zero duration")
	}
}
