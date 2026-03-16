// ABOUTME: Auto-update checking and notification system for git-zhi.
// ABOUTME: Handles background update checks, notifications, and update channel management.

package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/perigrin/git-zhi/internal/version"
)

// AutoUpdateManager handles automatic update checking and notifications.
type AutoUpdateManager struct {
	versionChecker *version.GitHubClient
	configPath     string
	config         *AutoUpdateConfig
}

// AutoUpdateConfig stores auto-update configuration.
type AutoUpdateConfig struct {
	Enabled           bool                `json:"enabled"`
	CheckInterval     time.Duration       `json:"check_interval"`
	Channel           UpdateChannel       `json:"channel"`
	LastCheckTime     time.Time           `json:"last_check_time"`
	LastNotification  time.Time           `json:"last_notification"`
	NotificationDelay time.Duration       `json:"notification_delay"`
	Repository        string              `json:"repository"`
	GitHubToken       string              `json:"github_token,omitempty"`
	QuietMode         bool                `json:"quiet_mode"`
	AutoInstall       bool                `json:"auto_install"`
	InstallationTime  AutoInstallSchedule `json:"installation_time"`
	// testClient is used for dependency injection in tests (not serialized).
	testClient version.GitHubClientInterface `json:"-"`
}

// UpdateChannel represents different update channels.
type UpdateChannel string

const (
	ChannelStable UpdateChannel = "stable"
	ChannelBeta   UpdateChannel = "beta"
	ChannelAlpha  UpdateChannel = "alpha"
)

// AutoInstallSchedule defines when auto-installation should occur.
type AutoInstallSchedule struct {
	Enabled    bool  `json:"enabled"`
	DaysOfWeek []int `json:"days_of_week"` // 0=Sunday, 1=Monday, etc.
	Hour       int   `json:"hour"`         // 0-23
	Minute     int   `json:"minute"`       // 0-59
}

// UpdateNotification represents an update notification.
type UpdateNotification struct {
	Available      bool          `json:"available"`
	CurrentVersion string        `json:"current_version"`
	LatestVersion  string        `json:"latest_version"`
	Channel        UpdateChannel `json:"channel"`
	ReleaseNotes   string        `json:"release_notes"`
	Urgent         bool          `json:"urgent"`
	SecurityUpdate bool          `json:"security_update"`
	DownloadSize   int64         `json:"download_size"`
	Timestamp      time.Time     `json:"timestamp"`
}

// DefaultAutoUpdateConfig returns default auto-update configuration.
func DefaultAutoUpdateConfig() *AutoUpdateConfig {
	return &AutoUpdateConfig{
		Enabled:           true,
		CheckInterval:     24 * time.Hour, // Check daily
		Channel:           ChannelStable,
		NotificationDelay: 4 * time.Hour, // Wait 4 hours between notifications
		Repository:        "perigrin/git-zhi",
		QuietMode:         false,
		AutoInstall:       false,
		InstallationTime: AutoInstallSchedule{
			Enabled:    false,
			DaysOfWeek: []int{0}, // Sunday
			Hour:       2,        // 2 AM
			Minute:     0,
		},
	}
}

// DefaultAutoUpdateConfigPath returns the platform-appropriate path for the config file.
func DefaultAutoUpdateConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "git-zhi", "auto-update.json")
	}
	return filepath.Join(configDir, "git-zhi", "auto-update.json")
}

// NewAutoUpdateManager creates a new auto-update manager, loading or creating config at configPath.
func NewAutoUpdateManager(configPath string) (*AutoUpdateManager, error) {
	manager := &AutoUpdateManager{
		versionChecker: version.NewGitHubClient(),
		configPath:     configPath,
	}

	// Load or create config.
	config, err := manager.loadConfig()
	if err != nil {
		// Config doesn't exist yet — write defaults.
		config = DefaultAutoUpdateConfig()
		if err := manager.saveConfig(config); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}

	manager.config = config
	return manager, nil
}

// NewAutoUpdateManagerWithToken creates a new auto-update manager with a GitHub token.
func NewAutoUpdateManagerWithToken(configPath, token string) (*AutoUpdateManager, error) {
	manager, err := NewAutoUpdateManager(configPath)
	if err != nil {
		return nil, err
	}

	manager.versionChecker = version.NewGitHubClientWithToken(token)
	manager.config.GitHubToken = token
	return manager, nil
}

