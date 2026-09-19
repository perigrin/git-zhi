// ABOUTME: Version information for git-zhi, set at build time via ldflags.
// ABOUTME: Provides the build-info string printed by the version command.
package version

import (
	"fmt"
	"runtime"
)

// Version is the current version of git-zhi, set at build time with ldflags
// by the Makefile. The default applies only to a binary built without them —
// `go build` run directly rather than `make build` — and says so, because a
// plausible-looking number there is indistinguishable from a real release
// and outlives the version it was copied from.
var Version = "unknown (built without make)"

// BuildTime is set at build time via ldflags.
var BuildTime = "unknown"

// CommitHash is set at build time via ldflags.
var CommitHash = "unknown"

// GetBuildInfo returns a formatted string with full build information.
func GetBuildInfo() string {
	return fmt.Sprintf("git-zhi %s (%s/%s)\nbuilt %s\ncommit %s",
		Version, runtime.GOOS, runtime.GOARCH, BuildTime, CommitHash)
}
