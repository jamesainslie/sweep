//go:build darwin

package tuner

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

// Detect detects available system resources (CPU and RAM).
// On darwin (macOS), it uses runtime.NumCPU() for CPU cores and
// unix.SysctlUint64 for memory information.
func Detect() (SystemResources, error) {
	resources := SystemResources{
		CPUCores: runtime.NumCPU(),
	}

	// Get total physical memory using sysctl
	totalRAM, err := getTotalRAM()
	if err != nil {
		return resources, fmt.Errorf("failed to get total RAM: %w", err)
	}
	resources.TotalRAM = totalRAM

	// Get available memory
	availableRAM, err := getAvailableRAM(totalRAM)
	if err != nil {
		return resources, fmt.Errorf("failed to get available RAM: %w", err)
	}
	resources.AvailableRAM = availableRAM

	return resources, nil
}

// getTotalRAM retrieves the total physical memory on darwin using sysctl.
func getTotalRAM() (int64, error) {
	// hw.memsize returns the total physical memory as a 64-bit value
	memsize, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
	}

	return int64(memsize), nil
}

// getAvailableRAM estimates available memory on darwin.
// Since getting precise available memory on macOS requires parsing vm_stat
// or using host_statistics, we use a conservative heuristic based on total RAM.
// This provides a reasonable estimate for queue sizing purposes.
func getAvailableRAM(totalRAM int64) (int64, error) {
	// Heuristic: assume 50% of total RAM is available for our purposes.
	// This is conservative and accounts for:
	// - OS and system processes
	// - File system cache (which macOS uses aggressively)
	// - Other applications
	//
	// For sweep's purposes, this is sufficient since we're sizing queues
	// and buffers, not actually using this much memory.
	availableRAM := totalRAM / 2

	return availableRAM, nil
}
