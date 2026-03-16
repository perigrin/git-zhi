// ABOUTME: Type definitions for version checking and update detection.
// ABOUTME: Shared data structures for GitHub releases, update info, and check options.

package version

import "time"

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	ID          int64         `json:"id"`
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	CreatedAt   time.Time     `json:"created_at"`
	PublishedAt *time.Time    `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
	TarballURL  string        `json:"tarball_url"`
	ZipballURL  string        `json:"zipball_url"`
	HTMLURL     string        `json:"html_url"`
}

// GitHubAsset represents a release asset
type GitHubAsset struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	ContentType        string    `json:"content_type"`
	Size               int64     `json:"size"`
	DownloadCount      int64     `json:"download_count"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

// GitHubClientInterface defines the methods needed from a GitHub client for version checking
type GitHubClientInterface interface {
	GetLatestRelease(owner, repo string) (*GitHubRelease, error)
	GetReleases(owner, repo string, includePrerelease bool) ([]GitHubRelease, error)
	GetReleaseByTag(owner, repo, tag string) (*GitHubRelease, error)
}

// UpdateInfo contains information about available updates
type UpdateInfo struct {
	CurrentVersion *SemanticVersion
	LatestVersion  *SemanticVersion
	Release        *GitHubRelease
	UpdateNeeded   bool
	IsPrerelease   bool
}

// VersionCheckResult represents the result of a version check
type VersionCheckResult struct {
	CurrentVersion  string     `json:"current_version"`
	LatestVersion   string     `json:"latest_version"`
	UpdateAvailable bool       `json:"update_available"`
	IsPrerelease    bool       `json:"is_prerelease"`
	ReleaseURL      string     `json:"release_url,omitempty"`
	ReleaseNotes    string     `json:"release_notes,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	Error           string     `json:"error,omitempty"`
}

// CheckOptions configures version checking behavior
type CheckOptions struct {
	IncludePrerelease bool
	Repository        string // Format: "owner/repo"
	GitHubToken       string // Optional for higher rate limits
	Timeout           time.Duration
	Client            GitHubClientInterface // Optional: inject alternate GitHub client
}

// DefaultCheckOptions returns default version check options
func DefaultCheckOptions() *CheckOptions {
	return &CheckOptions{
		IncludePrerelease: false,
		Repository:        "perigrin/git-zhi", // Default repository
		Timeout:           30 * time.Second,
	}
}
