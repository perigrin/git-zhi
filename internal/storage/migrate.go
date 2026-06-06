// ABOUTME: Migrates stranded worktree-local refs/zhi pointers into the shared namespace.
// ABOUTME: Older versions wrote chain refs under .git/worktrees/<name>/refs/zhi; newer versions read the common dir.

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MigrateLooseRefs walks srcDir (a loose-ref directory such as
// .git/worktrees/<name>/refs/zhi) and, for each ref file whose full ref name
// (refPrefix + "/" + relative path) is absent according to refExists, copies it
// into the shared namespace via setRef. Refs that already exist are left
// untouched. A missing srcDir is a no-op. Returns the number of refs migrated.
//
// Only the ref pointer (a SHA) is copied: the objects it references already
// live in the shared common-dir object store, so no object copy is needed. The
// copy is deliberately non-destructive — the source ref file is left in place.
//
// This handles loose refs only; packed refs are handled by MigratePackedRefs.
// Non-hash content (symbolic refs, lock files, malformed SHAs) is skipped
// rather than promoted.
func MigrateLooseRefs(srcDir, refPrefix string, refExists func(name string) bool, setRef func(name, sha string) error) (int, error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat ref source %s: %w", srcDir, err)
	}
	if !info.IsDir() {
		return 0, nil
	}

	migrated := 0
	walkErr := filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// Build the ref name as refPrefix + "/" + slash-separated relative path.
		refName := refPrefix + "/" + filepath.ToSlash(rel)

		if refExists(refName) {
			return nil
		}

		// Skip lock files and other non-ref detritus a gc/pack run may leave.
		if strings.HasSuffix(path, ".lock") {
			return nil
		}

		sha, err := readLooseRefSHA(path)
		if err != nil {
			return err
		}
		if !isHexSHA(sha) {
			// Symbolic ref, empty file, or malformed content — not a hash
			// pointer to a real object. Skip rather than promote a bogus ref;
			// plumbing.NewHash would silently zero-pad/truncate bad input.
			return nil
		}

		if err := setRef(refName, sha); err != nil {
			return fmt.Errorf("set ref %s: %w", refName, err)
		}
		migrated++
		return nil
	})
	if walkErr != nil {
		return migrated, fmt.Errorf("walk ref source %s: %w", srcDir, walkErr)
	}
	return migrated, nil
}

// MigratePackedRefs parses a git packed-refs file (such as
// .git/worktrees/<name>/packed-refs) and, for each entry whose ref name falls
// under refPrefix and is absent according to refExists, copies it into the
// shared namespace via setRef. A missing file is a no-op. Returns the number of
// refs migrated.
//
// Lines are "<sha> <refname>". Comment lines (#), peeled-tag lines (^<sha>),
// blank lines, non-refPrefix entries, and malformed entries are skipped. As
// with the loose path, only the ref pointer is copied and existing refs are
// left untouched.
func MigratePackedRefs(packedRefsPath, refPrefix string, refExists func(name string) bool, setRef func(name, sha string) error) (int, error) {
	data, err := os.ReadFile(packedRefsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read packed-refs %s: %w", packedRefsPath, err)
	}

	want := refPrefix + "/"
	migrated := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			// Blank, comment header, or peeled-tag continuation line.
			continue
		}

		sha, refName, ok := strings.Cut(line, " ")
		if !ok {
			// No ref name on the line; not a usable entry.
			continue
		}
		refName = strings.TrimSpace(refName)
		if !strings.HasPrefix(refName, want) {
			continue
		}
		if refExists(refName) {
			continue
		}
		if !isHexSHA(sha) {
			continue
		}
		if err := setRef(refName, sha); err != nil {
			return migrated, fmt.Errorf("set ref %s: %w", refName, err)
		}
		migrated++
	}
	return migrated, nil
}

// readLooseRefSHA reads a loose ref file and returns its hex SHA. A symbolic
// ref (starting with "ref:") returns an empty string and no error.
func readLooseRefSHA(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read ref %s: %w", path, err)
	}
	s := strings.TrimSpace(string(data))
	if strings.HasPrefix(s, "ref:") {
		return "", nil
	}
	return s, nil
}

// isHexSHA reports whether s is a well-formed git object name: 40 hex chars
// (SHA-1) or 64 hex chars (SHA-256).
func isHexSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return true
}
