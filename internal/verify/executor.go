// ABOUTME: Execute runs a shell command with a timeout and captures its exit code, stdout, and stderr.
// ABOUTME: On timeout the process group is killed and Result.TimedOut is set to true with ExitCode -1.
package verify

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// Result holds the outcome of a single command execution.
type Result struct {
	// Command is the original command string passed to Execute.
	Command string
	// ExitCode is the process exit code. -1 when the process was killed due to timeout.
	ExitCode int
	// Stdout is the captured standard output of the command.
	Stdout string
	// Stderr is the captured standard error of the command.
	Stderr string
	// TimedOut is true when the command was killed because it exceeded the timeout.
	TimedOut bool
	// NoTestsRan is true when the command completed without running any test.
	// A runner given a pattern that matches nothing exits 0, which is
	// indistinguishable from "everything passed" by exit code alone.
	NoTestsRan bool
}

// noTestMarkers are per-item markers a runner prints when a selection matched
// nothing. They are necessary but not sufficient: go emits them per package,
// so a run over several packages prints them alongside genuine results.
var noTestMarkers = []string{
	"[no tests to run]",  // go test -run <pattern> matching nothing in a package
	"[no test files]",    // go test over a package that has no tests at all
	"result: notests",    // prove, when no test file matched
	"no tests ran",       // pytest
	"no tests collected", // pytest, older phrasing
}

// ranTestsMarkers indicate at least one test actually executed. A run is only
// vacuous when a no-test marker appears and none of these do.
//
// These are deliberately specific. Bare "passed"/"failed" would match a
// wrapper's banner — `make test`, or a script ending in `echo "All tests
// passed"` — and suppress detection entirely, restoring the exit-code-only
// behaviour this exists to replace. Acceptance criteria are author-written
// shell strings, so wrappers are the common case, not the exotic one.
var ranTestsMarkers = []string{
	"--- pass", // go verbose
	"--- fail",
	"--- skip",     // a skipped test still ran the binary
	"result: pass", // prove
	"result: fail",
}

// detectNoTestsRan reports whether a completed run verified nothing.
//
// The markers are per package or per file, so matching them against the whole
// output is wrong: `go test ./...` over a repo where some packages have no
// tests prints "[no test files]" for those and "ok" for the rest, and treating
// that as vacuous would fail every full-suite criterion. The question is
// whether any line shows tests running, not whether some line says they did
// not.
func detectNoTestsRan(exitCode int, stdout, stderr string) bool {
	if exitCode != 0 && exitCode != 5 {
		return false
	}
	combined := strings.ToLower(stdout + "\n" + stderr)

	sawNoTests := false
	for _, line := range strings.Split(combined, "\n") {
		for _, marker := range noTestMarkers {
			if strings.Contains(line, marker) {
				sawNoTests = true
				break
			}
		}
		// An "ok <pkg>" line without a no-test marker means that package ran.
		if strings.HasPrefix(line, "ok  ") && !containsAny(line, noTestMarkers) {
			return false
		}
		if containsAny(line, ranTestsMarkers) {
			return false
		}
	}
	return sawNoTests
}

// containsAny reports whether s contains any of the substrings.
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Execute runs cmd via a shell in the given repoRoot directory, enforcing
// a maximum wall-clock duration of timeout. It always returns a populated Result:
//   - On success: ExitCode 0, Stdout/Stderr captured.
//   - On non-zero exit: ExitCode from exec.ExitError, Stdout/Stderr captured.
//   - On timeout: process killed, TimedOut true, ExitCode -1.
//
// On Unix, the command runs in its own process group so timeout handling
// can kill all spawned children, not just the direct child.
func Execute(cmd string, repoRoot string, timeout time.Duration) Result {
	var stdout, stderr bytes.Buffer
	c := newShellCommand(cmd)
	c.Dir = repoRoot
	c.Stdout = &stdout
	c.Stderr = &stderr
	setProcGroup(c)

	if err := c.Start(); err != nil {
		return Result{
			Command:  cmd,
			ExitCode: -1,
			Stderr:   err.Error(),
		}
	}

	// Set up a timer to kill the process when the timeout expires.
	// timedOut is accessed from two goroutines so we use atomic to avoid a race.
	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		killProcessGroup(c)
	})

	err := c.Wait()
	timer.Stop()

	result := Result{
		Command: cmd,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
	}

	if timedOut.Load() {
		result.TimedOut = true
		result.ExitCode = -1
		return result
	}

	if err == nil {
		result.ExitCode = 0
		result.NoTestsRan = detectNoTestsRan(0, result.Stdout, result.Stderr)
		return result
	}

	// Non-zero exit from the process itself.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.NoTestsRan = detectNoTestsRan(result.ExitCode, result.Stdout, result.Stderr)
		return result
	}

	// Any other error (e.g. "sh not found") is also treated as exit code -1.
	result.ExitCode = -1
	return result
}
