// ABOUTME: Tests for the HTTP downloader: basic download, checksum, progress, cancellation, cache hits, and error responses.
// ABOUTME: All tests use httptest servers — no mocks, no network.
package download_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/download"
)

// checksum computes the SHA256 hex digest of a byte slice.
func checksum(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestDownload_Basic(t *testing.T) {
	content := []byte("hello git-zhi")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	d := download.NewDownloader()
	dest := filepath.Join(t.TempDir(), "out.bin")

	result, err := d.Download(&download.DownloadOptions{
		URL:             srv.URL + "/file",
		DestinationPath: dest,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.FromCache {
		t.Error("expected FromCache=false for fresh download")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDownload_ChecksumPass(t *testing.T) {
	content := []byte("checksum test data")
	sum := checksum(content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	d := download.NewDownloader()
	dest := filepath.Join(t.TempDir(), "out.bin")

	result, err := d.Download(&download.DownloadOptions{
		URL:              srv.URL + "/file",
		DestinationPath:  dest,
		ExpectedChecksum: sum,
	})
	if err != nil {
		t.Fatalf("expected no error with correct checksum, got: %v", err)
	}
	if result.Checksum != sum {
		t.Errorf("result checksum %q != expected %q", result.Checksum, sum)
	}
}

func TestDownload_ChecksumFail(t *testing.T) {
	content := []byte("checksum test data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	d := download.NewDownloader()
	dest := filepath.Join(t.TempDir(), "out.bin")

	_, err := d.Download(&download.DownloadOptions{
		URL:              srv.URL + "/file",
		DestinationPath:  dest,
		ExpectedChecksum: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("expected error for checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected 'checksum mismatch' in error, got: %v", err)
	}

	// File should be removed after checksum failure
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("expected destination file to be removed after checksum failure")
	}
}

func TestDownload_ProgressCallback(t *testing.T) {
	// Use enough data to make the call non-trivial; the final callback is always fired.
	content := []byte(strings.Repeat("x", 1024))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	d := download.NewDownloader()
	dest := filepath.Join(t.TempDir(), "out.bin")

	var finalCalled bool
	_, err := d.Download(&download.DownloadOptions{
		URL:             srv.URL + "/file",
		DestinationPath: dest,
		ProgressCallback: func(total, transferred int64, done bool) {
			if done {
				finalCalled = true
			}
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalCalled {
		t.Error("expected progress callback to be called with done=true")
	}
}

func TestDownload_ContextCancellation(t *testing.T) {
	// Server that blocks until the client disconnects.
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Stream data slowly so the context cancel hits during transfer.
		for i := 0; i < 100; i++ {
			select {
			case <-unblock:
				return
			default:
				w.Write([]byte(strings.Repeat("y", 1024)))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}))
	defer srv.Close()
	defer close(unblock)

	ctx, cancel := context.WithCancel(context.Background())

	d := download.NewDownloaderWithTimeout(5 * time.Second)
	dest := filepath.Join(t.TempDir(), "out.bin")

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := d.Download(&download.DownloadOptions{
		URL:             srv.URL + "/file",
		DestinationPath: dest,
		Context:         ctx,
	})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestDownload_CacheHit(t *testing.T) {
	content := []byte("cached content")
	sum := checksum(content)

	// Server should not be called if file is already valid.
	serverCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	// Pre-create the file with the exact content.
	if err := os.WriteFile(dest, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	d := download.NewDownloader()
	result, err := d.Download(&download.DownloadOptions{
		URL:              srv.URL + "/file",
		DestinationPath:  dest,
		ExpectedChecksum: sum,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FromCache {
		t.Error("expected FromCache=true for pre-existing valid file")
	}
	if serverCalled {
		t.Error("expected server NOT to be called for a cache hit")
	}
}

func TestDownload_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	d := download.NewDownloader()
	dest := filepath.Join(t.TempDir(), "out.bin")

	_, err := d.Download(&download.DownloadOptions{
		URL:             srv.URL + "/missing",
		DestinationPath: dest,
	})
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected '404' in error, got: %v", err)
	}
}

func TestDownload_UserAgent(t *testing.T) {
	var receivedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	d := download.NewDownloader()
	dest := filepath.Join(t.TempDir(), "out.bin")
	_, err := d.Download(&download.DownloadOptions{
		URL:             srv.URL + "/file",
		DestinationPath: dest,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedUA != "git-zhi-updater/1.0" {
		t.Errorf("expected User-Agent 'git-zhi-updater/1.0', got %q", receivedUA)
	}
}
