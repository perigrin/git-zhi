// ABOUTME: Verifies a bare command group still prints help and succeeds; the
// ABOUTME: unknown-subcommand walk lives in cmd/git-zhi, which sees plugins.

package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/cli"
)

// TestGroupWithNoArgsStillShowsHelp verifies the bare form is unaffected:
// `git zhi issue` should print help and succeed.
func TestGroupWithNoArgsStillShowsHelp(t *testing.T) {
	out := new(bytes.Buffer)
	cmd := cli.NewRootCommand()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"issue"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("bare `issue` should print help and succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "Available Commands") {
		t.Errorf("expected help output, got: %s", out.String())
	}
}
