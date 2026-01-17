// Package tuner provides resource detection and optimal configuration calculation
// for the sweep disk analyzer.
package tuner

import (
	"runtime"
	"testing"
)

func TestDetect(t *testing.T) {
	resources, err := Detect()
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	// Verify CPU cores is reasonable
	if resources.CPUCores <= 0 {
		t.Errorf("CPUCores = %d, want > 0", resources.CPUCores)
	}

	// Should match runtime.NumCPU()
	if resources.CPUCores != runtime.NumCPU() {
		t.Errorf("CPUCores = %d, want %d (runtime.NumCPU())", resources.CPUCores, runtime.NumCPU())
	}

	// Verify total RAM is reasonable (at least 512MB)
	minRAM := int64(512 * 1024 * 1024)
	if resources.TotalRAM < minRAM {
		t.Errorf("TotalRAM = %d bytes, want >= %d bytes (512MB)", resources.TotalRAM, minRAM)
	}

	// Available RAM should be positive and <= total
	if resources.AvailableRAM <= 0 {
		t.Errorf("AvailableRAM = %d, want > 0", resources.AvailableRAM)
	}

	if resources.AvailableRAM > resources.TotalRAM {
		t.Errorf("AvailableRAM (%d) > TotalRAM (%d), available should be <= total",
			resources.AvailableRAM, resources.TotalRAM)
	}
}

func TestCalculate(t *testing.T) {
	tests := []struct {
		name      string
		resources SystemResources
		wantMin   OptimalConfig
		wantMax   OptimalConfig
	}{
		{
			name: "small system (2 cores, 4GB RAM)",
			resources: SystemResources{
				CPUCores:     2,
				TotalRAM:     4 * 1024 * 1024 * 1024,
				AvailableRAM: 2 * 1024 * 1024 * 1024,
			},
			wantMin: OptimalConfig{
				DirWorkers:    2, // min is NumCPU
				FileWorkers:   4, // min reasonable for I/O
				DirQueueSize:  100,
				FileQueueSize: 100,
				ResultBuffer:  100,
			},
			wantMax: OptimalConfig{
				DirWorkers:    64,
				FileWorkers:   64,
				DirQueueSize:  100000,
				FileQueueSize: 100000,
				ResultBuffer:  100000,
			},
		},
		{
			name: "medium system (8 cores, 16GB RAM)",
			resources: SystemResources{
				CPUCores:     8,
				TotalRAM:     16 * 1024 * 1024 * 1024,
				AvailableRAM: 8 * 1024 * 1024 * 1024,
			},
			wantMin: OptimalConfig{
				DirWorkers:    8,
				FileWorkers:   16,
				DirQueueSize:  100,
				FileQueueSize: 100,
				ResultBuffer:  100,
			},
			wantMax: OptimalConfig{
				DirWorkers:    64,
				FileWorkers:   64,
				DirQueueSize:  100000,
				FileQueueSize: 100000,
				ResultBuffer:  100000,
			},
		},
		{
			name: "large system (32 cores, 64GB RAM)",
			resources: SystemResources{
				CPUCores:     32,
				TotalRAM:     64 * 1024 * 1024 * 1024,
				AvailableRAM: 32 * 1024 * 1024 * 1024,
			},
			wantMin: OptimalConfig{
				DirWorkers:    32,
				FileWorkers:   64, // capped at max
				DirQueueSize:  100,
				FileQueueSize: 100,
				ResultBuffer:  100,
			},
			wantMax: OptimalConfig{
				DirWorkers:    64,
				FileWorkers:   64,
				DirQueueSize:  100000,
				FileQueueSize: 100000,
				ResultBuffer:  100000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Calculate(tt.resources)

			// Check DirWorkers bounds
			if got.DirWorkers < tt.wantMin.DirWorkers || got.DirWorkers > tt.wantMax.DirWorkers {
				t.Errorf("DirWorkers = %d, want in range [%d, %d]",
					got.DirWorkers, tt.wantMin.DirWorkers, tt.wantMax.DirWorkers)
			}

			// Check FileWorkers bounds
			if got.FileWorkers < tt.wantMin.FileWorkers || got.FileWorkers > tt.wantMax.FileWorkers {
				t.Errorf("FileWorkers = %d, want in range [%d, %d]",
					got.FileWorkers, tt.wantMin.FileWorkers, tt.wantMax.FileWorkers)
			}

			// Check queue sizes are positive and bounded
			if got.DirQueueSize < tt.wantMin.DirQueueSize || got.DirQueueSize > tt.wantMax.DirQueueSize {
				t.Errorf("DirQueueSize = %d, want in range [%d, %d]",
					got.DirQueueSize, tt.wantMin.DirQueueSize, tt.wantMax.DirQueueSize)
			}

			if got.FileQueueSize < tt.wantMin.FileQueueSize || got.FileQueueSize > tt.wantMax.FileQueueSize {
				t.Errorf("FileQueueSize = %d, want in range [%d, %d]",
					got.FileQueueSize, tt.wantMin.FileQueueSize, tt.wantMax.FileQueueSize)
			}

			if got.ResultBuffer < tt.wantMin.ResultBuffer || got.ResultBuffer > tt.wantMax.ResultBuffer {
				t.Errorf("ResultBuffer = %d, want in range [%d, %d]",
					got.ResultBuffer, tt.wantMin.ResultBuffer, tt.wantMax.ResultBuffer)
			}
		})
	}
}

