// ABOUTME: End-to-end tests for issue add's --label, --after and --before flags.
// ABOUTME: Covers label validation and merging, dependency edges, and flag precedence.
package cli_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// setupAddFlagsTest returns a repo and a runner whose stdin varies per call,
// so a test can create an issue and then feed frontmatter referring to it.
func setupAddFlagsTest(t *testing.T) (*cli.App, func(stdin string, args ...string) (string, error)) {
	t.Helper()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	run := func(stdin string, args ...string) (string, error) {
		out := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	return app, run
}

// issuesByTitle indexes every issue in the chain by title.
func issuesByTitle(t *testing.T, app *cli.App) map[string]*issue.Issue {
	t.Helper()
	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	byTitle := make(map[string]*issue.Issue, len(all))
	for _, iss := range all {
		byTitle[iss.Title] = iss
	}
	return byTitle
}

// TestIssueAdd_Label — a label attaches at creation, so filing a labelled
// issue is one command rather than add followed by edit.
func TestIssueAdd_Label(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Harvested debt", "--body", "found by ponytail",
		"--label", "debt"); err != nil {
		t.Fatalf("issue add --label failed: %v", err)
	}

	iss := issuesByTitle(t, app)["Harvested debt"]
	if iss == nil {
		t.Fatal("issue was not created")
	}
	if got := iss.Labels; len(got) != 1 || got[0] != "debt" {
		t.Errorf("expected labels [debt], got %v", got)
	}
}

// TestIssueAdd_LabelRepeatable — --label may be given more than once.
func TestIssueAdd_LabelRepeatable(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Two labels", "--body", "text",
		"--label", "debt", "--label", "docs"); err != nil {
		t.Fatalf("issue add with repeated --label failed: %v", err)
	}

	iss := issuesByTitle(t, app)["Two labels"]
	if iss == nil {
		t.Fatal("issue was not created")
	}
	if len(iss.Labels) != 2 || iss.Labels[0] != "debt" || iss.Labels[1] != "docs" {
		t.Errorf("expected labels [debt docs], got %v", iss.Labels)
	}
}

// TestIssueAdd_LabelRejectsInvalidName — the flag is held to the same rules as
// issue edit --label, so a name that would break the ref namespace is refused
// at creation rather than producing an unindexable issue.
func TestIssueAdd_LabelRejectsInvalidName(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	_, err := run("", "issue", "add", "Bad label", "--body", "text", "--label", "has/slash")
	if err == nil {
		t.Fatal("expected an error for a label containing '/', got nil")
	}
	// Name the offending label, so "unknown flag: --label" cannot satisfy this.
	if !strings.Contains(err.Error(), "has/slash") {
		t.Errorf("expected the error to name the rejected label, got: %v", err)
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("a rejected label must not leave an issue behind, got %d", len(all))
	}
}

// TestIssueAdd_LabelMergesWithFrontmatter — frontmatter labels and flag labels
// combine, and a label given both ways appears once.
func TestIssueAdd_LabelMergesWithFrontmatter(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	input := "---\ntitle: \"From stdin\"\nlabels: [docs, debt]\n---\n\nBody.\n"
	if _, err := run(input, "issue", "add", "--label", "debt", "--label", "infra"); err != nil {
		t.Fatalf("issue add --label failed: %v", err)
	}

	iss := issuesByTitle(t, app)["From stdin"]
	if iss == nil {
		t.Fatal("issue was not created")
	}
	seen := map[string]int{}
	for _, l := range iss.Labels {
		seen[l]++
	}
	if len(iss.Labels) != 3 {
		t.Fatalf("expected 3 labels after merge, got %v", iss.Labels)
	}
	for _, want := range []string{"docs", "debt", "infra"} {
		if seen[want] != 1 {
			t.Errorf("expected label %q exactly once, got %d in %v", want, seen[want], iss.Labels)
		}
	}
}

// TestIssueAdd_After — --after wires the dependency at creation, so a batch
// with internal ordering no longer needs N follow-up edit calls.
func TestIssueAdd_After(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Upstream", "--body", "first"); err != nil {
		t.Fatalf("creating upstream failed: %v", err)
	}
	upstreamID := issuesByTitle(t, app)["Upstream"].ID

	if _, err := run("", "issue", "add", "Downstream", "--body", "second",
		"--after", upstreamID.String()); err != nil {
		t.Fatalf("issue add --after failed: %v", err)
	}

	byTitle := issuesByTitle(t, app)
	upstream, downstream := byTitle["Upstream"], byTitle["Downstream"]
	if upstream == nil || downstream == nil {
		t.Fatal("expected both issues to exist")
	}

	if len(downstream.BlockedBy) != 1 || downstream.BlockedBy[0] != upstream.ID {
		t.Errorf("expected downstream blocked_by [%s], got %v", upstream.ID, downstream.BlockedBy)
	}
	if len(upstream.Blocks) != 1 || upstream.Blocks[0] != downstream.ID {
		t.Errorf("expected upstream blocks [%s], got %v", downstream.ID, upstream.Blocks)
	}
}

