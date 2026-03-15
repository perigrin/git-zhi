// ABOUTME: Telemetry computation for development signals (MPG, speed, buffer).
// ABOUTME: Stateless — derives all indicators from issue and milestone data.
package telemetry

// Status represents the fever chart health status of a milestone.
type Status string

const (
	StatusGreen  Status = "GREEN"
	StatusYellow Status = "YELLOW"
	StatusRed    Status = "RED"
)

// Stats holds the derived telemetry indicators for a milestone.
type Stats struct {
	MPG          float64 `yaml:"mpg" json:"mpg"`
	Speed        float64 `yaml:"speed" json:"speed"`
	BufferTotal  float64 `yaml:"buffer_total" json:"buffer_total"`
	BufferBurned float64 `yaml:"buffer_burned" json:"buffer_burned"`
	TimeInChain  float64 `yaml:"time_in_chain" json:"time_in_chain"`
	FeverStatus  Status  `yaml:"fever_status" json:"fever_status"`
}
