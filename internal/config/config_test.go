// ABOUTME: Tests for the Config default values: version 1 and
// ABOUTME: default milestone "v0.1".
package config_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()
	if cfg.Version != 1 {
		t.Fatalf("expected version 1, got %d", cfg.Version)
	}
	if cfg.DefaultMilestone != "v0.1" {
		t.Fatalf("expected default milestone %q, got %q", "v0.1", cfg.DefaultMilestone)
	}
}
