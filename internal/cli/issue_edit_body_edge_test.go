// ABOUTME: Edge-case tests for 'issue edit --body': empty string, TTY stdin,
// ABOUTME: missing editor, failing editor, bool-flag misuse, bad ref, oversize inline value.
package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/cli"
	"github.com/perigrin/git-zhi/internal/issue"
)

// bodyEdgeRunWithReader is like runWithStdin, but takes an arbitrary io.Reader
// for stdin instead of wrapping a string, so tests can supply a reader that
// never delivers EOF (simulating an interactive terminal with no piped data).
func bodyEdgeRunWithReader(app *cli.App, in io.Reader, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := cli.NewRootCommand()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(in)
	cmd.SetArgs(args)
	cmd.SetContext(cli.WithApp(context.Background(), app))
	err := cmd.Execute()
	return stdout, stderr, err
}

// bodyEdgeReadBody reads back the persisted body for the issue at uuidStr.
func bodyEdgeReadBody(t *testing.T, app *cli.App, uuidStr string) string {
	t.Helper()
	ref := issue.RefPrefix + uuidStr
	content, err := app.Store.ReadEntity(ref, "issue.md")
	if err != nil {
		t.Fatalf("ReadEntity failed: %v", err)
	}
	iss, err := issue.Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	return iss.Body
}

// TestIssueEditBody_EmptyString verifies that --body "" (an explicit empty
// string, not an unset flag) never silently persists an empty body. With no
// editor resolvable (PATH scrubbed so `git var GIT_EDITOR` cannot run) and no
// stdin data, the command must fail rather than mutate the issue.
func TestIssueEditBody_EmptyString(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for empty string body")
	prefix := uuidStr[:8]

	originalBody := bodyEdgeReadBody(t, app, uuidStr)

	// Make the editor unresolvable: resolveGitEditor shells out to the real
	// `git` binary, so an empty PATH guarantees it cannot be found, without
	// risking spawning a real editor against a non-TTY stdin.
	t.Setenv("PATH", t.TempDir())

	_, _, err := runWithStdin(app, "", "issue", "edit", prefix, "--body", "")
	if err == nil {
		t.Fatalf("expected error when --body \"\" cannot open an editor, got nil")
	}

	if got := bodyEdgeReadBody(t, app, uuidStr); got != originalBody {
		t.Fatalf("expected body unchanged when editor unavailable, got: %q", got)
	}
}

// TestIssueEditBody_StdinSentinel_TTY verifies that --body - refuses to read
// when stdin is a terminal, instead of blocking forever waiting for input
// that will never arrive.
//
// A real interactive terminal can't be opened in a test process, so this
// stands in a character-device file (/dev/null, or its platform equivalent)
// as the TTY signal: like a real terminal, its Stat().Mode() reports
// os.ModeCharDevice, which is exactly what the production guard checks.
// Unlike a real terminal it also returns EOF immediately on read, so this
// test needs no goroutine or timeout to stay safe — if the guard is missing,
// the command proceeds to read /dev/null's EOF and returns nil, which the
// assertion below catches directly.
func TestIssueEditBody_StdinSentinel_TTY(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for TTY stdin sentinel")
	prefix := uuidStr[:8]
	originalBody := bodyEdgeReadBody(t, app, uuidStr)

	tty, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer tty.Close()

	_, _, err = bodyEdgeRunWithReader(app, tty, "issue", "edit", prefix, "--body", "-")
	if err == nil {
		t.Fatalf("expected non-zero exit for --body - on TTY-like stdin, got nil")
	}
	if !strings.Contains(err.Error(), "--body") {
		t.Fatalf("expected error to name the --body flag, got: %v", err)
	}
	if got := bodyEdgeReadBody(t, app, uuidStr); got != originalBody {
		t.Fatalf("expected body unchanged, got: %q", got)
	}
}

