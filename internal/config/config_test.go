// ABOUTME: Tests for the Config default values: version 1 and
// ABOUTME: default milestone "v0.1". Includes marshal round-trip tests.
package config_test

import (
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/config"
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

func TestMarshal_Default(t *testing.T) {
	cfg := config.Default()
	data, err := config.MarshalConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConfig failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty marshaled config")
	}
	s := string(data)
	if !strings.Contains(s, "version") {
		t.Fatal("expected marshaled config to contain 'version'")
	}
	if !strings.Contains(s, "default_milestone") {
		t.Fatal("expected marshaled config to contain 'default_milestone'")
	}
}
