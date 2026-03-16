// ABOUTME: Tests for auto-update checking and notification system for git-zhi.
// ABOUTME: Validates configuration persistence, check scheduling, channels, and update detection.

package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/version"
)

// fakeGitHubClient implements version.GitHubClientInterface for testing without network calls.
type fakeGitHubClient struct {
	releases []version.GitHubRelease
	err      error
}

func (f *fakeGitHubClient) GetLatestRelease(owner, repo string) (*version.GitHubRelease, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.releases) == 0 {
		return nil, nil
	}
	return &f.releases[0], nil
}

func (f *fakeGitHubClient) GetReleases(owner, repo string, includePrerelease bool) ([]version.GitHubRelease, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.releases, nil
}

func (f *fakeGitHubClient) GetReleaseByTag(owner, repo, tag string) (*version.GitHubRelease, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, r := range f.releases {
		if r.TagName == tag {
			return &r, nil
		}
	}
	return &version.GitHubRelease{
		TagName: tag,
		Name:    "Test Release " + tag,
		Body:    "Test release notes for " + tag,
	}, nil
}

// TestAutoUpdateConfigPersistence creates a manager, verifies defaults, modifies the channel,
// re-loads via a second manager, and verifies the change was persisted.
func TestAutoUpdateConfigPersistence(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "auto-update.json")

	manager, err := NewAutoUpdateManager(configPath)
	if err != nil {
		t.Fatalf("NewAutoUpdateManager: %v", err)
	}

	// Verify defaults.
	cfg := manager.GetConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled=true by default")
	}
	if cfg.Channel != ChannelStable {
		t.Errorf("expected Channel=stable by default, got %q", cfg.Channel)
	}
	if cfg.Repository != "perigrin/git-zhi" {
		t.Errorf("expected Repository=perigrin/git-zhi, got %q", cfg.Repository)
	}
	if cfg.CheckInterval != 24*time.Hour {
		t.Errorf("expected CheckInterval=24h, got %v", cfg.CheckInterval)
	}
	if cfg.AutoInstall {
		t.Error("expected AutoInstall=false by default")
	}

	// Verify config file was created on disk.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("expected config file to exist after NewAutoUpdateManager")
	}

	// Modify channel and persist.
	cfg.Channel = ChannelBeta
	if err := manager.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// Re-load via a new manager and verify the change survived.
	manager2, err := NewAutoUpdateManager(configPath)
	if err != nil {
		t.Fatalf("NewAutoUpdateManager (reload): %v", err)
	}
	if got := manager2.GetConfig().Channel; got != ChannelBeta {
		t.Errorf("expected Channel=beta after reload, got %q", got)
	}
}

// TestCheckIntervalRespected verifies that CheckForUpdates returns nil when LastCheckTime
// is recent enough that the check interval has not elapsed.
func TestCheckIntervalRespected(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "auto-update.json")

	manager, err := NewAutoUpdateManager(configPath)
	if err != nil {
		t.Fatalf("NewAutoUpdateManager: %v", err)
	}

	// Set LastCheckTime to now so the interval has not elapsed.
	cfg := manager.GetConfig()
	cfg.LastCheckTime = time.Now()
	if err := manager.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	notification, err := manager.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if notification != nil {
		t.Errorf("expected nil notification when check interval not elapsed, got %+v", notification)
	}
}

// TestCheckWhenDisabled verifies that CheckForUpdates returns nil when Enabled is false.
func TestCheckWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "auto-update.json")

	manager, err := NewAutoUpdateManager(configPath)
	if err != nil {
		t.Fatalf("NewAutoUpdateManager: %v", err)
	}

	cfg := manager.GetConfig()
	cfg.Enabled = false
	// Also set LastCheckTime far in the past to ensure the only reason for nil is Enabled=false.
	cfg.LastCheckTime = time.Now().Add(-48 * time.Hour)
	if err := manager.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	notification, err := manager.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if notification != nil {
		t.Errorf("expected nil notification when disabled, got %+v", notification)
	}
}

// TestUpdateChannelValid verifies that stable/beta/alpha are valid and "invalid" is not.
func TestUpdateChannelValid(t *testing.T) {
	validChannels := []UpdateChannel{ChannelStable, ChannelBeta, ChannelAlpha}
	for _, ch := range validChannels {
		if !ch.IsValid() {
			t.Errorf("expected channel %q to be valid", ch)
		}
	}

	invalid := UpdateChannel("invalid")
	if invalid.IsValid() {
		t.Error("expected channel \"invalid\" to be invalid")
	}
}

// TestSecurityUpdateDetection verifies that CVE-containing text is flagged as a security
// update and normal release text is not.
func TestSecurityUpdateDetection(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewAutoUpdateManager(filepath.Join(dir, "auto-update.json"))
	if err != nil {
		t.Fatalf("NewAutoUpdateManager: %v", err)
	}

	cases := []struct {
		notes    string
		expected bool
	}{
		{"Security patch for CVE-2024-1234", true},
		{"This release fixes a security vulnerability", true},
		{"Fixed exploit in parser", true},
		{"Regular bug fixes and improvements", false},
		{"Performance optimization", false},
		{"", false},
	}

	for _, tc := range cases {
		got := manager.isSecurityUpdate(tc.notes)
		if got != tc.expected {
			t.Errorf("isSecurityUpdate(%q) = %v, want %v", tc.notes, got, tc.expected)
		}
	}
}

// TestUrgentUpdateDetection verifies that "critical hotfix" text is flagged as urgent
// and normal release text is not.
func TestUrgentUpdateDetection(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewAutoUpdateManager(filepath.Join(dir, "auto-update.json"))
	if err != nil {
		t.Fatalf("NewAutoUpdateManager: %v", err)
	}

	cases := []struct {
		notes    string
		expected bool
	}{
		{"Critical hotfix for data loss", true},
		{"Urgent patch for production issue", true},
		{"Emergency release", true},
		{"Regular bug fixes", false},
		{"New feature release", false},
		{"", false},
	}

	for _, tc := range cases {
		got := manager.isUrgentUpdate(tc.notes)
		if got != tc.expected {
			t.Errorf("isUrgentUpdate(%q) = %v, want %v", tc.notes, got, tc.expected)
		}
	}
}
