package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// StatusFile represents the daemon startup status.
type StatusFile struct {
	Status string `json:"status"`          // "ready" or "error"
	PID    int    `json:"pid,omitempty"`   // Process ID (only for ready status)
	Error  string `json:"error,omitempty"` // Error message (only for error status)
}

// WriteStatusReady writes a ready status file.
func WriteStatusReady(path string) error {
	status := StatusFile{
		Status: "ready",
		PID:    os.Getpid(),
	}
	return writeStatus(path, &status)
}

// WriteStatusError writes an error status file.
func WriteStatusError(path string, err error) error {
	status := StatusFile{
		Status: "error",
		Error:  err.Error(),
	}
	return writeStatus(path, &status)
}

func writeStatus(path string, status *StatusFile) error {
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadStatus reads a status file.
func ReadStatus(path string) (*StatusFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var status StatusFile
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// RemoveStatus removes the status file.
func RemoveStatus(path string) error {
	return os.Remove(path)
}

// StatusPath returns the status file path for a data directory.
func StatusPath(dataDir string) string {
	return filepath.Join(dataDir, "sweep.status")
}
