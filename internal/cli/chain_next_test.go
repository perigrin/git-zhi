// ABOUTME: Tests for chain next command: verifies it shows the next issue to work on,
// ABOUTME: equivalent to 'issue show HEAD'.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-chain/internal/issue"
)

// TestChainNext shows a pending issue via 'chain next' and verifies
// it returns the same HEAD issue as 'issue show'.
func TestChainNext(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "Next Issue To Work On", issue.StatePending, "Important work.")

	stdout, err := run("next")
	if err != nil {
		t.Fatalf("chain next failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Next Issue To Work On") {
		t.Fatalf("expected issue title in chain next output, got:\n%s", output)
	}
}

// TestChainNext_InProgress verifies chain next returns the in-progress issue
// when one exists, since HEAD resolves to in-progress first.
func TestChainNext_InProgress(t *testing.T) {
	app, run := setupShowTest(t)

	createTestIssue(t, app, "Pending One", issue.StatePending, "")
	createTestIssue(t, app, "Active Work", issue.StateInProgress, "")

	stdout, err := run("next")
	if err != nil {
		t.Fatalf("chain next failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Active Work") {
		t.Fatalf("expected in-progress issue 'Active Work' from chain next, got:\n%s", output)
	}
}
