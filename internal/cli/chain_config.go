// ABOUTME: Implementation of the chain config command: reads and displays current
// ABOUTME: chain configuration, and sets supported keys (default_milestone).
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/config"
)

// runChainConfig reads the current chain config when called with no args,
// or sets a supported config key when called with key and value arguments.
func runChainConfig(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("ensure initialized: %w", err)
	}

	if len(args) == 2 {
		return setChainConfig(cmd, app, args[0], args[1])
	}

	return displayChainConfig(cmd, app)
}

// displayChainConfig reads and renders the current chain configuration.
func displayChainConfig(cmd *cobra.Command, app *App) error {
	data, err := app.Store.ReadEntity("refs/zhi/_/config", "config.yaml")
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	cfg, err := config.UnmarshalConfig(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "version: %d\n", cfg.Version)
	fmt.Fprintf(w, "default_milestone: %s\n", cfg.DefaultMilestone)
	return nil
}

// setChainConfig updates a single supported config key and writes back to storage.
func setChainConfig(cmd *cobra.Command, app *App, key, value string) error {
	data, err := app.Store.ReadEntity("refs/zhi/_/config", "config.yaml")
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	cfg, err := config.UnmarshalConfig(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	switch key {
	case "default_milestone":
		cfg.DefaultMilestone = value
	default:
		return fmt.Errorf("unknown config key %q: supported keys are default_milestone", key)
	}

	newData, err := config.MarshalConfig(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := app.Store.WriteEntity("refs/zhi/_/config", "config.yaml", newData, "Set config "+key+"="+value); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
