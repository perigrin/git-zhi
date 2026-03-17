// ABOUTME: Tests for the Actor package: ParseActor, DeriveActor, and String()
// ABOUTME: covering human/agent type inference and "type:identifier" formatting.
package actor_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/actor"
)

func TestParseActor_Human(t *testing.T) {
	a := actor.ParseActor("human:perigrin")
	if a.Type != actor.TypeHuman {
		t.Fatalf("expected type %q, got %q", actor.TypeHuman, a.Type)
	}
	if a.ID != "perigrin" {
		t.Fatalf("expected ID %q, got %q", "perigrin", a.ID)
	}
}

func TestParseActor_Agent(t *testing.T) {
	a := actor.ParseActor("agent:claude-code-1")
	if a.Type != actor.TypeAgent {
		t.Fatalf("expected type %q, got %q", actor.TypeAgent, a.Type)
	}
	if a.ID != "claude-code-1" {
		t.Fatalf("expected ID %q, got %q", "claude-code-1", a.ID)
	}
}

func TestParseActor_UnknownFormatDefaultsToHuman(t *testing.T) {
	a := actor.ParseActor("perigrin")
	if a.Type != actor.TypeHuman {
		t.Fatalf("expected type %q for unrecognized format, got %q", actor.TypeHuman, a.Type)
	}
	if a.ID != "perigrin" {
		t.Fatalf("expected ID %q, got %q", "perigrin", a.ID)
	}
}

func TestParseActor_EmptyString(t *testing.T) {
	a := actor.ParseActor("")
	if a.Type != actor.TypeHuman {
		t.Fatalf("expected type %q for empty string, got %q", actor.TypeHuman, a.Type)
	}
	if a.ID != "" {
		t.Fatalf("expected empty ID, got %q", a.ID)
	}
}

func TestParseActor_UnknownTypePrefix(t *testing.T) {
	// "robot:hal9000" has a colon but "robot" is not a known type, so the
	// whole string becomes the ID and type defaults to human.
	a := actor.ParseActor("robot:hal9000")
	if a.Type != actor.TypeHuman {
		t.Fatalf("expected type %q for unknown prefix, got %q", actor.TypeHuman, a.Type)
	}
	if a.ID != "robot:hal9000" {
		t.Fatalf("expected ID %q, got %q", "robot:hal9000", a.ID)
	}
}

func TestParseActor_ColonInIdentifier(t *testing.T) {
	// "human:user:with:colons" — type is "human", everything after the first
	// colon is the identifier including any additional colons.
	a := actor.ParseActor("human:user:with:colons")
	if a.Type != actor.TypeHuman {
		t.Fatalf("expected type %q, got %q", actor.TypeHuman, a.Type)
	}
	if a.ID != "user:with:colons" {
		t.Fatalf("expected ID %q, got %q", "user:with:colons", a.ID)
	}
}

func TestDeriveActor_HumanEmail(t *testing.T) {
	a := actor.DeriveActor("perigrin", "perigrin@example.com")
	if a.Type != actor.TypeHuman {
		t.Fatalf("expected type %q for normal email, got %q", actor.TypeHuman, a.Type)
	}
	if a.ID != "perigrin" {
		t.Fatalf("expected ID %q, got %q", "perigrin", a.ID)
	}
}

func TestDeriveActor_AgentEmailContainsAgent(t *testing.T) {
	a := actor.DeriveActor("claude-code-1", "agent-claude@example.com")
	if a.Type != actor.TypeAgent {
		t.Fatalf("expected type %q for email containing 'agent', got %q", actor.TypeAgent, a.Type)
	}
	if a.ID != "claude-code-1" {
		t.Fatalf("expected ID %q, got %q", "claude-code-1", a.ID)
	}
}

func TestDeriveActor_AgentEmailContainsBot(t *testing.T) {
	a := actor.DeriveActor("dependabot", "dependabot@github.com")
	if a.Type != actor.TypeAgent {
		t.Fatalf("expected type %q for email containing 'bot', got %q", actor.TypeAgent, a.Type)
	}
	if a.ID != "dependabot" {
		t.Fatalf("expected ID %q, got %q", "dependabot", a.ID)
	}
}

func TestActorString(t *testing.T) {
	a := actor.Actor{Type: actor.TypeHuman, ID: "perigrin"}
	got := a.String()
	want := "human:perigrin"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestActorString_Agent(t *testing.T) {
	a := actor.Actor{Type: actor.TypeAgent, ID: "claude-code-1"}
	got := a.String()
	want := "agent:claude-code-1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
