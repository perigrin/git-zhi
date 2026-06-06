// ABOUTME: Archive extraction for downloaded update assets (.tar.gz and .zip).
// ABOUTME: Extracts the git-zhi binary from release archives before installation.

package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// binaryNameInArchive returns the canonical binary name written to disk after
// extraction (the name git-zhi is invoked by once installed).
func binaryNameInArchive() string {
	if runtime.GOOS == "windows" {
		return "git-zhi.exe"
	}
	return "git-zhi"
}

// isBinaryArchiveMember reports whether an archive member's base name is the
// git-zhi binary. The release workflow packages the binary with a platform
// suffix (git-zhi-<os>-<arch>[.exe]), while a bare git-zhi[.exe] is also
// accepted for archives that ship the canonical name.
func isBinaryArchiveMember(base string) bool {
	if base == binaryNameInArchive() {
		return true
	}
	suffixed := "git-zhi-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		suffixed += ".exe"
	}
	return base == suffixed
}

// extractBinaryFromArchive detects whether archivePath is a .tar.gz or .zip archive
// and extracts the git-zhi binary into destDir. Returns the path to the extracted binary.
// If archivePath is not a recognised archive format, it is returned as-is (plain binary).
func extractBinaryFromArchive(archivePath, destDir string) (string, error) {
	name := strings.ToLower(filepath.Base(archivePath))

	switch {
	case strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz"):
		return extractFromTarGz(archivePath, destDir)
	case strings.HasSuffix(name, ".zip"):
		return extractFromZip(archivePath, destDir)
	default:
		// Not an archive — assume it is a raw binary.
		return archivePath, nil
	}
}

// extractFromTarGz extracts the git-zhi binary from a .tar.gz archive.
func extractFromTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	binaryName := binaryNameInArchive()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar entry: %w", err)
		}

		// Match by base name to handle flat archives or single-directory archives.
		if !isBinaryArchiveMember(filepath.Base(hdr.Name)) {
			continue
		}

		destPath := filepath.Join(destDir, binaryName)
		if err := writeExtractedFile(tr, destPath, hdr.FileInfo().Mode()); err != nil {
			return "", err
		}
		return destPath, nil
	}

	return "", fmt.Errorf("binary %q not found in archive %s", binaryName, archivePath)
}

// extractFromZip extracts the git-zhi binary from a .zip archive.
func extractFromZip(archivePath, destDir string) (string, error) {
	rc, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening zip archive: %w", err)
	}
	defer rc.Close()

	binaryName := binaryNameInArchive()

	for _, f := range rc.File {
		if !isBinaryArchiveMember(filepath.Base(f.Name)) {
			continue
		}

		fr, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("opening zip entry: %w", err)
		}

		destPath := filepath.Join(destDir, binaryName)
		writeErr := writeExtractedFile(fr, destPath, f.Mode())
		fr.Close()
		if writeErr != nil {
			return "", writeErr
		}
		return destPath, nil
	}

	return "", fmt.Errorf("binary %q not found in archive %s", binaryName, archivePath)
}

// writeExtractedFile writes the contents of r to destPath with the given mode.
func writeExtractedFile(r io.Reader, destPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o111)
	if err != nil {
		return fmt.Errorf("creating extracted file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("writing extracted file: %w", err)
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("syncing extracted file: %w", err)
	}

	return nil
}
