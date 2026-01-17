// Package tuner provides resource detection and optimal configuration calculation
// for the sweep disk analyzer. It automatically detects system resources like CPU
// cores and RAM, then calculates optimal worker and queue configurations for
// efficient disk scanning.
package tuner

// SystemResources contains detected system resources.
type SystemResources struct {
	// CPUCores is the number of logical CPU cores available.
	CPUCores int

	// TotalRAM is the total physical RAM in bytes.
	TotalRAM int64

	// AvailableRAM is the available (free) RAM in bytes.
	// This may be an estimate based on system heuristics.
	AvailableRAM int64
}
