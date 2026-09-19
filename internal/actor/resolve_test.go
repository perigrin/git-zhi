// ABOUTME: Tests for actor identity resolution: explicit value, then ZHI_ACTOR,
// ABOUTME: then the git-author derivation, with strict prefix validation.
package actor_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/actor"
)

// TestResolveActor_ExplicitWinsOverEnvironment — an explicit value is how a
// caller overrides an ambient one, so it has to outrank it.
func TestResolveActor_ExplicitWinsOverEnvironment(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "agent:from-env")

	got, err := actor.Resolve("agent:explicit", "Chris Prather", "chris@example.com")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.String() != "agent:explicit" {
		t.Errorf("expected the explicit value to win, got %q", got.String())
	}
}

// TestResolveActor_EnvironmentWinsOverGitAuthor — the declared identity is the
// point of the exercise; git author is only the fallback.
func TestResolveActor_EnvironmentWinsOverGitAuthor(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "agent:worker-3")

	got, err := actor.Resolve("", "Chris Prather", "chris@example.com")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Type != actor.TypeAgent || got.ID != "worker-3" {
		t.Errorf("expected agent:worker-3, got %q", got.String())
	}
}

// TestResolveActor_UndeclaredIsUnchanged — with nothing declared the result is
// exactly what the tool records today, so no stored chain changes meaning.
func TestResolveActor_UndeclaredIsUnchanged(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "")

	got, err := actor.Resolve("", "Chris Prather", "chris@example.com")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := actor.DeriveActor("Chris Prather", "chris@example.com")
	if got != want {
		t.Errorf("expected the git-author derivation %q, got %q", want.String(), got.String())
	}
}

// TestResolveActor_EmptyEnvironmentIsUnset — an exported-but-empty variable is
// how a shell clears one; it must not be an error.
func TestResolveActor_EmptyEnvironmentIsUnset(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "   ")

	got, err := actor.Resolve("", "Chris Prather", "bot@example.com")
	if err != nil {
		t.Fatalf("whitespace-only ZHI_ACTOR should be treated as unset, got: %v", err)
	}
	want := actor.DeriveActor("Chris Prather", "bot@example.com")
	if got != want {
		t.Errorf("expected the derivation %q, got %q", want.String(), got.String())
	}
}

// TestResolveActor_UnprefixedEnvironmentIsRefused — the trust boundary. A bare
// value would be recorded as human, silently turning agents into people.
func TestResolveActor_UnprefixedEnvironmentIsRefused(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "worker-3")

	_, err := actor.Resolve("", "Chris Prather", "chris@example.com")
	if err == nil {
		t.Fatal("expected an unprefixed ZHI_ACTOR to be refused")
	}
	for _, want := range []string{"agent:", "human:", "worker-3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the error to mention %q, got: %v", want, err)
		}
	}
}

// TestResolveActor_UnknownPrefixIsRefused — a prefix that is not one of the two
// recognised types is a mistake, not a human named "robot".
func TestResolveActor_UnknownPrefixIsRefused(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "robot:worker-3")

	if _, err := actor.Resolve("", "Chris Prather", "chris@example.com"); err == nil {
		t.Fatal("expected an unrecognised type prefix to be refused")
	}
}

// TestResolveActor_HumanPrefixAccepted — both recognised types work.
func TestResolveActor_HumanPrefixAccepted(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "human:chris")

	got, err := actor.Resolve("", "Chris Prather", "chris@example.com")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Type != actor.TypeHuman || got.ID != "chris" {
		t.Errorf("expected human:chris, got %q", got.String())
	}
}

// TestResolveActor_ExplicitIsNotPrefixChecked — the flag keeps ParseActor's
// lenient default, because existing --actor callers pass bare worker names and
// the strictness belongs only where a new identity enters from the environment.
func TestResolveActor_ExplicitIsNotPrefixChecked(t *testing.T) {
	t.Setenv("ZHI_ACTOR", "")

	got, err := actor.Resolve("worker-3", "Chris Prather", "chris@example.com")
	if err != nil {
		t.Fatalf("an explicit bare value should still parse: %v", err)
	}
	if got.Type != actor.TypeHuman || got.ID != "worker-3" {
		t.Errorf("expected ParseActor's lenient default, got %q", got.String())
	}
}
