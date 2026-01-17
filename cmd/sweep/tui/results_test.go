package tui

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

func TestNewResultModel(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
	}

	m := NewResultModel(files)

	if len(m.files) != 2 {
		t.Errorf("expected 2 files, got %d", len(m.files))
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.cursor)
	}
	if m.HasSelection() {
		t.Error("expected no selection initially")
	}
}

func TestResultModelToggle(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
	}

	m := NewResultModel(files)

	// Toggle first file
	m.Toggle(0)
	if !m.selected[0] {
		t.Error("expected file 0 to be selected")
	}
	if m.SelectedCount() != 1 {
		t.Errorf("expected 1 selected, got %d", m.SelectedCount())
	}

	// Toggle again to deselect
	m.Toggle(0)
	if m.selected[0] {
		t.Error("expected file 0 to be deselected")
	}
	if m.SelectedCount() != 0 {
		t.Errorf("expected 0 selected, got %d", m.SelectedCount())
	}
}

func TestResultModelSelectAll(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
		{Path: "/test/file3.txt", Size: 300 * types.MiB},
	}

	m := NewResultModel(files)
	m.SelectAll()

	if m.SelectedCount() != 3 {
		t.Errorf("expected 3 selected, got %d", m.SelectedCount())
	}

	for i := range files {
		if !m.selected[i] {
			t.Errorf("expected file %d to be selected", i)
		}
	}
}

func TestResultModelSelectNone(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
	}

	m := NewResultModel(files)
	m.SelectAll()
	m.SelectNone()

	if m.SelectedCount() != 0 {
		t.Errorf("expected 0 selected, got %d", m.SelectedCount())
	}
	if m.HasSelection() {
		t.Error("expected no selection after SelectNone")
	}
}

func TestResultModelSelectedSize(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
		{Path: "/test/file3.txt", Size: 300 * types.MiB},
	}

	m := NewResultModel(files)
	m.Toggle(0)
	m.Toggle(2)

	expectedSize := int64(400 * types.MiB)
	if m.SelectedSize() != expectedSize {
		t.Errorf("expected selected size %d, got %d", expectedSize, m.SelectedSize())
	}
}

func TestResultModelTotalSize(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
		{Path: "/test/file3.txt", Size: 300 * types.MiB},
	}

	m := NewResultModel(files)

	expectedSize := int64(600 * types.MiB)
	if m.TotalSize() != expectedSize {
		t.Errorf("expected total size %d, got %d", expectedSize, m.TotalSize())
	}
}

func TestResultModelSelectedFiles(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
		{Path: "/test/file3.txt", Size: 300 * types.MiB},
	}

	m := NewResultModel(files)
	m.Toggle(0)
	m.Toggle(2)

	selected := m.SelectedFiles()
	if len(selected) != 2 {
		t.Errorf("expected 2 selected files, got %d", len(selected))
	}

	// Verify the correct files are returned
	paths := make(map[string]bool)
	for _, f := range selected {
		paths[f.Path] = true
	}
	if !paths["/test/file1.txt"] || !paths["/test/file3.txt"] {
		t.Error("expected file1.txt and file3.txt to be selected")
	}
}

func TestResultModelHandleKey(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
		{Path: "/test/file3.txt", Size: 300 * types.MiB},
	}

	m := NewResultModel(files)

	// Test down navigation
	m.HandleKey("down")
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", m.cursor)
	}

	// Test j (vim-style down)
	m.HandleKey("j")
	if m.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", m.cursor)
	}

	// Test up navigation
	m.HandleKey("up")
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", m.cursor)
	}

	// Test k (vim-style up)
	m.HandleKey("k")
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.cursor)
	}

	// Test space to toggle
	m.HandleKey(" ")
	if !m.selected[0] {
		t.Error("expected file 0 to be selected after space")
	}

	// Test 'a' for select all
	m.HandleKey("a")
	if m.SelectedCount() != 3 {
		t.Errorf("expected 3 selected after 'a', got %d", m.SelectedCount())
	}

	// Test 'n' for select none
	m.HandleKey("n")
	if m.SelectedCount() != 0 {
		t.Errorf("expected 0 selected after 'n', got %d", m.SelectedCount())
	}
}

func TestResultModelEmptyFiles(t *testing.T) {
	m := NewResultModel([]types.FileInfo{})

	if m.HasSelection() {
		t.Error("expected no selection for empty files")
	}
	if m.SelectedSize() != 0 {
		t.Error("expected 0 selected size for empty files")
	}
	if m.TotalSize() != 0 {
		t.Error("expected 0 total size for empty files")
	}

	// Navigation should not panic
	m.HandleKey("down")
	m.HandleKey("up")
	m.HandleKey(" ")
}

func TestResultModelBoundaryNavigation(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB},
		{Path: "/test/file2.txt", Size: 200 * types.MiB},
	}

	m := NewResultModel(files)

	// Can't go up from first item
	m.HandleKey("up")
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0 (boundary), got %d", m.cursor)
	}

	// Go to last item
	m.HandleKey("down")
	m.HandleKey("down")
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", m.cursor)
	}

	// Can't go past last item
	m.HandleKey("down")
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1 (boundary), got %d", m.cursor)
	}
}

func TestResultModelView(t *testing.T) {
	files := []types.FileInfo{
		{Path: "/test/file1.txt", Size: 100 * types.MiB, ModTime: time.Now()},
	}

	m := NewResultModel(files)
	m.SetDimensions(80, 24)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestResultModelEmptyView(t *testing.T) {
	m := NewResultModel([]types.FileInfo{})
	m.SetDimensions(80, 24)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for empty file list")
	}
}
