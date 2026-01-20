package filter

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	f := New()

	// Verify defaults
	if f.Limit != 50 {
		t.Errorf("Limit = %d, want 50", f.Limit)
	}
	if f.SortBy != SortSize {
		t.Errorf("SortBy = %v, want SortSize", f.SortBy)
	}
	if !f.SortDescending {
		t.Error("SortDescending should be true by default")
	}
	if f.MinSize != 0 {
		t.Errorf("MinSize = %d, want 0", f.MinSize)
	}
	if f.MaxDepth != 0 {
		t.Errorf("MaxDepth = %d, want 0", f.MaxDepth)
	}
	if len(f.Include) != 0 {
		t.Errorf("Include = %v, want empty", f.Include)
	}
	if len(f.Exclude) != 0 {
		t.Errorf("Exclude = %v, want empty", f.Exclude)
	}
	if len(f.Extensions) != 0 {
		t.Errorf("Extensions = %v, want empty", f.Extensions)
	}
}

func TestWithLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "positive limit", limit: 100, want: 100},
		{name: "zero limit (unlimited)", limit: 0, want: 0},
		{name: "negative becomes zero", limit: -1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(WithLimit(tt.limit))
			if f.Limit != tt.want {
				t.Errorf("Limit = %d, want %d", f.Limit, tt.want)
			}
		})
	}
}

func TestWithMinSize(t *testing.T) {
	tests := []struct {
		name    string
		minSize int64
		want    int64
	}{
		{name: "positive size", minSize: 1024 * 1024, want: 1024 * 1024},
		{name: "zero size", minSize: 0, want: 0},
		{name: "negative becomes zero", minSize: -1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(WithMinSize(tt.minSize))
			if f.MinSize != tt.want {
				t.Errorf("MinSize = %d, want %d", f.MinSize, tt.want)
			}
		})
	}
}

func TestWithInclude(t *testing.T) {
	patterns := []string{"*.mp4", "*.mkv"}
	f := New(WithInclude(patterns...))

	if len(f.Include) != 2 {
		t.Errorf("Include length = %d, want 2", len(f.Include))
	}
	if f.Include[0] != "*.mp4" || f.Include[1] != "*.mkv" {
		t.Errorf("Include = %v, want %v", f.Include, patterns)
	}
}

func TestWithExclude(t *testing.T) {
	patterns := []string{"**/node_modules/**", "**/.git/**"}
	f := New(WithExclude(patterns...))

	if len(f.Exclude) != 2 {
		t.Errorf("Exclude length = %d, want 2", len(f.Exclude))
	}
	if f.Exclude[0] != "**/node_modules/**" || f.Exclude[1] != "**/.git/**" {
		t.Errorf("Exclude = %v, want %v", f.Exclude, patterns)
	}
}

func TestWithExtensions(t *testing.T) {
	exts := []string{".mp4", ".mkv", ".avi"}
	f := New(WithExtensions(exts...))

	if len(f.Extensions) != 3 {
		t.Errorf("Extensions length = %d, want 3", len(f.Extensions))
	}
	// Extensions should be normalized to lowercase with dot prefix
	for i, ext := range exts {
		if f.Extensions[i] != ext {
			t.Errorf("Extensions[%d] = %q, want %q", i, f.Extensions[i], ext)
		}
	}
}

func TestWithExtensions_Normalization(t *testing.T) {
	// Test that extensions are normalized: lowercase and dot prefix added
	f := New(WithExtensions("MP4", "mkv", ".AVI", "txt"))

	expected := []string{".mp4", ".mkv", ".avi", ".txt"}
	if len(f.Extensions) != len(expected) {
		t.Fatalf("Extensions length = %d, want %d", len(f.Extensions), len(expected))
	}
	for i, ext := range expected {
		if f.Extensions[i] != ext {
			t.Errorf("Extensions[%d] = %q, want %q", i, f.Extensions[i], ext)
		}
	}
}

