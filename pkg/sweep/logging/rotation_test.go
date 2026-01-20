package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

func TestRotationBySize(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "size_rotate.log")

	writer, err := logging.NewRotatingWriter(logPath, logging.RotationConfig{
		MaxSize:    512, // 512 bytes - small for testing
		MaxAge:     7,
		MaxBackups: 3,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}

	// Write enough to trigger rotation (~100 bytes per message * 20 = ~2KB)
	for i := 0; i < 20; i++ {
		msg := strings.Repeat("x", 50) + "\n"
		if _, writeErr := writer.Write([]byte(msg)); writeErr != nil {
			t.Fatalf("Write() error = %v", writeErr)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Check that rotation happened
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	logFiles := 0
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "size_rotate") && strings.HasSuffix(f.Name(), ".log") {
			logFiles++
		}
	}

	if logFiles < 2 {
		t.Errorf("expected at least 2 log files after rotation, got %d", logFiles)
	}
}

func TestRotationMaxBackups(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "backup_limit.log")

	maxBackups := 2
	writer, err := logging.NewRotatingWriter(logPath, logging.RotationConfig{
		MaxSize:    256, // 256 bytes - very small for testing
		MaxAge:     7,
		MaxBackups: maxBackups,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}

	// Write enough to trigger multiple rotations
	for i := 0; i < 50; i++ {
		msg := strings.Repeat("y", 30) + "\n"
		if _, writeErr := writer.Write([]byte(msg)); writeErr != nil {
			t.Fatalf("Write() error = %v", writeErr)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	logFiles := 0
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "backup_limit") {
			logFiles++
		}
	}

	// Should have current + MaxBackups files at most
	maxExpected := maxBackups + 1
	if logFiles > maxExpected {
		t.Errorf("expected at most %d log files, got %d", maxExpected, logFiles)
	}
}

func TestRotationConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     logging.RotationConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: logging.RotationConfig{
				MaxSize:    10 * 1024 * 1024, // 10MB
				MaxAge:     30,
				MaxBackups: 5,
				Daily:      true,
			},
			wantErr: false,
		},
		{
			name: "zero max size uses default",
			cfg: logging.RotationConfig{
				MaxSize:    0,
				MaxAge:     30,
				MaxBackups: 5,
				Daily:      false,
			},
			wantErr: false,
		},
		{
			name: "negative max age ignored",
			cfg: logging.RotationConfig{
				MaxSize:    1024,
				MaxAge:     -1,
				MaxBackups: 5,
				Daily:      false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			logPath := filepath.Join(tempDir, "rotation_test.log")

			writer, err := logging.NewRotatingWriter(logPath, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRotatingWriter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if closeErr := writer.Close(); closeErr != nil {
					t.Errorf("Close() error = %v", closeErr)
				}
			}
		})
	}
}

func TestRotationFileNaming(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "naming.log")

	writer, err := logging.NewRotatingWriter(logPath, logging.RotationConfig{
		MaxSize:    128, // 128 bytes - very small for testing
		MaxAge:     7,
		MaxBackups: 5,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}

	// Trigger rotation - write more than max size several times
	for i := 0; i < 30; i++ {
		msg := strings.Repeat("z", 20) + "\n"
		if _, writeErr := writer.Write([]byte(msg)); writeErr != nil {
			t.Fatalf("Write() error = %v", writeErr)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	// Verify naming pattern: naming.log, naming.timestamp.log, etc.
	hasMainLog := false
	hasRotatedLog := false

	for _, f := range files {
		name := f.Name()
		if name == "naming.log" {
			hasMainLog = true
		} else if strings.HasPrefix(name, "naming.") && strings.HasSuffix(name, ".log") {
			hasRotatedLog = true
		}
	}

	if !hasMainLog {
		t.Error("expected main log file naming.log to exist")
	}
	if !hasRotatedLog {
		t.Error("expected rotated log files to exist")
	}
}

func TestRotationWriter(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "writer.log")

	writer, err := logging.NewRotatingWriter(logPath, logging.RotationConfig{
		MaxSize:    1024,
		MaxAge:     7,
		MaxBackups: 3,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}

	// Write some data
	data := []byte("test log line\n")
	n, err := writer.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() returned %d, want %d", n, len(data))
	}

	if err := writer.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify file was created
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("file content = %q, want %q", content, data)
	}
}

func TestRotationCleanupOldFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create some "old" rotated log files manually that match the pattern
	baseTime := time.Now().Add(-48 * time.Hour) // 2 days ago

	oldFiles := []string{
		filepath.Join(tempDir, "cleanup.2024-01-18-120000.log"),
		filepath.Join(tempDir, "cleanup.2024-01-19-120000.log"),
	}

	for _, f := range oldFiles {
		if err := os.WriteFile(f, []byte("old content"), 0o644); err != nil {
			t.Fatalf("failed to create old file: %v", err)
		}
		if err := os.Chtimes(f, baseTime, baseTime); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	logPath := filepath.Join(tempDir, "cleanup.log")

	writer, err := logging.NewRotatingWriter(logPath, logging.RotationConfig{
		MaxSize:    10 * 1024 * 1024,
		MaxAge:     1, // 1 day - old files should be cleaned
		MaxBackups: 5,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}

	// Write a log entry to trigger cleanup
	if _, writeErr := writer.Write([]byte("new log entry\n")); writeErr != nil {
		t.Errorf("Write() error = %v", writeErr)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify old files were cleaned up
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	for _, f := range files {
		for _, oldFile := range oldFiles {
			if f.Name() == filepath.Base(oldFile) {
				t.Errorf("expected old file %s to be cleaned up", oldFile)
			}
		}
	}
}

func TestRotationDirCreation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	nestedPath := filepath.Join(tempDir, "nested", "deep", "log.log")

	writer, err := logging.NewRotatingWriter(nestedPath, logging.RotationConfig{
		MaxSize:    1024,
		MaxAge:     7,
		MaxBackups: 3,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() should create parent dirs, error = %v", err)
	}

	if _, writeErr := writer.Write([]byte("test\n")); writeErr != nil {
		t.Errorf("Write() error = %v", writeErr)
	}

	if closeErr := writer.Close(); closeErr != nil {
		t.Errorf("Close() error = %v", closeErr)
	}

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Error("expected log file to be created in nested directory")
	}
}

func TestRotationConcurrentWrites(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "concurrent.log")

	writer, err := logging.NewRotatingWriter(logPath, logging.RotationConfig{
		MaxSize:    10 * 1024 * 1024, // Large to avoid rotation during concurrent writes
		MaxAge:     7,
		MaxBackups: 3,
		Daily:      false,
	})
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}

	const numGoroutines = 10
	const numWrites = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numWrites; j++ {
				msg := strings.Repeat("x", 50) + "\n"
				if _, writeErr := writer.Write([]byte(msg)); writeErr != nil {
					t.Errorf("Write() error = %v", writeErr)
				}
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify file contains data
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	expectedLines := numGoroutines * numWrites
	if len(lines) != expectedLines {
		t.Errorf("expected %d lines, got %d", expectedLines, len(lines))
	}
}
