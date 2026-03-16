// ABOUTME: Tests for the main updater orchestrator.
// ABOUTME: Covers default options, stage names, and constructor behavior.

package updater

import (
	"testing"
)

func TestDefaultUpdateOptions(t *testing.T) {
	opts := DefaultUpdateOptions()

	if opts.Repository != "perigrin/git-zhi" {
		t.Errorf("expected Repository %q, got %q", "perigrin/git-zhi", opts.Repository)
	}

	if !opts.Backup {
		t.Error("expected Backup to default to true")
	}

	if !opts.AutoRollback {
		t.Error("expected AutoRollback to default to true")
	}

	if opts.Context == nil {
		t.Error("expected Context to be non-nil")
	}
}

func TestUpdateStageString(t *testing.T) {
	tests := []struct {
		stage    UpdateStage
		expected string
	}{
		{StageCheckingVersion, "Checking for updates"},
		{StageDetectingPlatform, "Detecting platform"},
		{StageDownloading, "Downloading update"},
		{StageValidating, "Validating download"},
		{StageCreatingBackup, "Creating backup"},
		{StageReplacing, "Installing update"},
		{StageValidatingUpdate, "Validating installation"},
		{StageCleaningUp, "Cleaning up"},
		{StageRollingBack, "Rolling back"},
		{StageDone, "Update complete"},
		{UpdateStage(999), "Unknown stage"},
	}

	for _, tc := range tests {
		got := tc.stage.String()
		if got != tc.expected {
			t.Errorf("stage %d String() = %q, want %q", tc.stage, got, tc.expected)
		}
	}
}

func TestNewUpdater(t *testing.T) {
	u := NewUpdater()

	if u.versionChecker == nil {
		t.Error("expected versionChecker to be non-nil")
	}
	if u.downloader == nil {
		t.Error("expected downloader to be non-nil")
	}
	if u.replacer == nil {
		t.Error("expected replacer to be non-nil")
	}
	if u.rollbackMgr == nil {
		t.Error("expected rollbackMgr to be non-nil")
	}
	if u.recoveryMgr == nil {
		t.Error("expected recoveryMgr to be non-nil")
	}
}

func TestNewUpdaterWithToken(t *testing.T) {
	u := NewUpdaterWithToken("test-token")

	if u.versionChecker == nil {
		t.Error("expected versionChecker to be non-nil")
	}
	if u.downloader == nil {
		t.Error("expected downloader to be non-nil")
	}
	if u.replacer == nil {
		t.Error("expected replacer to be non-nil")
	}
	if u.rollbackMgr == nil {
		t.Error("expected rollbackMgr to be non-nil")
	}
	if u.recoveryMgr == nil {
		t.Error("expected recoveryMgr to be non-nil")
	}
}
