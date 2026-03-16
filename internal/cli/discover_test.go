// ABOUTME: Tests for external git-zhi-* subcommand discovery on $PATH.
// ABOUTME: Creates temporary executables and verifies they appear as commands.
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/perigrin/git-zhi/internal/cli"
)

func TestDiscoverExternalCommands(t *testing.T) {
	// Create a temp dir with a fake git-zhi-foo executable
	dir := t.TempDir()
	fooPath := filepath.Join(dir, "git-zhi-foo")
	os.WriteFile(fooPath, []byte("#!/bin/sh\necho foo"), 0755)

	// Prepend to PATH
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir+":"+origPath)

	cmds := cli.DiscoverExternalCommands()
	found := false
	for _, cmd := range cmds {
		if cmd.Use == "foo" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected to discover git-zhi-foo as 'foo' subcommand")
	}
}

func TestDiscoverExternalCommands_Empty(t *testing.T) {
	// Empty PATH should return no commands
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", t.TempDir())

	cmds := cli.DiscoverExternalCommands()
	if len(cmds) != 0 {
		t.Fatalf("expected 0 external commands on empty PATH, got %d", len(cmds))
	}
}

func TestDiscoverExternalCommands_SkipsNonExecutable(t *testing.T) {
	dir := t.TempDir()
	// Write a file without executable bit
	barPath := filepath.Join(dir, "git-zhi-bar")
	os.WriteFile(barPath, []byte("#!/bin/sh\necho bar"), 0644)

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir+":"+origPath)

	cmds := cli.DiscoverExternalCommands()
	for _, cmd := range cmds {
		if cmd.Use == "bar" {
			t.Fatal("expected non-executable git-zhi-bar to be skipped")
		}
	}
}

func TestDiscoverExternalCommands_SkipsBuiltins(t *testing.T) {
	dir := t.TempDir()
	// Write executables that shadow built-in command names
	builtins := []string{"issue", "milestone", "list", "config", "next"}
	for _, name := range builtins {
		p := filepath.Join(dir, "git-zhi-"+name)
		os.WriteFile(p, []byte("#!/bin/sh\necho "+name), 0755)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir+":"+origPath)

	cmds := cli.DiscoverExternalCommands()
	for _, cmd := range cmds {
		for _, builtin := range builtins {
			if cmd.Use == builtin {
				t.Fatalf("expected built-in name %q to be skipped by discovery", builtin)
			}
		}
	}
}

func TestDiscoverExternalCommands_FirstPathWins(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	// Place git-zhi-baz in both dirs; the first dir's entry should win
	os.WriteFile(filepath.Join(dir1, "git-zhi-baz"), []byte("#!/bin/sh\necho baz1"), 0755)
	os.WriteFile(filepath.Join(dir2, "git-zhi-baz"), []byte("#!/bin/sh\necho baz2"), 0755)

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir1+":"+dir2)

	cmds := cli.DiscoverExternalCommands()
	count := 0
	for _, cmd := range cmds {
		if cmd.Use == "baz" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 'baz' command (first PATH entry wins), got %d", count)
	}
}

func TestDiscoverExternalCommands_ShortDescriptionContainsBinaryName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "git-zhi-qux"), []byte("#!/bin/sh\necho qux"), 0755)

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", dir+":"+origPath)

	cmds := cli.DiscoverExternalCommands()
	for _, cmd := range cmds {
		if cmd.Use == "qux" {
			if cmd.Short == "" {
				t.Fatal("expected discovered command to have a non-empty Short description")
			}
			return
		}
	}
	t.Fatal("expected to discover git-zhi-qux")
}
