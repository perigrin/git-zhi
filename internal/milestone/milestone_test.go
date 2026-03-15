// ABOUTME: Tests for the Milestone domain model: struct construction
// ABOUTME: with name and description fields.
package milestone_test

import (
	"testing"
	"time"

	"github.com/perigrin/git-chain/internal/milestone"
)

func TestNewMilestone(t *testing.T) {
	ms := &milestone.Milestone{
		Name:        "v0.1",
		Description: "Initial parser implementation",
		Created:     time.Now(),
	}

	if ms.Name != "v0.1" {
		t.Fatalf("expected name %q, got %q", "v0.1", ms.Name)
	}
}