func TestCalculate_WorkerCaps(t *testing.T) {
	// Test that workers are capped at 64
	resources := SystemResources{
		CPUCores:     128,
		TotalRAM:     256 * 1024 * 1024 * 1024,
		AvailableRAM: 128 * 1024 * 1024 * 1024,
	}

	config := Calculate(resources)

	if config.DirWorkers > 64 {
		t.Errorf("DirWorkers = %d, want <= 64 (capped)", config.DirWorkers)
	}

	if config.FileWorkers > 64 {
		t.Errorf("FileWorkers = %d, want <= 64 (capped)", config.FileWorkers)
	}
}

func TestCalculate_DirWorkersMinimum(t *testing.T) {
	// Test that DirWorkers is at least max(NumCPU, 8)
	resources := SystemResources{
		CPUCores:     4,
		TotalRAM:     8 * 1024 * 1024 * 1024,
		AvailableRAM: 4 * 1024 * 1024 * 1024,
	}

	config := Calculate(resources)

	// DirWorkers should be at least 8 (the minimum) even with 4 cores
	if config.DirWorkers < 8 {
		t.Errorf("DirWorkers = %d, want >= 8 (minimum)", config.DirWorkers)
	}
}

func TestCalculateWithOverrides(t *testing.T) {
	resources := SystemResources{
		CPUCores:     8,
		TotalRAM:     16 * 1024 * 1024 * 1024,
		AvailableRAM: 8 * 1024 * 1024 * 1024,
	}

	tests := []struct {
		name            string
		workerOverride  int
		wantDirWorkers  int
		wantFileWorkers int
	}{
		{
			name:            "no override (0)",
			workerOverride:  0,
			wantDirWorkers:  8,  // default calculation
			wantFileWorkers: 32, // NumCPU * 4
		},
		{
			name:            "override with 16",
			workerOverride:  16,
			wantDirWorkers:  16,
			wantFileWorkers: 16,
		},
		{
			name:            "override capped at 64",
			workerOverride:  100,
			wantDirWorkers:  64,
			wantFileWorkers: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateWithOverrides(resources, tt.workerOverride)

			if tt.workerOverride > 0 {
				// With override, both should match (capped)
				expectedWorkers := tt.workerOverride
				if expectedWorkers > 64 {
					expectedWorkers = 64
				}
				if got.DirWorkers != expectedWorkers {
					t.Errorf("DirWorkers = %d, want %d", got.DirWorkers, expectedWorkers)
				}
				if got.FileWorkers != expectedWorkers {
					t.Errorf("FileWorkers = %d, want %d", got.FileWorkers, expectedWorkers)
				}
			}
		})
	}
}

func TestCalculate_Integration(t *testing.T) {
	// Use actual detected resources
	resources, err := Detect()
	if err != nil {
		t.Fatalf("Detect() failed: %v", err)
	}

	config := Calculate(resources)

	// Verify all values are positive
	if config.DirWorkers <= 0 {
		t.Errorf("DirWorkers = %d, want > 0", config.DirWorkers)
	}
	if config.FileWorkers <= 0 {
		t.Errorf("FileWorkers = %d, want > 0", config.FileWorkers)
	}
	if config.DirQueueSize <= 0 {
		t.Errorf("DirQueueSize = %d, want > 0", config.DirQueueSize)
	}
	if config.FileQueueSize <= 0 {
		t.Errorf("FileQueueSize = %d, want > 0", config.FileQueueSize)
	}
	if config.ResultBuffer <= 0 {
		t.Errorf("ResultBuffer = %d, want > 0", config.ResultBuffer)
	}

	// Verify caps are respected
	if config.DirWorkers > 64 {
		t.Errorf("DirWorkers = %d, want <= 64", config.DirWorkers)
	}
	if config.FileWorkers > 64 {
		t.Errorf("FileWorkers = %d, want <= 64", config.FileWorkers)
	}
}
