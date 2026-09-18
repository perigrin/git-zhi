// ABOUTME: Tests that milestone show renders body, resolution and postmortem
// ABOUTME: in human output, not only through --format json.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestMilestoneShow_HumanRendersPostmortem — a retrospective is written to be
// read later by a person, so it must be reachable without --format json.
func TestMilestoneShow_HumanRendersPostmortem(t *testing.T) {
	app, run := setupMilestoneTest(t)
	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")

	if _, err := run("milestone", "edit", "v0.1", "--postmortem", "what we learned"); err != nil {
		t.Fatalf("attaching postmortem failed: %v", err)
	}

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "what we learned") {
		t.Errorf("expected the postmortem text in human output, got:\n%s", out)
	}
	if !strings.Contains(out, "Postmortem:") {
		t.Errorf("expected a labelled Postmortem section, got:\n%s", out)
	}
}

// TestMilestoneShow_HumanRendersBody — the milestone's plan is as invisible as
// its retrospective was, and for the same reason.
func TestMilestoneShow_HumanRendersBody(t *testing.T) {
	app, run := setupMilestoneTest(t)
	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")

	if _, err := run("milestone", "edit", "v0.1", "--body", "ship the parser"); err != nil {
		t.Fatalf("attaching body failed: %v", err)
	}

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "ship the parser") {
		t.Errorf("expected the body text in human output, got:\n%s", out)
	}
}

// TestMilestoneShow_HumanRendersResolution — the resolution command is what a
// reader needs to know how the milestone is verified.
func TestMilestoneShow_HumanRendersResolution(t *testing.T) {
	app, run := setupMilestoneTest(t)
	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")

	if _, err := run("milestone", "edit", "v0.1", "--resolution", "go test ./..."); err != nil {
		t.Fatalf("attaching resolution failed: %v", err)
	}

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "go test ./...") {
		t.Errorf("expected the resolution command in human output, got:\n%s", out)
	}
}

// TestMilestoneShow_HumanOmitsEmptySections — a milestone with none of the
// three must not grow empty headings.
func TestMilestoneShow_HumanOmitsEmptySections(t *testing.T) {
	app, run := setupMilestoneTest(t)
	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show failed: %v", err)
	}

	out := stdout.String()
	for _, heading := range []string{"Postmortem:", "Body:", "Resolution:"} {
		if strings.Contains(out, heading) {
			t.Errorf("expected no %q heading when the field is empty, got:\n%s", heading, out)
		}
	}
}

// TestMilestoneShow_PostmortemFollowsIssues — a retrospective can be long, so
// it goes below the dashboard rather than pushing it off screen.
func TestMilestoneShow_PostmortemFollowsIssues(t *testing.T) {
	app, run := setupMilestoneTest(t)
	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")

	if _, err := run("milestone", "edit", "v0.1", "--postmortem", "what we learned"); err != nil {
		t.Fatalf("attaching postmortem failed: %v", err)
	}

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show failed: %v", err)
	}

	out := stdout.String()
	issuesAt := strings.Index(out, "Parser feature")
	postmortemAt := strings.Index(out, "what we learned")
	if issuesAt < 0 || postmortemAt < 0 {
		t.Fatalf("expected both the issue list and the postmortem, got:\n%s", out)
	}
	if postmortemAt < issuesAt {
		t.Errorf("expected the postmortem after the issue list, got:\n%s", out)
	}
}

// TestMilestoneShow_MultilinePostmortemSurvives — the frontmatter round trip
// is what #17 had to fix; a retrospective with its own "---" line must come
// back whole.
func TestMilestoneShow_MultilinePostmortemSurvives(t *testing.T) {
	app, run := setupMilestoneTest(t)
	createTestIssueWithMilestone(t, app, "Parser feature", issue.StatePending, "v0.1", "")

	pm := "## What went well\n\nThe parser landed.\n\n---\n\n## What did not\n\nThe timeline.\n"
	if _, err := run("milestone", "edit", "v0.1", "--postmortem", pm); err != nil {
		t.Fatalf("attaching postmortem failed: %v", err)
	}

	stdout, err := run("milestone", "show", "v0.1")
	if err != nil {
		t.Fatalf("milestone show failed: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"What went well", "The parser landed.", "What did not", "The timeline."} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the rendered postmortem, got:\n%s", want, out)
		}
	}
}
