package daemon_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon"
)

func TestRecoverFromStaleDaemon_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")
	socketPath := filepath.Join(dir, "sweep.sock")
	dataDir := dir

	// No PID file exists - should return nil (nothing to recover)
	err := daemon.RecoverFromStaleDaemon(pidPath, socketPath, dataDir)
	if err != nil {
		t.Errorf("Expected nil when no PID file exists, got %v", err)
	}
}

func TestRecoverFromStaleDaemon_ProcessRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")
	socketPath := filepath.Join(dir, "sweep.sock")
	dataDir := dir

	// Write current process PID (simulates a running daemon)
	currentPID := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(currentPID)), 0644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Should return ErrDaemonAlreadyRunning since current process is running
	err := daemon.RecoverFromStaleDaemon(pidPath, socketPath, dataDir)
	if !errors.Is(err, daemon.ErrDaemonAlreadyRunning) {
		t.Errorf("Expected ErrDaemonAlreadyRunning when process is running, got %v", err)
	}

	// PID file should NOT be removed
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("PID file should not have been removed when process is running")
	}
}

func TestRecoverFromStaleDaemon_StaleProcess(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")
	socketPath := filepath.Join(dir, "sweep.sock")
	dataDir := dir

	// Create the index.db directory for the BadgerDB lock file
	indexDBDir := filepath.Join(dataDir, "index.db")
	if err := os.MkdirAll(indexDBDir, 0755); err != nil {
		t.Fatalf("Failed to create index.db directory: %v", err)
	}
	lockPath := filepath.Join(indexDBDir, "LOCK")

	// Create stale files - use a PID that definitely doesn't exist
	stalePID := 999999999
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(stalePID)), 0644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}
	if err := os.WriteFile(socketPath, []byte("fake socket"), 0644); err != nil {
		t.Fatalf("Failed to write socket file: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("fake lock"), 0644); err != nil {
		t.Fatalf("Failed to write lock file: %v", err)
	}

	// Verify files exist before recovery
	for _, path := range []string{pidPath, socketPath, lockPath} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatalf("File %s should exist before recovery", path)
		}
	}

	// Should clean up stale files and return nil
	err := daemon.RecoverFromStaleDaemon(pidPath, socketPath, dataDir)
	if err != nil {
		t.Errorf("Expected nil after cleaning up stale daemon, got %v", err)
	}

	// Verify all stale files were removed
	for _, path := range []string{pidPath, socketPath, lockPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("File %s should have been removed after recovery", path)
		}
	}
}

func TestRecoverFromStaleDaemon_PartialStaleFiles(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")
	socketPath := filepath.Join(dir, "sweep.sock")
	dataDir := dir

	// Only create PID file (no socket or lock file)
	stalePID := 999999999
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(stalePID)), 0644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Should succeed even when some files don't exist
	err := daemon.RecoverFromStaleDaemon(pidPath, socketPath, dataDir)
	if err != nil {
		t.Errorf("Expected nil when cleaning up partial stale files, got %v", err)
	}

	// PID file should be removed
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should have been removed")
	}
}

func TestRecoverFromStaleDaemon_InvalidPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sweep.pid")
	socketPath := filepath.Join(dir, "sweep.sock")
	dataDir := dir

	// Write invalid PID content
	if err := os.WriteFile(pidPath, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Should return nil (treat as no valid PID file)
	err := daemon.RecoverFromStaleDaemon(pidPath, socketPath, dataDir)
	if err != nil {
		t.Errorf("Expected nil for invalid PID file, got %v", err)
	}
}

func TestIsProcessRunning(t *testing.T) {
	// Current process should be running
	if !daemon.IsProcessRunning(os.Getpid()) {
		t.Error("Expected current process to be running")
	}

	// Non-existent PID should not be running
	if daemon.IsProcessRunning(999999999) {
		t.Error("Expected non-existent PID to not be running")
	}
}
