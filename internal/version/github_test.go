// ABOUTME: Tests for the GitHub API client used to fetch git-zhi release information.
// ABOUTME: Uses httptest servers to exercise release listing, tag lookup, auth, and error paths.

package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// makeRelease is a test helper that builds a GitHubRelease with the given tag and flags.
func makeRelease(tag string, draft, prerelease bool, createdAt time.Time) GitHubRelease {
	return GitHubRelease{
		ID:         1,
		TagName:    tag,
		Name:       tag,
		Draft:      draft,
		Prerelease: prerelease,
		CreatedAt:  createdAt,
	}
}

func TestGetReleases_AllIncluded(t *testing.T) {
	releases := []GitHubRelease{
		makeRelease("v1.0.0", false, false, time.Now().Add(-2*time.Hour)),
		makeRelease("v1.1.0-rc1", false, true, time.Now().Add(-1*time.Hour)),
		makeRelease("v0.9.0-draft", true, false, time.Now().Add(-3*time.Hour)),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	got, err := client.GetReleases("owner", "repo", true)
	if err != nil {
		t.Fatalf("GetReleases returned error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 releases, got %d", len(got))
	}
}

func TestGetReleases_ExcludePrereleasesAndDrafts(t *testing.T) {
	releases := []GitHubRelease{
		makeRelease("v1.0.0", false, false, time.Now().Add(-2*time.Hour)),
		makeRelease("v1.1.0-rc1", false, true, time.Now().Add(-1*time.Hour)),
		makeRelease("v0.9.0-draft", true, false, time.Now().Add(-3*time.Hour)),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	got, err := client.GetReleases("owner", "repo", false)
	if err != nil {
		t.Fatalf("GetReleases returned error: %v", err)
	}
	// Only v1.0.0 is stable and not a draft
	if len(got) != 1 {
		t.Errorf("expected 1 release, got %d", len(got))
	}
	if got[0].TagName != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %s", got[0].TagName)
	}
}

func TestGetReleases_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	_, err := client.GetReleases("owner", "repo", true)
	if err == nil {
		t.Fatal("expected error from GetReleases on non-200 response, got nil")
	}
}

func TestGetReleaseByTag_Found(t *testing.T) {
	release := makeRelease("v1.2.3", false, false, time.Now())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/tags/v1.2.3" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	got, err := client.GetReleaseByTag("owner", "repo", "v1.2.3")
	if err != nil {
		t.Fatalf("GetReleaseByTag returned error: %v", err)
	}
	if got.TagName != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %s", got.TagName)
	}
}

func TestGetReleaseByTag_AddsVPrefix(t *testing.T) {
	release := makeRelease("v1.2.3", false, false, time.Now())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect v prefix to have been added
		if r.URL.Path != "/repos/owner/repo/releases/tags/v1.2.3" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	// Pass tag without v prefix
	got, err := client.GetReleaseByTag("owner", "repo", "1.2.3")
	if err != nil {
		t.Fatalf("GetReleaseByTag returned error: %v", err)
	}
	if got.TagName != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %s", got.TagName)
	}
}

func TestGetReleaseByTag_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	_, err := client.GetReleaseByTag("owner", "repo", "v9.9.9")
	if err == nil {
		t.Fatal("expected error for not-found release, got nil")
	}
}

func TestGetLatestRelease_PrefersStable(t *testing.T) {
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)

	releases := []GitHubRelease{
		makeRelease("v1.0.0", false, false, older),
		makeRelease("v1.1.0-rc1", false, true, newer),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	got, err := client.GetLatestRelease("owner", "repo")
	if err != nil {
		t.Fatalf("GetLatestRelease returned error: %v", err)
	}
	if got.TagName != "v1.0.0" {
		t.Errorf("expected stable v1.0.0 to be preferred, got %s", got.TagName)
	}
}

func TestGetLatestRelease_FallsBackToPrerelease(t *testing.T) {
	releases := []GitHubRelease{
		makeRelease("v1.1.0-rc1", false, true, time.Now()),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	got, err := client.GetLatestRelease("owner", "repo")
	if err != nil {
		t.Fatalf("GetLatestRelease returned error: %v", err)
	}
	if got.TagName != "v1.1.0-rc1" {
		t.Errorf("expected prerelease v1.1.0-rc1 as fallback, got %s", got.TagName)
	}
}

func TestGetLatestRelease_SkipsDrafts(t *testing.T) {
	releases := []GitHubRelease{
		makeRelease("v1.0.0-draft", true, false, time.Now()),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	_, err := client.GetLatestRelease("owner", "repo")
	if err == nil {
		t.Fatal("expected error when only drafts are available, got nil")
	}
}

func TestGetLatestRelease_NoReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GitHubRelease{})
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
	}

	_, err := client.GetLatestRelease("owner", "repo")
	if err == nil {
		t.Fatal("expected error for empty release list, got nil")
	}
}

func TestTokenAuth_SentInHeader(t *testing.T) {
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GitHubRelease{})
	}))
	defer srv.Close()

	client := &GitHubClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    srv.URL,
		token:      "mytoken123",
	}

	_, _ = client.GetReleases("owner", "repo", true)

	if receivedAuth != "token mytoken123" {
		t.Errorf("expected Authorization header 'token mytoken123', got %q", receivedAuth)
	}
}

func TestNewGitHubClient_NoToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	client := NewGitHubClient()
	if client == nil {
		t.Fatal("NewGitHubClient returned nil")
	}
	if client.token != "" {
		t.Errorf("expected empty token when GITHUB_TOKEN is unset, got %q", client.token)
	}
}

func TestNewGitHubClientWithToken_SetsToken(t *testing.T) {
	client := NewGitHubClientWithToken("supersecret")
	if client.token != "supersecret" {
		t.Errorf("expected token 'supersecret', got %q", client.token)
	}
}
