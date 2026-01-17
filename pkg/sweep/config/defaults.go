// Package config provides configuration management for the sweep disk analyzer.
package config

// Default configuration values for sweep.
const (
	// DefaultMinSize is the minimum file size to include in scans.
	DefaultMinSize = "100MB"

	// DefaultPath is the default path to scan when none is specified.
	DefaultPath = "."

	// DefaultConfigDir is the default configuration directory path.
	DefaultConfigDir = "~/.config/sweep"

	// DefaultManifestDir is the default directory for manifest files.
	DefaultManifestDir = "~/.config/sweep/.manifest"

	// DefaultRetentionDays is the default number of days to retain manifests.
	DefaultRetentionDays = 30

	// DefaultDirWorkers is the default number of directory walker workers.
	DefaultDirWorkers = 4

	// DefaultFileWorkers is the default number of file processing workers.
	DefaultFileWorkers = 8
)

// DefaultExclusions contains paths that should be excluded from scanning by default.
var DefaultExclusions = []string{
	"/proc",
	"/sys",
	"/dev",
}