func TestWithTypeGroups(t *testing.T) {
	// Using type groups should expand to their extensions
	f := New(WithTypeGroups("video", "audio"))

	// Should have video extensions + audio extensions
	videoExts := TypeGroups["video"]
	audioExts := TypeGroups["audio"]
	expectedLen := len(videoExts) + len(audioExts)

	if len(f.Extensions) != expectedLen {
		t.Errorf("Extensions length = %d, want %d", len(f.Extensions), expectedLen)
	}

	// Verify some expected extensions are present
	hasMP4 := false
	hasMP3 := false
	for _, ext := range f.Extensions {
		if ext == ".mp4" {
			hasMP4 = true
		}
		if ext == ".mp3" {
			hasMP3 = true
		}
	}
	if !hasMP4 {
		t.Error("Expected .mp4 extension from video type group")
	}
	if !hasMP3 {
		t.Error("Expected .mp3 extension from audio type group")
	}
}

func TestWithTypeGroups_InvalidGroup(t *testing.T) {
	// Invalid groups should be silently ignored
	f := New(WithTypeGroups("nonexistent", "video"))

	videoExts := TypeGroups["video"]
	if len(f.Extensions) != len(videoExts) {
		t.Errorf("Extensions length = %d, want %d", len(f.Extensions), len(videoExts))
	}
}

func TestWithOlderThan(t *testing.T) {
	dur := 24 * time.Hour
	f := New(WithOlderThan(dur))

	if f.OlderThan != dur {
		t.Errorf("OlderThan = %v, want %v", f.OlderThan, dur)
	}
}

func TestWithNewerThan(t *testing.T) {
	dur := 7 * 24 * time.Hour
	f := New(WithNewerThan(dur))

	if f.NewerThan != dur {
		t.Errorf("NewerThan = %v, want %v", f.NewerThan, dur)
	}
}

func TestWithMaxDepth(t *testing.T) {
	tests := []struct {
		name     string
		maxDepth int
		want     int
	}{
		{name: "positive depth", maxDepth: 5, want: 5},
		{name: "zero depth (unlimited)", maxDepth: 0, want: 0},
		{name: "negative becomes zero", maxDepth: -1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(WithMaxDepth(tt.maxDepth))
			if f.MaxDepth != tt.want {
				t.Errorf("MaxDepth = %d, want %d", f.MaxDepth, tt.want)
			}
		})
	}
}

func TestWithSortBy(t *testing.T) {
	tests := []struct {
		name   string
		sortBy SortField
		want   SortField
	}{
		{name: "sort by size", sortBy: SortSize, want: SortSize},
		{name: "sort by age", sortBy: SortAge, want: SortAge},
		{name: "sort by path", sortBy: SortPath, want: SortPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(WithSortBy(tt.sortBy))
			if f.SortBy != tt.want {
				t.Errorf("SortBy = %v, want %v", f.SortBy, tt.want)
			}
		})
	}
}

func TestWithSortDescending(t *testing.T) {
	tests := []struct {
		name string
		desc bool
		want bool
	}{
		{name: "descending true", desc: true, want: true},
		{name: "descending false", desc: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(WithSortDescending(tt.desc))
			if f.SortDescending != tt.want {
				t.Errorf("SortDescending = %v, want %v", f.SortDescending, tt.want)
			}
		})
	}
}

func TestMultipleOptions(t *testing.T) {
	f := New(
		WithLimit(25),
		WithMinSize(1024*1024),
		WithExtensions(".mp4", ".mkv"),
		WithSortBy(SortAge),
		WithSortDescending(false),
		WithMaxDepth(3),
	)

	if f.Limit != 25 {
		t.Errorf("Limit = %d, want 25", f.Limit)
	}
	if f.MinSize != 1024*1024 {
		t.Errorf("MinSize = %d, want %d", f.MinSize, 1024*1024)
	}
	if len(f.Extensions) != 2 {
		t.Errorf("Extensions length = %d, want 2", len(f.Extensions))
	}
	if f.SortBy != SortAge {
		t.Errorf("SortBy = %v, want SortAge", f.SortBy)
	}
	if f.SortDescending {
		t.Error("SortDescending should be false")
	}
	if f.MaxDepth != 3 {
		t.Errorf("MaxDepth = %d, want 3", f.MaxDepth)
	}
}

