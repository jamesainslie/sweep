package tui

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/filter"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

func TestToFilterFileInfo(t *testing.T) {
	f := types.FileInfo{
		Path:    "/test/path/file.mp4",
		Size:    100 * types.MiB,
		ModTime: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		Owner:   "testuser",
	}

	fi := toFilterFileInfo(f)

	if fi.Path != "/test/path/file.mp4" {
		t.Errorf("expected path /test/path/file.mp4, got %s", fi.Path)
	}
	if fi.Name != "file.mp4" {
		t.Errorf("expected name file.mp4, got %s", fi.Name)
	}
	if fi.Dir != "/test/path" {
		t.Errorf("expected dir /test/path, got %s", fi.Dir)
	}
	if fi.Ext != ".mp4" {
		t.Errorf("expected ext .mp4, got %s", fi.Ext)
	}
	if fi.Size != 100*types.MiB {
		t.Errorf("expected size %d, got %d", 100*types.MiB, fi.Size)
	}
	if fi.Owner != "testuser" {
		t.Errorf("expected owner testuser, got %s", fi.Owner)
	}
}

func TestFromFilterFileInfo(t *testing.T) {
	modTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fi := filter.FileInfo{
		Path:    "/test/path/file.mp4",
		Name:    "file.mp4",
		Dir:     "/test/path",
		Ext:     ".mp4",
		Size:    100 * types.MiB,
		ModTime: modTime,
		Owner:   "testuser",
	}

	f := fromFilterFileInfo(fi)

	if f.Path != "/test/path/file.mp4" {
		t.Errorf("expected path /test/path/file.mp4, got %s", f.Path)
	}
	if f.Size != 100*types.MiB {
		t.Errorf("expected size %d, got %d", 100*types.MiB, f.Size)
	}
	if f.ModTime != modTime {
		t.Errorf("expected modTime %v, got %v", modTime, f.ModTime)
	}
	if f.Owner != "testuser" {
		t.Errorf("expected owner testuser, got %s", f.Owner)
	}
}

func TestFilePassesFilterNilFilter(t *testing.T) {
	m := &Model{
		options: Options{
			Filter: nil,
		},
	}

	// With no filter, all files should pass
	f := types.FileInfo{
		Path: "/test/file.txt",
		Size: 1,
	}

	if !m.filePassesFilter(f) {
		t.Error("expected file to pass with nil filter")
	}
}

func TestFilePassesFilterWithMinSize(t *testing.T) {
	// Create a filter that requires min size of 50MiB
	flt := filter.New(filter.WithMinSize(50 * types.MiB))

	m := &Model{
		options: Options{
			Filter: flt,
		},
	}

	// File that should pass (100 MiB)
	bigFile := types.FileInfo{
		Path: "/test/bigfile.mp4",
		Size: 100 * types.MiB,
	}

	// File that should NOT pass (10 MiB)
	smallFile := types.FileInfo{
		Path: "/test/smallfile.txt",
		Size: 10 * types.MiB,
	}

	if !m.filePassesFilter(bigFile) {
		t.Error("expected big file to pass filter")
	}

	if m.filePassesFilter(smallFile) {
		t.Error("expected small file to NOT pass filter")
	}
}

func TestFilePassesFilterWithExtensions(t *testing.T) {
	// Create a filter that only allows video files
	flt := filter.New(filter.WithExtensions(".mp4", ".mkv"))

	m := &Model{
		options: Options{
			Filter: flt,
		},
	}

	videoFile := types.FileInfo{
		Path: "/test/movie.mp4",
		Size: 100 * types.MiB,
	}

	textFile := types.FileInfo{
		Path: "/test/document.txt",
		Size: 100 * types.MiB,
	}

	if !m.filePassesFilter(videoFile) {
		t.Error("expected video file to pass filter")
	}

	if m.filePassesFilter(textFile) {
		t.Error("expected text file to NOT pass filter")
	}
}

func TestApplyFilterToFilesNilFilter(t *testing.T) {
	m := &Model{
		options: Options{
			Filter: nil,
		},
	}

	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 10 * types.MiB},
		{Path: "/test/file2.txt", Size: 20 * types.MiB},
	}

	result := m.applyFilterToFiles(files)

	// With nil filter, should return the same slice
	if len(result) != 2 {
		t.Errorf("expected 2 files, got %d", len(result))
	}
}

func TestApplyFilterToFilesWithFilter(t *testing.T) {
	// Create a filter that requires min size of 15 MiB and limits to 2
	flt := filter.New(
		filter.WithMinSize(15*types.MiB),
		filter.WithLimit(2),
	)

	m := &Model{
		options: Options{
			Filter: flt,
		},
	}

	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 10 * types.MiB},  // too small
		{Path: "/test/file2.txt", Size: 20 * types.MiB},  // passes
		{Path: "/test/file3.txt", Size: 30 * types.MiB},  // passes
		{Path: "/test/file4.txt", Size: 100 * types.MiB}, // passes but may be limited
	}

	result := m.applyFilterToFiles(files)

	// Should have 2 files (limited) - all >= 15 MiB
	if len(result) != 2 {
		t.Errorf("expected 2 files, got %d", len(result))
	}

	// Files should be sorted by size (largest first by default)
	if result[0].Size != 100*types.MiB {
		t.Errorf("expected first file to be 100 MiB, got %d", result[0].Size)
	}
	if result[1].Size != 30*types.MiB {
		t.Errorf("expected second file to be 30 MiB, got %d", result[1].Size)
	}
}

func TestApplyFilterToFilesEmptySlice(t *testing.T) {
	flt := filter.New()

	m := &Model{
		options: Options{
			Filter: flt,
		},
	}

	result := m.applyFilterToFiles([]types.FileInfo{})

	if len(result) != 0 {
		t.Errorf("expected 0 files, got %d", len(result))
	}
}

func TestOptionsWithFilter(t *testing.T) {
	// Test that Options struct can hold a filter
	flt := filter.New(filter.WithMinSize(100 * types.MiB))

	opts := Options{
		Root:    "/test",
		MinSize: 50 * types.MiB,
		Filter:  flt,
	}

	if opts.Filter == nil {
		t.Error("expected Filter to be set")
	}

	if opts.Filter.MinSize != 100*types.MiB {
		t.Errorf("expected filter MinSize to be 100 MiB, got %d", opts.Filter.MinSize)
	}
}
