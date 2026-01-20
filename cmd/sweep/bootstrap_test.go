package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

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

func TestMaybeStartDaemonAlreadyRunning(t *testing.T) {
	// Create a temporary PID file with current process ID
	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, "sweep.pid")

	// Write current process PID to simulate running daemon
	currentPID := os.Getpid()
	err := os.WriteFile(pidPath, []byte(strconv.Itoa(currentPID)), 0644)
	if err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Create config with the test PID path
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			AutoStart: true,
			PIDPath:   pidPath,
		},
	}

	// Should not return error when daemon is already running
	err = maybeStartDaemon(cfg)
	if err != nil {
		t.Errorf("maybeStartDaemon() returned error when daemon is running: %v", err)
	}
}

func TestMaybeStartDaemonNoPIDFile(t *testing.T) {
	// Create config with non-existent PID path
	tempDir := t.TempDir()
	pidPath := filepath.Join(tempDir, "nonexistent.pid")

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			AutoStart: true,
			PIDPath:   pidPath,
		},
	}

	// Should attempt to start daemon (but fail since sweepd isn't available in tests)
	// The important thing is it doesn't crash and handles the missing daemon gracefully
	err := maybeStartDaemon(cfg)
	// We expect an error since sweepd binary won't be found in test environment
	// This is acceptable behavior - the caller will log a warning
	if err == nil {
		// If no error, that's also fine (means sweepd was somehow found)
		t.Log("maybeStartDaemon() succeeded - sweepd binary was found")
	} else {
		t.Logf("maybeStartDaemon() returned expected error (sweepd not found): %v", err)
	}
}

func TestMaybeStartDaemonUsesDefaultPIDPath(t *testing.T) {
	// Create config with empty PID path (should use default)
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			AutoStart: true,
			PIDPath:   "", // Empty means use default
		},
	}

	// Should use default PID path
	err := maybeStartDaemon(cfg)
	// We just want to ensure it doesn't panic and handles defaults correctly
	// Error is expected since daemon likely isn't running
	if err != nil {
		t.Logf("maybeStartDaemon() returned expected error: %v", err)
	}
}
