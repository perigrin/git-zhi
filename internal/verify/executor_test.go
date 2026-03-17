// ABOUTME: Tests for Execute: runs shell commands with timeout and captures exit code, stdout, stderr.
// ABOUTME: Covers success, non-zero exit, and timeout (process killed) scenarios.
package verify_test

import (
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/verify"
)

func TestExecute_Success(t *testing.T) {
	dir := t.TempDir()
	result := verify.Execute("echo hello", dir, 5*time.Second)

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if result.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", result.Stderr)
	}
	if result.TimedOut {
		t.Errorf("TimedOut = true, want false")
	}
	if result.Command != "echo hello" {
		t.Errorf("Command = %q, want %q", result.Command, "echo hello")
	}
}

func TestExecute_Failure(t *testing.T) {
	dir := t.TempDir()
	result := verify.Execute("exit 1", dir, 5*time.Second)

	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
	if result.TimedOut {
		t.Errorf("TimedOut = true, want false")
	}
}

func TestExecute_NonZeroExitCapturesOutput(t *testing.T) {
	dir := t.TempDir()
	// Write to stderr and exit with code 2
	result := verify.Execute("echo oops >&2; exit 2", dir, 5*time.Second)

	if result.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", result.ExitCode)
	}
	if result.Stderr != "oops\n" {
		t.Errorf("Stderr = %q, want %q", result.Stderr, "oops\n")
	}
	if result.TimedOut {
		t.Errorf("TimedOut = true, want false")
	}
}

func TestExecute_Timeout(t *testing.T) {
	dir := t.TempDir()
	result := verify.Execute("sleep 10", dir, 100*time.Millisecond)

	if !result.TimedOut {
		t.Errorf("TimedOut = false, want true")
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 on timeout", result.ExitCode)
	}
}
