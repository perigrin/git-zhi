// ABOUTME: Tests for CheckForUpdates and GetUpdateInfo using a mock GitHub client.
// ABOUTME: Verifies update detection, current-version detection, and dependency injection.

package version

import (
	"fmt"
	"testing"
	"time"
)

// mockGitHubClient implements GitHubClientInterface for testing without network access.
type mockGitHubClient struct {
	latestRelease *GitHubRelease
	latestErr     error
	releases      []GitHubRelease
	releasesErr   error
	byTagRelease  *GitHubRelease
	byTagErr      error
}

func (m *mockGitHubClient) GetLatestRelease(owner, repo string) (*GitHubRelease, error) {
	return m.latestRelease, m.latestErr
}

func (m *mockGitHubClient) GetReleases(owner, repo string, includePrerelease bool) ([]GitHubRelease, error) {
	return m.releases, m.releasesErr
}

func (m *mockGitHubClient) GetReleaseByTag(owner, repo, tag string) (*GitHubRelease, error) {
	return m.byTagRelease, m.byTagErr
}

func TestCheckForUpdates_UpdateAvailable(t *testing.T) {
	pub := time.Now()
	mock := &mockGitHubClient{
		latestRelease: &GitHubRelease{
			TagName:     "v0.2.0",
			HTMLURL:     "https://github.com/perigrin/git-zhi/releases/tag/v0.2.0",
			Body:        "release notes",
			PublishedAt: &pub,
		},
	}

	// Temporarily override the package-level Version variable.
	orig := Version
	Version = "0.1.0"
	defer func() { Version = orig }()

	opts := &CheckOptions{
		Repository: "perigrin/git-zhi",
		Client:     mock,
	}

	result, err := CheckForUpdates(opts)
	if err != nil {
		t.Fatalf("CheckForUpdates returned unexpected error: %v", err)
	}

	if !result.UpdateAvailable {
		t.Errorf("expected UpdateAvailable true, got false")
	}
	if result.CurrentVersion != "0.1.0" {
		t.Errorf("expected CurrentVersion %q, got %q", "0.1.0", result.CurrentVersion)
	}
	if result.LatestVersion != "v0.2.0" {
		t.Errorf("expected LatestVersion %q, got %q", "v0.2.0", result.LatestVersion)
	}
	if result.ReleaseURL != "https://github.com/perigrin/git-zhi/releases/tag/v0.2.0" {
		t.Errorf("unexpected ReleaseURL: %q", result.ReleaseURL)
	}
}

func TestCheckForUpdates_AlreadyCurrent(t *testing.T) {
	pub := time.Now()
	mock := &mockGitHubClient{
		latestRelease: &GitHubRelease{
			TagName:     "v0.2.0",
			PublishedAt: &pub,
		},
	}

	orig := Version
	Version = "0.2.0"
	defer func() { Version = orig }()

	opts := &CheckOptions{
		Repository: "perigrin/git-zhi",
		Client:     mock,
	}

	result, err := CheckForUpdates(opts)
	if err != nil {
		t.Fatalf("CheckForUpdates returned unexpected error: %v", err)
	}

	if result.UpdateAvailable {
		t.Errorf("expected UpdateAvailable false, got true")
	}
}

func TestCheckForUpdates_InvalidRepository(t *testing.T) {
	mock := &mockGitHubClient{}

	opts := &CheckOptions{
		Repository: "not-valid",
		Client:     mock,
	}

	_, err := CheckForUpdates(opts)
	if err == nil {
		t.Fatal("expected error for invalid repository format, got nil")
	}
}

func TestCheckForUpdates_ClientError(t *testing.T) {
	mock := &mockGitHubClient{
		latestErr: fmt.Errorf("network timeout"),
	}

	opts := &CheckOptions{
		Repository: "perigrin/git-zhi",
		Client:     mock,
	}

	_, err := CheckForUpdates(opts)
	if err == nil {
		t.Fatal("expected error when GitHub client returns error, got nil")
	}
}

func TestCheckForUpdates_NilOpts(t *testing.T) {
	// When opts is nil, CheckForUpdates must not panic; it should use defaults
	// and attempt a real network call (which we allow to fail in test environments).
	// We verify that nil opts do not cause a nil pointer dereference.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CheckForUpdates panicked with nil opts: %v", r)
		}
	}()
	// We don't assert success here — just that it doesn't panic.
	CheckForUpdates(nil) //nolint:errcheck
}

func TestGetUpdateInfo_UpdateAvailable(t *testing.T) {
	pub := time.Now()
	release := &GitHubRelease{
		TagName:     "v0.2.0",
		HTMLURL:     "https://github.com/perigrin/git-zhi/releases/tag/v0.2.0",
		PublishedAt: &pub,
	}
	mock := &mockGitHubClient{
		latestRelease: release,
		byTagRelease:  release,
	}

	orig := Version
	Version = "0.1.0"
	defer func() { Version = orig }()

	opts := &CheckOptions{
		Repository: "perigrin/git-zhi",
		Client:     mock,
	}

	info, err := GetUpdateInfo(opts)
	if err != nil {
		t.Fatalf("GetUpdateInfo returned unexpected error: %v", err)
	}

	if !info.UpdateNeeded {
		t.Errorf("expected UpdateNeeded true, got false")
	}
	if info.CurrentVersion == nil {
		t.Fatal("expected CurrentVersion to be set, got nil")
	}
	if info.CurrentVersion.String() != "0.1.0" {
		t.Errorf("expected CurrentVersion 0.1.0, got %s", info.CurrentVersion.String())
	}
	if info.LatestVersion == nil {
		t.Fatal("expected LatestVersion to be set, got nil")
	}
	if info.LatestVersion.String() != "0.2.0" {
		t.Errorf("expected LatestVersion 0.2.0, got %s", info.LatestVersion.String())
	}
}
