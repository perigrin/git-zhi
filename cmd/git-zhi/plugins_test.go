// ABOUTME: Walks the full command tree as main wires it and asserts every
// ABOUTME: group rejects an unknown subcommand instead of exiting zero.

package main

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/cli"
)

// positionalGroups are groups that legitimately take a bare positional, so
// cobra.NoArgs is the wrong tool for them. Each still has to reject an unknown
// word its own way; they are listed explicitly rather than inferred, so adding
// a group is a conscious decision visible in the diff instead of a silent
// exclusion by a heuristic.
var positionalGroups = map[string]bool{
	"git-zhi jira":   true, // jira <ticket-key> — a bare word is a real ticket
	"git-zhi sanbao": true, // sanbao <milestone>
}

// fullTree builds the command tree exactly as main does.
func fullTree() *cobra.Command {
	root := cli.NewRootCommand()
	root.AddCommand(plugins()...)
	return root
}

// commandGroups walks the tree and returns every command that holds
// subcommands, recursing into nested ones.
func commandGroups(root *cobra.Command) []*cobra.Command {
	var groups []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.HasSubCommands() {
			groups = append(groups, c)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return groups
}

// TestUnknownSubcommandIsAnErrorEverywhere checks every group in the real tree.
// A group that prints help and exits 0 makes a typo indistinguishable from
// success to any script driving the CLI.
func TestUnknownSubcommandIsAnErrorEverywhere(t *testing.T) {
	for _, group := range commandGroups(fullTree()) {
		path := group.CommandPath()
		t.Run(path, func(t *testing.T) {
			out := new(bytes.Buffer)
			root := fullTree()
			root.SetOut(out)
			root.SetErr(out)

			args := append(strings.Fields(path)[1:], "definitelynotasubcommand")
			root.SetArgs(args)

			// A group that takes a bare positional cannot tell a typo from a
			// legitimate argument. Validate its Args directly rather than
			// executing: running it would reach the real command, which for
			// jira means a live HTTP request when credentials are configured,
			// and for anything repo-backed means operating on whatever
			// repository the test happens to run in.
			if positionalGroups[path] {
				if group.Args == nil {
					t.Errorf("%s takes positionals but declares no Args validator", path)
				}
				return
			}

			err := root.Execute()
			if err == nil {
				t.Fatalf("%s accepted an unknown subcommand and returned nil", path)
			}
			if !strings.Contains(err.Error(), "definitelynotasubcommand") {
				t.Errorf("error should name the unknown subcommand, got: %v", err)
			}
		})
	}
}

// TestLeafCommandsRejectStrayArguments covers commands with no subcommands.
// cobra's default discards positionals silently for those, so `git zhi sync
// push` ran the default round-trip and exited 0 — the same failure shape as a
// mistyped subcommand, which the group walk cannot see.
func TestLeafCommandsRejectStrayArguments(t *testing.T) {
	// Leaves that legitimately take a positional and validate it themselves.
	takesPositional := map[string]bool{
		"git-zhi issue add": true, "git-zhi issue show": true,
		"git-zhi issue edit": true, "git-zhi issue list": true,
		"git-zhi milestone add": true, "git-zhi milestone show": true,
		"git-zhi milestone edit": true, "git-zhi milestone list": true,
		"git-zhi config": true, "git-zhi jira": true, "git-zhi sanbao": true,
		"git-zhi verify": true, "git-zhi mermaid gantt": true,
		"git-zhi mermaid dag": true, "git-zhi project show": true,
		"git-zhi project next": true, "git-zhi jira sync pull": true,
		"git-zhi jira sync push": true, "git-zhi historian triage": true,
		"git-zhi docs init": true, "git-zhi docs check": true,
		"git-zhi docs health": true, "git-zhi historian": true,
		"git-zhi config reindex": true,
	}

	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		path := c.CommandPath()
		if !c.HasSubCommands() && !takesPositional[path] {
			t.Run(path, func(t *testing.T) {
				if c.Args == nil {
					t.Errorf("%s accepts and silently discards positional arguments; declare cobra.NoArgs", path)
				}
			})
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(fullTree())
}

// TestCommandGroupInventory pins the set of groups. It fails when a group is
// added or removed, so coverage cannot quietly shrink — the previous version
// of this test inferred which commands to skip from their Use string, which
// meant editing a Use string silently dropped a command from the checks.
func TestCommandGroupInventory(t *testing.T) {
	var got []string
	for _, g := range commandGroups(fullTree()) {
		got = append(got, g.CommandPath())
	}
	sort.Strings(got)

	want := []string{
		"git-zhi",
		"git-zhi config",
		"git-zhi docs",
		"git-zhi historian",
		"git-zhi issue",
		"git-zhi jira",
		"git-zhi jira sync",
		"git-zhi mermaid",
		"git-zhi milestone",
		"git-zhi project",
		"git-zhi sanbao",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("command groups changed.\n got: %v\nwant: %v\n\nIf this is intentional, update the list and make sure the new group rejects unknown subcommands.", got, want)
	}
}
