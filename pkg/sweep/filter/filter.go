package filter

import (
	"strings"
	"time"
)

// Filter defines criteria for filtering, sorting, and limiting file lists.
type Filter struct {
	// MinSize is the minimum file size in bytes. Files smaller are excluded.
	MinSize int64

	// Include contains glob patterns. If non-empty, files must match at least one.
	Include []string

	// Exclude contains glob patterns. Matching files are excluded.
	Exclude []string

	// Extensions contains file extensions to include (e.g., ".mp4", ".mkv").
	// If non-empty, only files with matching extensions are included.
	Extensions []string

	// OlderThan excludes files modified more recently than this duration ago.
	OlderThan time.Duration

	// NewerThan excludes files modified longer ago than this duration.
	NewerThan time.Duration

	// MaxDepth limits how deep into the directory tree to include files.
	// 0 means unlimited.
	MaxDepth int

	// SortBy specifies the field to sort results by.
	SortBy SortField

	// SortDescending specifies whether to sort in descending order.
	SortDescending bool

	// Limit is the maximum number of files to return. 0 means unlimited.
	Limit int
}

// Option is a functional option for configuring a Filter.
type Option func(*Filter)

// New creates a new Filter with the given options.
// Default values:
//   - Limit: 50
//   - SortBy: SortSize
//   - SortDescending: true
func New(opts ...Option) *Filter {
	f := &Filter{
		Limit:          50,
		SortBy:         SortSize,
		SortDescending: true,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

// WithLimit sets the maximum number of files to return.
// If limit <= 0, it is set to 0 (unlimited).
func WithLimit(limit int) Option {
	return func(f *Filter) {
		if limit < 0 {
			limit = 0
		}
		f.Limit = limit
	}
}

// WithMinSize sets the minimum file size in bytes.
// If minSize < 0, it is set to 0.
func WithMinSize(minSize int64) Option {
	return func(f *Filter) {
		if minSize < 0 {
			minSize = 0
		}
		f.MinSize = minSize
	}
}

// WithInclude sets the include glob patterns.
// If any patterns are specified, files must match at least one to be included.
func WithInclude(patterns ...string) Option {
	return func(f *Filter) {
		f.Include = patterns
	}
}

// WithExclude sets the exclude glob patterns.
// Files matching any pattern are excluded.
func WithExclude(patterns ...string) Option {
	return func(f *Filter) {
		f.Exclude = patterns
	}
}

// WithExtensions sets the file extensions to include.
// Extensions are normalized: lowercase and prefixed with "." if missing.
func WithExtensions(extensions ...string) Option {
	return func(f *Filter) {
		normalized := make([]string, 0, len(extensions))
		for _, ext := range extensions {
			ext = strings.ToLower(ext)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			normalized = append(normalized, ext)
		}
		f.Extensions = normalized
	}
}

// WithTypeGroups expands type group names to their extensions and sets them.
// Unknown group names are silently ignored.
func WithTypeGroups(groups ...string) Option {
	return func(f *Filter) {
		var extensions []string
		for _, group := range groups {
			if exts, ok := TypeGroups[group]; ok {
				extensions = append(extensions, exts...)
			}
		}
		f.Extensions = extensions
	}
}

// WithOlderThan sets the minimum age of files to include.
// Files modified more recently than this duration ago are excluded.
func WithOlderThan(d time.Duration) Option {
	return func(f *Filter) {
		f.OlderThan = d
	}
}

// WithNewerThan sets the maximum age of files to include.
// Files modified longer ago than this duration are excluded.
func WithNewerThan(d time.Duration) Option {
	return func(f *Filter) {
		f.NewerThan = d
	}
}

// WithMaxDepth sets the maximum directory depth to include.
// 0 means unlimited. Negative values are set to 0.
func WithMaxDepth(depth int) Option {
	return func(f *Filter) {
		if depth < 0 {
			depth = 0
		}
		f.MaxDepth = depth
	}
}

// WithSortBy sets the field to sort results by.
func WithSortBy(field SortField) Option {
	return func(f *Filter) {
		f.SortBy = field
	}
}

// WithSortDescending sets whether to sort in descending order.
func WithSortDescending(desc bool) Option {
	return func(f *Filter) {
		f.SortDescending = desc
	}
}
