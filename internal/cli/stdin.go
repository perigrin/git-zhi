// ABOUTME: Shared guard against reading "-" (stdin sentinel) flags when
// ABOUTME: stdin is an interactive terminal, so the read never hangs forever.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rejectInteractiveStdin returns an error if cmd's stdin is a terminal, so
// callers about to read the "-" sentinel for flagName can fail fast instead
// of blocking on io.ReadAll with no data ever arriving. It only recognizes
// *os.File readers (what cmd.InOrStdin() returns by default, i.e. os.Stdin,
// when no test or caller has substituted another reader), so piped or
// programmatically supplied input is never affected.
func rejectInteractiveStdin(cmd *cobra.Command, flagName string) error {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return nil
	}
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	return fmt.Errorf("--%s -: stdin is a character device, not a pipe or file; pipe input instead, e.g. echo \"text\" | git zhi ... --%s -", flagName, flagName)
}
