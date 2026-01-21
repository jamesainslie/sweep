package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
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

	// root is the resolved absolute path being scanned.
	root string

	// walkComplete indicates directory traversal is finished.
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

	// Scan directories using fastwalk.
	if err := s.executeWalk(ctx); err != nil {
		return nil, err
	}

	// Signal walk completion so TUI can freeze elapsed time display.
	s.walkComplete.Store(true)
	s.reportProgressForce()

	// Build final result.
	return &types.ScanResult{
		Files:        s.results,
		DirsScanned:  s.dirsScanned.Load(),
		FilesScanned: s.filesScanned.Load(),
		TotalSize:    s.bytesScanned.Load(),
		Elapsed:      time.Since(startTime),
		Errors:       s.errors,
	}, nil
}

// executeWalk runs fastwalk on the root directory.
func (s *Scanner) executeWalk(ctx context.Context) error {
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

	walkErr := fastwalk.Walk(&conf, s.root, s.walkCallback(done))

	if walkErr != nil && !errors.Is(walkErr, context.Canceled) && !errors.Is(walkErr, fastwalk.ErrSkipFiles) {
		return walkErr
	}
	return nil
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
