package scanner

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// dirWorker processes directories from the dir queue.
// For each directory:
//  1. ReadDir to get entries
//  2. For each entry:
//     - If directory: push to dirQueue (unless excluded)
//     - If file: push to fileQueue
//
// The worker exits when:
//   - Context is cancelled
//   - The queue is closed
func (s *Scanner) dirWorker(ctx context.Context, dirQueue chan string, fileQueue chan<- string, inFlight *atomic.Int64, done context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return

		case dir, ok := <-dirQueue:
			if !ok {
				// Channel closed, exit.
				return
			}

			// Process the directory.
			s.processDirectory(ctx, dir, dirQueue, fileQueue, inFlight)

			// Decrement inFlight after processing.
			// If it reaches 0, all work is done.
			if inFlight.Add(-1) == 0 {
				done()
				return
			}
		}
	}
}

// processDirectory reads a directory and dispatches entries.
func (s *Scanner) processDirectory(ctx context.Context, dir string, dirQueue chan string, fileQueue chan<- string, inFlight *atomic.Int64) {
	// Update progress.
	s.currentPath.Store(dir)
	s.dirsScanned.Add(1)

	// Report progress periodically.
	if s.dirsScanned.Load()%100 == 0 {
		s.reportProgress()
	}

	// Get directory info for cache
	dirInfo, statErr := os.Stat(dir)
	if statErr != nil {
		s.addError(dir, statErr)
		return
	}

	// Read directory entries.
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.addError(dir, err)
		return
	}

	// Collect child names for cache entry
	var childNames []string

	for _, entry := range entries {
		// Check for cancellation.
		select {
		case <-ctx.Done():
			return
		default:
		}

		name := entry.Name()
		fullPath := filepath.Join(dir, name)

		// Check exclusions before processing.
		if s.isExcluded(fullPath) {
			continue
		}

		// Add to children list for cache
		childNames = append(childNames, name)

		// Use DirEntry.Type() which doesn't require a stat call.
		if entry.IsDir() {
			// Increment inFlight BEFORE sending to queue.
			inFlight.Add(1)

			// Push directory to queue for processing.
			select {
			case dirQueue <- fullPath:
			case <-ctx.Done():
				// Decrement since we didn't actually queue it.
				inFlight.Add(-1)
				return
			}
		} else if entry.Type().IsRegular() {
			// Push regular file to file queue.
			select {
			case fileQueue <- fullPath:
			case <-ctx.Done():
				return
			}
		}
		// Skip symlinks and special files.
	}

	// Add directory to cache entries
	s.addCacheEntry(dir, &cache.CachedEntry{
		IsDir:    true,
		Size:     0,
		Mtime:    dirInfo.ModTime().UnixNano(),
		Children: childNames,
	})
}

// fileWorker processes files from the file queue.
// For each file:
//  1. Stat the file
//  2. Check if size >= MinSize
//  3. If yes, build FileInfo and send to results
func (s *Scanner) fileWorker(ctx context.Context, fileQueue <-chan string, results chan<- types.FileInfo) {
	for {
		select {
		case <-ctx.Done():
			// Drain the file queue before returning to avoid blocking dir workers.
			for range fileQueue {
			}
			return

		case path, ok := <-fileQueue:
			if !ok {
				// Channel closed, exit.
				return
			}

			// Process the file.
			s.processFile(ctx, path, results)
		}
	}
}

// processFile stats a file and adds it to results if it meets criteria.
func (s *Scanner) processFile(ctx context.Context, path string, results chan<- types.FileInfo) {
	// Stat the file.
	info, err := os.Lstat(path)
	if err != nil {
		s.addError(path, err)
		return
	}

	// Skip non-regular files (symlinks, etc.).
	if !info.Mode().IsRegular() {
		return
	}

	size := info.Size()

	// Update atomic counters.
	s.filesScanned.Add(1)
	s.bytesScanned.Add(size)

	// Add file to cache entries (regardless of size filtering)
	s.addCacheEntry(path, &cache.CachedEntry{
		IsDir:    false,
		Size:     size,
		Mtime:    info.ModTime().UnixNano(),
		Children: nil,
	})

	// Early filtering - skip small files before building FileInfo.
	if size < s.opts.MinSize {
		return
	}

	// Build FileInfo for large files.
	fi := types.FileInfo{
		Path:    path,
		Size:    size,
		ModTime: info.ModTime(),
		Mode:    info.Mode(),
	}

	// Get ownership information (macOS/Unix specific).
	fi.Owner, fi.Group = getOwnership(info)

	// Get creation time if available.
	fi.CreateTime = getCreateTime(info)

	// Increment large files counter.
	s.largeFiles.Add(1)

	// Send to results.
	select {
	case results <- fi:
	case <-ctx.Done():
	}
}

// addCacheEntry adds an entry to the cache entries map thread-safely.
// The path is converted to a relative path from the root before storing.
func (s *Scanner) addCacheEntry(fullPath string, entry *cache.CachedEntry) {
	if s.cacheEntries == nil {
		return // Cache not enabled for this scan
	}

	// Get the root path from opts
	root, _ := filepath.Abs(s.opts.Root)

	// Calculate relative path
	relPath := ""
	if fullPath != root {
		relPath = strings.TrimPrefix(fullPath, root+string(filepath.Separator))
	}

	s.cacheEntriesMu.Lock()
	s.cacheEntries[relPath] = entry
	s.cacheEntriesMu.Unlock()
}

// isExcluded checks if a path matches any exclusion pattern.
func (s *Scanner) isExcluded(path string) bool {
	for _, pattern := range s.opts.Exclude {
		// Check if the path starts with the exclusion pattern (for directories).
		if pattern != "" && len(path) >= len(pattern) {
			if path == pattern || (len(path) > len(pattern) && path[:len(pattern)+1] == pattern+string(filepath.Separator)) {
				return true
			}
		}

		// Try glob matching.
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}

		// Try matching against full path.
		matched, err = filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// getOwnership returns the owner and group names for a file.
// Falls back to UID/GID strings if names cannot be resolved.
func getOwnership(info os.FileInfo) (owner, group string) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "unknown", "unknown"
	}

	// Try to resolve UID to username.
	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	if u, err := user.LookupId(uid); err == nil {
		owner = u.Username
	} else {
		owner = uid
	}

	// Try to resolve GID to group name.
	gid := strconv.FormatUint(uint64(stat.Gid), 10)
	if g, err := user.LookupGroupId(gid); err == nil {
		group = g.Name
	} else {
		group = gid
	}

	return owner, group
}
