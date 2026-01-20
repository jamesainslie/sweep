package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Scanner performs parallel directory scanning using fastwalk.
type Scanner struct {
	opts Options

	// Atomic counters for thread-safe progress reporting.
	dirsScanned  atomic.Int64
	filesScanned atomic.Int64
	largeFiles   atomic.Int64
	bytesScanned atomic.Int64
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64

	// currentPath is the path currently being scanned (for progress).
	currentPath atomic.Value

	// errors collects scan errors without stopping the scan.
	errors   []types.ScanError
	errorsMu sync.Mutex

	// results collects files matching the criteria.
	results   []types.FileInfo
	resultsMu sync.Mutex

	// lastProgress tracks when we last reported progress to avoid excessive callbacks.
	lastProgress atomic.Int64

	// cacheEntries collects entries for cache updates during scan.
	cacheEntries   map[string]*cache.CachedEntry
	cacheEntriesMu sync.Mutex

	// dirChildren tracks children for each directory during scanning.
	// Key is relative path, value is list of child names.
	dirChildren   map[string][]string
	dirChildrenMu sync.Mutex

	// root is the resolved absolute path being scanned.
	root string

	// walkComplete indicates directory traversal is finished (cache flush may be ongoing).
	walkComplete atomic.Bool
}

// New creates a new Scanner with the given options.
// Options are validated and defaults are applied.
func New(opts Options) *Scanner {
	// Validate sets defaults for invalid values; it currently doesn't return errors
	// but we call it to ensure options are properly initialized.
	_ = opts.Validate()

	s := &Scanner{
		opts:    opts,
		errors:  make([]types.ScanError, 0),
		results: make([]types.FileInfo, 0),
	}
	s.currentPath.Store("")
	return s
}

// Scan performs the scan and returns results.
// It blocks until complete or context is cancelled.
func (s *Scanner) Scan(ctx context.Context) (*types.ScanResult, error) {
	startTime := time.Now()

	// Resolve and validate root path.
	root, err := s.validateRoot()
	if err != nil {
		return nil, err
	}
	s.root = root

	// Report initial progress immediately.
	s.currentPath.Store(root)
	s.reportProgressForce()

	// Phase 1: Check cache for valid entries.
	dirsToScan, earlyResult := s.validateCache(startTime)
	if earlyResult != nil {
		return earlyResult, nil
	}

	// Initialize cache entries map for collecting during scan.
	s.initCacheCollectors()

	// Phase 2: Scan directories using fastwalk.
	if err := s.executeWalk(ctx, dirsToScan); err != nil {
		return nil, err
	}

	// Signal walk completion so TUI can freeze elapsed time display.
	s.walkComplete.Store(true)
	s.currentPath.Store("(updating cache...)")
	s.reportProgressForce()

	// Phase 3: Update cache with collected entries.
	s.flushCacheEntries()

	// Build final result.
	return &types.ScanResult{
		Files:        s.results,
		DirsScanned:  s.dirsScanned.Load(),
		FilesScanned: s.filesScanned.Load(),
		TotalSize:    s.bytesScanned.Load(),
		Elapsed:      time.Since(startTime),
		Errors:       s.errors,
		CacheHits:    s.cacheHits.Load(),
		CacheMisses:  s.cacheMisses.Load(),
	}, nil
}

// validateCache checks cache for valid entries and returns directories to scan.
// If all entries are valid, returns an early result. Otherwise returns nil.
func (s *Scanner) validateCache(startTime time.Time) (dirsToScan []string, earlyResult *types.ScanResult) {
	if s.opts.Cache == nil {
		return nil, nil
	}

	validFiles, staleDirs, cacheErr := s.opts.Cache.ValidateAndGetStale(s.root)
	if cacheErr != nil || len(staleDirs) > 0 {
		// Cache miss or stale dirs - need to scan.
		return s.handleStaleDirs(validFiles, staleDirs), nil
	}

	// Everything cached and valid - filter by MinSize and return early.
	return nil, s.buildCacheHitResult(validFiles, startTime)
}

// buildCacheHitResult creates a result from fully cached data.
func (s *Scanner) buildCacheHitResult(validFiles []types.FileInfo, startTime time.Time) *types.ScanResult {
	s.cacheHits.Store(int64(len(validFiles)))
	s.filesScanned.Store(int64(len(validFiles)))

	var totalBytes int64
	for _, f := range validFiles {
		totalBytes += f.Size
		if f.Size >= s.opts.MinSize && !s.isExcluded(f.Path) {
			s.results = append(s.results, f)
			s.largeFiles.Add(1)
		}
	}
	s.bytesScanned.Store(totalBytes)

	s.currentPath.Store("(from cache)")
	s.reportProgressForce()

	return &types.ScanResult{
		Files:        s.results,
		DirsScanned:  0,
		FilesScanned: int64(len(validFiles)),
		TotalSize:    totalBytes,
		Elapsed:      time.Since(startTime),
		Errors:       s.errors,
		CacheHits:    s.cacheHits.Load(),
		CacheMisses:  0,
	}
}

