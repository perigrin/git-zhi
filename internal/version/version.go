// ABOUTME: Version information for git-zhi, set at build time via ldflags.
// ABOUTME: Provides GetVersion, GetBuildInfo, and the version command output.
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// Version is the current version of git-zhi.
// Overridden at build time with ldflags.
var Version = "0.1.0-dev"

// BuildTime is set at build time via ldflags.
var BuildTime = "unknown"

// CommitHash is set at build time via ldflags.
var CommitHash = "unknown"

// GetVersion returns the current version string.
func GetVersion() string {
	return Version
}

// GetBuildInfo returns a formatted string with full build information.
func GetBuildInfo() string {
	return fmt.Sprintf("git-zhi %s (%s/%s)\nbuilt %s\ncommit %s",
		Version, runtime.GOOS, runtime.GOARCH, BuildTime, CommitHash)
}

// GetCurrentVersion returns the current version as a SemanticVersion.
func GetCurrentVersion() (*SemanticVersion, error) {
	return ParseVersion(Version)
}

// CheckForUpdates checks for available updates using the GitHub API.
func CheckForUpdates(opts *CheckOptions) (*VersionCheckResult, error) {
	if opts == nil {
		opts = DefaultCheckOptions()
	}

	result := &VersionCheckResult{
		CurrentVersion: Version,
	}

	// Parse current version.
	currentVer, err := ParseVersion(Version)
	if err != nil {
		result.Error = fmt.Sprintf("invalid current version: %v", err)
		return result, err
	}

	// Parse repository into owner and repo components.
	parts := strings.Split(opts.Repository, "/")
	if len(parts) != 2 {
		result.Error = "invalid repository format, expected owner/repo"
		return result, fmt.Errorf("invalid repository format: %s", opts.Repository)
	}
	owner, repo := parts[0], parts[1]

	// Select GitHub client: injected > token-based > default.
	var client GitHubClientInterface
	switch {
	case opts.Client != nil:
		client = opts.Client
	case opts.GitHubToken != "":
		client = NewGitHubClientWithToken(opts.GitHubToken)
	default:
		client = NewGitHubClient()
	}

	// Fetch the latest release via the appropriate API call.
	var release *GitHubRelease
	if opts.IncludePrerelease {
		releases, err := client.GetReleases(owner, repo, true)
		if err != nil {
			result.Error = fmt.Sprintf("failed to fetch releases: %v", err)
			return result, err
		}
		// Find the newest non-draft release (may be a prerelease).
		for i := range releases {
			r := &releases[i]
			if r.Draft {
				continue
			}
			if release == nil || r.CreatedAt.After(release.CreatedAt) {
				release = r
			}
		}
	} else {
		var err error
		release, err = client.GetLatestRelease(owner, repo)
		if err != nil {
			result.Error = fmt.Sprintf("failed to fetch latest release: %v", err)
			return result, err
		}
	}

	if release == nil {
		result.Error = "no releases found"
		return result, fmt.Errorf("no releases found")
	}

	// Parse the latest version tag.
	latestVer, err := ParseVersion(release.TagName)
	if err != nil {
		result.Error = fmt.Sprintf("invalid latest version: %v", err)
		return result, err
	}

	result.LatestVersion = release.TagName
	result.UpdateAvailable = latestVer.IsNewer(currentVer)
	result.IsPrerelease = latestVer.IsPrerelease()
	result.ReleaseURL = release.HTMLURL
	result.ReleaseNotes = release.Body
	result.PublishedAt = release.PublishedAt
	result.Release = release

	return result, nil
}

// GetUpdateInfo returns structured update information including full release details.
func GetUpdateInfo(opts *CheckOptions) (*UpdateInfo, error) {
	if opts == nil {
		opts = DefaultCheckOptions()
	}

	// Resolve current version.
	currentVer, err := GetCurrentVersion()
	if err != nil {
		return nil, fmt.Errorf("getting current version: %w", err)
	}

	// Check GitHub for a newer release.
	// CheckForUpdates populates result.Release, avoiding an extra API call.
	result, err := CheckForUpdates(opts)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}

	// Parse the latest version string returned by the check.
	latestVer, err := ParseVersion(result.LatestVersion)
	if err != nil {
		return nil, fmt.Errorf("parsing latest version: %w", err)
	}

	return &UpdateInfo{
		CurrentVersion: currentVer,
		LatestVersion:  latestVer,
		Release:        result.Release,
		UpdateNeeded:   result.UpdateAvailable,
		IsPrerelease:   result.IsPrerelease,
	}, nil
}
