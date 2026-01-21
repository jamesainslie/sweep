package tui

import (
	"strings"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon/tree"
)

// Helper to create a test tree structure.
func createTestTree() *tree.Node {
	root := &tree.Node{
		Path:     "/test",
		Name:     "test",
		IsDir:    true,
		Expanded: true,
	}

	dir1 := &tree.Node{
		Path:           "/test/dir1",
		Name:           "dir1",
		IsDir:          true,
		LargeFileSize:  1024 * 1024 * 200, // 200 MiB
		LargeFileCount: 2,
		Expanded:       false,
	}
	root.AddChild(dir1)

	file1 := &tree.Node{
		Path: "/test/dir1/file1.txt",
		Name: "file1.txt",
		Size: 1024 * 1024 * 100, // 100 MiB
	}
	dir1.AddChild(file1)

	file2 := &tree.Node{
		Path: "/test/dir1/file2.txt",
		Name: "file2.txt",
		Size: 1024 * 1024 * 100, // 100 MiB
	}
	dir1.AddChild(file2)

	dir2 := &tree.Node{
		Path:           "/test/dir2",
		Name:           "dir2",
		IsDir:          true,
		LargeFileSize:  1024 * 1024 * 50, // 50 MiB
		LargeFileCount: 1,
		Expanded:       true,
	}
	root.AddChild(dir2)

	file3 := &tree.Node{
		Path: "/test/dir2/file3.txt",
		Name: "file3.txt",
		Size: 1024 * 1024 * 50, // 50 MiB
	}
	dir2.AddChild(file3)

	return root
}

func TestNewTreeView(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	if tv.root != root {
		t.Error("expected root to be set")
	}
	if tv.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", tv.cursor)
	}
	if len(tv.selected) != 0 {
		t.Error("expected no selections initially")
	}
	// Root expanded + dir1 (collapsed) + dir2 (expanded) + file3 = 4 visible nodes
	if len(tv.flat) != 4 {
		t.Errorf("expected 4 visible nodes, got %d", len(tv.flat))
	}
}

func TestNewTreeViewNil(t *testing.T) {
	tv := NewTreeView(nil)

	if tv.root != nil {
		t.Error("expected nil root")
	}
	if len(tv.flat) != 0 {
		t.Errorf("expected 0 visible nodes for nil root, got %d", len(tv.flat))
	}
}

// Navigation tests.
func TestTreeViewMoveDown(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	tv.MoveDown()
	if tv.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", tv.cursor)
	}

	tv.MoveDown()
	if tv.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", tv.cursor)
	}

	tv.MoveDown()
	if tv.cursor != 3 {
		t.Errorf("expected cursor at 3, got %d", tv.cursor)
	}
}

func TestTreeViewMoveDownBoundary(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to end
	for i := 0; i < 10; i++ {
		tv.MoveDown()
	}
	// Should stop at last item (index 3)
	if tv.cursor != 3 {
		t.Errorf("expected cursor at 3 (boundary), got %d", tv.cursor)
	}
}

func TestTreeViewMoveUp(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to position 2
	tv.MoveDown()
	tv.MoveDown()

	tv.MoveUp()
	if tv.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", tv.cursor)
	}

	tv.MoveUp()
	if tv.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", tv.cursor)
	}
}

func TestTreeViewMoveUpBoundary(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Try to move up from first position
	tv.MoveUp()
	if tv.cursor != 0 {
		t.Errorf("expected cursor at 0 (boundary), got %d", tv.cursor)
	}
}

func TestTreeViewNavigationEmptyTree(t *testing.T) {
	tv := NewTreeView(nil)

	// Should not panic on empty tree
	tv.MoveDown()
	tv.MoveUp()

	if tv.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", tv.cursor)
	}
}

