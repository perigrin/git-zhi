// ABOUTME: Unix-specific process group handling for command execution.
// ABOUTME: Sets Setpgid and uses syscall.Kill to kill the entire process tree on timeout.
//go:build !windows

package verify

import (
	"os/exec"
	"syscall"
)

// newShellCommand creates a command that runs via sh -c on Unix.
func newShellCommand(cmd string) *exec.Cmd {
	return exec.Command("sh", "-c", cmd)
}

// setProcGroup configures the command to run in its own process group so
// the entire tree can be killed on timeout.
func setProcGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group (negative PID = group ID).
func killProcessGroup(c *exec.Cmd) {
	if c.Process != nil {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
