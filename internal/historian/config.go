// ABOUTME: Historian config persistence: read/write clustering thresholds and weights
// ABOUTME: from refs/zhi/_/historian/config as YAML. Falls back to DefaultConfig when absent.
package historian

import (
	"fmt"

	"github.com/goccy/go-yaml"

	"github.com/perigrin/git-zhi/internal/historian/cluster"
	"github.com/perigrin/git-zhi/internal/storage"
)

// configRef is the ref path where the historian stores its clustering config.
const configRef = "refs/zhi/_/historian/config"

// LoadConfig reads the clustering config from refs/zhi/_/historian/config.
// Returns DefaultConfig when the ref does not exist (first run or never configured).
func LoadConfig(store *storage.Store) (cluster.Config, error) {
	if !store.RefExists(configRef) {
		return cluster.DefaultConfig(), nil
	}
	data, err := store.ReadEntity(configRef, "config.yaml")
	if err != nil {
		return cluster.Config{}, fmt.Errorf("read historian config: %w", err)
	}

	cfg := cluster.DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cluster.Config{}, fmt.Errorf("parse historian config: %w", err)
	}
	return cfg, nil
}

// SaveConfig writes the clustering config to refs/zhi/_/historian/config as YAML.
func SaveConfig(store *storage.Store, cfg cluster.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal historian config: %w", err)
	}
	return store.WriteEntity(configRef, "config.yaml", data, "historian: update config")
}

