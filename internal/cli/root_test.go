// ABOUTME: Tests for the Cobra root command: help output, subcommand registration,
// ABOUTME: persistent flag presence, and flag inheritance by subcommands.
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/cli"
)

func TestRootCommand_Help(t *testing.T) {
	cmd := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "refs/zhi/") {
		t.Fatalf("expected help output to contain 'refs/zhi/', got:\n%s", output)
	}
}

func TestRootCommand_HasIssueSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"issue"})
	if sub.Name() != "issue" {
		t.Fatalf("expected subcommand 'issue' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasMilestoneSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"milestone"})
	if sub.Name() != "milestone" {
		t.Fatalf("expected subcommand 'milestone' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasListSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"list"})
	if sub.Name() != "list" {
		t.Fatalf("expected subcommand 'list' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasConfigSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"config"})
	if sub.Name() != "config" {
		t.Fatalf("expected subcommand 'config' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasNextSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"next"})
	if sub.Name() != "next" {
		t.Fatalf("expected subcommand 'next' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasFormatFlag(t *testing.T) {
	cmd := cli.NewRootCommand()
	flag := cmd.PersistentFlags().Lookup("format")
	if flag == nil {
		t.Fatal("expected --format persistent flag on root command")
	}
}

func TestRootCommand_FormatFlagInheritedBySubcommands(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"issue"})
	flag := sub.InheritedFlags().Lookup("format")
	if flag == nil {
		t.Fatal("expected --format flag to be inherited by 'issue' subcommand")
	}
}
