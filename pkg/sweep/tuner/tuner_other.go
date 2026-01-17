//go:build !darwin

package tuner

import (
	"runtime"
)

// defaultTotalRAM is the fallback total RAM value when detection fails.
// Set to 8GB as a reasonable default for modern systems.
const defaultTotalRAM = 8 * 1024 * 1024 * 1024

// Detect detects available system resources (CPU and RAM).
// On non-darwin platforms, this uses runtime.NumCPU() for CPU cores
// and falls back to reasonable defaults for memory.
//
// TODO: Implement platform-specific memory detection for linux using
// /proc/meminfo or syscall.Sysinfo.
func Detect() (SystemResources, error) {
	totalRAM := int64(defaultTotalRAM)

	return SystemResources{
		CPUCores:     runtime.NumCPU(),
		TotalRAM:     totalRAM,
		AvailableRAM: totalRAM / 2, // Conservative 50% estimate
	}, nil
}
