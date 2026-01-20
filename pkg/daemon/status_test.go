package daemon_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon"
)

func TestWriteStatusReady(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "sweep.status")

	err := daemon.WriteStatusReady(statusPath)
	if err != nil {
		t.Fatalf("WriteStatusReady failed: %v", err)
	}

	// Read raw file and verify JSON structure
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}

	var status map[string]any
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("Failed to parse status JSON: %v", err)
	}

	if status["status"] != "ready" {
		t.Errorf("Expected status 'ready', got %v", status["status"])
	}

	pid, ok := status["pid"].(float64)
	if !ok {
		t.Fatalf("Expected pid to be a number, got %T", status["pid"])
	}
	if int(pid) != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), int(pid))
	}

	// Error field should not be present
	if _, exists := status["error"]; exists {
		t.Error("Error field should not be present in ready status")
	}
}

func TestWriteStatusError(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "sweep.status")

	testErr := errors.New("daemon startup failed: port already in use")
	err := daemon.WriteStatusError(statusPath, testErr)
	if err != nil {
		t.Fatalf("WriteStatusError failed: %v", err)
	}

	// Read raw file and verify JSON structure
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}

	var status map[string]any
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("Failed to parse status JSON: %v", err)
	}

	if status["status"] != "error" {
		t.Errorf("Expected status 'error', got %v", status["status"])
	}

	if status["error"] != testErr.Error() {
		t.Errorf("Expected error '%s', got %v", testErr.Error(), status["error"])
	}

	// PID field should not be present in error status
	if _, exists := status["pid"]; exists {
		t.Error("PID field should not be present in error status")
	}
}

func TestReadStatus(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "sweep.status")

	t.Run("read ready status", func(t *testing.T) {
		// Write a ready status first
		if err := daemon.WriteStatusReady(statusPath); err != nil {
			t.Fatalf("WriteStatusReady failed: %v", err)
		}

		status, err := daemon.ReadStatus(statusPath)
		if err != nil {
			t.Fatalf("ReadStatus failed: %v", err)
		}

		if status.Status != "ready" {
			t.Errorf("Expected status 'ready', got %s", status.Status)
		}
		if status.PID != os.Getpid() {
			t.Errorf("Expected PID %d, got %d", os.Getpid(), status.PID)
		}
		if status.Error != "" {
			t.Errorf("Expected empty error, got %s", status.Error)
		}
	})

	t.Run("read error status", func(t *testing.T) {
		testErr := errors.New("test error message")
		if err := daemon.WriteStatusError(statusPath, testErr); err != nil {
			t.Fatalf("WriteStatusError failed: %v", err)
		}

		status, err := daemon.ReadStatus(statusPath)
		if err != nil {
			t.Fatalf("ReadStatus failed: %v", err)
		}

		if status.Status != "error" {
			t.Errorf("Expected status 'error', got %s", status.Status)
		}
		if status.Error != testErr.Error() {
			t.Errorf("Expected error '%s', got %s", testErr.Error(), status.Error)
		}
		if status.PID != 0 {
			t.Errorf("Expected PID 0, got %d", status.PID)
		}
	})

	t.Run("read non-existent file", func(t *testing.T) {
		_, err := daemon.ReadStatus(filepath.Join(dir, "nonexistent.status"))
		if err == nil {
			t.Error("Expected error when reading non-existent file")
		}
	})

	t.Run("read invalid JSON", func(t *testing.T) {
		invalidPath := filepath.Join(dir, "invalid.status")
		if err := os.WriteFile(invalidPath, []byte("not json"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := daemon.ReadStatus(invalidPath)
		if err == nil {
			t.Error("Expected error when reading invalid JSON")
		}
	})
}

func TestRemoveStatus(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "sweep.status")

	// Write a status file
	if err := daemon.WriteStatusReady(statusPath); err != nil {
		t.Fatalf("WriteStatusReady failed: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		t.Fatal("Status file should exist")
	}

	// Remove it
	if err := daemon.RemoveStatus(statusPath); err != nil {
		t.Fatalf("RemoveStatus failed: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Error("Status file should have been removed")
	}
}

func TestStatusPath(t *testing.T) {
	dataDir := "/home/user/.sweep"
	expected := "/home/user/.sweep/sweep.status"

	result := daemon.StatusPath(dataDir)
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}