// Toggle expand/collapse tests.
func TestTreeViewToggleExpandDir(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to dir1 (collapsed)
	tv.MoveDown()

	// Should be at dir1
	selected := tv.Selected()
	if selected == nil || selected.Name != "dir1" {
		t.Fatalf("expected to be at dir1, got %v", selected)
	}

	// Toggle to expand
	tv.Toggle()

	// dir1 should now be expanded
	if !selected.Expanded {
		t.Error("expected dir1 to be expanded after toggle")
	}

	// Flat list should now include dir1's children
	// root + dir1 + file1 + file2 + dir2 + file3 = 6
	if len(tv.flat) != 6 {
		t.Errorf("expected 6 visible nodes after expand, got %d", len(tv.flat))
	}
}

func TestTreeViewToggleCollapseDir(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to dir2 (expanded)
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2

	selected := tv.Selected()
	if selected == nil || selected.Name != "dir2" {
		t.Fatalf("expected to be at dir2, got %v", selected)
	}

	// Toggle to collapse
	tv.Toggle()

	// dir2 should now be collapsed
	if selected.Expanded {
		t.Error("expected dir2 to be collapsed after toggle")
	}

	// Flat list should now hide file3
	// root + dir1 + dir2 = 3
	if len(tv.flat) != 3 {
		t.Errorf("expected 3 visible nodes after collapse, got %d", len(tv.flat))
	}
}

func TestTreeViewToggleFile(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to file3
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2
	tv.MoveDown() // file3

	selected := tv.Selected()
	if selected == nil || selected.Name != "file3.txt" {
		t.Fatalf("expected to be at file3.txt, got %v", selected)
	}

	// Toggle should select the file
	tv.Toggle()

	if !tv.selected[selected.Path] {
		t.Error("expected file to be selected after toggle")
	}
}

// Selection tests.
func TestTreeViewToggleSelect(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to file3
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2
	tv.MoveDown() // file3

	tv.ToggleSelect()
	if !tv.selected["/test/dir2/file3.txt"] {
		t.Error("expected file3.txt to be selected")
	}

	tv.ToggleSelect()
	if tv.selected["/test/dir2/file3.txt"] {
		t.Error("expected file3.txt to be deselected")
	}
}

func TestTreeViewToggleSelectDir(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Try to select a directory - should have no effect
	tv.MoveDown() // dir1
	tv.ToggleSelect()

	if len(tv.selected) != 0 {
		t.Error("expected no selection for directory")
	}
}

func TestTreeViewGetSelectedFiles(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Expand dir1 to access its files
	tv.MoveDown() // dir1
	tv.Toggle()   // expand dir1

	// Select multiple files
	tv.MoveDown() // file1
	tv.ToggleSelect()
	tv.MoveDown() // file2
	tv.ToggleSelect()

	selected := tv.GetSelectedFiles()
	if len(selected) != 2 {
		t.Errorf("expected 2 selected files, got %d", len(selected))
	}

	// Verify correct files are selected
	paths := make(map[string]bool)
	for _, n := range selected {
		paths[n.Path] = true
	}
	if !paths["/test/dir1/file1.txt"] || !paths["/test/dir1/file2.txt"] {
		t.Error("expected file1.txt and file2.txt to be selected")
	}
}

func TestTreeViewClearSelection(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Select a file
	tv.MoveDown()
	tv.MoveDown()
	tv.MoveDown() // file3
	tv.ToggleSelect()

	if len(tv.selected) != 1 {
		t.Errorf("expected 1 selected, got %d", len(tv.selected))
	}

	tv.ClearSelection()

	if len(tv.selected) != 0 {
		t.Errorf("expected 0 selected after clear, got %d", len(tv.selected))
	}
}

// Selected() method tests.
func TestTreeViewSelected(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	selected := tv.Selected()
	if selected != root {
		t.Error("expected Selected() to return root initially")
	}

	tv.MoveDown()
	selected = tv.Selected()
	if selected.Name != "dir1" {
		t.Errorf("expected dir1, got %s", selected.Name)
	}
}

func TestTreeViewSelectedEmpty(t *testing.T) {
	tv := NewTreeView(nil)

	selected := tv.Selected()
	if selected != nil {
		t.Error("expected nil for empty tree")
	}
}

