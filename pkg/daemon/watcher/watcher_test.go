// Package watcher provides filesystem watching for real-time index updates.
package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

// setupTestStore creates a temporary store for testing.
func setupTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "testdb")
	s, err := store.Open(storePath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	return s, func() {
		s.Close()
	}
}

func TestNew(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	if w.store != s {
		t.Error("New() did not set store correctly")
	}

	if w.watcher == nil {
		t.Error("New() did not create fsnotify watcher")
	}
}

func TestWatch(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	// Create test directory structure
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Watch the directory
	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Verify paths are tracked
	w.mu.RLock()
	_, rootTracked := w.paths[tmpDir]
	_, subDirTracked := w.paths[subDir]
	w.mu.RUnlock()

	if !rootTracked {
		t.Error("Watch() did not track root directory")
	}
	if !subDirTracked {
		t.Error("Watch() did not track subdirectory")
	}
}

func TestWatchNonExistent(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	err = w.Watch("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("Watch() should return error for non-existent path")
	}
}

func TestUnwatch(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	// Create test directory
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Watch then unwatch
	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	w.Unwatch(tmpDir)

	// Verify paths are no longer tracked
	w.mu.RLock()
	_, rootTracked := w.paths[tmpDir]
	_, subDirTracked := w.paths[subDir]
	w.mu.RUnlock()

	if rootTracked {
		t.Error("Unwatch() did not remove root directory")
	}
	if subDirTracked {
		t.Error("Unwatch() did not remove subdirectory")
	}
}

func TestRunDetectsFileCreate(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	tmpDir := t.TempDir()

	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		mu         sync.Mutex
		events     []string
		operations []fsnotify.Op
	)

	// Start watcher in background
	go w.Run(ctx, func(path string, op fsnotify.Op) {
		mu.Lock()
		events = append(events, path)
		operations = append(operations, op)
		mu.Unlock()
	})

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create a file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for event with timeout
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		found := len(events) > 0
		mu.Unlock()
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Error("Run() did not detect file creation event")
	}

	foundCreate := false
	for i, path := range events {
		if path == testFile && (operations[i]&fsnotify.Create != 0 || operations[i]&fsnotify.Write != 0) {
			foundCreate = true
			break
		}
	}

	if !foundCreate {
		t.Errorf("Run() did not detect correct file creation event, got events: %v, ops: %v", events, operations)
	}
}

func TestRunDetectsFileModify(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	tmpDir := t.TempDir()

	// Create file before watching
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		mu         sync.Mutex
		events     []string
		operations []fsnotify.Op
	)

	go w.Run(ctx, func(path string, op fsnotify.Op) {
		mu.Lock()
		events = append(events, path)
		operations = append(operations, op)
		mu.Unlock()
	})

	time.Sleep(100 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(testFile, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		found := len(events) > 0
		mu.Unlock()
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Error("Run() did not detect file modification event")
	}

	foundWrite := false
	for i, path := range events {
		if path == testFile && operations[i]&fsnotify.Write != 0 {
			foundWrite = true
			break
		}
	}

	if !foundWrite {
		t.Errorf("Run() did not detect correct write event, got events: %v, ops: %v", events, operations)
	}
}

func TestRunDetectsFileDelete(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	tmpDir := t.TempDir()

	// Create file before watching
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		mu         sync.Mutex
		events     []string
		operations []fsnotify.Op
	)

	go w.Run(ctx, func(path string, op fsnotify.Op) {
		mu.Lock()
		events = append(events, path)
		operations = append(operations, op)
		mu.Unlock()
	})

	time.Sleep(100 * time.Millisecond)

	// Delete the file
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("failed to delete test file: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		found := len(events) > 0
		mu.Unlock()
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Error("Run() did not detect file deletion event")
	}

	foundRemove := false
	for i, path := range events {
		if path == testFile && operations[i]&fsnotify.Remove != 0 {
			foundRemove = true
			break
		}
	}

	if !foundRemove {
		t.Errorf("Run() did not detect correct remove event, got events: %v, ops: %v", events, operations)
	}
}

func TestRunContextCancellation(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	tmpDir := t.TempDir()
	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Run(ctx, nil)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success - Run returned after context cancellation
	case <-time.After(2 * time.Second):
		t.Error("Run() did not return after context cancellation")
	}
}

func TestWatchRecursive(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	// Create nested directory structure
	tmpDir := t.TempDir()
	level1 := filepath.Join(tmpDir, "level1")
	level2 := filepath.Join(level1, "level2")
	level3 := filepath.Join(level2, "level3")

	if err := os.MkdirAll(level3, 0o755); err != nil {
		t.Fatalf("failed to create nested dirs: %v", err)
	}

	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Verify all levels are tracked
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, dir := range []string{tmpDir, level1, level2, level3} {
		if _, tracked := w.paths[dir]; !tracked {
			t.Errorf("Watch() did not track nested directory: %s", dir)
		}
	}
}

func TestWatchIgnoresSymlinks(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	tmpDir := t.TempDir()
	realDir := filepath.Join(tmpDir, "real")
	symlink := filepath.Join(tmpDir, "symlink")

	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}

	if err := os.Symlink(realDir, symlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Verify symlink is not followed (symlink path itself may be tracked as a file entry)
	w.mu.RLock()
	defer w.mu.RUnlock()

	// The real dir should be tracked
	if _, tracked := w.paths[realDir]; !tracked {
		t.Error("Watch() did not track real directory")
	}
}

func TestClose(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Calling Close again should not panic
	if err := w.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestNewDirectoryWatchAdded(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	w, err := New(s)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	tmpDir := t.TempDir()

	if err := w.Watch(tmpDir); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		mu         sync.Mutex
		events     []string
		operations []fsnotify.Op
	)

	go w.Run(ctx, func(path string, op fsnotify.Op) {
		mu.Lock()
		events = append(events, path)
		operations = append(operations, op)
		mu.Unlock()
	})

	time.Sleep(100 * time.Millisecond)

	// Create a new directory
	newDir := filepath.Join(tmpDir, "newdir")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("failed to create new dir: %v", err)
	}

	// Wait for event and watch to be added
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		found := len(events) > 0
		mu.Unlock()
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Give time for the directory to be added to watch list
	time.Sleep(200 * time.Millisecond)

	// Verify the new directory was added to watch list
	w.mu.RLock()
	_, tracked := w.paths[newDir]
	w.mu.RUnlock()

	if !tracked {
		t.Error("Run() did not add watch for newly created directory")
	}
}
