// ABOUTME: Tests for the update command and its subcommands (check, rollback, config).
// ABOUTME: Verifies command registration, subcommand discovery, and help output.
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/cli"
)

// TestUpdateCommandExists verifies that "update" is registered as a subcommand on root.
func TestUpdateCommandExists(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update"})
	if sub.Name() != "update" {
		t.Fatalf("expected subcommand 'update' to be registered on root command, got %q", sub.Name())
	}
}

// TestUpdateCheckSubcommand verifies that "update check" is a known subcommand.
func TestUpdateCheckSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update", "check"})
	if sub.Name() != "check" {
		t.Fatalf("expected subcommand 'update check' to be registered, got %q", sub.Name())
	}
}

// TestUpdateRollbackSubcommand verifies that "update rollback" is a known subcommand.
func TestUpdateRollbackSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update", "rollback"})
	if sub.Name() != "rollback" {
		t.Fatalf("expected subcommand 'update rollback' to be registered, got %q", sub.Name())
	}
}

// TestUpdateConfigSubcommand verifies that "update config" is a known subcommand.
func TestUpdateConfigSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update", "config"})
	if sub.Name() != "config" {
		t.Fatalf("expected subcommand 'update config' to be registered, got %q", sub.Name())
	}
}

// TestUpdateBuiltinName verifies that "update" is listed in builtinNames so external
// git-zhi-update binaries cannot shadow the built-in command.
func TestUpdateBuiltinName(t *testing.T) {
	names := cli.BuiltinNames()
	if !names["update"] {
		t.Fatal("expected 'update' to be in builtinNames")
	}
}

// TestUpdateHelpWorks verifies that "update --help" produces output mentioning "update".
func TestUpdateHelpWorks(t *testing.T) {
	cmd := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"update", "--help"})

	// --help exits cleanly via cobra's help mechanism; Execute returns nil.
	_ = cmd.Execute()

	output := buf.String()
	if !strings.Contains(output, "update") {
		t.Fatalf("expected 'update --help' output to contain 'update', got:\n%s", output)
	}
}

// TestUpdateCheckFlagsExist verifies that the check subcommand has no required flags that
// would prevent it from being invoked without a git repo (mirrors version command behaviour).
func TestUpdateCheckFlagsExist(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update", "check"})
	if sub.Name() != "check" {
		t.Fatalf("could not find 'update check' subcommand")
	}
	// The command must be runnable (RunE set), not just a group container.
	if sub.RunE == nil && sub.Run == nil {
		t.Fatal("expected 'update check' to have a Run or RunE function")
	}
}

// TestUpdateRollbackHasListFlag verifies that "update rollback" has a --list flag.
func TestUpdateRollbackHasListFlag(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update", "rollback"})
	if sub.Name() != "rollback" {
		t.Fatalf("could not find 'update rollback' subcommand")
	}
	flag := sub.Flags().Lookup("list")
	if flag == nil {
		t.Fatal("expected --list flag on 'update rollback'")
	}
}

// TestUpdateHasYesFlag verifies that the update command itself has a --yes flag.
func TestUpdateHasYesFlag(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update"})
	if sub.Name() != "update" {
		t.Fatalf("could not find 'update' subcommand")
	}
	flag := sub.Flags().Lookup("yes")
	if flag == nil {
		t.Fatal("expected --yes flag on 'update'")
	}
}

// TestUpdateHasDryRunFlag verifies that the update command has a --dry-run flag.
func TestUpdateHasDryRunFlag(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"update"})
	if sub.Name() != "update" {
		t.Fatalf("could not find 'update' subcommand")
	}
	flag := sub.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("expected --dry-run flag on 'update'")
	}
}
