// ABOUTME: Tests for Jira ↔ git-zhi actor identity mapping.
// ABOUTME: Exercises MapActorToJira and MapJiraToActor with git config subsections.
package identity_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/jira/identity"
)

// TestMapActorToJira_configured verifies that a configured actor name maps to
// the expected Jira email when the mapping is present in git config.
func TestMapActorToJira_configured(t *testing.T) {
	cfg := map[string]string{
		"perigrin": "p.prather@example.com",
		"alice":    "alice@example.com",
	}
	got := identity.MapActorToJira(cfg, "perigrin")
	if got != "p.prather@example.com" {
		t.Errorf("MapActorToJira: got %q, want %q", got, "p.prather@example.com")
	}
}

// TestMapActorToJira_missing verifies that an unconfigured actor returns empty string.
func TestMapActorToJira_missing(t *testing.T) {
	cfg := map[string]string{
		"alice": "alice@example.com",
	}
	got := identity.MapActorToJira(cfg, "bob")
	if got != "" {
		t.Errorf("MapActorToJira(missing): got %q, want empty", got)
	}
}

// TestMapActorToJira_empty verifies that an empty config returns empty string.
func TestMapActorToJira_empty(t *testing.T) {
	got := identity.MapActorToJira(nil, "perigrin")
	if got != "" {
		t.Errorf("MapActorToJira(nil cfg): got %q, want empty", got)
	}
}

// TestMapJiraToActor_configured verifies reverse lookup by Jira display name.
func TestMapJiraToActor_configured(t *testing.T) {
	cfg := map[string]string{
		"perigrin": "p.prather@example.com",
		"alice":    "alice@example.com",
	}
	// The config maps actor → email; reverse by display name requires an
	// additional display-name mapping. The config format is:
	//   zhi.sync.jira.actor.<actorname> = jira-email
	// MapJiraToActor searches the values for jiraDisplayName and returns the key.
	got := identity.MapJiraToActor(cfg, "p.prather@example.com")
	if got != "perigrin" {
		t.Errorf("MapJiraToActor: got %q, want %q", got, "perigrin")
	}
}

// TestMapJiraToActor_missing verifies that an unconfigured Jira display name returns empty.
func TestMapJiraToActor_missing(t *testing.T) {
	cfg := map[string]string{
		"alice": "alice@example.com",
	}
	got := identity.MapJiraToActor(cfg, "bob@example.com")
	if got != "" {
		t.Errorf("MapJiraToActor(missing): got %q, want empty", got)
	}
}

// TestMapJiraToActor_nil verifies that a nil config returns empty string.
func TestMapJiraToActor_nil(t *testing.T) {
	got := identity.MapJiraToActor(nil, "anyone@example.com")
	if got != "" {
		t.Errorf("MapJiraToActor(nil cfg): got %q, want empty", got)
	}
}
