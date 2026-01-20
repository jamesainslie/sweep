// Package logging provides a unified logging system with rotation support
// for the sweep disk analyzer. Both CLI/TUI and daemon share this package.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// RotationConfig configures log file rotation behavior.
type RotationConfig struct {
	// MaxSize is the maximum size in bytes before rotation.
	// Zero means no size-based rotation (use default of 10MB).
	MaxSize int64

	// MaxAge is the maximum number of days to retain old log files.
	// Zero means no age-based cleanup.
	MaxAge int

	// MaxBackups is the maximum number of old log files to keep.
	// Zero means keep all old files (subject to MaxAge).
	MaxBackups int

	// Daily rotates the log file daily at midnight.
	Daily bool
}

// DefaultRotationConfig returns sensible defaults for rotation.
func DefaultRotationConfig() RotationConfig {
	return RotationConfig{
		MaxSize:    10 * 1024 * 1024, // 10MB
		MaxAge:     30,               // 30 days
		MaxBackups: 5,
		Daily:      true,
	}
}

// RotatingWriter implements io.WriteCloser with log rotation support.
// It is safe for concurrent use from multiple goroutines and uses
// file locking for safe access from multiple processes.
type RotatingWriter struct {
	path       string
	cfg        RotationConfig
	mu         sync.Mutex
	file       *os.File
	size       int64
	lastRotate time.Time
}

// NewRotatingWriter creates a new rotating writer for the given log path.
// It creates parent directories if they don't exist.
func NewRotatingWriter(path string, cfg RotationConfig) (*RotatingWriter, error) {
	// Apply defaults for zero values
	if cfg.MaxSize == 0 {
		cfg.MaxSize = DefaultRotationConfig().MaxSize
	}

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	w := &RotatingWriter{
		path:       path,
		cfg:        cfg,
		lastRotate: time.Now(),
	}

	if err := w.openFile(); err != nil {
		return nil, err
	}

	// Clean up old files on startup
	w.cleanup()

	return w, nil
}

// Write writes data to the log file with rotation support.
// It acquires a file lock for safe concurrent access from multiple processes.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation is needed
	if w.shouldRotate(int64(len(p))) {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("rotating log file: %w", err)
		}
	}

	// Acquire file lock for safe multi-process access
	if err := w.lock(); err != nil {
		return 0, fmt.Errorf("acquiring file lock: %w", err)
	}
	defer w.unlock()

	n, err := w.file.Write(p)
	if err != nil {
		return n, fmt.Errorf("writing to log file: %w", err)
	}

	w.size += int64(n)
	return n, nil
}

// Close closes the log file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("syncing log file: %w", err)
	}

	err := w.file.Close()
	w.file = nil
	return err
}

// openFile opens or creates the log file.
func (w *RotatingWriter) openFile() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("stat failed: %w; close failed: %w", err, closeErr)
		}
		return fmt.Errorf("stat log file: %w", err)
	}

	w.file = file
	w.size = info.Size()
	w.lastRotate = info.ModTime()

	return nil
}

// shouldRotate checks if the log file should be rotated.
func (w *RotatingWriter) shouldRotate(writeSize int64) bool {
	// Size-based rotation
	if w.size+writeSize > w.cfg.MaxSize {
		return true
	}

	// Daily rotation
	if w.cfg.Daily {
		now := time.Now()
		// Rotate if we crossed midnight since last rotation
		if now.YearDay() != w.lastRotate.YearDay() || now.Year() != w.lastRotate.Year() {
			return true
		}
	}

	return false
}

// rotate rotates the current log file.
func (w *RotatingWriter) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("closing current file: %w", err)
		}
		w.file = nil
	}

	// Generate rotated filename with timestamp
	timestamp := time.Now().Format("2006-01-02-150405")
	ext := filepath.Ext(w.path)
	base := strings.TrimSuffix(w.path, ext)
	rotatedPath := fmt.Sprintf("%s.%s%s", base, timestamp, ext)

	// Rename current file to rotated name
	if _, err := os.Stat(w.path); err == nil {
		if err := os.Rename(w.path, rotatedPath); err != nil {
			return fmt.Errorf("renaming log file: %w", err)
		}
	}

	// Open new log file
	if err := w.openFile(); err != nil {
		return err
	}

	w.lastRotate = time.Now()

	// Clean up old files after rotation
	w.cleanup()

	return nil
}

// cleanup removes old log files based on MaxBackups and MaxAge.
func (w *RotatingWriter) cleanup() {
	dir := filepath.Dir(w.path)
	base := filepath.Base(w.path)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return // ignore cleanup errors
	}

	// Find all rotated log files
	type logFile struct {
		path    string
		modTime time.Time
	}
	var rotated []logFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Match pattern: prefix.timestamp.ext (e.g., sweep.2024-01-20-150405.log)
		if !strings.HasPrefix(name, prefix+".") || !strings.HasSuffix(name, ext) {
			continue
		}

		// Skip the main log file
		if name == base {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		rotated = append(rotated, logFile{
			path:    filepath.Join(dir, name),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time, newest first
	sort.Slice(rotated, func(i, j int) bool {
		return rotated[i].modTime.After(rotated[j].modTime)
	})

	now := time.Now()

	for i, lf := range rotated {
		shouldDelete := false

		// Delete if over MaxBackups (keeping newest)
		if w.cfg.MaxBackups > 0 && i >= w.cfg.MaxBackups {
			shouldDelete = true
		}

		// Delete if over MaxAge
		if w.cfg.MaxAge > 0 {
			age := now.Sub(lf.modTime)
			maxAge := time.Duration(w.cfg.MaxAge) * 24 * time.Hour
			if age > maxAge {
				shouldDelete = true
			}
		}

		if shouldDelete {
			_ = os.Remove(lf.path) // ignore errors during cleanup
		}
	}
}

// lock acquires an exclusive lock on the log file.
func (w *RotatingWriter) lock() error {
	return syscall.Flock(int(w.file.Fd()), syscall.LOCK_EX)
}

// unlock releases the lock on the log file.
func (w *RotatingWriter) unlock() {
	_ = syscall.Flock(int(w.file.Fd()), syscall.LOCK_UN) // ignore unlock errors
}