// TestMatch_MinSize tests the Match method with MinSize filter.
func TestMatch_MinSize(t *testing.T) {
	f := New(WithMinSize(1024))

	tests := []struct {
		name string
		size int64
		want bool
	}{
		{name: "above threshold", size: 2048, want: true},
		{name: "at threshold", size: 1024, want: true},
		{name: "below threshold", size: 512, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{Size: tt.size}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_Extensions(t *testing.T) {
	f := New(WithExtensions(".mp4", ".mkv"))

	tests := []struct {
		name string
		ext  string
		want bool
	}{
		{name: "matching mp4", ext: ".mp4", want: true},
		{name: "matching mkv", ext: ".mkv", want: true},
		{name: "non-matching avi", ext: ".avi", want: false},
		{name: "no extension", ext: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{Ext: tt.ext}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_MaxDepth(t *testing.T) {
	f := New(WithMaxDepth(2))

	tests := []struct {
		name  string
		depth int
		want  bool
	}{
		{name: "depth 0", depth: 0, want: true},
		{name: "depth 1", depth: 1, want: true},
		{name: "depth 2", depth: 2, want: true},
		{name: "depth 3", depth: 3, want: false},
		{name: "depth 10", depth: 10, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{Depth: tt.depth}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_OlderThan(t *testing.T) {
	now := time.Now()
	f := New(WithOlderThan(24 * time.Hour))

	tests := []struct {
		name    string
		modTime time.Time
		want    bool
	}{
		{name: "modified 2 days ago", modTime: now.Add(-48 * time.Hour), want: true},
		{name: "modified 1 day ago", modTime: now.Add(-24 * time.Hour), want: true},
		{name: "modified 12 hours ago", modTime: now.Add(-12 * time.Hour), want: false},
		{name: "modified now", modTime: now, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{ModTime: tt.modTime}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_NewerThan(t *testing.T) {
	now := time.Now()
	f := New(WithNewerThan(24 * time.Hour))

	tests := []struct {
		name    string
		modTime time.Time
		want    bool
	}{
		{name: "modified 2 days ago", modTime: now.Add(-48 * time.Hour), want: false},
		{name: "modified 23 hours ago", modTime: now.Add(-23 * time.Hour), want: true},
		{name: "modified 12 hours ago", modTime: now.Add(-12 * time.Hour), want: true},
		{name: "modified now", modTime: now, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{ModTime: tt.modTime}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_Exclude(t *testing.T) {
	f := New(WithExclude("**/node_modules/**", "**/*.tmp"))

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "normal file", path: "/home/user/project/file.txt", want: true},
		{name: "node_modules file", path: "/home/user/project/node_modules/pkg/index.js", want: false},
		{name: "tmp file", path: "/home/user/cache.tmp", want: false},
		{name: "similar to node_modules", path: "/home/user/node_modules_backup/file.txt", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{Path: tt.path}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatch_Include(t *testing.T) {
	f := New(WithInclude("**/*.mp4", "**/*.mkv", "**/videos/**"))

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "matching mp4", path: "/home/user/movie.mp4", want: true},
		{name: "matching mkv", path: "/home/user/show.mkv", want: true},
		{name: "matching videos dir", path: "/home/user/videos/clip.avi", want: true},
		{name: "non-matching", path: "/home/user/document.pdf", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := FileInfo{Path: tt.path}
			got := f.Match(fi)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatch_CombinedFilters(t *testing.T) {
	now := time.Now()
	f := New(
		WithMinSize(1024),
		WithExtensions(".mp4", ".mkv"),
		WithMaxDepth(3),
		WithOlderThan(24*time.Hour),
		WithExclude("**/cache/**"),
	)

	tests := []struct {
		name string
		fi   FileInfo
		want bool
	}{
		{
			name: "passes all filters",
			fi: FileInfo{
				Path:    "/home/user/videos/movie.mp4",
				Ext:     ".mp4",
				Size:    2048,
				ModTime: now.Add(-48 * time.Hour),
				Depth:   2,
			},
			want: true,
		},
		{
			name: "fails size",
			fi: FileInfo{
				Path:    "/home/user/videos/movie.mp4",
				Ext:     ".mp4",
				Size:    512,
				ModTime: now.Add(-48 * time.Hour),
				Depth:   2,
			},
			want: false,
		},
		{
			name: "fails extension",
			fi: FileInfo{
				Path:    "/home/user/videos/doc.pdf",
				Ext:     ".pdf",
				Size:    2048,
				ModTime: now.Add(-48 * time.Hour),
				Depth:   2,
			},
			want: false,
		},
		{
			name: "fails depth",
			fi: FileInfo{
				Path:    "/home/user/videos/sub/sub2/movie.mp4",
				Ext:     ".mp4",
				Size:    2048,
				ModTime: now.Add(-48 * time.Hour),
				Depth:   5,
			},
			want: false,
		},
		{
			name: "fails age",
			fi: FileInfo{
				Path:    "/home/user/videos/movie.mp4",
				Ext:     ".mp4",
				Size:    2048,
				ModTime: now.Add(-12 * time.Hour),
				Depth:   2,
			},
			want: false,
		},
		{
			name: "fails exclude",
			fi: FileInfo{
				Path:    "/home/user/cache/videos/movie.mp4",
				Ext:     ".mp4",
				Size:    2048,
				ModTime: now.Add(-48 * time.Hour),
				Depth:   2,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Match(tt.fi)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatch_NoFilters(t *testing.T) {
	// With no filters, everything should match
	f := New(WithLimit(0)) // Set limit to 0, but Match doesn't use limit

	fi := FileInfo{
		Path:    "/any/path/file.txt",
		Ext:     ".txt",
		Size:    100,
		ModTime: time.Now(),
		Depth:   10,
	}

	if !f.Match(fi) {
		t.Error("Match() should return true when no filters are set")
	}
}

// TestSort_BySize tests sorting files by size.
func TestSort_BySize(t *testing.T) {
	files := []FileInfo{
		{Path: "/a", Size: 100},
		{Path: "/b", Size: 300},
		{Path: "/c", Size: 200},
	}

	t.Run("descending", func(t *testing.T) {
		f := New(WithSortBy(SortSize), WithSortDescending(true))
		sorted := f.Sort(files)

		if sorted[0].Size != 300 || sorted[1].Size != 200 || sorted[2].Size != 100 {
			t.Errorf("Sort descending = %v", sorted)
		}
	})

	t.Run("ascending", func(t *testing.T) {
		f := New(WithSortBy(SortSize), WithSortDescending(false))
		sorted := f.Sort(files)

		if sorted[0].Size != 100 || sorted[1].Size != 200 || sorted[2].Size != 300 {
			t.Errorf("Sort ascending = %v", sorted)
		}
	})
}

// TestSort_ByAge tests sorting files by modification time.
func TestSort_ByAge(t *testing.T) {
	now := time.Now()
	files := []FileInfo{
		{Path: "/a", ModTime: now.Add(-1 * time.Hour)}, // 1 hour ago
		{Path: "/b", ModTime: now.Add(-3 * time.Hour)}, // 3 hours ago (oldest)
		{Path: "/c", ModTime: now.Add(-2 * time.Hour)}, // 2 hours ago
	}

	t.Run("descending (oldest first)", func(t *testing.T) {
		f := New(WithSortBy(SortAge), WithSortDescending(true))
		sorted := f.Sort(files)

		// Descending by age means oldest files first
		if sorted[0].Path != "/b" || sorted[1].Path != "/c" || sorted[2].Path != "/a" {
			t.Errorf("Sort by age descending: got paths %s, %s, %s", sorted[0].Path, sorted[1].Path, sorted[2].Path)
		}
	})

	t.Run("ascending (newest first)", func(t *testing.T) {
		f := New(WithSortBy(SortAge), WithSortDescending(false))
		sorted := f.Sort(files)

		// Ascending by age means newest files first
		if sorted[0].Path != "/a" || sorted[1].Path != "/c" || sorted[2].Path != "/b" {
			t.Errorf("Sort by age ascending: got paths %s, %s, %s", sorted[0].Path, sorted[1].Path, sorted[2].Path)
		}
	})
}

// TestSort_ByPath tests sorting files by path.
func TestSort_ByPath(t *testing.T) {
	files := []FileInfo{
		{Path: "/home/user/charlie.txt"},
		{Path: "/home/user/alice.txt"},
		{Path: "/home/user/bob.txt"},
	}

	t.Run("descending", func(t *testing.T) {
		f := New(WithSortBy(SortPath), WithSortDescending(true))
		sorted := f.Sort(files)

		if sorted[0].Path != "/home/user/charlie.txt" || sorted[2].Path != "/home/user/alice.txt" {
			t.Errorf("Sort by path descending = %v", sorted)
		}
	})

	t.Run("ascending", func(t *testing.T) {
		f := New(WithSortBy(SortPath), WithSortDescending(false))
		sorted := f.Sort(files)

		if sorted[0].Path != "/home/user/alice.txt" || sorted[2].Path != "/home/user/charlie.txt" {
			t.Errorf("Sort by path ascending = %v", sorted)
		}
	})
}

// TestSort_DoesNotModifyOriginal tests that Sort returns a new slice.
func TestSort_DoesNotModifyOriginal(t *testing.T) {
	files := []FileInfo{
		{Path: "/a", Size: 100},
		{Path: "/b", Size: 300},
		{Path: "/c", Size: 200},
	}

	f := New(WithSortBy(SortSize), WithSortDescending(true))
	_ = f.Sort(files)

	// Original should be unchanged
	if files[0].Size != 100 || files[1].Size != 300 || files[2].Size != 200 {
		t.Error("Sort modified original slice")
	}
}

// TestSort_EmptySlice tests sorting an empty slice.
func TestSort_EmptySlice(t *testing.T) {
	f := New()
	sorted := f.Sort([]FileInfo{})

	if len(sorted) != 0 {
		t.Error("Sort of empty slice should return empty slice")
	}
}

// TestApply tests the complete Apply method.
func TestApply(t *testing.T) {
	now := time.Now()
	files := []FileInfo{
		{Path: "/a.mp4", Ext: ".mp4", Size: 500, ModTime: now, Depth: 1},
		{Path: "/b.mp4", Ext: ".mp4", Size: 300, ModTime: now, Depth: 1},
		{Path: "/c.txt", Ext: ".txt", Size: 400, ModTime: now, Depth: 1}, // Wrong extension
		{Path: "/d.mp4", Ext: ".mp4", Size: 100, ModTime: now, Depth: 1}, // Too small
		{Path: "/e.mp4", Ext: ".mp4", Size: 600, ModTime: now, Depth: 1},
		{Path: "/f.mp4", Ext: ".mp4", Size: 700, ModTime: now, Depth: 1},
	}

	f := New(
		WithMinSize(200),
		WithExtensions(".mp4"),
		WithSortBy(SortSize),
		WithSortDescending(true),
		WithLimit(3),
	)

	result := f.Apply(files)

	// Should filter out c.txt (wrong ext) and d.mp4 (too small)
	// Then sort by size descending: f(700), e(600), a(500), b(300)
	// Then limit to 3: f, e, a
	if len(result) != 3 {
		t.Fatalf("Apply result length = %d, want 3", len(result))
	}
	if result[0].Size != 700 {
		t.Errorf("result[0].Size = %d, want 700", result[0].Size)
	}
	if result[1].Size != 600 {
		t.Errorf("result[1].Size = %d, want 600", result[1].Size)
	}
	if result[2].Size != 500 {
		t.Errorf("result[2].Size = %d, want 500", result[2].Size)
	}
}

// TestApply_NoLimit tests Apply with no limit (Limit=0).
func TestApply_NoLimit(t *testing.T) {
	files := []FileInfo{
		{Path: "/a", Size: 100},
		{Path: "/b", Size: 200},
		{Path: "/c", Size: 300},
	}

	f := New(WithLimit(0)) // Unlimited
	result := f.Apply(files)

	if len(result) != 3 {
		t.Errorf("Apply with no limit: got %d results, want 3", len(result))
	}
}

// TestApply_LimitLargerThanResults tests Apply when limit exceeds results.
func TestApply_LimitLargerThanResults(t *testing.T) {
	files := []FileInfo{
		{Path: "/a", Size: 100},
		{Path: "/b", Size: 200},
	}

	f := New(WithLimit(100))
	result := f.Apply(files)

	if len(result) != 2 {
		t.Errorf("Apply: got %d results, want 2", len(result))
	}
}

// TestApply_AllFiltered tests Apply when all files are filtered out.
func TestApply_AllFiltered(t *testing.T) {
	files := []FileInfo{
		{Path: "/a.txt", Ext: ".txt", Size: 100},
		{Path: "/b.txt", Ext: ".txt", Size: 200},
	}

	f := New(WithExtensions(".mp4")) // Filter out all .txt files
	result := f.Apply(files)

	if len(result) != 0 {
		t.Errorf("Apply: got %d results, want 0", len(result))
	}
}
