// Package indexer provides filesystem indexing capabilities using fastwalk.
package indexer

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

// Progress reports indexing progress.
type Progress struct {
	Path         string
	DirsScanned  int64
	FilesScanned int64
	CurrentPath  string
}

// Result contains the final indexing results.
type Result struct {
	Path          string
	DirsIndexed   int64
	FilesIndexed  int64
	TotalSize     int64
	Duration      time.Duration
	Cached        bool     // True if path was already covered by an indexed path
	CoveredBy     string   // Parent path that covers this one (if Cached is true)
	SubsumedPaths []string // Child paths that were subsumed by this indexing operation
}

// ProgressFunc is called with progress updates.
type ProgressFunc func(Progress)

// DefaultMinLargeFileSize is the default threshold for files to be added to the large files index.
// Files >= this size are indexed for fast large file queries.
const DefaultMinLargeFileSize = 10 * 1024 * 1024 // 10 MiB

// Indexer indexes filesystem paths into the store.
type Indexer struct {
	store            *store.Store
	MinLargeFileSize int64 // Threshold for large files index (default: DefaultMinLargeFileSize)
}

// New creates a new indexer with default settings.
func New(s *store.Store) *Indexer {
	return &Indexer{
		store:            s,
		MinLargeFileSize: DefaultMinLargeFileSize,
	}
}

// indexState holds the state during indexing.
type indexState struct {
	dirsScanned  atomic.Int64
	filesScanned atomic.Int64
	totalSize    atomic.Int64
	currentPath  atomic.Value
	entriesMu    sync.Mutex
	entries      []*store.Entry
	largeFiles   []*store.Entry // Files >= MinLargeFileSize for fast queries
}

// Index indexes a path and stores results.
func (idx *Indexer) Index(ctx context.Context, root string, onProgress ProgressFunc) (*Result, error) {
	startTime := time.Now()

	// Resolve to absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// Check if this path is already covered by an indexed path
	if covered, coveringPath := idx.store.IsPathCovered(absRoot); covered {
		return &Result{
			Path:      absRoot,
			Duration:  time.Since(startTime),
			Cached:    true,
			CoveredBy: coveringPath,
		}, nil
	}

	state := &indexState{}
	state.currentPath.Store("")

	// Start progress reporting
	done := idx.startProgressReporter(ctx, absRoot, state, onProgress)
	defer func() {
		close(done)
		// Send final progress
		idx.sendProgress(absRoot, state, onProgress)
	}()

	// Walk the filesystem
	err = idx.walkFilesystem(ctx, absRoot, state)
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	// Write remaining entries
	if err := idx.flushRemainingEntries(state); err != nil {
		return nil, err
	}

	// Save metadata for fast status lookups
	files := state.filesScanned.Load()
	dirs := state.dirsScanned.Load()
	_ = idx.store.SetIndexMeta(absRoot, &store.IndexMeta{
		Files: files,
		Dirs:  dirs,
	})

	// Ensure schema is up to date (new indexes are always current version)
	if schema := idx.store.GetSchema(); schema == nil || schema.Version < store.CurrentSchemaVersion {
		_ = idx.store.SetSchema(&store.Schema{Version: store.CurrentSchemaVersion})
	}

	// Add this path to the indexed paths list, subsuming any child paths
	subsumedPaths, err := idx.store.AddIndexedPathWithSubsumption(absRoot)
	if err != nil {
		return nil, err
	}

	return &Result{
		Path:          absRoot,
		DirsIndexed:   dirs,
		FilesIndexed:  files,
		TotalSize:     state.totalSize.Load(),
		Duration:      time.Since(startTime),
		SubsumedPaths: subsumedPaths,
	}, nil
}

// sendProgress sends a progress update if callback is provided.
func (idx *Indexer) sendProgress(absRoot string, state *indexState, onProgress ProgressFunc) {
	if onProgress != nil {
		cp, _ := state.currentPath.Load().(string)
		onProgress(Progress{
			Path:         absRoot,
			DirsScanned:  state.dirsScanned.Load(),
			FilesScanned: state.filesScanned.Load(),
			CurrentPath:  cp,
		})
	}
}

// startProgressReporter starts the progress reporting goroutine.
func (idx *Indexer) startProgressReporter(ctx context.Context, absRoot string, state *indexState, onProgress ProgressFunc) chan struct{} {
	done := make(chan struct{})

	// Send initial progress
	idx.sendProgress(absRoot, state, onProgress)

	if onProgress != nil {
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					idx.sendProgress(absRoot, state, onProgress)
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return done
}

// walkFilesystem performs the filesystem walk.
func (idx *Indexer) walkFilesystem(ctx context.Context, absRoot string, state *indexState) error {
	conf := fastwalk.Config{
		Follow: false,
	}

	return fastwalk.Walk(&conf, absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip entries with errors - intentionally continue walking
		if walkErr != nil {
			return nil //nolint:nilerr // Intentionally skip errors and continue walking
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil //nolint:nilerr // Intentionally skip entries we can't stat
		}

		return idx.processEntry(path, info, d.IsDir(), state)
	})
}

// processEntry processes a single filesystem entry.
func (idx *Indexer) processEntry(path string, info fs.FileInfo, isDir bool, state *indexState) error {
	entry := &store.Entry{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime().Unix(),
		IsDir:   isDir,
	}

	state.entriesMu.Lock()
	state.entries = append(state.entries, entry)
	// Track large files for fast queries
	if !isDir && info.Size() >= idx.MinLargeFileSize {
		state.largeFiles = append(state.largeFiles, entry)
	}
	state.entriesMu.Unlock()

	if isDir {
		state.dirsScanned.Add(1)
		state.currentPath.Store(path)
	} else {
		state.filesScanned.Add(1)
		state.totalSize.Add(info.Size())
	}

	// Batch write every 1000 entries
	return idx.flushBatchIfNeeded(state)
}

// flushBatchIfNeeded writes entries to store if batch size is reached.
func (idx *Indexer) flushBatchIfNeeded(state *indexState) error {
	state.entriesMu.Lock()
	if len(state.entries) >= 1000 {
		batch := state.entries
		state.entries = nil
		state.entriesMu.Unlock()
		return idx.store.PutBatch(batch)
	}
	state.entriesMu.Unlock()
	return nil
}

// flushRemainingEntries writes any remaining entries to the store.
func (idx *Indexer) flushRemainingEntries(state *indexState) error {
	state.entriesMu.Lock()
	remaining := state.entries
	largeFiles := state.largeFiles
	state.entries = nil
	state.largeFiles = nil
	state.entriesMu.Unlock()

	if len(remaining) > 0 {
		if err := idx.store.PutBatch(remaining); err != nil {
			return err
		}
	}

	// Write large files to the fast-query index
	if len(largeFiles) > 0 {
		if err := idx.store.AddLargeFileBatch(largeFiles); err != nil {
			return err
		}
	}

	return nil
}

// IsIndexed checks if a path has been indexed.
func (idx *Indexer) IsIndexed(root string) bool {
	return idx.store.HasIndex(root)
}
