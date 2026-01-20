// Package filter provides filtering, sorting, and limiting functionality
// for file lists in the sweep disk analyzer. It supports filtering by size,
// age, file type, patterns, and depth, with configurable sorting and limits.
package filter

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// SortField specifies the field to sort files by.
type SortField int

const (
	// SortSize sorts files by size in bytes.
	SortSize SortField = iota
	// SortAge sorts files by modification time.
	SortAge
	// SortPath sorts files by path alphabetically.
	SortPath
)

// Sort field string constants.
const (
	sortFieldSize = "size"
	sortFieldAge  = "age"
	sortFieldPath = "path"
)

// String returns the string representation of the sort field.
func (s SortField) String() string {
	switch s {
	case SortSize:
		return sortFieldSize
	case SortAge:
		return sortFieldAge
	case SortPath:
		return sortFieldPath
	default:
		return sortFieldSize
	}
}

// ErrInvalidSortField indicates that the sort field string could not be parsed.
var ErrInvalidSortField = errors.New("invalid sort field")

// ParseSortField parses a string into a SortField.
// Valid values are "size", "age", and "path" (case-insensitive).
func ParseSortField(s string) (SortField, error) {
	switch strings.ToLower(s) {
	case sortFieldSize:
		return SortSize, nil
	case sortFieldAge:
		return SortAge, nil
	case sortFieldPath:
		return SortPath, nil
	default:
		return SortSize, fmt.Errorf("%w: %q", ErrInvalidSortField, s)
	}
}

// TypeGroups maps file type group names to their associated file extensions.
// Each group contains common extensions for that category.
var TypeGroups = map[string][]string{
	"video": {
		".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpeg", ".mpg",
	},
	"audio": {
		".mp3", ".flac", ".wav", ".aac", ".ogg", ".wma", ".m4a", ".opus", ".aiff", ".alac",
	},
	"image": {
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".tif", ".webp", ".svg", ".ico", ".heic", ".heif", ".raw",
	},
	"archive": {
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".tgz", ".tbz2", ".tar.gz", ".tar.bz2", ".tar.xz",
	},
	"document": {
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp", ".rtf", ".txt", ".epub",
	},
	"code": {
		".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".hpp", ".rs", ".rb", ".php", ".swift", ".kt", ".scala", ".cs", ".sh", ".bash", ".zsh", ".fish",
	},
	"log": {
		".log", ".logs",
	},
}

// FileInfo contains metadata about a file for filtering and sorting.
// It provides all the information needed to apply filter criteria and sort results.
type FileInfo struct {
	// Path is the absolute path to the file.
	Path string

	// Name is the base name of the file (without directory).
	Name string

	// Dir is the directory containing the file.
	Dir string

	// Ext is the file extension including the dot (e.g., ".txt").
	Ext string

	// Size is the file size in bytes.
	Size int64

	// ModTime is the last modification time of the file.
	ModTime time.Time

	// Mode is the file's permission and mode bits.
	Mode os.FileMode

	// Owner is the username of the file's owner.
	Owner string

	// Depth is the directory depth relative to the scan root.
	Depth int
}
