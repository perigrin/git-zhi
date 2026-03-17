// ABOUTME: Tests for the Actor package: ParseActor, DeriveActor, and String()
// ABOUTME: covering human/agent type inference and "type:identifier" formatting.
package actor_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/actor"
)

func TestParseActor_Human(t *testing.T) {
	a := actor.ParseActor("human:perigrin")
	if a.Type != "human" {
		t.Fatalf("expected type %q, got %q", "human", a.Type)
	}
	if a.ID != "perigrin" {
		t.Fatalf("expected ID %q, got %q", "perigrin", a.ID)
	}
}

func TestParseActor_Agent(t *testing.T) {
	a := actor.ParseActor("agent:claude-code-1")
	if a.Type != "agent" {
		t.Fatalf("expected type %q, got %q", "agent", a.Type)
	}
	if a.ID != "claude-code-1" {
		t.Fatalf("expected ID %q, got %q", "claude-code-1", a.ID)
	}
}

func TestParseActor_UnknownFormatDefaultsToHuman(t *testing.T) {
	a := actor.ParseActor("perigrin")
	if a.Type != "human" {
		t.Fatalf("expected type %q for unrecognized format, got %q", "human", a.Type)
	}
	if a.ID != "perigrin" {
		t.Fatalf("expected ID %q, got %q", "perigrin", a.ID)
	}
}

func TestDeriveActor_HumanEmail(t *testing.T) {
	a := actor.DeriveActor("perigrin", "perigrin@example.com")
	if a.Type != "human" {
		t.Fatalf("expected type %q for normal email, got %q", "human", a.Type)
	}
	if a.ID != "perigrin" {
		t.Fatalf("expected ID %q, got %q", "perigrin", a.ID)
	}
}

func TestDeriveActor_AgentEmailContainsAgent(t *testing.T) {
	a := actor.DeriveActor("claude-code-1", "agent-claude@example.com")
	if a.Type != "agent" {
		t.Fatalf("expected type %q for email containing 'agent', got %q", "agent", a.Type)
	}
	if a.ID != "claude-code-1" {
		t.Fatalf("expected ID %q, got %q", "claude-code-1", a.ID)
	}
}

func TestDeriveActor_AgentEmailContainsBot(t *testing.T) {
	a := actor.DeriveActor("dependabot", "dependabot@github.com")
	if a.Type != "agent" {
		t.Fatalf("expected type %q for email containing 'bot', got %q", "agent", a.Type)
	}
	if a.ID != "dependabot" {
		t.Fatalf("expected ID %q, got %q", "dependabot", a.ID)
	}
}

func TestActorString(t *testing.T) {
	a := actor.Actor{Type: "human", ID: "perigrin"}
	got := a.String()
	want := "human:perigrin"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestActorString_Agent(t *testing.T) {
	a := actor.Actor{Type: "agent", ID: "claude-code-1"}
	got := a.String()
	want := "agent:claude-code-1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
