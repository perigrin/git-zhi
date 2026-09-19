// ABOUTME: Resolve decides who is acting: an explicit value, then ZHI_ACTOR,
// ABOUTME: then the git author derivation that was the only source before.
package actor

import (
	"fmt"
	"os"
	"strings"
)

// EnvVar is the environment variable a process uses to declare its identity.
// It is per-process by construction, which is the property that makes it the
// right carrier: git config — repository, global or worktree-scoped — is shared
// by every process that reads it.
const EnvVar = "ZHI_ACTOR"

// Resolve returns the actor to record for the current process.
//
// The first source that is set wins: explicit, then EnvVar, then the derivation
// from git author name and email. With nothing declared the result is exactly
// what the tool recorded before this existed, so no stored chain changes
// meaning and no migration is needed.
//
// A value from the environment must name its type — "agent:" or "human:" — and
// is refused otherwise. ParseActor defaults a bare string to human, which is
// load-bearing for stored values and for flags whose callers already pass bare
// worker names; but at the point a new identity enters the system, defaulting
// would record agents as humans and say nothing. explicit keeps the lenient
// parse for exactly that reason: it is a flag, not a new boundary.
func Resolve(explicit, gitName, gitEmail string) (Actor, error) {
	if explicit != "" {
		return ParseActor(explicit), nil
	}

	if declared := strings.TrimSpace(os.Getenv(EnvVar)); declared != "" {
		typ, id, ok := strings.Cut(declared, ":")
		if !ok || (typ != TypeHuman && typ != TypeAgent) || id == "" {
			return Actor{}, fmt.Errorf(
				"%s=%q must name its type: %q or %q followed by an identifier, e.g. %s=agent:worker-3",
				EnvVar, declared, TypeAgent+":", TypeHuman+":", EnvVar)
		}
		return Actor{Type: typ, ID: id}, nil
	}

	return DeriveActor(gitName, gitEmail), nil
}
