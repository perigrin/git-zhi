// ABOUTME: Windows-specific process handling for command execution.
// ABOUTME: Uses cmd.exe and Process.Kill since process groups work differently on Windows.
//go:build windows

package verify

import (
	"os/exec"
)

// newShellCommand creates a command that runs via cmd.exe /C on Windows.
func newShellCommand(cmd string) *exec.Cmd {
	return exec.Command("cmd", "/C", cmd)
}

// setProcGroup is a no-op on Windows — process groups require different
// handling via Job Objects which are not implemented here.
func setProcGroup(c *exec.Cmd) {}

// killProcessGroup kills the process directly on Windows. This does not
// kill child processes spawned by the command.
func killProcessGroup(c *exec.Cmd) {
	if c.Process != nil {
		_ = c.Process.Kill()
	}
}
