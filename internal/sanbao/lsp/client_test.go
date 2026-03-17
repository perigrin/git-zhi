// ABOUTME: Tests for the LSP JSON-RPC client that speaks to language servers.
// ABOUTME: Uses real gopls against temp Go modules — no mocks.
package lsp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/perigrin/git-zhi/internal/sanbao/lsp"
)

// skipIfNoGopls skips the test when gopls is not available on $PATH.
func skipIfNoGopls(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH; skipping LSP integration test")
	}
}

// makeTempGoModule creates a temporary directory with a minimal Go module and
// the provided file contents. The moduleContent map is { "relpath": "content" }.
// Returns the module root directory.
func makeTempGoModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	gomod := `module testmod

go 1.21
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	for relPath, content := range files {
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", relPath, err)
		}
	}

	return dir
}

// TestLSPInit verifies that NewClient spawns gopls, sends initialize, and
// receives capabilities. The client is then shut down cleanly.
func TestLSPInit(t *testing.T) {
	skipIfNoGopls(t)

	dir := makeTempGoModule(t, map[string]string{
		"main.go": `package main

func main() {}
`,
	})

	client, err := lsp.NewClient("gopls", dir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Shutdown: %v", err)
		}
	}()

	// The client must be non-nil after successful initialization.
	if client == nil {
		t.Fatal("expected non-nil client after successful init")
	}
}

// TestLSPDiagnostics verifies that Diagnostics returns diagnostic entries for
// a Go file with a known error (unused import).
func TestLSPDiagnostics(t *testing.T) {
	skipIfNoGopls(t)

	// This file has an unused import — gopls reports it as an error.
	badGo := `package main

import "fmt"

func main() {
	_ = 0
}
`
	dir := makeTempGoModule(t, map[string]string{
		"main.go": badGo,
	})

	client, err := lsp.NewClient("gopls", dir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Shutdown: %v", err)
		}
	}()

	filePath := filepath.Join(dir, "main.go")
	diags, err := client.Diagnostics(filePath)
	if err != nil {
		t.Fatalf("Diagnostics: %v", err)
	}

	// We expect at least one diagnostic for the unused import.
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic for unused import, got none")
	}

	// All diagnostics must reference the correct file.
	for _, d := range diags {
		if d.File != filePath {
			t.Errorf("diagnostic file mismatch: got %q, want %q", d.File, filePath)
		}
		// Severity must be one of the valid values.
		switch d.Severity {
		case "error", "warning", "info", "hint":
			// valid
		default:
			t.Errorf("unexpected severity %q in diagnostic", d.Severity)
		}
		if d.Message == "" {
			t.Error("diagnostic has empty message")
		}
		if d.Line < 1 {
			t.Errorf("diagnostic line must be >= 1, got %d", d.Line)
		}
	}

	// At least one diagnostic must mention "fmt" (the unused import).
	found := false
	for _, d := range diags {
		if containsSubstring(d.Message, "fmt") || containsSubstring(d.Message, "imported") || containsSubstring(d.Message, "unused") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a diagnostic about unused import 'fmt', got: %+v", diags)
	}
}

// TestLSPCleanFile verifies that a valid Go file returns no diagnostics.
func TestLSPCleanFile(t *testing.T) {
	skipIfNoGopls(t)

	cleanGo := `package main

func main() {
}
`
	dir := makeTempGoModule(t, map[string]string{
		"main.go": cleanGo,
	})

	client, err := lsp.NewClient("gopls", dir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Shutdown: %v", err)
		}
	}()

	filePath := filepath.Join(dir, "main.go")
	diags, err := client.Diagnostics(filePath)
	if err != nil {
		t.Fatalf("Diagnostics: %v", err)
	}

	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for clean file, got %d: %+v", len(diags), diags)
	}
}

// TestLSPShutdown verifies that Shutdown sends the LSP shutdown/exit sequence
// and the server process terminates without hanging.
func TestLSPShutdown(t *testing.T) {
	skipIfNoGopls(t)

	dir := makeTempGoModule(t, map[string]string{
		"main.go": `package main

func main() {}
`,
	})

	client, err := lsp.NewClient("gopls", dir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestLSPMissingBinary verifies that NewClient with a non-existent server
// binary returns a clear error and does not panic.
func TestLSPMissingBinary(t *testing.T) {
	dir := t.TempDir()
	client, err := lsp.NewClient("definitely-not-a-real-lsp-server-xyz123", dir)
	if err == nil {
		// Must not succeed — clean up if it somehow did.
		if client != nil {
			_ = client.Shutdown()
		}
		t.Fatal("expected error for missing binary, got nil")
	}
	if client != nil {
		t.Error("expected nil client for missing binary")
	}
}

// containsSubstring is a simple helper to avoid importing strings in tests.
func containsSubstring(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
