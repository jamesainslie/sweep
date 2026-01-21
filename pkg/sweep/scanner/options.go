// Package scanner provides high-performance parallel directory scanning
// for the sweep disk analyzer. It uses fastwalk for maximum throughput.
package scanner

import (
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

	// OnFile is called for each file that matches the MinSize threshold.
	// It allows streaming results as files are found rather than waiting
	// for the entire scan to complete. Must be safe for concurrent calls.
	OnFile func(types.FileInfo)
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
