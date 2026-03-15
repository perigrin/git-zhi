package resolve_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/resolve"
)

func TestIsHead(t *testing.T) {
	if !resolve.IsHead("HEAD") {
		t.Fatal("expected 'HEAD' to be recognized as HEAD")
	}
	if !resolve.IsHead("") {
		t.Fatal("expected empty string to be recognized as HEAD (no explicit target defaults to HEAD)")
	}
	if resolve.IsHead("019444a1") {
		t.Fatal("expected UUID prefix not to be recognized as HEAD")
	}
}
