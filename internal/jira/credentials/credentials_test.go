// ABOUTME: Tests for Jira credential resolution from env vars and git config.
// ABOUTME: Covers all combinations of env-var overrides and config fallbacks.
package credentials_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/jira/credentials"
)

// gitCfg is a simple stub for the raw config values used in tests.
// The real caller passes values read from repo.Config().Raw.
type testGitCfg struct {
	token string
	email string
	url   string
}

// TestLoadCredentials_fromEnv verifies that env vars take precedence over
// git config values.
func TestLoadCredentials_fromEnv(t *testing.T) {
	t.Setenv("ZHI_JIRA_TOKEN", "tok-env")
	t.Setenv("ZHI_JIRA_EMAIL", "user@env.com")
	t.Setenv("ZHI_JIRA_URL", "https://env.atlassian.net")

	cfg := testGitCfg{token: "tok-cfg", email: "user@cfg.com", url: "https://cfg.atlassian.net"}
	got, err := credentials.Load(cfg.token, cfg.email, cfg.url)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if got.Token != "tok-env" {
		t.Errorf("Token: got %q, want %q", got.Token, "tok-env")
	}
	if got.Email != "user@env.com" {
		t.Errorf("Email: got %q, want %q", got.Email, "user@env.com")
	}
	if got.URL != "https://env.atlassian.net" {
		t.Errorf("URL: got %q, want %q", got.URL, "https://env.atlassian.net")
	}
}

// TestLoadCredentials_fromConfig verifies that git config values are used when
// env vars are absent.
func TestLoadCredentials_fromConfig(t *testing.T) {
	// Clear env vars to ensure config fallback.
	t.Setenv("ZHI_JIRA_TOKEN", "")
	t.Setenv("ZHI_JIRA_EMAIL", "")
	t.Setenv("ZHI_JIRA_URL", "")

	cfg := testGitCfg{token: "tok-cfg", email: "user@cfg.com", url: "https://cfg.atlassian.net"}
	got, err := credentials.Load(cfg.token, cfg.email, cfg.url)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if got.Token != "tok-cfg" {
		t.Errorf("Token: got %q, want %q", got.Token, "tok-cfg")
	}
	if got.Email != "user@cfg.com" {
		t.Errorf("Email: got %q, want %q", got.Email, "user@cfg.com")
	}
	if got.URL != "https://cfg.atlassian.net" {
		t.Errorf("URL: got %q, want %q", got.URL, "https://cfg.atlassian.net")
	}
}

// TestLoadCredentials_missingToken verifies a clear error when the token is
// absent from both env and config.
func TestLoadCredentials_missingToken(t *testing.T) {
	t.Setenv("ZHI_JIRA_TOKEN", "")
	t.Setenv("ZHI_JIRA_EMAIL", "user@example.com")
	t.Setenv("ZHI_JIRA_URL", "https://example.atlassian.net")

	_, err := credentials.Load("", "user@example.com", "https://example.atlassian.net")
	if err == nil {
		t.Fatal("Load with missing token: expected error, got nil")
	}
}

// TestLoadCredentials_missingEmail verifies a clear error when the email is absent.
func TestLoadCredentials_missingEmail(t *testing.T) {
	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "")
	t.Setenv("ZHI_JIRA_URL", "https://example.atlassian.net")

	_, err := credentials.Load("tok", "", "https://example.atlassian.net")
	if err == nil {
		t.Fatal("Load with missing email: expected error, got nil")
	}
}

// TestLoadCredentials_missingURL verifies a clear error when the URL is absent.
func TestLoadCredentials_missingURL(t *testing.T) {
	t.Setenv("ZHI_JIRA_TOKEN", "tok")
	t.Setenv("ZHI_JIRA_EMAIL", "user@example.com")
	t.Setenv("ZHI_JIRA_URL", "")

	_, err := credentials.Load("tok", "user@example.com", "")
	if err == nil {
		t.Fatal("Load with missing URL: expected error, got nil")
	}
}

// TestLoadCredentials_envOverridesPartial verifies that env vars override only
// the specific fields where they are set, falling back to config for others.
func TestLoadCredentials_envOverridesPartial(t *testing.T) {
	t.Setenv("ZHI_JIRA_TOKEN", "tok-env")
	t.Setenv("ZHI_JIRA_EMAIL", "")
	t.Setenv("ZHI_JIRA_URL", "")

	cfg := testGitCfg{token: "tok-cfg", email: "user@cfg.com", url: "https://cfg.atlassian.net"}
	got, err := credentials.Load(cfg.token, cfg.email, cfg.url)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if got.Token != "tok-env" {
		t.Errorf("Token: got %q, want %q (env should override)", got.Token, "tok-env")
	}
	if got.Email != "user@cfg.com" {
		t.Errorf("Email: got %q, want %q (should fall back to config)", got.Email, "user@cfg.com")
	}
}
