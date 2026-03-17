// ABOUTME: Actor package for git-zhi. Defines the Actor struct representing a
// ABOUTME: human or agent identity, with parsing and type inference utilities.
package actor

import (
	"fmt"
	"strings"
)

// TypeHuman identifies an Actor as a human operator.
const TypeHuman = "human"

// TypeAgent identifies an Actor as an automated agent or bot.
const TypeAgent = "agent"

// Actor represents the identity of a human or automated agent performing
// actions on issues and transitions.
type Actor struct {
	// Type is either TypeHuman or TypeAgent.
	Type string
	// ID is the unique identifier for this actor (e.g., a username or agent name).
	ID string
}

// ParseActor parses a string in "type:identifier" format into an Actor.
// If the format is not recognized (no colon separator with a known type),
// the entire string is treated as the ID and the type defaults to "human".
func ParseActor(s string) Actor {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		t := parts[0]
		if t == TypeHuman || t == TypeAgent {
			return Actor{Type: t, ID: parts[1]}
		}
	}
	return Actor{Type: TypeHuman, ID: s}
}

// DeriveActor infers an Actor from git config name and email values.
// Email addresses containing "agent" or "bot" (case-insensitive) are
// classified as agent type; all others are classified as human.
func DeriveActor(name, email string) Actor {
	lower := strings.ToLower(email)
	if strings.Contains(lower, "agent") || strings.Contains(lower, "bot") {
		return Actor{Type: TypeAgent, ID: name}
	}
	return Actor{Type: TypeHuman, ID: name}
}

// String returns the actor in "type:identifier" format.
func (a Actor) String() string {
	return fmt.Sprintf("%s:%s", a.Type, a.ID)
}