// handleStaleDirs processes valid files from cache that are not under stale directories.
func (s *Scanner) handleStaleDirs(validFiles []types.FileInfo, staleDirs []string) []string {
	if len(staleDirs) == 0 {
		return nil
	}

	var validCount int64
	for _, f := range validFiles {
		if !isUnderStaleDirs(f.Path, staleDirs) {
			validCount++
			if f.Size >= s.opts.MinSize && !s.isExcluded(f.Path) {
				s.results = append(s.results, f)
				s.largeFiles.Add(1)
			}
		}
	}
	s.cacheHits.Store(validCount)
	s.reportProgressForce()

	return staleDirs
}

// initCacheCollectors initializes maps for collecting cache entries during scan.
func (s *Scanner) initCacheCollectors() {
	if s.opts.Cache != nil {
		s.cacheEntries = make(map[string]*cache.CachedEntry)
		s.dirChildren = make(map[string][]string)
	}
}

// executeWalk runs fastwalk on the specified directories or the root.
func (s *Scanner) executeWalk(ctx context.Context, dirsToScan []string) error {
	conf := fastwalk.Config{
		Follow: false, // Don't follow symlinks.
	}

	walkCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})
	go func() {
		<-walkCtx.Done()
		close(done)
	}()

	var walkErr error
	if len(dirsToScan) > 0 {
		walkErr = s.walkDirs(conf, dirsToScan, done)
	} else {
		walkErr = fastwalk.Walk(&conf, s.root, s.walkCallback(done))
	}

	if walkErr != nil && !errors.Is(walkErr, context.Canceled) && !errors.Is(walkErr, fastwalk.ErrSkipFiles) {
		return walkErr
	}
	return nil
}

// walkDirs walks multiple directories.
func (s *Scanner) walkDirs(conf fastwalk.Config, dirs []string, done <-chan struct{}) error {
	for _, dir := range dirs {
		err := fastwalk.Walk(&conf, dir, s.walkCallback(done))
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, fastwalk.ErrSkipFiles) {
			return err
		}
	}
	return nil
}

// flushCacheEntries writes collected entries to the cache.
func (s *Scanner) flushCacheEntries() {
	if s.opts.Cache == nil || len(s.cacheEntries) == 0 {
		return
	}

	// Merge children into directory entries.
	s.dirChildrenMu.Lock()
	for relPath, children := range s.dirChildren {
		if entry, ok := s.cacheEntries[relPath]; ok && entry.IsDir {
			entry.Children = children
		}
	}
	s.dirChildrenMu.Unlock()

	if err := s.opts.Cache.Update(s.root, s.cacheEntries); err != nil {
		s.addError("cache update", err)
	}
}

// validateRoot resolves the root path to absolute and verifies it exists.
func (s *Scanner) validateRoot() (string, error) {
	root, err := filepath.Abs(s.opts.Root)
	if err != nil {
		return "", err
	}

	rootInfo, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !rootInfo.IsDir() {
		return "", os.ErrInvalid
	}

	return root, nil
}

// walkCallback returns the callback function for fastwalk.Walk.
func (s *Scanner) walkCallback(done <-chan struct{}) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		// Check for cancellation.
		select {
		case <-done:
			return fastwalk.ErrSkipFiles
		default:
		}

		// Handle errors gracefully - log and continue.
		if err != nil {
			s.addError(path, err)
			return nil
		}

		// Check exclusions.
		if s.isExcluded(path) {
			if d.IsDir() {
				return fastwalk.SkipDir
			}
			return nil
		}

		// Handle directories.
		if d.IsDir() {
			s.handleDirectory(path)
			return nil
		}

		// Process regular files.
		if d.Type().IsRegular() {
			s.processFile(path, d)
		}

		return nil
	}
}

