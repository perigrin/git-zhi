// ABOUTME: Execute runs a shell command with a timeout and captures its exit code, stdout, and stderr.
// ABOUTME: On timeout the process group is killed and Result.TimedOut is set to true with ExitCode -1.
package verify

import (
	"bytes"
	"errors"
	"os/exec"
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
		return result
	}

	// Non-zero exit from the process itself.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result
	}

	// Any other error (e.g. "sh not found") is also treated as exit code -1.
	result.ExitCode = -1
	return result
}
