package daemon

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

// RecoverFromStaleDaemon checks for and cleans up stale daemon artifacts.
// Returns nil if cleanup succeeded or wasn't needed.
// Returns ErrDaemonAlreadyRunning if a daemon is actually running.
func RecoverFromStaleDaemon(pidPath, socketPath, dataDir string) error {
	pid, err := ReadPIDFile(pidPath)
	if err != nil {
		// No PID file or invalid PID means nothing to recover - this is success, not an error
		return nil //nolint:nilerr // intentional: missing/invalid PID file is not an error condition
	}

	// Check if process is running
	if IsProcessRunning(pid) {
		return ErrDaemonAlreadyRunning
	}

	// Stale daemon - clean up
	log := logging.Get("daemon")
	log.Warn("cleaning up stale daemon files", "stale_pid", pid)

	// Remove stale files (ignore errors - files may not exist)
	_ = os.Remove(pidPath)
	_ = os.Remove(socketPath)
	_ = os.Remove(filepath.Join(dataDir, "index.db", "LOCK"))

	return nil
}

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
