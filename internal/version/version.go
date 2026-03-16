// ABOUTME: Version information for git-zhi, set at build time via ldflags.
// ABOUTME: Provides GetVersion, GetBuildInfo, and the version command output.
package version

import (
	"fmt"
	"runtime"
)

// Version is the current version of git-zhi.
// Overridden at build time with ldflags.
var Version = "0.1.0-dev"

// BuildTime is set at build time via ldflags.
var BuildTime = "unknown"

// CommitHash is set at build time via ldflags.
var CommitHash = "unknown"

// GetVersion returns the current version string.
func GetVersion() string {
	return Version
}

// GetBuildInfo returns a formatted string with full build information.
func GetBuildInfo() string {
	return fmt.Sprintf("git-zhi %s (%s/%s)\nbuilt %s\ncommit %s",
		Version, runtime.GOOS, runtime.GOARCH, BuildTime, CommitHash)
}
