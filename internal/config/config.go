// ABOUTME: Chain configuration stored at refs/zhi/_/config: version, default
// ABOUTME: milestone and WIP limit, serialized as YAML.
package config

// Config represents the chain configuration.
type Config struct {
	Version          int    `yaml:"version" json:"version"`
	DefaultMilestone string `yaml:"default_milestone" json:"default_milestone"`
	// WIPLimit caps how many issues may be in progress across the chain at
	// once. Zero means no limit, so existing chains are unaffected.
	WIPLimit int `yaml:"wip_limit,omitempty" json:"wip_limit"`
}

// Default returns the default configuration for a new chain.
func Default() *Config {
	return &Config{
		Version:          1,
		DefaultMilestone: "v0.1",
	}
}
