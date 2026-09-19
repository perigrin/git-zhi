// ABOUTME: Tests that project next accepts a declared identity in place of
// ABOUTME: --actor, and still says which source is missing when neither is set.
package project_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/project"
)

// TestProjectNextActor_DeclaredIdentitySatisfiesFlag — project next has
// required --actor since it shipped, and nothing could declare an identity, so
// the cross-repo layer has been unreachable rather than merely awkward.
func TestProjectNextActor_DeclaredIdentitySatisfiesFlag(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	writeMilestone(t, appA, &milestone.Milestone{Name: "v1.0", Created: now})
	writeIssue(t, appA, &issue.Issue{
		Title: "Next task", State: issue.StatePending, Milestone: "v1.0",
		Assigned: "alice", Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:    "Declared Identity",
		Repos:   []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
		Workers: []project.WorkerDef{{Name: "alice", Repos: []string{dirA}}},
	}
	projectFile := writeProjectFile(t, def)

	t.Setenv("ZHI_ACTOR", "human:alice")

	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"next", projectFile})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("a declared identity should satisfy project next: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Next task") {
		t.Errorf("expected the issue title, got:\n%s", got)
	}
}

// TestProjectNextActor_FlagStillWins — an explicit value overrides an ambient
// one here as everywhere else.
func TestProjectNextActor_FlagStillWins(t *testing.T) {
	dirA, appA := makeTestRepo(t)

	now := time.Now()
	writeMilestone(t, appA, &milestone.Milestone{Name: "v1.0", Created: now})
	writeIssue(t, appA, &issue.Issue{
		Title: "Bob's task", State: issue.StatePending, Milestone: "v1.0",
		Assigned: "bob", Created: now, Updated: now,
	})

	def := &project.ProjectDef{
		Name:  "Flag Wins",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
		Workers: []project.WorkerDef{
			{Name: "alice", Repos: []string{dirA}},
			{Name: "bob", Repos: []string{dirA}},
		},
	}
	projectFile := writeProjectFile(t, def)

	t.Setenv("ZHI_ACTOR", "human:alice")

	cmd := project.NewProjectCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"next", "--actor", "bob", projectFile})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Bob's task") {
		t.Errorf("the flag should have won over the environment, got:\n%s", got)
	}
}

// TestProjectNextActor_NeitherSourceNamesBoth — with no flag and nothing
// declared, the error has to say both ways of supplying one.
func TestProjectNextActor_NeitherSourceNamesBoth(t *testing.T) {
	dirA, _ := makeTestRepo(t)

	def := &project.ProjectDef{
		Name:  "Neither",
		Repos: []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
	}
	projectFile := writeProjectFile(t, def)

	t.Setenv("ZHI_ACTOR", "")

	cmd := project.NewProjectCommand()
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"next", projectFile})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error when neither flag nor environment supplies an identity")
	}
	for _, want := range []string{"--actor", "ZHI_ACTOR"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the error to name %q, got: %v", want, err)
		}
	}
}

// TestProjectNextActorUnknownWorker — a declared identity that is not a worker
// in the project definition fails there, not at the flag check, and the message
// has to distinguish the two so the fix is obvious.
func TestProjectNextActorUnknownWorker(t *testing.T) {
	dirA, _ := makeTestRepo(t)

	def := &project.ProjectDef{
		Name:    "Unknown Worker",
		Repos:   []project.RepoDef{{Path: dirA, Milestone: "v1.0"}},
		Workers: []project.WorkerDef{{Name: "alice", Repos: []string{dirA}}},
	}
	projectFile := writeProjectFile(t, def)

	t.Setenv("ZHI_ACTOR", "agent:not-in-the-project")

	cmd := project.NewProjectCommand()
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"next", projectFile})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an identity absent from the project definition")
	}
	if !strings.Contains(err.Error(), "not-in-the-project") {
		t.Errorf("expected the error to name the identity, got: %v", err)
	}
}
