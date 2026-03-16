// ABOUTME: Tests for the Milestone domain model: struct construction
// ABOUTME: with name and description fields. Includes marshal round-trip tests.
package milestone_test

import (
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/milestone"
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

func TestMarshalMilestone(t *testing.T) {
	ms := &milestone.Milestone{
		Name:        "v0.1",
		Description: "Initial implementation",
		Created:     time.Now(),
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty marshaled milestone")
	}
	s := string(data)
	if !strings.Contains(s, "v0.1") {
		t.Fatal("expected marshaled milestone to contain 'v0.1'")
	}
}
