// ABOUTME: Milestone domain model for git-chain. Defines the Milestone struct
// ABOUTME: as pure YAML (no markdown body). Milestones group issues for delivery.
package milestone

import "time"

// Milestone represents a delivery grouping of issues with an optional due date.
type Milestone struct {
	Name        string     `yaml:"name" json:"name"`
	Due         *time.Time `yaml:"due,omitempty" json:"due,omitempty"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Resolution  string     `yaml:"resolution,omitempty" json:"resolution,omitempty"`
	Created     time.Time  `yaml:"created" json:"created"`
}
