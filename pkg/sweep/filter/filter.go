package filter

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/gobwas/glob"
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

// Match returns true if the file matches all filter criteria.
// It checks MinSize, Extensions, OlderThan, NewerThan, MaxDepth,
// Exclude patterns, and Include patterns in that order.
func (f *Filter) Match(fi FileInfo) bool {
	if !f.matchSize(fi) {
		return false
	}
	if !f.matchExtension(fi) {
		return false
	}
	if !f.matchDepth(fi) {
		return false
	}
	if !f.matchAge(fi) {
		return false
	}
	if !f.matchPatterns(fi) {
		return false
	}
	return true
}

// matchSize checks if the file meets the minimum size requirement.
func (f *Filter) matchSize(fi FileInfo) bool {
	return f.MinSize <= 0 || fi.Size >= f.MinSize
}

// matchExtension checks if the file has an allowed extension.
func (f *Filter) matchExtension(fi FileInfo) bool {
	if len(f.Extensions) == 0 {
		return true
	}
	ext := strings.ToLower(fi.Ext)
	for _, e := range f.Extensions {
		if e == ext {
			return true
		}
	}
	return false
}

// matchDepth checks if the file is within the maximum depth.
func (f *Filter) matchDepth(fi FileInfo) bool {
	return f.MaxDepth <= 0 || fi.Depth <= f.MaxDepth
}

// matchAge checks if the file meets the age requirements.
func (f *Filter) matchAge(fi FileInfo) bool {
	now := time.Now()

	// Check older than (file must be older than this duration)
	if f.OlderThan > 0 {
		threshold := now.Add(-f.OlderThan)
		if fi.ModTime.After(threshold) {
			return false
		}
	}

	// Check newer than (file must be newer than this duration)
	if f.NewerThan > 0 {
		threshold := now.Add(-f.NewerThan)
		if fi.ModTime.Before(threshold) {
			return false
		}
	}

	return true
}

// matchPatterns checks if the file matches include/exclude patterns.
func (f *Filter) matchPatterns(fi FileInfo) bool {
	// Check exclude patterns
	if f.matchesAnyPattern(fi.Path, f.Exclude) {
		return false
	}

	// Check include patterns (if any specified, must match at least one)
	if len(f.Include) > 0 && !f.matchesAnyPattern(fi.Path, f.Include) {
		return false
	}

	return true
}

// matchesAnyPattern returns true if the path matches any of the glob patterns.
func (f *Filter) matchesAnyPattern(path string, patterns []string) bool {
	for _, pattern := range patterns {
		g, err := glob.Compile(pattern, '/')
		if err != nil {
			continue // Skip invalid patterns
		}
		if g.Match(path) {
			return true
		}
	}
	return false
}

// Sort returns a sorted copy of the files slice based on the filter's sort settings.
// The original slice is not modified.
func (f *Filter) Sort(files []FileInfo) []FileInfo {
	if len(files) == 0 {
		return []FileInfo{}
	}

	// Make a copy to avoid modifying the original
	sorted := make([]FileInfo, len(files))
	copy(sorted, files)

	slices.SortFunc(sorted, func(a, b FileInfo) int {
		var result int
		switch f.SortBy {
		case SortSize:
			result = cmp.Compare(a.Size, b.Size)
		case SortAge:
			// Sort by age: compare timestamps
			// ModTime.Compare returns -1 if a is older, 1 if a is newer
			// We want to treat "age" as the actual age value (older = higher age)
			// So we negate to get: older files have higher "age" values
			result = -a.ModTime.Compare(b.ModTime)
		case SortPath:
			result = cmp.Compare(a.Path, b.Path)
		default:
			result = cmp.Compare(a.Size, b.Size)
		}

		if f.SortDescending {
			return -result
		}
		return result
	})

	return sorted
}

// Apply runs the complete filtering pipeline: Match, Sort, and Limit.
// It returns a new slice containing only the files that pass all filters,
// sorted according to the filter settings, and limited to the specified count.
func (f *Filter) Apply(files []FileInfo) []FileInfo {
	// Filter
	var matched []FileInfo
	for _, fi := range files {
		if f.Match(fi) {
			matched = append(matched, fi)
		}
	}

	// Sort
	sorted := f.Sort(matched)

	// Limit
	if f.Limit > 0 && len(sorted) > f.Limit {
		return sorted[:f.Limit]
	}

	return sorted
}
