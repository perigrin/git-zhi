// ABOUTME: Tests for the update command, which points users at install.sh
// ABOUTME: rather than replacing the running binary in-process.
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

// TestUpdatePrintsInstallCommand verifies that running "update" emits the
// install.sh one-liner and the releases URL, since that is the whole contract.
func TestUpdatePrintsInstallCommand(t *testing.T) {
	var out bytes.Buffer
	cmd := cli.NewUpdateCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("update command returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"install.sh",
		"curl -fsSL",
		"github.com/perigrin/git-zhi/releases",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("update output missing %q\ngot:\n%s", want, got)
		}
	}
}
