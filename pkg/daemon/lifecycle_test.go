package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon"
)

func TestWriteAndReadPID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")

	// Write PID
	err := daemon.WritePIDFile(pidPath)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Read and verify
	pid, err := daemon.ReadPIDFile(pidPath)
	if err != nil {
		t.Fatalf("ReadPIDFile failed: %v", err)
	}

	if pid != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), pid)
	}
}

func TestIsDaemonRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")

	// No PID file = not running
	if daemon.IsDaemonRunning(pidPath) {
		t.Error("Expected false when PID file doesn't exist")
	}

	// Write current PID = running
	if err := daemon.WritePIDFile(pidPath); err != nil {
		t.Fatal(err)
	}

	if !daemon.IsDaemonRunning(pidPath) {
		t.Error("Expected true when PID file has current process")
	}

	// Write invalid PID = not running
	if err := os.WriteFile(pidPath, []byte("999999999"), 0644); err != nil {
		t.Fatal(err)
	}
	if daemon.IsDaemonRunning(pidPath) {
		t.Error("Expected false when PID is invalid")
	}
}

func TestRemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")

	// Write PID file
	if err := daemon.WritePIDFile(pidPath); err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Fatal("PID file should exist")
	}

	// Remove it
	if err := daemon.RemovePIDFile(pidPath); err != nil {
		t.Fatalf("RemovePIDFile failed: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should have been removed")
	}
}
