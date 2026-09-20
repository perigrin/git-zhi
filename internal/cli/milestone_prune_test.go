// ABOUTME: Tests for the milestone prune command: removes milestones with
// ABOUTME: zero issues and no authored content, sparing anything a human made.
package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
)

// writeTestMilestone writes ms directly to the store, bypassing the CLI so
// tests can construct milestones with arbitrary combinations of fields
// (postmortem without --state complete, a completed state without running
// the gate, and so on).
func writeTestMilestone(t *testing.T, app *cli.App, ms *milestone.Milestone) {
	t.Helper()
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	refPath := milestone.RefPrefix + ms.Name
	if err := app.Store.WriteEntity(refPath, "milestone.yaml", data, "Add test milestone: "+ms.Name); err != nil {
		t.Fatalf("WriteEntity milestone: %v", err)
	}
}

// TestMilestonePrune_RemovesBareMilestone verifies that a milestone with zero
// issues and no authored content — exactly what EnsureInitialized manufactures
// as v0.1 — is removed by prune.
func TestMilestonePrune_RemovesBareMilestone(t *testing.T) {
	app, run := setupMilestoneTest(t)

	if !app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Fatalf("expected v0.1 to exist before prune")
	}

	if _, err := run("milestone", "prune"); err != nil {
		t.Fatalf("milestone prune failed: %v", err)
	}

	if app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Fatalf("expected v0.1 to be pruned")
	}
}

// TestMilestonePrune_ReportsExamined verifies the summary line names both how
// many milestones were examined and how many were pruned.
func TestMilestonePrune_ReportsExamined(t *testing.T) {
	_, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "prune")
	if err != nil {
		t.Fatalf("milestone prune failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "examined 1 milestones, pruned 1") {
		t.Errorf("expected examined/pruned summary in output, got:\n%s", output)
	}
}

// TestMilestonePrune_DryRun verifies --dry-run reports what would be removed
// and removes nothing.
func TestMilestonePrune_DryRun(t *testing.T) {
	app, run := setupMilestoneTest(t)

	stdout, err := run("milestone", "prune", "--dry-run")
	if err != nil {
		t.Fatalf("milestone prune --dry-run failed: %v", err)
	}

	if !app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Fatalf("expected --dry-run to remove nothing, but v0.1 is gone")
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.1") {
		t.Errorf("expected v0.1 named in dry-run output, got:\n%s", output)
	}
	if !strings.Contains(output, "would prune") {
		t.Errorf("expected 'would prune' in dry-run output, got:\n%s", output)
	}
}

// TestMilestonePrune_SparesPopulated verifies a milestone with issues is
// never pruned, however few.
func TestMilestonePrune_SparesPopulated(t *testing.T) {
	app, run := setupMilestoneTest(t)

	createTestIssueWithMilestone(t, app, "Issue A", issue.StatePending, "v0.1", "")

	if _, err := run("milestone", "prune"); err != nil {
		t.Fatalf("milestone prune failed: %v", err)
	}

	if !app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Fatalf("expected v0.1 to be spared: it has an issue")
	}
}

// TestMilestonePrune_SparesAuthored verifies a milestone with a body,
// resolution, postmortem, or due date is spared even with zero issues.
func TestMilestonePrune_SparesAuthored(t *testing.T) {
	app, run := setupMilestoneTest(t)

	due := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	writeTestMilestone(t, app, &milestone.Milestone{Name: "has-body", Created: time.Now(), Body: "plan text"})
	writeTestMilestone(t, app, &milestone.Milestone{Name: "has-resolution", Created: time.Now(), Resolution: "make test"})
	writeTestMilestone(t, app, &milestone.Milestone{Name: "has-postmortem", Created: time.Now(), Postmortem: "retro text"})
	writeTestMilestone(t, app, &milestone.Milestone{Name: "has-due", Created: time.Now(), Due: &due})

	if _, err := run("milestone", "prune"); err != nil {
		t.Fatalf("milestone prune failed: %v", err)
	}

	for _, name := range []string{"has-body", "has-resolution", "has-postmortem", "has-due"} {
		if !app.Store.RefExists(milestone.RefPrefix + name) {
			t.Errorf("expected %s to be spared: it has authored content", name)
		}
	}
	// v0.1 remains bare and should still be pruned.
	if app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Errorf("expected bare v0.1 to be pruned alongside the authored spares")
	}
}

// TestMilestonePrune_SparesTagged verifies a tagged milestone is spared and
// the skip is reported, so no tag is left pointing at a deleted ref.
func TestMilestonePrune_SparesTagged(t *testing.T) {
	app, run := setupMilestoneTest(t)

	if _, err := run("milestone", "edit", "v0.1", "--tag", "current-sprint"); err != nil {
		t.Fatalf("milestone edit --tag failed: %v", err)
	}

	stdout, err := run("milestone", "prune")
	if err != nil {
		t.Fatalf("milestone prune failed: %v", err)
	}

	if !app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Fatalf("expected tagged v0.1 to be spared")
	}
	if !app.Store.RefExists("refs/zhi/_/tags/current-sprint") {
		t.Fatalf("expected tag ref to survive prune")
	}

	output := stdout.String()
	if !strings.Contains(output, "v0.1") || !strings.Contains(output, "tagged") {
		t.Errorf("expected the skip to be reported, got:\n%s", output)
	}
}

// TestMilestonePrune_SparesCompleted verifies a completed milestone is
// spared regardless of content — closed history is not swept.
func TestMilestonePrune_SparesCompleted(t *testing.T) {
	app, run := setupMilestoneTest(t)

	now := time.Now()
	writeTestMilestone(t, app, &milestone.Milestone{
		Name:      "done-and-bare",
		Created:   now,
		State:     "completed",
		Completed: &now,
	})

	if _, err := run("milestone", "prune"); err != nil {
		t.Fatalf("milestone prune failed: %v", err)
	}

	if !app.Store.RefExists(milestone.RefPrefix + "done-and-bare") {
		t.Fatalf("expected completed milestone to be spared")
	}
}

// TestMilestonePrune_CleanRun verifies a run that prunes nothing still exits
// 0 and reports what it examined, rather than printing nothing.
func TestMilestonePrune_CleanRun(t *testing.T) {
	app, run := setupMilestoneTest(t)

	// Give v0.1 authored content so nothing is prunable.
	if _, err := run("milestone", "edit", "v0.1", "--body", "in flight"); err != nil {
		t.Fatalf("milestone edit --body failed: %v", err)
	}

	stdout, err := run("milestone", "prune")
	if err != nil {
		t.Fatalf("milestone prune on a clean chain should exit 0, got: %v", err)
	}

	if !app.Store.RefExists(milestone.RefPrefix + "v0.1") {
		t.Fatalf("expected v0.1 to survive: it has a body")
	}

	output := stdout.String()
	if !strings.Contains(output, "examined 1 milestones, pruned 0") {
		t.Errorf("expected a clean-run summary naming what was examined, got:\n%s", output)
	}
}
