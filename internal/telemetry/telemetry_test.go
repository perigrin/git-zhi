package telemetry_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/telemetry"
)

func TestStatusConstants(t *testing.T) {
	if telemetry.StatusGreen != "GREEN" {
		t.Fatalf("expected %q, got %q", "GREEN", telemetry.StatusGreen)
	}
	if telemetry.StatusYellow != "YELLOW" {
		t.Fatalf("expected %q, got %q", "YELLOW", telemetry.StatusYellow)
	}
	if telemetry.StatusRed != "RED" {
		t.Fatalf("expected %q, got %q", "RED", telemetry.StatusRed)
	}
}

func TestStats_ZeroValue(t *testing.T) {
	s := telemetry.Stats{}
	if s.FeverStatus != "" {
		t.Fatalf("expected zero-value FeverStatus to be empty, got %q", s.FeverStatus)
	}
}
