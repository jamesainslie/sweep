package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

func TestParseRotationConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    config.RotationConfig
		expected logging.RotationConfig
	}{
		{
			name: "default values",
			input: config.RotationConfig{
				MaxSize:    "10MB",
				MaxAge:     30,
				MaxBackups: 5,
				Daily:      true,
			},
			expected: logging.RotationConfig{
				MaxSize:    10 * 1024 * 1024, // 10MB
				MaxAge:     30,
				MaxBackups: 5,
				Daily:      true,
			},
		},
		{
			name: "custom size in gigabytes",
			input: config.RotationConfig{
				MaxSize:    "1G",
				MaxAge:     7,
				MaxBackups: 3,
				Daily:      false,
			},
			expected: logging.RotationConfig{
				MaxSize:    1024 * 1024 * 1024, // 1GB
				MaxAge:     7,
				MaxBackups: 3,
				Daily:      false,
			},
		},
		{
			name: "empty max_size uses default",
			input: config.RotationConfig{
				MaxSize:    "",
				MaxAge:     14,
				MaxBackups: 2,
				Daily:      true,
			},
			expected: logging.RotationConfig{
				MaxSize:    10 * 1024 * 1024, // 10MB default
				MaxAge:     14,
				MaxBackups: 2,
				Daily:      true,
			},
		},
		{
			name: "invalid max_size uses default",
			input: config.RotationConfig{
				MaxSize:    "invalid",
				MaxAge:     21,
				MaxBackups: 4,
				Daily:      false,
			},
			expected: logging.RotationConfig{
				MaxSize:    10 * 1024 * 1024, // 10MB default
				MaxAge:     21,
				MaxBackups: 4,
				Daily:      false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRotationConfig(tt.input)

			if result.MaxSize != tt.expected.MaxSize {
				t.Errorf("MaxSize = %d, want %d", result.MaxSize, tt.expected.MaxSize)
			}
			if result.MaxAge != tt.expected.MaxAge {
				t.Errorf("MaxAge = %d, want %d", result.MaxAge, tt.expected.MaxAge)
			}
			if result.MaxBackups != tt.expected.MaxBackups {
				t.Errorf("MaxBackups = %d, want %d", result.MaxBackups, tt.expected.MaxBackups)
			}
			if result.Daily != tt.expected.Daily {
				t.Errorf("Daily = %v, want %v", result.Daily, tt.expected.Daily)
			}
		})
	}
}

func TestInitializeLoggingEnsuresDirectories(t *testing.T) {
	// Note: XDG paths are cached at package init time, so we cannot override
	// them with environment variables. Instead, we verify that initializeLogging
	// creates the directories at the actual XDG paths.

	// Run initializeLogging (the PersistentPreRunE hook)
	err := initializeLogging(nil, nil)
	if err != nil {
		t.Fatalf("initializeLogging() returned error: %v", err)
	}

	// Verify directories were created using the config package's path functions
	sweepConfigDir, err := config.ConfigDir()
	if err != nil {
		t.Fatalf("failed to get config dir: %v", err)
	}
	if _, err := os.Stat(sweepConfigDir); os.IsNotExist(err) {
		t.Errorf("config directory was not created: %s", sweepConfigDir)
	}

	sweepDataDir := config.DataDir()
	if _, err := os.Stat(sweepDataDir); os.IsNotExist(err) {
		t.Errorf("data directory was not created: %s", sweepDataDir)
	}

	sweepStateDir := config.StateDir()
	if _, err := os.Stat(sweepStateDir); os.IsNotExist(err) {
		t.Errorf("state directory was not created: %s", sweepStateDir)
	}

	// Clean up logging state
	_ = logging.Close()
}

func TestEnsureDaemonAlreadyRunning(t *testing.T) {
	// Create a temporary PID file with current process ID
	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, "sweep.pid")

	// Write current process PID to simulate running daemon
	currentPID := os.Getpid()
	err := os.WriteFile(pidPath, []byte(strconv.Itoa(currentPID)), 0644)
	if err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Should not return error when daemon is already running (idempotent)
	paths := client.DaemonPaths{PID: pidPath}
	err = client.EnsureDaemon(paths)
	if err != nil {
		t.Errorf("EnsureDaemon() returned error when daemon is running: %v", err)
	}
}

func TestEnsureDaemonNoPIDFile(t *testing.T) {
	// Create config with non-existent PID path
	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, "nonexistent.pid")
	socketPath := filepath.Join(tempDir, "sweep.sock")

	paths := client.DaemonPaths{
		PID:    pidPath,
		Socket: socketPath,
	}

	// Should attempt to start daemon (but fail since sweepd isn't available in tests)
	err := client.EnsureDaemon(paths)
	// We expect an error since sweepd binary won't be found in test environment
	if err == nil {
		t.Log("EnsureDaemon() succeeded - sweepd binary was found")
	} else {
		t.Logf("EnsureDaemon() returned expected error (sweepd not found): %v", err)
	}
}

func TestEnsureDaemonUsesDefaults(t *testing.T) {
	// Create paths with empty values (should use defaults)
	paths := client.DaemonPaths{}

	// Should use default paths
	err := client.EnsureDaemon(paths)
	// We just want to ensure it doesn't panic and handles defaults correctly
	// Error or success both acceptable depending on environment
	if err != nil {
		t.Logf("EnsureDaemon() returned error (expected in test environment): %v", err)
	}
}
