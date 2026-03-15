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
	MPG          float64
	Speed        float64
	BufferTotal  float64
	BufferBurned float64
	TimeInChain  float64
	FeverStatus  Status
}
