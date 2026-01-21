// Package types provides core data types for the sweep disk analyzer.
// It includes structures for file information, scan results, and configuration options,
// along with utility functions for parsing and formatting file sizes.
package types

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// Size constants for binary (IEC) units.
const (
	KiB int64 = 1024
	MiB int64 = 1024 * KiB
	GiB int64 = 1024 * MiB
	TiB int64 = 1024 * GiB
)

// FileInfo contains detailed information about a file.
// It captures metadata needed for disk analysis including size, timestamps,
// and ownership information.
type FileInfo struct {
	// Path is the absolute path to the file.
	Path string `json:"path"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// ModTime is the last modification time of the file.
	ModTime time.Time `json:"mod_time"`

	// CreateTime is the creation time of the file (may be zero on some systems).
	CreateTime time.Time `json:"create_time,omitempty"`

	// Mode is the file's permission and mode bits.
	Mode os.FileMode `json:"mode"`

	// Owner is the username of the file's owner.
	Owner string `json:"owner"`

	// Group is the group name of the file's group.
	Group string `json:"group"`
}

// HumanSize returns the file size formatted as a human-readable string.
// It uses binary (IEC) units (KiB, MiB, GiB, TiB).
func (f *FileInfo) HumanSize() string {
	return FormatSize(f.Size)
}

// ScanResult contains the aggregated results of a scan operation.
// It includes all discovered files meeting the criteria, statistics about
// the scan, and any errors encountered during the scan.
type ScanResult struct {
	// Files contains all files that matched the scan criteria.
	Files []FileInfo `json:"files"`

	// DirsScanned is the total number of directories traversed.
	DirsScanned int64 `json:"dirs_scanned"`

	// FilesScanned is the total number of files examined.
	FilesScanned int64 `json:"files_scanned"`

	// TotalSize is the sum of all file sizes in bytes.
	TotalSize int64 `json:"total_size"`

	// Elapsed is the total time taken to complete the scan.
	Elapsed time.Duration `json:"elapsed"`

	// Errors contains any errors encountered during scanning.
	Errors []ScanError `json:"errors,omitempty"`
}

// ScanError represents an error encountered during scanning.
// It pairs a file path with the error message for debugging and reporting.
type ScanError struct {
	// Path is the file or directory path where the error occurred.
	Path string `json:"path"`

	// Error is the error message describing what went wrong.
	Error string `json:"error"`
}

// ScanOptions configures the scanner behavior.
// It allows customization of the scan root, size thresholds,
// exclusion patterns, and concurrency settings.
type ScanOptions struct {
	// Root is the starting directory for the scan.
	Root string `json:"root"`

	// MinSize is the minimum file size in bytes to include in results.
	// Files smaller than this are excluded from the results.
	MinSize int64 `json:"min_size"`

	// Exclude contains glob patterns for paths to skip during scanning.
	Exclude []string `json:"exclude"`

	// DirWorkers is the number of concurrent workers for directory traversal.
	DirWorkers int `json:"dir_workers"`

	// FileWorkers is the number of concurrent workers for file stat operations.
	FileWorkers int `json:"file_workers"`
}

// ScanProgress reports real-time scan progress.
// It provides a snapshot of the current scan state for progress reporting.
type ScanProgress struct {
	// DirsScanned is the number of directories processed so far.
	DirsScanned int64 `json:"dirs_scanned"`

	// FilesScanned is the number of files examined so far.
	FilesScanned int64 `json:"files_scanned"`

	// LargeFiles is the number of files found exceeding the minimum size threshold.
	LargeFiles int64 `json:"large_files"`

	// CurrentPath is the path currently being scanned.
	CurrentPath string `json:"current_path"`

	// BytesScanned is the total bytes of all files examined so far.
	BytesScanned int64 `json:"bytes_scanned"`

	// WalkComplete indicates that directory traversal is finished.
	// The TUI uses this to freeze the displayed elapsed time.
	WalkComplete bool `json:"walk_complete,omitempty"`
}

// sizePattern matches size strings like "100M", "2G", "500K", "1.5GB", etc.
var sizePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([KMGT]?(?:i?B)?)\s*$`)

// ErrInvalidSize indicates that the size string could not be parsed.
var ErrInvalidSize = errors.New("invalid size format")

// ErrNegativeSize indicates that a negative size value was provided.
var ErrNegativeSize = errors.New("size cannot be negative")

// ParseSize parses a human-readable size string and returns the size in bytes.
// It supports the following formats:
//   - Plain bytes: "1024", "0"
//   - With byte suffix: "512B", "512b"
//   - Kilobytes: "100K", "100k", "100KB", "100KiB"
//   - Megabytes: "50M", "50m", "50MB", "50MiB"
//   - Gigabytes: "2G", "2g", "2GB", "2GiB"
//   - Terabytes: "1T", "1t", "1TB", "1TiB"
//
// Decimal values are supported and truncated to the nearest byte.
// Leading and trailing whitespace is ignored.
//
// Returns ErrInvalidSize if the format is not recognized.
// Returns ErrNegativeSize if the value is negative.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidSize)
	}

	// Check for negative values
	if strings.HasPrefix(s, "-") {
		return 0, ErrNegativeSize
	}

	matches := sizePattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidSize, s)
	}

	// Parse the numeric value
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidSize, s)
	}

	// Determine the multiplier based on the suffix
	suffix := strings.ToUpper(matches[2])
	// Remove 'B' or 'IB' suffix to get just the unit letter
	suffix = strings.TrimSuffix(suffix, "IB")
	suffix = strings.TrimSuffix(suffix, "B")

	var multiplier int64
	switch suffix {
	case "":
		multiplier = 1
	case "K":
		multiplier = KiB
	case "M":
		multiplier = MiB
	case "G":
		multiplier = GiB
	case "T":
		multiplier = TiB
	default:
		return 0, fmt.Errorf("%w: unknown suffix %q", ErrInvalidSize, suffix)
	}

	return int64(value * float64(multiplier)), nil
}

// FormatSize converts a size in bytes to a human-readable string.
// It uses binary (IEC) units (KiB, MiB, GiB, TiB) for consistency
// with common filesystem tools.
//
// Examples:
//   - FormatSize(0) returns "0 B"
//   - FormatSize(1024) returns "1.0 KiB"
//   - FormatSize(1536*1024) returns "1.5 MiB"
func FormatSize(bytes int64) string {
	return humanize.IBytes(uint64(bytes))
}
