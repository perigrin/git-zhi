// ABOUTME: Implementation of the chain next command: shows the next issue to work on
// ABOUTME: by delegating to runIssueShow with no arguments (HEAD resolution).
package cli

import "github.com/spf13/cobra"

// runChainNext delegates to runIssueShow with no args so HEAD is resolved,
// returning the current in-progress issue or the next issue on the critical chain.
func runChainNext(cmd *cobra.Command, args []string) error {
	return runIssueShow(cmd, []string{})
}
