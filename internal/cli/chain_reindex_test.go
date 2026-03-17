// ABOUTME: Tests for 'config reindex' command: rebuilds label indexes and
// ABOUTME: reports how many labels and issues were indexed.
package cli_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/issue"
)

// TestChainConfigReindex_Basic verifies that 'config reindex' succeeds and
// reports the number of labels and issues indexed.
func TestChainConfigReindex_Basic(t *testing.T) {
	app, run := setupChainConfigTest(t)

	// Create some labeled issues.
	createTestIssueWithLabels(t, app, "Task A", issue.StatePending, []string{"frontend"})
	createTestIssueWithLabels(t, app, "Task B", issue.StatePending, []string{"backend"})
	createTestIssueWithLabels(t, app, "Task C", issue.StatePending, []string{"frontend", "backend"})

	stdout, err := run("config", "reindex")
	if err != nil {
		t.Fatalf("config reindex failed: %v", err)
	}

	output := stdout.String()
	// Output must report labels and issues.
	if !strings.Contains(output, "label") {
		t.Errorf("expected 'label' in reindex output, got:\n%s", output)
	}
	if !strings.Contains(output, "issue") {
		t.Errorf("expected 'issue' in reindex output, got:\n%s", output)
	}
}

// TestChainConfigReindex_NoLabels verifies that 'config reindex' with no
// labeled issues reports 0 labels and 0 issues.
func TestChainConfigReindex_NoLabels(t *testing.T) {
	_, run := setupChainConfigTest(t)

	// No labeled issues created.
	stdout, err := run("config", "reindex")
	if err != nil {
		t.Fatalf("config reindex on empty store failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "0") {
		t.Errorf("expected '0' in reindex output when no labels, got:\n%s", output)
	}
}

// TestChainConfigReindex_Idempotent verifies that running 'config reindex'
// twice produces the same result without error.
func TestChainConfigReindex_Idempotent(t *testing.T) {
	app, run := setupChainConfigTest(t)

	createTestIssueWithLabels(t, app, "Idempotent task", issue.StatePending, []string{"ops"})

	if _, err := run("config", "reindex"); err != nil {
		t.Fatalf("first config reindex failed: %v", err)
	}

	stdout, err := run("config", "reindex")
	if err != nil {
		t.Fatalf("second config reindex failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "1") {
		t.Errorf("expected '1' label in second reindex output, got:\n%s", output)
	}
}

// TestChainConfigReindex_IndexesAreQueryable verifies that after 'config reindex',
// LoadLabelIndex returns the expected issue IDs.
func TestChainConfigReindex_IndexesAreQueryable(t *testing.T) {
	app, run := setupChainConfigTest(t)

	id1 := createTestIssueWithLabels(t, app, "Alpha task", issue.StatePending, []string{"ops"})
	id2 := createTestIssueWithLabels(t, app, "Beta task", issue.StatePending, []string{"ops", "infra"})

	if _, err := run("config", "reindex"); err != nil {
		t.Fatalf("config reindex failed: %v", err)
	}

	// After reindex, LoadLabelIndex should find both ops issues.
	opsIDs, err := issue.LoadLabelIndex(app.Store, "ops")
	if err != nil {
		t.Fatalf("LoadLabelIndex(ops): %v", err)
	}
	if len(opsIDs) != 2 {
		t.Fatalf("expected 2 ops issues after reindex, got %d", len(opsIDs))
	}

	// Verify both IDs are present.
	found1, found2 := false, false
	for _, id := range opsIDs {
		if id == id1 {
			found1 = true
		}
		if id == id2 {
			found2 = true
		}
	}
	if !found1 {
		t.Errorf("expected id1 (%s) in ops index, got %v", id1, opsIDs)
	}
	if !found2 {
		t.Errorf("expected id2 (%s) in ops index, got %v", id2, opsIDs)
	}

	// infra index should have only id2.
	infraIDs, err := issue.LoadLabelIndex(app.Store, "infra")
	if err != nil {
		t.Fatalf("LoadLabelIndex(infra): %v", err)
	}
	if len(infraIDs) != 1 || infraIDs[0] != id2 {
		t.Fatalf("expected only id2 (%s) in infra index, got %v", id2, infraIDs)
	}
}
