// ABOUTME: Tests that a transition records the identity the process declared,
// ABOUTME: so two workers in one repository are distinguishable.
package cli_test

import (
	"testing"

	"github.com/perigrin/git-zhi/internal/actor"
	"github.com/perigrin/git-zhi/internal/issue"
)

// lastActor returns the actor on an issue's most recent transition.
func lastActor(t *testing.T, app interface{}, all []*issue.Issue, uuidStr string) string {
	t.Helper()
	for _, iss := range all {
		if iss.ID.String() != uuidStr {
			continue
		}
		if len(iss.Transitions) == 0 {
			t.Fatalf("issue %s has no transitions", uuidStr[:8])
		}
		return iss.Transitions[len(iss.Transitions)-1].Actor
	}
	t.Fatalf("issue %s not found", uuidStr[:8])
	return ""
}

// TestTransitionActor_RecordsDeclaredIdentity — the load-bearing behaviour.
// Until a transition carries a declared identity, headForActor sees one actor
// for every worker and its per-worker rules collapse.
func TestTransitionActor_RecordsDeclaredIdentity(t *testing.T) {
	app, run := setupEditTest(t)
	t.Setenv("ZHI_ACTOR", "agent:worker-one")

	id := createEditTestIssue(t, app, "Declared work")
	if _, _, err := run("issue", "edit", id, "--state", "start"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if got := lastActor(t, app, all, id); got != "agent:worker-one" {
		t.Errorf("expected agent:worker-one on the transition, got %q", got)
	}
}

// TestTransitionActor_TwoIdentitiesStayDistinct — contract behaviour 1 from the
// decision: one repository, one git author config, two declared identities,
// two different recorded actors.
func TestTransitionActor_TwoIdentitiesStayDistinct(t *testing.T) {
	app, run := setupEditTest(t)

	first := createEditTestIssue(t, app, "Work by one")
	second := createEditTestIssue(t, app, "Work by two")

	t.Setenv("ZHI_ACTOR", "agent:worker-one")
	if _, _, err := run("issue", "edit", first, "--state", "start"); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	t.Setenv("ZHI_ACTOR", "agent:worker-two")
	if _, _, err := run("issue", "edit", second, "--state", "start"); err != nil {
		t.Fatalf("second start failed: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	a, b := lastActor(t, app, all, first), lastActor(t, app, all, second)
	if a == b {
		t.Fatalf("both issues recorded the same actor %q", a)
	}
	if a != "agent:worker-one" || b != "agent:worker-two" {
		t.Errorf("expected worker-one and worker-two, got %q and %q", a, b)
	}
}

// TestTransitionActorUndeclared — with nothing declared the recorded actor is
// exactly what it was before this existed. No stored chain changes meaning.
func TestTransitionActorUndeclared(t *testing.T) {
	app, run := setupEditTest(t)
	t.Setenv("ZHI_ACTOR", "")

	id := createEditTestIssue(t, app, "Undeclared work")
	if _, _, err := run("issue", "edit", id, "--state", "start"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	name, email := app.Store.AuthorInfo()
	want := actor.DeriveActor(name, email).String()
	if got := lastActor(t, app, all, id); got != want {
		t.Errorf("expected the git-author derivation %q, got %q", want, got)
	}
}

// TestTransitionActor_RefusesUnprefixedIdentity — the trust boundary reaches
// the command, not just the resolver.
func TestTransitionActor_RefusesUnprefixedIdentity(t *testing.T) {
	app, run := setupEditTest(t)
	t.Setenv("ZHI_ACTOR", "worker-one")

	id := createEditTestIssue(t, app, "Bad identity")
	if _, _, err := run("issue", "edit", id, "--state", "start"); err == nil {
		t.Fatal("expected an unprefixed ZHI_ACTOR to refuse the transition")
	}
}
