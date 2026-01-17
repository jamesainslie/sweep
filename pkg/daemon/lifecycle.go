package daemon

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// WritePIDFile writes the current process ID to a file.
func WritePIDFile(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

// ReadPIDFile reads a PID from a file.
func ReadPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// RemovePIDFile removes the PID file.
func RemovePIDFile(path string) error {
	return os.Remove(path)
}

// IsDaemonRunning checks if a daemon is running based on PID file.
func IsDaemonRunning(pidPath string) bool {
	pid, err := ReadPIDFile(pidPath)
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ErrDaemonAlreadyRunning is returned when trying to start a daemon that's already running.
var ErrDaemonAlreadyRunning = errors.New("daemon already running")
