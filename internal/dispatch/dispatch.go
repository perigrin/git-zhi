// ABOUTME: Busybox-style argv[0] name resolution and companion link management.
// ABOUTME: Provides ResolveName, Setup, and CompanionNames for the unified binary.
package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CompanionNames lists all companion binary names that `setup` creates links
// for. Exported so the setup subcommand and tests can reference the same list.
var CompanionNames = []string{
	"git-zhi-historian",
	"git-zhi-jira",
	"git-zhi-project",
	"git-zhi-mermaid",
	"git-zhi-verify",
	"git-zhi-sanbao",
	"git-zhi-docs",
}

// ResolveName extracts the invocation name from os.Args[0], stripping the
// directory, any .exe suffix, and normalizing to a dispatch key.
func ResolveName(argv0 string) string {
	name := filepath.Base(argv0)
	name = strings.TrimSuffix(name, ".exe")
	name = strings.TrimSuffix(name, ".cmd")
	return name
}

// Setup creates symlinks (or hardlinks/wrappers on Windows) for all companion
// names in the same directory as the running binary. Returns the list of
// created link paths and any error.
func Setup() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks: %w", err)
	}

	binDir := filepath.Dir(exe)
	exeName := filepath.Base(exe)
	var created []string

	for _, companion := range CompanionNames {
		linkName := companion
		if runtime.GOOS == "windows" {
			linkName += ".exe"
		}
		linkPath := filepath.Join(binDir, linkName)

		// Check if the link already exists and points to us.
		if target, err := os.Readlink(linkPath); err == nil {
			if filepath.Base(target) == exeName || target == exe {
				continue
			}
			// Stale symlink — remove and recreate.
			os.Remove(linkPath)
		} else if _, err := os.Stat(linkPath); err == nil {
			// File exists but is not a symlink. Check if it's a hardlink to us.
			exeInfo, _ := os.Stat(exe)
			linkInfo, _ := os.Stat(linkPath)
			if os.SameFile(exeInfo, linkInfo) {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: %s exists and is not a link to git-zhi; skipping\n", linkPath)
			continue
		}

		if err := CreateLink(exe, linkPath); err != nil {
			return created, fmt.Errorf("create link %s: %w", linkPath, err)
		}
		created = append(created, linkPath)
	}

	return created, nil
}

// CreateLink creates a symlink on Unix or a hardlink on Windows. Falls back
// to a .cmd wrapper script on Windows if hardlinks fail.
func CreateLink(target, linkPath string) error {
	if runtime.GOOS == "windows" {
		if err := os.Link(target, linkPath); err == nil {
			return nil
		}
		cmdPath := strings.TrimSuffix(linkPath, ".exe") + ".cmd"
		content := fmt.Sprintf("@echo off\n\"%%~dp0%s\" %%*\n", filepath.Base(target))
		return os.WriteFile(cmdPath, []byte(content), 0o755)
	}
	return os.Symlink(filepath.Base(target), linkPath)
}
