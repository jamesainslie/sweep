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
