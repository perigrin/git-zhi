// ABOUTME: Tests for platform detection and GitHub release asset matching.
// ABOUTME: Covers DetectPlatform, AssetPattern, IsSupported, and SelectBestAsset.

package version

import (
	"runtime"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	p := DetectPlatform()

	if p == nil {
		t.Fatal("DetectPlatform returned nil")
	}

	if p.OS != runtime.GOOS {
		t.Errorf("expected OS %q, got %q", runtime.GOOS, p.OS)
	}

	if p.Architecture != runtime.GOARCH {
		t.Errorf("expected Architecture %q, got %q", runtime.GOARCH, p.Architecture)
	}

	if runtime.GOOS == "windows" {
		if p.Extension != ".exe" {
			t.Errorf("expected Extension %q on windows, got %q", ".exe", p.Extension)
		}
	} else {
		if p.Extension != "" {
			t.Errorf("expected empty Extension on non-windows, got %q", p.Extension)
		}
	}
}

func TestAssetPatternLinux(t *testing.T) {
	p := &Platform{
		OS:           "linux",
		Architecture: "amd64",
		Extension:    "",
	}

	got := p.AssetPattern()
	want := "git-zhi-linux-amd64"

	if got != want {
		t.Errorf("AssetPattern() = %q, want %q", got, want)
	}
}

func TestAssetPatternWindows(t *testing.T) {
	p := &Platform{
		OS:           "windows",
		Architecture: "amd64",
		Extension:    ".exe",
	}

	got := p.AssetPattern()
	want := "git-zhi-windows-amd64.exe"

	if got != want {
		t.Errorf("AssetPattern() = %q, want %q", got, want)
	}
}

func TestAssetPatternDarwinArm64(t *testing.T) {
	p := &Platform{
		OS:           "darwin",
		Architecture: "arm64",
		Extension:    "",
	}

	got := p.AssetPattern()
	want := "git-zhi-darwin-arm64"

	if got != want {
		t.Errorf("AssetPattern() = %q, want %q", got, want)
	}
}

func TestIsSupportedTrue(t *testing.T) {
	cases := []struct {
		os   string
		arch string
	}{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
	}

	for _, tc := range cases {
		p := &Platform{OS: tc.os, Architecture: tc.arch}
		if !p.IsSupported() {
			t.Errorf("IsSupported() = false for %s/%s, want true", tc.os, tc.arch)
		}
	}
}

func TestIsSupportedFalse(t *testing.T) {
	cases := []struct {
		os   string
		arch string
	}{
		{"linux", "386"},
		{"windows", "arm64"},
		{"freebsd", "amd64"},
		{"plan9", "amd64"},
	}

	for _, tc := range cases {
		p := &Platform{OS: tc.os, Architecture: tc.arch}
		if p.IsSupported() {
			t.Errorf("IsSupported() = true for %s/%s, want false", tc.os, tc.arch)
		}
	}
}

func TestSelectBestAssetMatch(t *testing.T) {
	p := &Platform{
		OS:           "linux",
		Architecture: "amd64",
		Extension:    "",
	}

	assets := []GitHubAsset{
		{Name: "git-zhi-linux-amd64", BrowserDownloadURL: "https://example.com/git-zhi-linux-amd64", Size: 1024},
		{Name: "git-zhi-darwin-arm64", BrowserDownloadURL: "https://example.com/git-zhi-darwin-arm64", Size: 1024},
	}

	asset, err := SelectBestAsset(assets, p)
	if err != nil {
		t.Fatalf("SelectBestAsset returned error: %v", err)
	}

	if asset.Name != "git-zhi-linux-amd64" {
		t.Errorf("SelectBestAsset returned %q, want %q", asset.Name, "git-zhi-linux-amd64")
	}
}

func TestSelectBestAssetNoMatch(t *testing.T) {
	p := &Platform{
		OS:           "linux",
		Architecture: "amd64",
		Extension:    "",
	}

	assets := []GitHubAsset{
		{Name: "git-zhi-darwin-arm64", BrowserDownloadURL: "https://example.com/git-zhi-darwin-arm64", Size: 1024},
		{Name: "git-zhi-windows-amd64.exe", BrowserDownloadURL: "https://example.com/git-zhi-windows-amd64.exe", Size: 1024},
	}

	_, err := SelectBestAsset(assets, p)
	if err == nil {
		t.Fatal("SelectBestAsset expected error for no matching assets, got nil")
	}
}

func TestSelectBestAssetVersionedName(t *testing.T) {
	p := &Platform{
		OS:           "darwin",
		Architecture: "arm64",
		Extension:    "",
	}

	assets := []GitHubAsset{
		{Name: "git-zhi-1.2.3-darwin-arm64.tar.gz", BrowserDownloadURL: "https://example.com/git-zhi-1.2.3-darwin-arm64.tar.gz", Size: 2048},
		{Name: "git-zhi-1.2.3-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/git-zhi-1.2.3-linux-amd64.tar.gz", Size: 2048},
	}

	asset, err := SelectBestAsset(assets, p)
	if err != nil {
		t.Fatalf("SelectBestAsset returned error: %v", err)
	}

	if asset.Name != "git-zhi-1.2.3-darwin-arm64.tar.gz" {
		t.Errorf("SelectBestAsset returned %q, want %q", asset.Name, "git-zhi-1.2.3-darwin-arm64.tar.gz")
	}
}
