// Package manifest provides manifest logging for disk analyzer operations.
package manifest

import "time"

// OperationType represents the type of operation.
type OperationType string

const (
	// OpScan represents a scan operation.
	OpScan OperationType = "scan"
	// OpDelete represents a delete operation.
	OpDelete OperationType = "delete"
)

// Entry represents a single manifest entry.
type Entry struct {
	ID        string        `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	Operation OperationType `json:"operation"`
	Files     []FileRecord  `json:"files"`
	Summary   Summary       `json:"summary"`
}

// FileRecord represents a file in the manifest.
type FileRecord struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	SHA256    string    `json:"sha256,omitempty"`     // Optional checksum
	DeletedAt time.Time `json:"deleted_at,omitempty"` // Set when file is deleted
}

// Summary contains operation summary.
type Summary struct {
	TotalFiles int64 `json:"total_files"`
	TotalBytes int64 `json:"total_bytes"`
}
