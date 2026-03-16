// ABOUTME: Discovers external git-zhi-* subcommands on $PATH and registers
// ABOUTME: them as Cobra commands for --help listing and direct invocation.
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// builtinNames is the set of command names built into git-zhi. External
// commands with these names are silently skipped to prevent shadowing.
var builtinNames = map[string]bool{
	"issue":     true,
	"milestone": true,
	"list":      true,
	"config":    true,
	"next":      true,
}

// DiscoverExternalCommands scans $PATH for executables matching git-zhi-*
// and returns Cobra commands that delegate to them. Built-in command names are
// never overridden. The first matching executable on PATH wins for each name.
func DiscoverExternalCommands() []*cobra.Command {
	seen := make(map[string]bool)
	var cmds []*cobra.Command

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range pathDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, "git-zhi-") {
				continue
			}
			subName := strings.TrimPrefix(name, "git-zhi-")
			if subName == "" || seen[subName] || builtinNames[subName] {
				continue
			}
			// Verify the file is executable before registering it.
			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil || info.Mode()&0111 == 0 {
				continue
			}
			seen[subName] = true
			cmdPath := fullPath // capture for closure
			cmds = append(cmds, &cobra.Command{
				Use:   subName,
				Short: "External: " + name,
				// DisableFlagParsing passes all arguments through unchanged so
				// the external command receives its own flags unmodified.
				DisableFlagParsing: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					binary, err := exec.LookPath(cmdPath)
					if err != nil {
						return err
					}
					return syscall.Exec(binary, append([]string{cmdPath}, args...), os.Environ())
				},
			})
		}
	}
	return cmds
}