// handleDirectory processes a directory entry during walk.
func (s *Scanner) handleDirectory(path string) {
	s.dirsScanned.Add(1)
	s.currentPath.Store(path)
	s.reportProgress()

	if s.opts.Cache == nil {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	s.addCacheEntry(path, &cache.CachedEntry{
		IsDir:    true,
		Size:     0,
		Mtime:    info.ModTime().UnixNano(),
		Children: nil, // Will be filled by child entries.
	})
}

// processFile handles a regular file entry.
func (s *Scanner) processFile(path string, d fs.DirEntry) {
	// Get file info (this triggers a stat call).
	info, err := d.Info()
	if err != nil {
		s.addError(path, err)
		return
	}

	size := info.Size()

	// Update counters.
	s.filesScanned.Add(1)
	s.bytesScanned.Add(size)
	s.cacheMisses.Add(1) // This file was scanned fresh, not from cache.

	// Add file to cache entries (regardless of size filtering).
	if s.opts.Cache != nil {
		s.addCacheEntry(path, &cache.CachedEntry{
			IsDir:    false,
			Size:     size,
			Mtime:    info.ModTime().UnixNano(),
			Children: nil,
		})
	}

	// Filter by minimum size.
	if size < s.opts.MinSize {
		return
	}

	// Build FileInfo for large files.
	fi := types.FileInfo{
		Path:       path,
		Size:       size,
		ModTime:    info.ModTime(),
		Mode:       info.Mode(),
		CreateTime: getCreateTime(info),
	}
	fi.Owner, fi.Group = getOwnership(info)

	// Increment large files counter.
	s.largeFiles.Add(1)

	// Add to results.
	s.resultsMu.Lock()
	s.results = append(s.results, fi)
	s.resultsMu.Unlock()

	// Call streaming callback if set.
	if s.opts.OnFile != nil {
		s.opts.OnFile(fi)
	}
}

// addCacheEntry adds an entry to the cache entries map thread-safely.
// The path is converted to a relative path from the root before storing.
func (s *Scanner) addCacheEntry(fullPath string, entry *cache.CachedEntry) {
	if s.cacheEntries == nil {
		return // Cache not enabled for this scan.
	}

	// Calculate relative path.
	relPath := ""
	if fullPath != s.root {
		relPath = strings.TrimPrefix(fullPath, s.root+string(filepath.Separator))
	}

	s.cacheEntriesMu.Lock()
	s.cacheEntries[relPath] = entry
	s.cacheEntriesMu.Unlock()

	// Track this entry as a child of its parent directory.
	s.addChildToParent(fullPath)
}

// addChildToParent adds an entry's name to its parent directory's children list.
func (s *Scanner) addChildToParent(fullPath string) {
	if s.dirChildren == nil {
		return
	}

	// Calculate relative path of parent.
	parentPath := filepath.Dir(fullPath)
	parentRelPath := ""
	if parentPath != s.root {
		parentRelPath = strings.TrimPrefix(parentPath, s.root+string(filepath.Separator))
	}

	// Get child name.
	childName := filepath.Base(fullPath)

	s.dirChildrenMu.Lock()
	s.dirChildren[parentRelPath] = append(s.dirChildren[parentRelPath], childName)
	s.dirChildrenMu.Unlock()
}

// addError adds an error to the error list thread-safely.
func (s *Scanner) addError(path string, err error) {
	s.errorsMu.Lock()
	s.errors = append(s.errors, types.ScanError{
		Path:  path,
		Error: err.Error(),
	})
	s.errorsMu.Unlock()
}

// reportProgress calls the progress callback if configured.
// Throttles calls to avoid excessive overhead.
func (s *Scanner) reportProgress() {
	if s.opts.OnProgress == nil {
		return
	}

	// Throttle progress updates to every 10ms.
	now := time.Now().UnixMilli()
	last := s.lastProgress.Load()
	if now-last < 10 {
		return
	}
	if !s.lastProgress.CompareAndSwap(last, now) {
		return // Another goroutine updated it.
	}

	s.sendProgress()
}

// reportProgressForce calls the progress callback immediately, bypassing throttle.
// Use for important state changes like scan start/end.
func (s *Scanner) reportProgressForce() {
	if s.opts.OnProgress == nil {
		return
	}
	s.lastProgress.Store(time.Now().UnixMilli())
	s.sendProgress()
}

// sendProgress sends the current progress to the callback.
func (s *Scanner) sendProgress() {
	currentPath, _ := s.currentPath.Load().(string)

	s.opts.OnProgress(types.ScanProgress{
		DirsScanned:  s.dirsScanned.Load(),
		FilesScanned: s.filesScanned.Load(),
		LargeFiles:   s.largeFiles.Load(),
		CurrentPath:  currentPath,
		BytesScanned: s.bytesScanned.Load(),
		CacheHits:    s.cacheHits.Load(),
		CacheMisses:  s.cacheMisses.Load(),
		WalkComplete: s.walkComplete.Load(),
	})
}

// isExcluded checks if a path matches any exclusion pattern.
func (s *Scanner) isExcluded(path string) bool {
	for _, pattern := range s.opts.Exclude {
		if s.matchesExclusionPattern(path, pattern) {
			return true
		}
	}
	return false
}

// matchesExclusionPattern checks if a path matches a single exclusion pattern.
func (s *Scanner) matchesExclusionPattern(path, pattern string) bool {
	if pattern == "" {
		return false
	}

	// Check if the path starts with the exclusion pattern (for directories).
	if len(path) >= len(pattern) {
		if path == pattern {
			return true
		}
		if len(path) > len(pattern) && path[:len(pattern)+1] == pattern+string(filepath.Separator) {
			return true
		}
	}

	// Try glob matching against basename.
	if matched, err := filepath.Match(pattern, filepath.Base(path)); err == nil && matched {
		return true
	}

	// Try matching against full path.
	if matched, err := filepath.Match(pattern, path); err == nil && matched {
		return true
	}

	return false
}

// isUnderStaleDirs checks if a path is under any of the stale directories.
func isUnderStaleDirs(path string, staleDirs []string) bool {
	for _, staleDir := range staleDirs {
		if strings.HasPrefix(path, staleDir+string(filepath.Separator)) || path == staleDir {
			return true
		}
	}
	return false
}
