// ABOUTME: Tests that a bare `next` honours a declared identity, while an
// ABOUTME: undeclared process keeps the global WIP=1 behaviour unchanged.
package cli_test

import (
	"strings"
	"testing"
)

// TestChainNextActor_DeclaredIdentityGetsPerWorkerResolution — the point of the
// default. A worker that exported an identity should not be handed work another
// worker is holding, without having to repeat itself on every command.
func TestChainNextActor_DeclaredIdentityGetsPerWorkerResolution(t *testing.T) {
	app, run := setupEditTest(t)

	held := createEditTestIssue(t, app, "Held by worker one")
	free := createEditTestIssue(t, app, "Available work")

	t.Setenv("ZHI_ACTOR", "agent:worker-one")
	if _, _, err := run("issue", "edit", held, "--state", "start"); err != nil {
		t.Fatalf("starting held issue failed: %v", err)
	}

	t.Setenv("ZHI_ACTOR", "agent:worker-two")
	stdout, _, err := run("next")
	if err != nil {
		t.Fatalf("next failed for worker-two: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "Held by worker one") {
		t.Errorf("worker-two was handed worker-one's in-progress issue:\n%s", out)
	}
	if !strings.Contains(out, "Available work") {
		t.Errorf("expected the free issue, got:\n%s", out)
	}
	_ = free
}

// TestChainNextActorUndeclared — with nothing declared, `next` keeps global
// WIP=1 semantics and returns the in-progress issue regardless of who holds it.
// Resolve always yields an actor, so using it unconditionally here would switch
// every existing caller to per-worker resolution silently.
func TestChainNextActorUndeclared(t *testing.T) {
	app, run := setupEditTest(t)

	held := createEditTestIssue(t, app, "Held by worker one")
	createEditTestIssue(t, app, "Available work")

	t.Setenv("ZHI_ACTOR", "agent:worker-one")
	if _, _, err := run("issue", "edit", held, "--state", "start"); err != nil {
		t.Fatalf("starting held issue failed: %v", err)
	}

	t.Setenv("ZHI_ACTOR", "")
	stdout, _, err := run("next")
	if err != nil {
		t.Fatalf("next failed with nothing declared: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "Held by worker one") {
		t.Errorf("global resolution should return the in-progress issue, got:\n%s", out)
	}
}

// TestChainNextActor_FlagOverridesEnvironment — an explicit value has to be able
// to beat an ambient one, which is the whole reason the flag outranks it.
func TestChainNextActor_FlagOverridesEnvironment(t *testing.T) {
	app, run := setupEditTest(t)

	held := createEditTestIssue(t, app, "Held by worker one")
	createEditTestIssue(t, app, "Available work")

	t.Setenv("ZHI_ACTOR", "agent:worker-one")
	if _, _, err := run("issue", "edit", held, "--state", "start"); err != nil {
		t.Fatalf("starting held issue failed: %v", err)
	}

	// The environment says worker-one, who would resume the held issue. The
	// flag says worker-two, who must not be given it.
	stdout, _, err := run("next", "--actor", "agent:worker-two")
	if err != nil {
		t.Fatalf("next --actor failed: %v", err)
	}
	if out := stdout.String(); strings.Contains(out, "Held by worker one") {
		t.Errorf("the flag did not override the environment:\n%s", out)
	}
}