// TestIssueAdd_Before — the reverse edge, same one-command shape.
func TestIssueAdd_Before(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Existing", "--body", "first"); err != nil {
		t.Fatalf("creating existing failed: %v", err)
	}
	existingID := issuesByTitle(t, app)["Existing"].ID

	if _, err := run("", "issue", "add", "Prerequisite", "--body", "second",
		"--before", existingID.String()); err != nil {
		t.Fatalf("issue add --before failed: %v", err)
	}

	byTitle := issuesByTitle(t, app)
	existing, prereq := byTitle["Existing"], byTitle["Prerequisite"]
	if existing == nil || prereq == nil {
		t.Fatal("expected both issues to exist")
	}

	if len(prereq.Blocks) != 1 || prereq.Blocks[0] != existing.ID {
		t.Errorf("expected prerequisite blocks [%s], got %v", existing.ID, prereq.Blocks)
	}
	if len(existing.BlockedBy) != 1 || existing.BlockedBy[0] != prereq.ID {
		t.Errorf("expected existing blocked_by [%s], got %v", prereq.ID, existing.BlockedBy)
	}
}

// TestIssueAdd_AfterUnknownRefIsRefused — an unresolvable ref must fail loudly
// rather than create an issue with no edge, which is the silent no-op the
// "(not yet implemented)" annotation invited.
func TestIssueAdd_AfterUnknownRefIsRefused(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Orphan", "--body", "text",
		"--after", "ffffffff"); err == nil {
		t.Fatal("expected an error for an unresolvable --after ref, got nil")
	}

	all, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		t.Fatalf("LoadAllIssues: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("a failed --after must not leave an issue behind, got %d", len(all))
	}
}

// TestIssueAdd_AfterAppliesToWholeBatch — one flag, every issue in the batch,
// which is the case that made this worth wiring up.
func TestIssueAdd_AfterAppliesToWholeBatch(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Gate", "--body", "must land first"); err != nil {
		t.Fatalf("creating gate failed: %v", err)
	}
	gateID := issuesByTitle(t, app)["Gate"].ID

	batch := "---\ntitle: \"One\"\n---\n\nFirst.\n\n---\ntitle: \"Two\"\n---\n\nSecond.\n"
	if _, err := run(batch, "issue", "add", "--after", gateID.String()); err != nil {
		t.Fatalf("batch issue add --after failed: %v", err)
	}

	byTitle := issuesByTitle(t, app)
	for _, title := range []string{"One", "Two"} {
		iss := byTitle[title]
		if iss == nil {
			t.Fatalf("issue %q was not created", title)
		}
		if len(iss.BlockedBy) != 1 || iss.BlockedBy[0] != gateID {
			t.Errorf("expected %q blocked_by [%s], got %v", title, gateID, iss.BlockedBy)
		}
	}

	// Both forward edges must survive on the shared target. Resolving the ref
	// twice yields two loaded copies unless they are reconciled, and writing
	// them in turn would leave only the last one's edge.
	gate := byTitle["Gate"]
	if gate == nil {
		t.Fatal("Gate disappeared")
	}
	if len(gate.Blocks) != 2 {
		t.Fatalf("expected the gate to block both new issues, got %v", gate.Blocks)
	}
	for _, title := range []string{"One", "Two"} {
		if !slices.Contains(gate.Blocks, byTitle[title].ID) {
			t.Errorf("expected gate.Blocks to contain %q (%s), got %v",
				title, byTitle[title].ID, gate.Blocks)
		}
	}
}

// TestIssueAdd_FrontmatterAfterWinsOverFlag — the flag supplies a default for
// issues that do not state their own dependency, matching how --milestone
// yields to a frontmatter milestone.
func TestIssueAdd_FrontmatterAfterWinsOverFlag(t *testing.T) {
	app, run := setupAddFlagsTest(t)

	if _, err := run("", "issue", "add", "Alpha", "--body", "a"); err != nil {
		t.Fatalf("creating alpha failed: %v", err)
	}
	if _, err := run("", "issue", "add", "Beta", "--body", "b"); err != nil {
		t.Fatalf("creating beta failed: %v", err)
	}
	byTitle := issuesByTitle(t, app)
	alphaID, betaID := byTitle["Alpha"].ID, byTitle["Beta"].ID

	// Frontmatter names Beta, the flag names Alpha. Frontmatter wins.
	input := "---\ntitle: \"Gamma\"\nafter: \"" + betaID.String() + "\"\n---\n\nBody.\n"
	if _, err := run(input, "issue", "add", "--after", alphaID.String()); err != nil {
		t.Fatalf("issue add failed: %v", err)
	}

	gamma := issuesByTitle(t, app)["Gamma"]
	if gamma == nil {
		t.Fatal("Gamma was not created")
	}
	if len(gamma.BlockedBy) != 1 || gamma.BlockedBy[0] != betaID {
		t.Errorf("expected frontmatter to win with blocked_by [%s], got %v", betaID, gamma.BlockedBy)
	}
}

// TestIssueAdd_HelpDoesNotAdvertiseUnimplemented — --help must not describe a
// flag as unimplemented once it works; a skill author reads help as the
// contract.
func TestIssueAdd_HelpDoesNotAdvertiseUnimplemented(t *testing.T) {
	_, run := setupAddFlagsTest(t)

	out, err := run("", "issue", "add", "--help")
	if err != nil {
		t.Fatalf("issue add --help failed: %v", err)
	}
	if strings.Contains(out, "not yet implemented") {
		t.Errorf("help still advertises a flag as unimplemented:\n%s", out)
	}
	for _, flag := range []string{"--after", "--before", "--label"} {
		if !strings.Contains(out, flag) {
			t.Errorf("expected %s in issue add --help, got:\n%s", flag, out)
		}
	}
}