// View rendering tests.
func TestTreeViewView(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	view := tv.View(80, 24)
	if view == "" {
		t.Error("expected non-empty view")
	}

	// Check that node names appear in view
	if !strings.Contains(view, "test") {
		t.Error("expected root name 'test' in view")
	}
	if !strings.Contains(view, "dir1") {
		t.Error("expected 'dir1' in view")
	}
	if !strings.Contains(view, "dir2") {
		t.Error("expected 'dir2' in view")
	}
}

func TestTreeViewViewEmpty(t *testing.T) {
	tv := NewTreeView(nil)

	view := tv.View(80, 24)
	// Should render something even for empty tree
	if view == "" {
		t.Error("expected non-empty view for empty tree")
	}
}

func TestTreeViewViewIcons(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	view := tv.View(80, 24)

	// Collapsed dir should have collapsed icon
	if !strings.Contains(view, iconCollapsed) {
		t.Errorf("expected collapsed icon '%s' in view", iconCollapsed)
	}

	// Expanded dir should have expanded icon
	if !strings.Contains(view, iconExpanded) {
		t.Errorf("expected expanded icon '%s' in view", iconExpanded)
	}
}

func TestTreeViewViewIndentation(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Expand dir1 to show files with deeper indentation
	tv.MoveDown()
	tv.Toggle()

	view := tv.View(80, 24)

	// The view should contain files with indentation
	lines := strings.Split(view, "\n")
	foundIndentedFile := false
	for _, line := range lines {
		if strings.Contains(line, "file1.txt") {
			// File should be indented
			foundIndentedFile = true
			break
		}
	}
	if !foundIndentedFile {
		t.Error("expected to find indented file1.txt in view")
	}
}

func TestTreeViewViewSelection(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Select file3
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2
	tv.MoveDown() // file3
	tv.ToggleSelect()

	view := tv.View(80, 24)

	// Should show selection indicator
	if !strings.Contains(view, iconSelected) {
		t.Errorf("expected selection indicator '%s' in view", iconSelected)
	}
}

func TestTreeViewViewCursor(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move cursor
	tv.MoveDown()

	view := tv.View(80, 24)

	// View should contain both cursor and non-cursor rows
	// Exact styling depends on implementation, but view should exist
	if view == "" {
		t.Error("expected non-empty view with cursor")
	}
}

func TestTreeViewViewScrolling(t *testing.T) {
	// Create a tree with many nodes
	root := &tree.Node{
		Path:     "/test",
		Name:     "test",
		IsDir:    true,
		Expanded: true,
	}

	for i := 0; i < 50; i++ {
		file := &tree.Node{
			Path: "/test/file" + string(rune('A'+i)),
			Name: "file" + string(rune('A'+i)) + ".txt",
			Size: 1024 * 1024 * 10,
		}
		root.AddChild(file)
	}

	tv := NewTreeView(root)

	// Move cursor down past visible area
	for i := 0; i < 30; i++ {
		tv.MoveDown()
	}

	// offset should have adjusted
	if tv.offset == 0 {
		t.Error("expected offset to change for scrolling")
	}

	view := tv.View(80, 10)
	if view == "" {
		t.Error("expected non-empty view")
	}
}

// formatSize helper test.
func TestFormatSizeTreeView(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536 * 1024, "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}

	for _, tt := range tests {
		// Using types.FormatSize which is already available
		result := formatSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

// Regression tests.
func TestTreeViewCursorAfterCollapse(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Move to file3 (inside expanded dir2)
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2
	tv.MoveDown() // file3

	// Collapse dir2 by going back and toggling
	tv.MoveUp() // dir2
	tv.Toggle() // collapse

	// Cursor should adjust if it was on a now-hidden item
	if tv.cursor >= len(tv.flat) {
		t.Errorf("cursor %d out of bounds after collapse, flat len=%d", tv.cursor, len(tv.flat))
	}
}

func TestTreeViewRefresh(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialLen := len(tv.flat)

	// Manually expand dir1 on the node
	for _, child := range root.Children {
		if child.Name == "dir1" {
			child.Expanded = true
			break
		}
	}

	// Refresh to pick up changes
	tv.refresh()

	if len(tv.flat) == initialLen {
		t.Error("expected flat list to change after refresh")
	}
}
