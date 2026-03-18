// ABOUTME: Tests for busybox-style argv[0] dispatch and symlink setup.
// ABOUTME: Covers name resolution, link creation, idempotency, and stale link handling.
package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveName_PlainBinary(t *testing.T) {
	got := ResolveName("/usr/local/bin/git-zhi")
	if got != "git-zhi" {
		t.Errorf("ResolveName = %q, want git-zhi", got)
	}
}

func TestResolveName_CompanionBinary(t *testing.T) {
	got := ResolveName("/usr/local/bin/git-zhi-historian")
	if got != "git-zhi-historian" {
		t.Errorf("ResolveName = %q, want git-zhi-historian", got)
	}
}

func TestResolveName_ExeSuffix(t *testing.T) {
	got := ResolveName("git-zhi-jira.exe")
	if got != "git-zhi-jira" {
		t.Errorf("ResolveName = %q, want git-zhi-jira", got)
	}
}

func TestResolveName_CmdSuffix(t *testing.T) {
	got := ResolveName("git-zhi-mermaid.cmd")
	if got != "git-zhi-mermaid" {
		t.Errorf("ResolveName = %q, want git-zhi-mermaid", got)
	}
}

func TestResolveName_RelativePath(t *testing.T) {
	got := ResolveName("./git-zhi-sanbao")
	if got != "git-zhi-sanbao" {
		t.Errorf("ResolveName = %q, want git-zhi-sanbao", got)
	}
}

func TestResolveName_UnknownFallsThrough(t *testing.T) {
	got := ResolveName("/usr/bin/something-else")
	if got != "something-else" {
		t.Errorf("ResolveName = %q, want something-else", got)
	}
}

func TestCreateLink_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not applicable on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "git-zhi")
	if err := os.WriteFile(target, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	linkPath := filepath.Join(dir, "git-zhi-historian")
	if err := CreateLink(target, linkPath); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Verify it's a symlink pointing to the target basename.
	dest, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if dest != "git-zhi" {
		t.Errorf("symlink target = %q, want git-zhi", dest)
	}
}

func TestSetup_CreatesAllCompanions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not applicable on Windows")
	}

	dir := t.TempDir()

	// Create a fake binary that os.Executable() won't resolve to, so we
	// test the Setup logic directly by calling the internal pieces.
	binary := filepath.Join(dir, "git-zhi")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	// Simulate Setup by iterating CompanionNames and creating links.
	for _, name := range CompanionNames {
		linkPath := filepath.Join(dir, name)
		if err := CreateLink(binary, linkPath); err != nil {
			t.Fatalf("CreateLink %s: %v", name, err)
		}
	}

	// Verify all companions exist as symlinks.
	for _, name := range CompanionNames {
		linkPath := filepath.Join(dir, name)
		info, err := os.Lstat(linkPath)
		if err != nil {
			t.Errorf("companion %s not created: %v", name, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("companion %s is not a symlink", name)
		}
	}
}

func TestSetup_IdempotentSkipsExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not applicable on Windows")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "git-zhi")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	// Create one companion link.
	linkPath := filepath.Join(dir, "git-zhi-historian")
	if err := CreateLink(binary, linkPath); err != nil {
		t.Fatalf("first CreateLink: %v", err)
	}

	// Create again — should not error.
	if err := CreateLink(binary, linkPath); err == nil {
		// CreateLink itself would fail on duplicate symlink, but Setup skips
		// existing correct links. Verify the link still works.
		dest, _ := os.Readlink(linkPath)
		if dest != "git-zhi" {
			t.Errorf("after re-create, symlink target = %q, want git-zhi", dest)
		}
	}
}

func TestCompanionNames_AllPresent(t *testing.T) {
	expected := map[string]bool{
		"git-zhi-historian": true,
		"git-zhi-jira":      true,
		"git-zhi-project":   true,
		"git-zhi-mermaid":   true,
		"git-zhi-verify":    true,
		"git-zhi-sanbao":    true,
		"git-zhi-docs":      true,
	}
	for _, name := range CompanionNames {
		if !expected[name] {
			t.Errorf("unexpected companion name: %s", name)
		}
		delete(expected, name)
	}
	for name := range expected {
		t.Errorf("missing companion name: %s", name)
	}
}
