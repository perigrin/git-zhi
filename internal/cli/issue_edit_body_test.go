// ABOUTME: Tests for 'issue edit --body': stdin body replacement, combined
// ABOUTME: with other flags, and YAML-like body content handling.
package cli_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

func TestIssueEditBody_StdinReplace(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue with body to replace")
	prefix := uuidStr[:8]

	newBody := "Updated acceptance criteria\n\n- [ ] New criterion"
	_, _, err := runWithStdin(app, newBody, "issue", "edit", prefix, "--body")
	if err != nil {
		t.Fatalf("issue edit --body failed: %v", err)
	}

	// Verify body was replaced
	ref := issue.RefPrefix + uuidStr
	content, readErr := app.Store.ReadEntity(ref, "issue.md")
	if readErr != nil {
		t.Fatalf("ReadEntity failed: %v", readErr)
	}
	iss, parseErr := issue.Parse(content)
	if parseErr != nil {
		t.Fatalf("Parse failed: %v", parseErr)
	}
	if !strings.Contains(iss.Body, "Updated acceptance criteria") {
		t.Fatalf("expected body to contain new content, got: %q", iss.Body)
	}
	if !strings.Contains(iss.Body, "New criterion") {
		t.Fatalf("expected body to contain checkbox, got: %q", iss.Body)
	}
}

func TestIssueEditBody_CombinedWithState(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for combined edit")
	prefix := uuidStr[:8]

	newBody := "New body after state change"
	_, _, err := runWithStdin(app, newBody, "issue", "edit", prefix, "--body", "--state", "start")
	if err != nil {
		t.Fatalf("issue edit --body --state start failed: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	content, readErr := app.Store.ReadEntity(ref, "issue.md")
	if readErr != nil {
		t.Fatalf("ReadEntity failed: %v", readErr)
	}
	iss, parseErr := issue.Parse(content)
	if parseErr != nil {
		t.Fatalf("Parse failed: %v", parseErr)
	}
	if iss.State != issue.StateInProgress {
		t.Fatalf("expected state %q, got %q", issue.StateInProgress, iss.State)
	}
	if !strings.Contains(iss.Body, "New body after state change") {
		t.Fatalf("expected new body content, got: %q", iss.Body)
	}
}

func TestIssueEditBody_YAMLLikeContent(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue with tricky body")
	prefix := uuidStr[:8]

	// Body contains YAML frontmatter markers — should be stored as body, not parsed
	newBody := "---\ntitle: \"This is NOT a new issue\"\n---\n\nJust some markdown with dashes."
	_, _, err := runWithStdin(app, newBody, "issue", "edit", prefix, "--body")
	if err != nil {
		t.Fatalf("issue edit --body failed: %v", err)
	}

	ref := issue.RefPrefix + uuidStr
	content, readErr := app.Store.ReadEntity(ref, "issue.md")
	if readErr != nil {
		t.Fatalf("ReadEntity failed: %v", readErr)
	}
	iss, parseErr := issue.Parse(content)
	if parseErr != nil {
		t.Fatalf("Parse failed: %v", parseErr)
	}
	// The YAML-like content should be in the body, not interpreted as frontmatter
	if !strings.Contains(iss.Body, "This is NOT a new issue") {
		t.Fatalf("expected YAML-like content in body, got: %q", iss.Body)
	}
	// Title should be unchanged
	if iss.Title != "Issue with tricky body" {
		t.Fatalf("expected original title, got: %q", iss.Title)
	}
}

func TestIssueEditBody_EmptyStdin(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue should keep body")

	// Set an initial body
	ref := issue.RefPrefix + uuidStr
	content, _ := app.Store.ReadEntity(ref, "issue.md")
	iss, _ := issue.Parse(content)
	originalBody := iss.Body

	prefix := uuidStr[:8]
	// Empty stdin — body should not change (or error gracefully)
	_, _, err := runWithStdin(app, "", "issue", "edit", prefix, "--body")
	// Either succeeds with no change or errors — both acceptable
	if err == nil {
		content2, _ := app.Store.ReadEntity(ref, "issue.md")
		iss2, _ := issue.Parse(content2)
		if iss2.Body != originalBody {
			t.Fatalf("empty stdin should not change body, was %q, now %q", originalBody, iss2.Body)
		}
	}
	_ = fmt.Sprintf("error case also acceptable: %v", err)
}
