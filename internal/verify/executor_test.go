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

// TestExecute_NoTestsRan verifies Execute flags runs which completed without
// running any test, and — more importantly — that it does NOT flag runs which
// did. The markers are per package or per file, so a full-suite run mixing
// tested and untested packages must stay a pass; getting that wrong fails
// every real acceptance criterion, which is worse than the vacuous pass this
// detection exists to catch.
func TestExecute_NoTestsRan(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		vacuous bool
	}{
		// genuinely vacuous
		{"go single pkg, pattern matched nothing",
			`echo "ok  	example.com/t	0.5s [no tests to run]"`, true},
		{"go single pkg with no test files",
			`echo "?   	example.com/t	[no test files]"`, true},
		{"prove matched no files",
			`echo "Files=0, Tests=0"; echo "Result: NOTESTS"`, true},
		{"pytest collected nothing",
			`echo "no tests ran in 0.01s"; exit 5`, true},

		// NOT vacuous — these are the false positives that matter
		{"go multi-pkg: some have no test files, others pass",
			`echo "?   	example.com/a	[no test files]"; echo "ok  	example.com/b	0.49s"`, false},
		{"go -run over ./...: matched in one pkg, not others",
			`echo "?   	example.com/a	[no test files]"; echo "ok  	example.com/b	0.20s [no tests to run]"; echo "ok  	example.com/c	0.11s"`, false},
		{"go verbose with a real pass alongside an empty pkg",
			`echo "?   	example.com/a	[no test files]"; echo "--- PASS: TestReal (0.00s)"`, false},
		{"perl Test::More with five failures exits 5",
			`echo "# Looks like you failed 5 tests of 5."; exit 5`, false},
		{"plain passing run", `echo "ok  	example.com/t	0.5s"`, false},
		{"plain failing run", `echo "FAIL"; exit 1`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := verify.Execute(tc.cmd, t.TempDir(), 10*time.Second)
			if result.NoTestsRan != tc.vacuous {
				t.Errorf("NoTestsRan = %v, want %v (exit=%d)\nstdout:\n%s",
					result.NoTestsRan, tc.vacuous, result.ExitCode, result.Stdout)
			}
		})
	}
}

// TestExecute_WrapperBannerDoesNotSuppressDetection verifies a wrapper's
// summary line cannot mask a vacuous run. Acceptance criteria are shell
// strings, so `make test` or a script printing "All tests passed" is the
// normal shape — matching bare "passed" there would silently restore the
// exit-code-only behaviour.
func TestExecute_WrapperBannerDoesNotSuppressDetection(t *testing.T) {
	cmd := `echo "Running test suite..."; echo "?   ex/a	[no test files]"; echo "ok  	ex/b	0.2s [no tests to run]"; echo "All tests passed."`
	result := verify.Execute(cmd, t.TempDir(), 10*time.Second)
	if !result.NoTestsRan {
		t.Errorf("wrapper banner suppressed detection of a vacuous run\nstdout:\n%s", result.Stdout)
	}
}
