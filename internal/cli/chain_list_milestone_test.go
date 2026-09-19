// ABOUTME: Tests that the top-level list command honours --milestone on both
// ABOUTME: the default and --ready paths, in JSON and human output.
package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// listedTitles decodes the JSON from a chain list run and returns issue titles.
func listedTitles(t *testing.T, out string) []string {
	t.Helper()
	var payload struct {
		Issues []struct {
			Title     string `json:"title"`
			Milestone string `json:"milestone"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode list JSON: %v\noutput: %s", err, out)
	}
	titles := make([]string, 0, len(payload.Issues))
	for _, iss := range payload.Issues {
		titles = append(titles, iss.Title)
	}
	return titles
}

// TestChainList_MilestoneFilterIsHonoured — the top-level list command computed
// a milestone-filtered display set and then rendered the graph's own sort
// instead, so the filter did nothing at all.
func TestChainList_MilestoneFilterIsHonoured(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestMilestone(t, app, "alpha", nil)
	createTestMilestone(t, app, "alpha-extra", nil)
	createTestIssueWithDeps(t, app, "In alpha", issue.StatePending, "alpha", nil)
	createTestIssueWithDeps(t, app, "In alpha-extra", issue.StatePending, "alpha-extra", nil)

	stdout, err := run("--format", "json", "list", "--milestone", "alpha")
	if err != nil {
		t.Fatalf("list --milestone failed: %v", err)
	}

	titles := listedTitles(t, stdout.String())
	if len(titles) != 1 || titles[0] != "In alpha" {
		t.Errorf("expected only [In alpha], got %v", titles)
	}
}

// TestChainList_MilestoneFilterDoesNotLeakPrefixSibling — a milestone whose
// name is a prefix of another must not pull in the longer one's issues. This is
// the shape that hands an executing agent work from the wrong milestone.
func TestChainList_MilestoneFilterDoesNotLeakPrefixSibling(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestMilestone(t, app, "rfc-0001", nil)
	createTestMilestone(t, app, "rfc-0001-followups", nil)
	createTestIssueWithDeps(t, app, "Followup work", issue.StatePending, "rfc-0001-followups", nil)

	stdout, err := run("--format", "json", "list", "--milestone", "rfc-0001")
	if err != nil {
		t.Fatalf("list --milestone failed: %v", err)
	}

	if titles := listedTitles(t, stdout.String()); len(titles) != 0 {
		t.Errorf("rfc-0001 has no issues of its own; got %v", titles)
	}
}

// TestChainList_UnknownMilestoneListsNothing — the clearest statement of the
// bug: a filter naming no milestone at all returned every issue.
func TestChainList_UnknownMilestoneListsNothing(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestMilestone(t, app, "alpha", nil)
	createTestIssueWithDeps(t, app, "In alpha", issue.StatePending, "alpha", nil)

	stdout, err := run("--format", "json", "list", "--milestone", "nosuchmilestone")
	if err != nil {
		t.Fatalf("list --milestone failed: %v", err)
	}

	if titles := listedTitles(t, stdout.String()); len(titles) != 0 {
		t.Errorf("expected no issues for an unknown milestone, got %v", titles)
	}
}

// TestChainList_ReadyHonoursMilestoneFilter — --ready narrowed by label but not
// by milestone, and it is the query crochet:execute uses to pick up work.
func TestChainList_ReadyHonoursMilestoneFilter(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestMilestone(t, app, "alpha", nil)
	createTestMilestone(t, app, "alpha-extra", nil)
	createTestIssueWithDeps(t, app, "In alpha", issue.StatePending, "alpha", nil)
	createTestIssueWithDeps(t, app, "In alpha-extra", issue.StatePending, "alpha-extra", nil)

	stdout, err := run("--format", "json", "list", "--ready", "--milestone", "alpha")
	if err != nil {
		t.Fatalf("list --ready --milestone failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "In alpha\"") && !strings.Contains(out, "In alpha") {
		t.Errorf("expected the alpha issue in the ready set, got:\n%s", out)
	}
	if strings.Contains(out, "In alpha-extra") {
		t.Errorf("ready set leaked an issue from alpha-extra:\n%s", out)
	}
}

// TestChainList_HumanOutputHonoursMilestoneFilter — the grouped human view
// re-added any milestone name seen on an issue, so a filtered-out milestone
// came back as its own heading.
func TestChainList_HumanOutputHonoursMilestoneFilter(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestMilestone(t, app, "alpha", nil)
	createTestMilestone(t, app, "alpha-extra", nil)
	createTestIssueWithDeps(t, app, "In alpha", issue.StatePending, "alpha", nil)
	createTestIssueWithDeps(t, app, "In alpha-extra", issue.StatePending, "alpha-extra", nil)

	stdout, err := run("list", "--milestone", "alpha")
	if err != nil {
		t.Fatalf("list --milestone failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "In alpha") {
		t.Errorf("expected the alpha issue, got:\n%s", out)
	}
	if strings.Contains(out, "alpha-extra") {
		t.Errorf("human output leaked the alpha-extra milestone:\n%s", out)
	}
}

// TestChainList_NoFilterStillListsEverything — the narrowing must not become
// the new bug. Without a filter, every active issue is still listed.
func TestChainList_NoFilterStillListsEverything(t *testing.T) {
	app, run := setupChainListTest(t)

	createTestMilestone(t, app, "alpha", nil)
	createTestMilestone(t, app, "beta", nil)
	createTestIssueWithDeps(t, app, "In alpha", issue.StatePending, "alpha", nil)
	createTestIssueWithDeps(t, app, "In beta", issue.StatePending, "beta", nil)

	stdout, err := run("--format", "json", "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if titles := listedTitles(t, stdout.String()); len(titles) != 2 {
		t.Errorf("expected both issues with no filter, got %v", titles)
	}
}
