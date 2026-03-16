// ABOUTME: Tests for chain config command: read/display current config and
// ABOUTME: set supported keys (default_milestone), verifying persistence.
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/config"
)

// setupChainConfigTest creates a temporary git repo with initialized chain state
// and returns an App plus a run function.
func setupChainConfigTest(t *testing.T) (*cli.App, func(args ...string) (*bytes.Buffer, error)) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo failed: %v", err)
	}
	if err := app.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}

	run := func(args ...string) (*bytes.Buffer, error) {
		stdout := new(bytes.Buffer)
		cmd := cli.NewRootCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		cmd.SetContext(cli.WithApp(context.Background(), app))
		err := cmd.Execute()
		return stdout, err
	}

	return app, run
}

// TestChainConfig_Display verifies that 'chain config' with no args displays
// the current configuration in human-readable format.
func TestChainConfig_Display(t *testing.T) {
	_, run := setupChainConfigTest(t)

	stdout, err := run("config")
	if err != nil {
		t.Fatalf("chain config (display) failed: %v", err)
	}

	output := stdout.String()
	// Default config has version and default_milestone.
	if !strings.Contains(output, "default_milestone") {
		t.Errorf("expected 'default_milestone' in config output, got:\n%s", output)
	}
	// Default milestone is v0.1.
	if !strings.Contains(output, "v0.1") {
		t.Errorf("expected 'v0.1' as default milestone in config output, got:\n%s", output)
	}
}

// TestChainConfig_JsonDisplay verifies that 'chain config --format json' returns
// valid JSON with the config fields.
func TestChainConfig_JsonDisplay(t *testing.T) {
	_, run := setupChainConfigTest(t)

	stdout, err := run("config", "--format", "json")
	if err != nil {
		t.Fatalf("chain config --format json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, stdout.String())
	}

	if _, ok := result["default_milestone"]; !ok {
		t.Fatalf("expected 'default_milestone' in JSON output, got keys: %v", keys(result))
	}
	if _, ok := result["version"]; !ok {
		t.Fatalf("expected 'version' in JSON output, got keys: %v", keys(result))
	}
}

// TestChainConfig_Set verifies that 'chain config default_milestone v0.2' writes
// the new value and a subsequent read reflects it.
func TestChainConfig_Set(t *testing.T) {
	app, run := setupChainConfigTest(t)

	_, err := run("config", "default_milestone", "v0.2")
	if err != nil {
		t.Fatalf("chain config set failed: %v", err)
	}

	// Read back the config directly from storage to verify persistence.
	data, err := app.Store.ReadEntity("refs/zhi/_/config", "config.yaml")
	if err != nil {
		t.Fatalf("ReadEntity config: %v", err)
	}
	cfg, err := config.UnmarshalConfig(data)
	if err != nil {
		t.Fatalf("UnmarshalConfig: %v", err)
	}
	if cfg.DefaultMilestone != "v0.2" {
		t.Errorf("expected default_milestone to be 'v0.2' after set, got %q", cfg.DefaultMilestone)
	}
}

// TestChainConfig_SetUnknownKey verifies that setting an unsupported key returns an error.
func TestChainConfig_SetUnknownKey(t *testing.T) {
	_, run := setupChainConfigTest(t)

	_, err := run("config", "unknown_key", "some_value")
	if err == nil {
		t.Fatal("expected error for unknown config key, got nil")
	}
}
