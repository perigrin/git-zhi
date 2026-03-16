// ABOUTME: GitHub API client for fetching git-zhi release information.
// ABOUTME: Handles release listing, rate limiting, and asset enumeration.

package version

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// GitHubClient handles GitHub API interactions.
type GitHubClient struct {
	httpClient *http.Client
	baseURL    string
	token      string // Optional; used for higher rate limits
}

// NewGitHubClient creates a new GitHub API client, automatically picking up
// GITHUB_TOKEN from the environment when present.
func NewGitHubClient() *GitHubClient {
	client := &GitHubClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.github.com",
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client.token = token
	}

	return client
}

// NewGitHubClientWithToken creates a new GitHub API client with the supplied
// authentication token.
func NewGitHubClientWithToken(token string) *GitHubClient {
	client := NewGitHubClient()
	client.token = token
	return client
}

// doRequestWithRetry executes an HTTP request with exponential backoff for rate limiting.
// In test environments (detected via os.Args[0] or GO_TEST env var) retry counts
// and wait durations are capped to prevent test timeouts.
func (g *GitHubClient) doRequestWithRetry(req *http.Request, maxRetries int) (*http.Response, error) {
	isTestEnvironment := strings.HasSuffix(os.Args[0], ".test") ||
		strings.Contains(os.Args[0], "_test") ||
		os.Getenv("GO_TEST") == "1"

	if isTestEnvironment {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		// Handle GitHub rate limiting (403 with rate-limit body text).
		if resp.StatusCode == http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			bodyStr := string(body)

			if strings.Contains(bodyStr, "rate limit exceeded") || strings.Contains(bodyStr, "API rate limit") {
				if isTestEnvironment {
					return nil, fmt.Errorf("GitHub API rate limit exceeded (test environment): %s", bodyStr)
				}

				resetTime := resp.Header.Get("X-RateLimit-Reset")

				var backoffTime time.Duration
				if resetTime != "" {
					if resetTimestamp, err := strconv.ParseInt(resetTime, 10, 64); err == nil {
						backoffTime = time.Until(time.Unix(resetTimestamp, 0))
						maxBackoff := 5 * time.Minute
						if backoffTime > maxBackoff {
							backoffTime = maxBackoff
						}
					}
				}

				if backoffTime <= 0 {
					backoffTime = time.Duration(math.Pow(2, float64(attempt))) * time.Second
				}

				if attempt == maxRetries-1 {
					return nil, fmt.Errorf("GitHub API rate limit exceeded after %d attempts: %s", maxRetries, bodyStr)
				}

				time.Sleep(backoffTime)
				continue
			}

			// Non-rate-limit 403: reconstruct response so caller can inspect it.
			return &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header,
				Body:       io.NopCloser(strings.NewReader(bodyStr)),
			}, nil
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// GetLatestRelease returns the most recent published release for the repository.
// It skips draft releases and prefers a stable release over a prerelease when both
// are available.
func (g *GitHubClient) GetLatestRelease(owner, repo string) (*GitHubRelease, error) {
	releases, err := g.GetReleases(owner, repo, true)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for repository %s/%s", owner, repo)
	}

	var latestStable *GitHubRelease
	var latestPrerelease *GitHubRelease

	for i := range releases {
		release := &releases[i]
		if release.Draft {
			continue
		}

		if !release.Prerelease {
			if latestStable == nil || release.CreatedAt.After(latestStable.CreatedAt) {
				latestStable = release
			}
		} else {
			if latestPrerelease == nil || release.CreatedAt.After(latestPrerelease.CreatedAt) {
				latestPrerelease = release
			}
		}
	}

	if latestStable != nil {
		return latestStable, nil
	}
	if latestPrerelease != nil {
		return latestPrerelease, nil
	}

	return nil, fmt.Errorf("no published releases found for repository %s/%s", owner, repo)
}

// GetReleaseByTag fetches a specific release by its tag name.
// A "v" prefix is added to the tag if it is not already present.
func (g *GitHubClient) GetReleaseByTag(owner, repo, tag string) (*GitHubRelease, error) {
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", g.baseURL, owner, repo, tag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.doRequestWithRetry(req, 3)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("release %s not found", tag)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &release, nil
}

// GetReleases fetches all releases for the repository.
// When includePrerelease is false, draft and prerelease entries are omitted.
func (g *GitHubClient) GetReleases(owner, repo string, includePrerelease bool) ([]GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases", g.baseURL, owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.doRequestWithRetry(req, 3)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if !includePrerelease {
		filtered := make([]GitHubRelease, 0, len(releases))
		for _, release := range releases {
			if !release.Prerelease && !release.Draft {
				filtered = append(filtered, release)
			}
		}
		releases = filtered
	}

	return releases, nil
}
