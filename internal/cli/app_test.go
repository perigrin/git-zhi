// ABOUTME: Tests for the App context helpers: nil safety on bare context
// ABOUTME: and round-trip through WithApp/GetApp.
package cli_test

import (
	"context"
	"testing"

	"github.com/perigrin/git-chain/internal/cli"
)

func TestGetApp_NilOnBareContext(t *testing.T) {
	app := cli.GetApp(context.Background())
	if app != nil {
		t.Fatal("expected nil App from bare context")
	}
}

func TestWithApp_RoundTrip(t *testing.T) {
	original := &cli.App{}
	ctx := cli.WithApp(context.Background(), original)
	retrieved := cli.GetApp(ctx)
	if retrieved != original {
		t.Fatal("expected GetApp to return the same App that was set with WithApp")
	}
}
