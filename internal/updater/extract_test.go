// ABOUTME: Tests for archive extraction used when installing update assets.
// ABOUTME: Covers .tar.gz, .zip, and plain binary (no extraction needed) cases.

package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// createFakeBinaryContent returns a byte slice that looks like a binary (>1MB, all 0xFF).
func createFakeBinaryContent(size int) []byte {
	content := make([]byte, size)
	for i := range content {
		content[i] = 0xFF
	}
	return content
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	dir := t.TempDir()

	binaryName := binaryNameInArchive()
	binaryContent := createFakeBinaryContent(minBinarySize)

	// Build a .tar.gz archive containing the binary.
	archivePath := filepath.Join(dir, "git-zhi-1.0.0-linux-amd64.tar.gz")
	if err := makeTarGz(archivePath, binaryName, binaryContent); err != nil {
		t.Fatalf("makeTarGz: %v", err)
	}

	destDir := t.TempDir()
	extractedPath, err := extractBinaryFromArchive(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractBinaryFromArchive(.tar.gz): %v", err)
	}

	got, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("reading extracted binary: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Error("extracted binary content does not match original")
	}

	// Verify the file is executable.
	info, err := os.Stat(extractedPath)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Error("extracted binary is not executable")
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	dir := t.TempDir()

	binaryName := binaryNameInArchive()
	binaryContent := createFakeBinaryContent(minBinarySize)

	// Build a .zip archive containing the binary.
	archivePath := filepath.Join(dir, "git-zhi-1.0.0-windows-amd64.zip")
	if err := makeZip(archivePath, binaryName, binaryContent); err != nil {
		t.Fatalf("makeZip: %v", err)
	}

	destDir := t.TempDir()
	extractedPath, err := extractBinaryFromArchive(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractBinaryFromArchive(.zip): %v", err)
	}

	got, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("reading extracted binary: %v", err)
	}
	if !bytes.Equal(got, binaryContent) {
		t.Error("extracted binary content does not match original")
	}
}

func TestExtractBinaryPlainFile(t *testing.T) {
	dir := t.TempDir()

	// A file without a recognised archive extension should be returned as-is.
	binaryPath := filepath.Join(dir, "git-zhi")
	if err := os.WriteFile(binaryPath, createFakeBinaryContent(minBinarySize), 0o755); err != nil {
		t.Fatalf("writing plain binary: %v", err)
	}

	destDir := t.TempDir()
	result, err := extractBinaryFromArchive(binaryPath, destDir)
	if err != nil {
		t.Fatalf("extractBinaryFromArchive (plain file): %v", err)
	}
	if result != binaryPath {
		t.Errorf("expected plain file to be returned unchanged; got %q, want %q", result, binaryPath)
	}
}

func TestExtractBinaryMissingFromArchive(t *testing.T) {
	dir := t.TempDir()

	// Build a .tar.gz that contains a file with a different name.
	archivePath := filepath.Join(dir, "archive.tar.gz")
	if err := makeTarGz(archivePath, "wrong-name", []byte("content")); err != nil {
		t.Fatalf("makeTarGz: %v", err)
	}

	destDir := t.TempDir()
	_, err := extractBinaryFromArchive(archivePath, destDir)
	if err == nil {
		t.Fatal("expected error when binary name is not found in archive, got nil")
	}
}

// makeTarGz writes a .tar.gz archive at dest containing a single file with name and content.
func makeTarGz(dest, name string, content []byte) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(content)
	return err
}

// makeZip writes a .zip archive at dest containing a single file with name and content.
func makeZip(dest, name string, content []byte) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(content)
	return err
}
