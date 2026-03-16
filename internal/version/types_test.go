// ABOUTME: Tests for version check type definitions and default options.
// ABOUTME: Verifies DefaultCheckOptions returns correct defaults for git-zhi.

package version

import (
	"testing"
	"time"
)

func TestDefaultCheckOptions(t *testing.T) {
	opts := DefaultCheckOptions()

	if opts.Repository != "perigrin/git-zhi" {
		t.Errorf("expected repository %q, got %q", "perigrin/git-zhi", opts.Repository)
	}

	if opts.IncludePrerelease != false {
		t.Errorf("expected IncludePrerelease false, got %v", opts.IncludePrerelease)
	}

	expected := 30 * time.Second
	if opts.Timeout != expected {
		t.Errorf("expected Timeout %v, got %v", expected, opts.Timeout)
	}
}
