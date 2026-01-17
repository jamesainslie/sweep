// Package scanner provides high-performance parallel directory scanning
// for the sweep disk analyzer. It uses a dual worker pool architecture
// with bounded channels and atomic counters for maximum throughput.
package scanner

import (
	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Options configures the scanner behavior.
type Options struct {
	// Root is the starting directory for the scan.
	Root string

	// MinSize is the minimum file size in bytes to include in results.
	// Files smaller than this are filtered out.
	MinSize int64

	// Exclude contains glob patterns for paths to skip during scanning.
	// Patterns are matched against the full path.
	Exclude []string

	// DirWorkers is the number of concurrent workers for directory traversal.
	// More workers help with directories containing many subdirectories.
	DirWorkers int

	// FileWorkers is the number of concurrent workers for file stat operations.
	// More workers help when scanning storage with high latency.
	FileWorkers int

	// OnProgress is called periodically with scan progress updates.
	// It must be safe to call from multiple goroutines.
	OnProgress func(types.ScanProgress)

	// Cache is an optional cache for speeding up repeat scans.
	// If nil, caching is disabled.
	Cache *cache.Cache
}

// DefaultOptions returns options with sensible defaults for most systems.
func DefaultOptions() Options {
	return Options{
		Root:        config.DefaultPath,
		MinSize:     100 * types.MiB, // 100 MiB default
		Exclude:     config.DefaultExclusions,
		DirWorkers:  config.DefaultDirWorkers,
		FileWorkers: config.DefaultFileWorkers,
		OnProgress:  nil,
	}
}

// Validate checks if the options are valid and returns an error if not.
func (o *Options) Validate() error {
	if o.Root == "" {
		o.Root = config.DefaultPath
	}
	if o.DirWorkers < 1 {
		o.DirWorkers = config.DefaultDirWorkers
	}
	if o.FileWorkers < 1 {
		o.FileWorkers = config.DefaultFileWorkers
	}
	return nil
}
