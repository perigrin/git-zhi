// ABOUTME: The update command checks for and installs git-zhi updates.
// ABOUTME: Subcommands: check (check only), rollback (restore backup), config (settings).

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/updater"
)

// NewUpdateCommand creates the 'update' command with subcommands.
func NewUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and install git-zhi updates",
		Long: `Check for available updates and install them automatically.

git-zhi can update itself from GitHub releases. By default it checks the
current version against the latest release and prompts before installing.

Use 'git zhi update check' to inspect available updates without installing,
'git zhi update rollback' to restore a previous version, or
'git zhi update config' to view or change auto-update settings.`,
		RunE: runUpdate,
	}

	cmd.Flags().Bool("yes", false, "install without prompting for confirmation")
	cmd.Flags().Bool("force", false, "install even if already up to date")
	cmd.Flags().Bool("dry-run", false, "show what would happen without making changes")
	cmd.Flags().Bool("include-prerelease", false, "consider pre-release versions")

	cmd.AddCommand(
		newUpdateCheckCommand(),
		newUpdateRollbackCommand(),
		newUpdateConfigCommand(),
	)

	return cmd
}

// runUpdate performs the full update process.
func runUpdate(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	includePrerelease, _ := cmd.Flags().GetBool("include-prerelease")

	opts := updater.DefaultUpdateOptions()
	opts.DryRun = dryRun
	opts.Force = force
	opts.IncludePrerelease = includePrerelease

	// Report progress to stderr so stdout stays clean for piping.
	opts.ProgressCallback = func(stage updater.UpdateStage, message string, progress float64) {
		fmt.Fprintf(cmd.OutOrStderr(), "[%.0f%%] %s\n", progress*100, message)
	}

	u := updater.NewUpdater()
	result, err := u.PerformUpdate(opts)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		data, jsonErr := json.Marshal(result)
		if jsonErr != nil {
			return fmt.Errorf("encoding result: %w", jsonErr)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), result.Message)
	return nil
}

// newUpdateCheckCommand creates the 'update check' subcommand.
func newUpdateCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer version of git-zhi is available",
		Long:  `Queries the GitHub releases API and reports whether a newer version is available.`,
		RunE:  runUpdateCheck,
	}
}

// runUpdateCheck checks for an available update and reports it.
func runUpdateCheck(cmd *cobra.Command, args []string) error {
	opts := updater.DefaultUpdateOptions()

	u := updater.NewUpdater()
	info, err := u.CheckForUpdates(opts)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		type checkResult struct {
			UpdateNeeded   bool   `json:"update_needed"`
			CurrentVersion string `json:"current_version"`
			LatestVersion  string `json:"latest_version,omitempty"`
		}
		out := checkResult{
			UpdateNeeded:   info.UpdateNeeded,
			CurrentVersion: info.CurrentVersion.String(),
		}
		if info.UpdateNeeded {
			out.LatestVersion = info.LatestVersion.String()
		}
		data, jsonErr := json.Marshal(out)
		if jsonErr != nil {
			return fmt.Errorf("encoding result: %w", jsonErr)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	if !info.UpdateNeeded {
		fmt.Fprintf(cmd.OutOrStdout(), "Already up to date (version %s)\n", info.CurrentVersion.String())
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Update available: %s -> %s\n",
		info.CurrentVersion.String(), info.LatestVersion.String())
	return nil
}

// newUpdateRollbackCommand creates the 'update rollback' subcommand.
func newUpdateRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Restore the previous version of git-zhi from a backup",
		Long:  `Restores a previous binary from the most recent backup created during an update.`,
		RunE:  runUpdateRollback,
	}

	cmd.Flags().Bool("list", false, "list available backups instead of rolling back")

	return cmd
}

