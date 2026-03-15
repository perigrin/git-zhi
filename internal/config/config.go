// ABOUTME: Chain configuration stored at refs/chain/_/config. Minimal for v0.1:
// ABOUTME: just version and default_milestone. Read/write via config command.
package config

// Config represents the chain configuration.
type Config struct {
	Version          int    `yaml:"version" json:"version"`
	DefaultMilestone string `yaml:"default_milestone" json:"default_milestone"`
}

// Default returns the default configuration for a new chain.
func Default() *Config {
	return &Config{
		Version:          1,
		DefaultMilestone: "v0.1",
	}
}
