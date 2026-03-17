// ABOUTME: Jira credential resolution for git-zhi. Reads token, email, and URL
// ABOUTME: from ZHI_JIRA_TOKEN/EMAIL/URL env vars, falling back to git config values.
package credentials

import (
	"fmt"
	"os"
	"strings"
)

// Credentials holds the resolved Jira authentication details.
type Credentials struct {
	Token string
	Email string
	URL   string
}

// Load resolves Jira credentials by checking environment variables first, then
// falling back to the supplied git config values (cfgToken, cfgEmail, cfgURL).
// Callers should pass the values read from git config zhi.sync.jira.token,
// zhi.sync.jira.email, and zhi.sync.jira.url respectively. An empty string is
// an acceptable config value that triggers the env-var-only path.
// Returns a clear error if any required credential remains missing after both
// sources are checked.
func Load(cfgToken, cfgEmail, cfgURL string) (*Credentials, error) {
	token := resolve("ZHI_JIRA_TOKEN", cfgToken)
	email := resolve("ZHI_JIRA_EMAIL", cfgEmail)
	url := resolve("ZHI_JIRA_URL", cfgURL)

	if token == "" {
		return nil, fmt.Errorf(
			"Jira token not configured: set ZHI_JIRA_TOKEN env var or run " +
				"'git config zhi.sync.jira.token <token>'",
		)
	}
	if email == "" {
		return nil, fmt.Errorf(
			"Jira email not configured: set ZHI_JIRA_EMAIL env var or run " +
				"'git config zhi.sync.jira.email <email>'",
		)
	}
	if url == "" {
		return nil, fmt.Errorf(
			"Jira URL not configured: set ZHI_JIRA_URL env var or run " +
				"'git config zhi.sync.jira.url <url>'",
		)
	}
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf(
			"Jira URL must use HTTPS (got %q): credentials would be sent in cleartext over HTTP",
			url,
		)
	}

	return &Credentials{Token: token, Email: email, URL: url}, nil
}

// resolve returns the env var value if non-empty, otherwise returns the
// provided fallback value from git config.
func resolve(envKey, fallback string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fallback
}

// ResolveField returns the value of the named env var if non-empty, otherwise
// returns the fallback. Callers that need to resolve a single credential field
// without the full Load validation (e.g. when using --jira-url override) use
// this helper directly.
func ResolveField(envKey, fallback string) string {
	return resolve(envKey, fallback)
}