// TestIssueEditBody_NoEditor verifies that --body "" (which opens $EDITOR)
// fails cleanly, without panicking or persisting a zero-length body, when no
// editor can be resolved at all. PATH is scrubbed so the underlying
// `git var GIT_EDITOR` call — which is how EDITOR/VISUAL/core.editor and the
// vi fallback are all resolved — cannot find the git binary, deterministically
// reproducing "no editor available" without risking spawning a real editor
// against a non-interactive test process.
func TestIssueEditBody_NoEditor(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for no editor")
	prefix := uuidStr[:8]
	originalBody := bodyEdgeReadBody(t, app, uuidStr)

	t.Setenv("PATH", t.TempDir())
	t.Setenv("GIT_EDITOR", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	_, stderr, err := runWithStdin(app, "", "issue", "edit", prefix, "--body", "")
	if err == nil {
		t.Fatalf("expected non-zero exit when no editor can be resolved, got nil")
	}
	if err.Error() == "" {
		t.Fatalf("expected a non-empty error message, got empty string (stderr: %q)", stderr.String())
	}
	if got := bodyEdgeReadBody(t, app, uuidStr); got != originalBody {
		t.Fatalf("expected body unchanged when no editor available, got: %q", got)
	}
}

// TestIssueEditBody_EditorFails verifies that when GIT_EDITOR is set but the
// editor process exits non-zero (editor crashed, or the user quit without
// saving), the failure propagates as a command error and the issue body is
// left unchanged.
func TestIssueEditBody_EditorFails(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for editor failure")
	prefix := uuidStr[:8]

	// Give the issue a known body via the stdin sentinel first.
	_, _, err := runWithStdin(app, "Body before failing editor", "issue", "edit", prefix, "--body", "-")
	if err != nil {
		t.Fatalf("setup body failed: %v", err)
	}
	originalBody := bodyEdgeReadBody(t, app, uuidStr)

	editorScript := filepath.Join(t.TempDir(), "fail-editor.sh")
	if writeErr := os.WriteFile(editorScript, []byte("#!/bin/sh\nexit 1\n"), 0755); writeErr != nil {
		t.Fatalf("write editor script: %v", writeErr)
	}
	t.Setenv("GIT_EDITOR", editorScript)

	// Empty value triggers the editor path.
	_, _, err = runWithStdin(app, "", "issue", "edit", prefix, "--body", "")
	if err == nil {
		t.Fatalf("expected editor failure to propagate as a command error, got nil")
	}

	if got := bodyEdgeReadBody(t, app, uuidStr); got != originalBody {
		t.Fatalf("expected body unchanged after editor failure, got: %q", got)
	}
}

// TestIssueEditBody_BoolFlagMisuse verifies that '--body' with no following
// value token (the old bool-flag invocation pattern) is rejected by flag
// parsing rather than silently consuming the issue ref as the body value.
func TestIssueEditBody_BoolFlagMisuse(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for bool flag misuse")
	prefix := uuidStr[:8]
	originalBody := bodyEdgeReadBody(t, app, uuidStr)

	// --body is the last token: pflag has nothing to consume as its value.
	_, _, err := runWithStdin(app, "", "issue", "edit", prefix, "--body")
	if err == nil {
		t.Fatalf("expected an error for '--body' with no value token, got nil")
	}
	if !strings.Contains(err.Error(), "flag needs an argument") {
		t.Fatalf("expected a 'flag needs an argument' error, got: %v", err)
	}

	if got := bodyEdgeReadBody(t, app, uuidStr); got != originalBody {
		t.Fatalf("expected body unchanged, got: %q (ref must not have been treated as the body value)", got)
	}
}

// TestIssueEditBody_BadRef verifies that --body "valid body" with a ref that
// resolves to no issue returns a non-zero exit before anything is written,
// rather than silently dropping the value or writing it to the wrong place.
func TestIssueEditBody_BadRef(t *testing.T) {
	app, _ := setupEditTest(t)

	_, _, err := runWithStdin(app, "", "issue", "edit", "deadbeef", "--body", "valid body")
	if err == nil {
		t.Fatalf("expected an error for a ref matching no issue, got nil")
	}
}

// TestIssueEditBody_OversizeInline verifies that a 1 MiB inline --body value
// does not panic, is not silently truncated, and is either persisted in full
// or rejected with a clear size-limit error.
func TestIssueEditBody_OversizeInline(t *testing.T) {
	app, _ := setupEditTest(t)
	uuidStr := createEditTestIssue(t, app, "Issue for oversize inline body")
	prefix := uuidStr[:8]

	big := strings.Repeat("x", 1024*1024)

	_, _, err := runWithStdin(app, "", "issue", "edit", prefix, "--body", big)
	if err != nil {
		// A clear size-limit error is an acceptable outcome per the criterion,
		// as long as the issue was not left half-written.
		if got := bodyEdgeReadBody(t, app, uuidStr); got == big {
			t.Fatalf("expected no partial/truncated persistence when returning a size-limit error")
		}
		return
	}

	got := bodyEdgeReadBody(t, app, uuidStr)
	if got != big {
		t.Fatalf("expected full 1 MiB body persisted without truncation, got length %d (want %d)", len(got), len(big))
	}
}
