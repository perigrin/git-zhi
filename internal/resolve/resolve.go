// ABOUTME: Ref argument resolution for CLI commands. Resolves user input
// ABOUTME: (HEAD, tag, UUID prefix, title substring) to an entity ref path.
package resolve

// IsHead returns true if the input resolves to the HEAD reference.
// An empty ref argument means the caller passed no explicit target,
// which resolves to HEAD — the current in-progress issue or next on
// the critical chain.
func IsHead(input string) bool {
	return input == "" || input == "HEAD"
}