// runUpdateRollback performs a rollback or lists available backups.
func runUpdateRollback(cmd *cobra.Command, args []string) error {
	listOnly, _ := cmd.Flags().GetBool("list")

	rm := updater.NewRollbackManager()

	if listOnly {
		backups, err := rm.FindAvailableBackups()
		if err != nil {
			return fmt.Errorf("listing backups: %w", err)
		}

		format, _ := cmd.Root().PersistentFlags().GetString("format")
		if format == "json" {
			data, jsonErr := json.Marshal(backups)
			if jsonErr != nil {
				return fmt.Errorf("encoding backups: %w", jsonErr)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		}

		if len(backups) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No backups available.")
			return nil
		}
		for _, b := range backups {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n",
				b.CreatedAt.Format("2006-01-02 15:04:05"), b.BackupPath)
		}
		return nil
	}

	// Perform the actual rollback.
	currentPath, err := updater.GetCurrentBinaryPath()
	if err != nil {
		return fmt.Errorf("detecting current binary: %w", err)
	}

	opts := &updater.RollbackOptions{
		TargetPath:     currentPath,
		ValidateBackup: true,
		DryRun:         false,
	}

	result, err := rm.PerformRollback(opts)
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		data, jsonErr := json.Marshal(result)
		if jsonErr != nil {
			return fmt.Errorf("encoding result: %w", jsonErr)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Rollback successful: restored from %s\n", result.BackupPath)
	return nil
}

// newUpdateConfigCommand creates the 'update config' subcommand.
func newUpdateConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config [key] [value]",
		Short: "View or change auto-update configuration",
		Long: `View or update auto-update settings.

With no arguments, prints the current configuration.
With one argument (a key), prints the value of that setting.
With two arguments (a key and a value), sets the setting.

Available keys: enabled, channel, auto-install, quiet`,
		RunE: runUpdateConfig,
	}
}

// runUpdateConfig views or modifies the auto-update configuration.
func runUpdateConfig(cmd *cobra.Command, args []string) error {
	configPath := updater.DefaultAutoUpdateConfigPath()
	mgr, err := updater.NewAutoUpdateManager(configPath)
	if err != nil {
		return fmt.Errorf("loading update config: %w", err)
	}

	cfg := mgr.GetConfig()
	format, _ := cmd.Root().PersistentFlags().GetString("format")

	// No arguments: show the full config.
	if len(args) == 0 {
		if format == "json" {
			data, jsonErr := json.MarshalIndent(cfg, "", "  ")
			if jsonErr != nil {
				return fmt.Errorf("encoding config: %w", jsonErr)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "enabled:      %v\n", cfg.Enabled)
		fmt.Fprintf(cmd.OutOrStdout(), "channel:      %s\n", cfg.Channel)
		fmt.Fprintf(cmd.OutOrStdout(), "auto-install: %v\n", cfg.AutoInstall)
		fmt.Fprintf(cmd.OutOrStdout(), "quiet:        %v\n", cfg.QuietMode)
		return nil
	}

	key := strings.ToLower(args[0])

	// One argument: show a single key.
	if len(args) == 1 {
		return printConfigKey(cmd, cfg, key, format)
	}

	// Two arguments: set a key.
	value := args[1]
	if err := setConfigKey(cfg, key, value); err != nil {
		return err
	}

	if err := mgr.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, value)
	return nil
}

// printConfigKey prints the value of a single configuration key.
func printConfigKey(cmd *cobra.Command, cfg *updater.AutoUpdateConfig, key, format string) error {
	var value interface{}
	switch key {
	case "enabled":
		value = cfg.Enabled
	case "channel":
		value = cfg.Channel
	case "auto-install":
		value = cfg.AutoInstall
	case "quiet":
		value = cfg.QuietMode
	default:
		return fmt.Errorf("unknown config key %q; valid keys: enabled, channel, auto-install, quiet", key)
	}

	if format == "json" {
		type kv struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		}
		data, jsonErr := json.Marshal(kv{Key: key, Value: value})
		if jsonErr != nil {
			return fmt.Errorf("encoding config key: %w", jsonErr)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s: %v\n", key, value)
	return nil
}

// setConfigKey updates one field of the config by key name.
func setConfigKey(cfg *updater.AutoUpdateConfig, key, value string) error {
	switch key {
	case "enabled":
		v, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for 'enabled': %w", err)
		}
		cfg.Enabled = v
	case "channel":
		ch := updater.UpdateChannel(value)
		if !ch.IsValid() {
			return fmt.Errorf("invalid channel %q; valid channels: stable, beta, alpha", value)
		}
		cfg.Channel = ch
	case "auto-install":
		v, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for 'auto-install': %w", err)
		}
		cfg.AutoInstall = v
	case "quiet":
		v, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for 'quiet': %w", err)
		}
		cfg.QuietMode = v
	default:
		return fmt.Errorf("unknown config key %q; valid keys: enabled, channel, auto-install, quiet", key)
	}
	return nil
}

// parseBool accepts "true"/"1"/"yes" and "false"/"0"/"no".
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("expected true/false, got %q", s)
	}
}