// CheckForUpdates performs an update check and returns a notification if an update is available.
// Returns nil, nil when updates are disabled, when the check interval has not elapsed, or when
// notifications are being rate-limited.
func (m *AutoUpdateManager) CheckForUpdates(ctx context.Context) (*UpdateNotification, error) {
	if !m.config.Enabled {
		return nil, nil
	}

	// Return early if the check interval has not elapsed since the last check.
	if time.Since(m.config.LastCheckTime) < m.config.CheckInterval {
		return nil, nil
	}

	// Build check options, injecting the test client when present.
	checkOpts := &version.CheckOptions{
		IncludePrerelease: m.shouldIncludePrerelease(),
		Repository:        m.config.Repository,
		GitHubToken:       m.config.GitHubToken,
		Timeout:           30 * time.Second,
		Client:            m.config.testClient,
	}

	updateInfo, err := version.GetUpdateInfo(checkOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	// Record the check time regardless of whether an update was found.
	m.config.LastCheckTime = time.Now()
	if err := m.saveConfig(m.config); err != nil {
		// Non-fatal: log the error by swallowing it; the check still succeeded.
		_ = err
	}

	if !updateInfo.UpdateNeeded {
		return &UpdateNotification{
			Available:      false,
			CurrentVersion: updateInfo.CurrentVersion.String(),
			Channel:        m.config.Channel,
			Timestamp:      time.Now(),
		}, nil
	}

	// Rate-limit notifications unless quiet mode bypasses the delay.
	if !m.config.QuietMode && time.Since(m.config.LastNotification) < m.config.NotificationDelay {
		return nil, nil
	}

	notification := &UpdateNotification{
		Available:      true,
		CurrentVersion: updateInfo.CurrentVersion.String(),
		LatestVersion:  updateInfo.LatestVersion.String(),
		Channel:        m.config.Channel,
		Timestamp:      time.Now(),
	}

	if updateInfo.Release != nil {
		notification.ReleaseNotes = updateInfo.Release.Body
		notification.SecurityUpdate = m.isSecurityUpdate(updateInfo.Release.Body)
		notification.Urgent = m.isUrgentUpdate(updateInfo.Release.Body)

		if asset, err := version.GetUpdateAsset(updateInfo.Release, version.DetectPlatform()); err == nil {
			notification.DownloadSize = asset.Size
		}
	}

	// Update the last-notification timestamp so notifications are rate-limited.
	if !m.config.QuietMode {
		m.config.LastNotification = time.Now()
		if err := m.saveConfig(m.config); err != nil {
			_ = err
		}
	}

	return notification, nil
}

// ShouldAutoInstall reports whether the current time falls within the configured auto-install window.
func (m *AutoUpdateManager) ShouldAutoInstall() bool {
	if !m.config.AutoInstall || !m.config.InstallationTime.Enabled {
		return false
	}

	now := time.Now()
	schedule := m.config.InstallationTime

	currentDay := int(now.Weekday())
	dayMatches := false
	for _, day := range schedule.DaysOfWeek {
		if day == currentDay {
			dayMatches = true
			break
		}
	}

	if !dayMatches {
		return false
	}

	return now.Hour() == schedule.Hour
}

// GetConfig returns the current configuration.
func (m *AutoUpdateManager) GetConfig() *AutoUpdateConfig {
	return m.config
}

// UpdateConfig replaces the current configuration and persists it to disk.
func (m *AutoUpdateManager) UpdateConfig(config *AutoUpdateConfig) error {
	m.config = config
	return m.saveConfig(config)
}

// SetTestClient injects an alternate GitHub client for use in tests.
func (m *AutoUpdateManager) SetTestClient(client version.GitHubClientInterface) {
	m.config.testClient = client
}

// shouldIncludePrerelease reports whether the configured channel should receive pre-release builds.
func (m *AutoUpdateManager) shouldIncludePrerelease() bool {
	switch m.config.Channel {
	case ChannelStable:
		return false
	case ChannelBeta, ChannelAlpha:
		return true
	default:
		return false
	}
}

// isSecurityUpdate reports whether the release notes mention a security issue.
func (m *AutoUpdateManager) isSecurityUpdate(releaseNotes string) bool {
	keywords := []string{"security", "vulnerability", "CVE-", "exploit", "patch"}
	lower := strings.ToLower(releaseNotes)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// isUrgentUpdate reports whether the release notes indicate an urgent or critical update.
func (m *AutoUpdateManager) isUrgentUpdate(releaseNotes string) bool {
	keywords := []string{"urgent", "critical", "emergency", "hotfix", "immediate"}
	lower := strings.ToLower(releaseNotes)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// loadConfig reads and parses the configuration file from disk.
func (m *AutoUpdateManager) loadConfig() (*AutoUpdateConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, err
	}

	var config AutoUpdateConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// saveConfig writes the configuration to disk, creating the parent directory as needed.
func (m *AutoUpdateManager) saveConfig(config *AutoUpdateConfig) error {
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// String returns the string representation of the update channel.
func (c UpdateChannel) String() string {
	return string(c)
}

// IsValid reports whether the channel is a recognized update channel.
func (c UpdateChannel) IsValid() bool {
	switch c {
	case ChannelStable, ChannelBeta, ChannelAlpha:
		return true
	default:
		return false
	}
}
