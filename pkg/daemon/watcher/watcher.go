// Package watcher provides filesystem watching for real-time index updates.
package watcher

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/jamesainslie/sweep/pkg/daemon/broadcaster"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

// Watcher watches directories for filesystem changes and updates the store.
type Watcher struct {
	store            *store.Store
	watcher          *fsnotify.Watcher
	paths            map[string]bool
	mu               sync.RWMutex
	closed           bool
	broadcaster      *broadcaster.Broadcaster
	minLargeFileSize int64 // Threshold for large files index
}

// New creates a new Watcher.
func New(s *store.Store) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		store:   s,
		watcher: fsw,
		paths:   make(map[string]bool),
	}, nil
}

// SetBroadcaster sets the broadcaster for sending file events to clients.
func (w *Watcher) SetBroadcaster(b *broadcaster.Broadcaster) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.broadcaster = b
}

// SetMinLargeFileSize sets the threshold for large files.
func (w *Watcher) SetMinLargeFileSize(size int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.minLargeFileSize = size
}

// Watch starts watching a path recursively.
// It adds watches to the root directory and all subdirectories.
// Symlinks are not followed to avoid loops.
func (w *Watcher) Watch(root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	// Check if path exists
	info, err := os.Lstat(absRoot)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return nil // Only watch directories
	}

	// Walk and add all directories
	return filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // Skip entries with errors
		}

		// Skip symlinks to avoid loops
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			return w.addWatch(path)
		}

		return nil
	})
}

// addWatch adds a single directory to the watch list.
func (w *Watcher) addWatch(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	// Already watching this path
	if w.paths[path] {
		return nil
	}

	if err := w.watcher.Add(path); err != nil {
		logging.Get("watcher").Warn("failed to add watch", "path", path, "error", err)
		return err
	}

	w.paths[path] = true
	return nil
}

// Unwatch stops watching a path and all its subdirectories.
func (w *Watcher) Unwatch(root string) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	// Remove all paths under root
	for path := range w.paths {
		if path == absRoot || isSubPath(path, absRoot) {
			_ = w.watcher.Remove(path)
			delete(w.paths, path)
		}
	}
}

// Run starts the event loop. It blocks until the context is cancelled.
// The onChange callback is called for each filesystem event with the path and operation.
func (w *Watcher) Run(ctx context.Context, onChange func(path string, op fsnotify.Op)) {
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			w.handleEvent(event, onChange)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logging.Get("watcher").Error("watcher error", "error", err)
		}
	}
}

// handleEvent processes a single filesystem event.
func (w *Watcher) handleEvent(event fsnotify.Event, onChange func(path string, op fsnotify.Op)) {
	// Handle different event types
	switch {
	case event.Op&fsnotify.Create != 0:
		w.handleCreate(event.Name)
	case event.Op&fsnotify.Write != 0:
		w.handleWrite(event.Name)
	case event.Op&fsnotify.Remove != 0:
		w.handleRemove(event.Name)
	case event.Op&fsnotify.Rename != 0:
		// Rename is treated as a remove - the new name will trigger a create
		w.handleRemove(event.Name)
	}

	// Call the callback if provided
	if onChange != nil {
		onChange(event.Name, event.Op)
	}
}

// handleCreate handles file/directory creation events.
func (w *Watcher) handleCreate(path string) {
	info, err := os.Lstat(path)
	if err != nil {
		return // File might have been deleted already
	}

	// Skip symlinks
	if info.Mode()&fs.ModeSymlink != 0 {
		return
	}

	// If it's a directory, add a watch for it
	if info.IsDir() {
		// Add watch to this directory
		_ = w.addWatch(path)

		// Also walk any subdirectories that were created with it
		_ = filepath.WalkDir(path, func(subpath string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // Skip entries with errors
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil // Skip symlinks
			}
			if d.IsDir() && subpath != path {
				_ = w.addWatch(subpath)
			}
			return nil
		})
	}

	// Update store with new entry
	entry := &store.Entry{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}

	_ = w.store.Put(entry)

	// Update large files index if this is a large file
	if !info.IsDir() && w.minLargeFileSize > 0 && info.Size() >= w.minLargeFileSize {
		_ = w.store.AddLargeFile(path, info.Size(), info.ModTime().Unix())
	}

	// Notify broadcaster for files (not directories)
	if w.broadcaster != nil && !info.IsDir() {
		w.broadcaster.Notify(path, broadcaster.EventCreated, info.Size())
	}
}

// handleWrite handles file modification events.
func (w *Watcher) handleWrite(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return // File might have been deleted
	}

	// Update store with modified entry
	entry := &store.Entry{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}

	_ = w.store.Put(entry)

	// Update large files index based on new size
	if !info.IsDir() && w.minLargeFileSize > 0 {
		if info.Size() >= w.minLargeFileSize {
			_ = w.store.AddLargeFile(path, info.Size(), info.ModTime().Unix())
		} else {
			// File shrank below threshold, remove from large files index
			_ = w.store.RemoveLargeFile(path)
		}
	}

	// Notify broadcaster for files (not directories)
	if w.broadcaster != nil && !info.IsDir() {
		w.broadcaster.Notify(path, broadcaster.EventModified, info.Size())
	}
}

// handleRemove handles file/directory deletion events.
func (w *Watcher) handleRemove(path string) {
	// Notify broadcaster before cleanup (size 0 for deletions)
	if w.broadcaster != nil {
		w.broadcaster.Notify(path, broadcaster.EventDeleted, 0)
	}

	// Remove watch if it was a directory
	w.mu.Lock()
	if w.paths[path] {
		_ = w.watcher.Remove(path)
		delete(w.paths, path)
	}

	// Also remove any child watches
	for childPath := range w.paths {
		if isSubPath(childPath, path) {
			_ = w.watcher.Remove(childPath)
			delete(w.paths, childPath)
		}
	}
	w.mu.Unlock()

	// Delete from store - this deletes the path and all children
	_ = w.store.DeletePrefix(path)
}

// Close closes the watcher and releases resources.
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	w.paths = make(map[string]bool)
	return w.watcher.Close()
}

// isSubPath checks if path is under parent directory.
func isSubPath(path, parent string) bool {
	return len(path) > len(parent) && path[:len(parent)+1] == parent+string(filepath.Separator)
}
