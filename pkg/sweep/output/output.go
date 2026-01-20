// Package output provides formatters for displaying sweep scan results
// in various output formats (pretty, plain, json, yaml, etc.).
//
// The package uses a registry pattern to allow registration of multiple
// formatter implementations that can be selected at runtime.
//
// Basic usage:
//
//	formatter, err := output.Get("pretty")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	var buf bytes.Buffer
//	if err := formatter.Format(&buf, result); err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Print(buf.String())
package output

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

// logger is the package-level logger for output operations.
var logger = logging.Get("output")

// FileInfo contains detailed information about a file for output formatting.
// It extends the basic file metadata with computed fields like human-readable
// size and age for easier formatting.
type FileInfo struct {
	// Path is the absolute path to the file.
	Path string `json:"path" yaml:"path"`

	// Name is the base name of the file.
	Name string `json:"name" yaml:"name"`

	// Dir is the directory containing the file.
	Dir string `json:"dir" yaml:"dir"`

	// Ext is the file extension including the dot (e.g., ".zip").
	Ext string `json:"ext" yaml:"ext"`

	// Size is the file size in bytes.
	Size int64 `json:"size" yaml:"size"`

	// SizeHuman is the human-readable file size (e.g., "1.5 GiB").
	SizeHuman string `json:"size_human" yaml:"size_human"`

	// ModTime is the last modification time of the file.
	ModTime time.Time `json:"mod_time" yaml:"mod_time"`

	// Age is the time since the file was last modified.
	Age time.Duration `json:"age" yaml:"age"`

	// Perms is the human-readable permission string (e.g., "-rw-r--r--").
	Perms string `json:"perms" yaml:"perms"`

	// Mode is the file's permission and mode bits.
	Mode os.FileMode `json:"mode" yaml:"mode"`

	// Owner is the username of the file's owner.
	Owner string `json:"owner" yaml:"owner"`

	// Depth is the directory depth relative to the scan root.
	Depth int `json:"depth" yaml:"depth"`
}

// ScanStats contains statistics about a scan operation.
type ScanStats struct {
	// DirsScanned is the total number of directories traversed.
	DirsScanned int64 `json:"dirs_scanned" yaml:"dirs_scanned"`

	// FilesScanned is the total number of files examined.
	FilesScanned int64 `json:"files_scanned" yaml:"files_scanned"`

	// LargeFiles is the number of files exceeding the minimum size threshold.
	LargeFiles int64 `json:"large_files" yaml:"large_files"`

	// Duration is the total time taken to complete the scan.
	Duration time.Duration `json:"duration" yaml:"duration"`
}

// Result contains the complete output data for formatting.
// It includes all files found, scan statistics, and metadata about
// the scan operation.
type Result struct {
	// Files contains all files matching the scan criteria, sorted by size descending.
	Files []FileInfo `json:"files" yaml:"files"`

	// Stats contains scan statistics.
	Stats ScanStats `json:"stats" yaml:"stats"`

	// Source is the root path that was scanned.
	Source string `json:"source" yaml:"source"`

	// IndexAge is the age of the index if using cached results.
	IndexAge time.Duration `json:"index_age" yaml:"index_age"`

	// DaemonUp indicates if the sweep daemon is running.
	DaemonUp bool `json:"daemon_up" yaml:"daemon_up"`

	// WatchActive indicates if file watching is active on the source.
	WatchActive bool `json:"watch_active" yaml:"watch_active"`

	// TotalFiles is the total number of files in the result.
	TotalFiles int `json:"total_files" yaml:"total_files"`

	// Warnings contains any warning messages generated during the scan.
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`

	// Interrupted indicates if the scan was interrupted by the user.
	Interrupted bool `json:"interrupted" yaml:"interrupted"`
}

// TotalSize returns the sum of all file sizes in the result.
func (r *Result) TotalSize() int64 {
	var total int64
	for _, f := range r.Files {
		total += f.Size
	}
	return total
}

// Formatter is the interface that all output formatters must implement.
type Formatter interface {
	// Format writes the formatted output to the buffer.
	// It returns an error if formatting fails.
	Format(w *bytes.Buffer, r *Result) error
}

// FormatterFactory is a function that creates a new Formatter instance.
type FormatterFactory func() Formatter

// Registry manages formatter registration and lookup.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]FormatterFactory
}

// NewRegistry creates a new formatter registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]FormatterFactory),
	}
}

// Register adds a formatter factory to the registry.
// It will replace any existing formatter with the same name.
func (r *Registry) Register(name string, factory FormatterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Get returns a new formatter instance by name.
// It returns an error if the formatter is not found.
func (r *Registry) Get(name string) (Formatter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("unknown formatter: %s", name)
	}
	return factory(), nil
}

// Available returns a sorted list of all registered formatter names.
func (r *Registry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultRegistry is the global formatter registry.
var DefaultRegistry = NewRegistry()

// Register adds a formatter factory to the default registry.
func Register(name string, factory FormatterFactory) {
	DefaultRegistry.Register(name, factory)
}

// Get returns a new formatter instance from the default registry.
func Get(name string) (Formatter, error) {
	return DefaultRegistry.Get(name)
}

// Available returns all formatter names from the default registry.
func Available() []string {
	return DefaultRegistry.Available()
}
