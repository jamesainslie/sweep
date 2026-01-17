package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Channel buffer sizes for bounded channels.
const (
	// dirQueueSize is the buffer size for the directory queue.
	// Larger values allow more directories to be queued but use more memory.
	dirQueueSize = 1024

	// fileQueueSize is the buffer size for the file queue.
	// Larger values allow more files to be queued but use more memory.
	fileQueueSize = 4096

	// resultQueueSize is the buffer size for the results channel.
	resultQueueSize = 1024
)

// Scanner performs parallel directory scanning.
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

	// cacheEntries collects entries for cache updates during scan.
	cacheEntries   map[string]*cache.CachedEntry
	cacheEntriesMu sync.Mutex
}

// New creates a new Scanner with the given options.
func New(opts Options) *Scanner {
	if err := opts.Validate(); err != nil {
		// Validation only sets defaults, doesn't return errors currently
		_ = err
	}

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

	// Resolve root path to absolute.
	root, err := filepath.Abs(s.opts.Root)
	if err != nil {
		return nil, err
	}

	// Verify root exists and is a directory.
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() {
		return nil, os.ErrInvalid
	}

	// Phase 1: Check cache for valid entries
	var dirsToScan []string
	if s.opts.Cache != nil {
		validFiles, staleDirs, cacheErr := s.opts.Cache.ValidateAndGetStale(root)
		if cacheErr == nil && len(staleDirs) == 0 {
			// Everything cached and valid - filter by MinSize and return
			for _, f := range validFiles {
				if f.Size >= s.opts.MinSize && !s.isExcluded(f.Path) {
					s.results = append(s.results, f)
				}
			}
			s.filesScanned.Store(int64(len(validFiles)))
			return &types.ScanResult{
				Files:        s.results,
				DirsScanned:  0,
				FilesScanned: int64(len(validFiles)),
				Elapsed:      time.Since(startTime),
				Errors:       s.errors,
			}, nil
		}
		// If we have stale dirs, we need to scan them
		if len(staleDirs) > 0 {
			dirsToScan = staleDirs
			// Add valid files that pass the filter and are NOT under a stale directory
			for _, f := range validFiles {
				if f.Size >= s.opts.MinSize && !s.isExcluded(f.Path) && !isUnderStaleDirs(f.Path, staleDirs) {
					s.results = append(s.results, f)
					s.largeFiles.Add(1)
				}
			}
		}
	}

	// Initialize cache entries map for collecting during scan
	s.cacheEntries = make(map[string]*cache.CachedEntry)

	// Phase 2: Scan directories (either all from root, or just stale dirs)
	// Create bounded channels.
	dirQueue := make(chan string, dirQueueSize)
	fileQueue := make(chan string, fileQueueSize)
	resultChan := make(chan types.FileInfo, resultQueueSize)

	// inFlight tracks the number of directories being processed or queued.
	// When it reaches 0, all directory work is complete.
	var inFlight atomic.Int64

	// WaitGroups for coordination.
	var fileWG sync.WaitGroup

	// Create a context for directory workers that we can cancel.
	dirCtx, dirCancel := context.WithCancel(ctx)
	defer dirCancel()

	// Start directory workers.
	for range s.opts.DirWorkers {
		go func() {
			s.dirWorker(dirCtx, dirQueue, fileQueue, &inFlight, dirCancel)
		}()
	}

	// Start file workers.
	for range s.opts.FileWorkers {
		fileWG.Add(1)
		go func() {
			defer fileWG.Done()
			s.fileWorker(ctx, fileQueue, resultChan)
		}()
	}

	// Collect results in a separate goroutine.
	var collectWG sync.WaitGroup
	collectWG.Add(1)
	go func() {
		defer collectWG.Done()
		for fi := range resultChan {
			s.resultsMu.Lock()
			s.results = append(s.results, fi)
			s.resultsMu.Unlock()
		}
	}()

	// Seed the directory queue.
	if len(dirsToScan) > 0 {
		// Scan only stale directories
		for _, dir := range dirsToScan {
			inFlight.Add(1)
			dirQueue <- dir
		}
	} else {
		// Full scan from root
		inFlight.Add(1)
		dirQueue <- root
	}

	// Wait for directory context to be cancelled (signals completion).
	<-dirCtx.Done()

	// Close directory queue - workers will drain and exit.
	close(dirQueue)

	// Close file queue to signal file workers to stop.
	close(fileQueue)

	// Wait for all file workers to complete.
	fileWG.Wait()

	// Close result channel to signal collector to stop.
	close(resultChan)

	// Wait for result collector to finish.
	collectWG.Wait()

	// Phase 3: Update cache with collected entries
	if s.opts.Cache != nil && len(s.cacheEntries) > 0 {
		if updateErr := s.opts.Cache.Update(root, s.cacheEntries); updateErr != nil {
			// Log error but don't fail the scan
			s.addError("cache update", updateErr)
		}
	}

	// Build final result.
	result := &types.ScanResult{
		Files:        s.results,
		DirsScanned:  s.dirsScanned.Load(),
		FilesScanned: s.filesScanned.Load(),
		TotalSize:    s.bytesScanned.Load(),
		Elapsed:      time.Since(startTime),
		Errors:       s.errors,
	}

	return result, nil
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
func (s *Scanner) reportProgress() {
	if s.opts.OnProgress == nil {
		return
	}

	currentPath, _ := s.currentPath.Load().(string)

	s.opts.OnProgress(types.ScanProgress{
		DirsScanned:  s.dirsScanned.Load(),
		FilesScanned: s.filesScanned.Load(),
		LargeFiles:   s.largeFiles.Load(),
		CurrentPath:  currentPath,
		BytesScanned: s.bytesScanned.Load(),
	})
}
