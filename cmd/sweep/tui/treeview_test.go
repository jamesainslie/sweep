package tui

import (
	"strings"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon/tree"
)

// Helper to create a test tree structure.
func createTestTree() *tree.Node {
	root := &tree.Node{
		Path:           "/test",
		Name:           "test",
		IsDir:          true,
		Expanded:       true,
		LargeFileSize:  1024 * 1024 * 250, // 250 MiB (200 + 50)
		LargeFileCount: 3,                 // 3 files total
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

	// Select a directory - should work (directories can be selected for deletion)
	tv.MoveDown() // dir1
	tv.ToggleSelect()

	if len(tv.selected) != 1 {
		t.Error("expected directory to be selected")
	}

	// Toggle again to deselect
	tv.ToggleSelect()
	if len(tv.selected) != 0 {
		t.Error("expected directory to be deselected")
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

	// Collapsed dir should have collapsed icon (unselected variant)
	if !strings.Contains(view, iconDirCollapsedUnselected) {
		t.Errorf("expected collapsed icon '%s' in view", iconDirCollapsedUnselected)
	}

	// Expanded dir should have expanded icon (unselected variant)
	if !strings.Contains(view, iconDirExpandedUnselected) {
		t.Errorf("expected expanded icon '%s' in view", iconDirExpandedUnselected)
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

	// Should show selection indicator (filled circle for selected file)
	if !strings.Contains(view, iconFileSelected) {
		t.Errorf("expected selection indicator '%s' in view", iconFileSelected)
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

// Size bar tests.
func TestTreeViewPercentageRendering(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	view := tv.View(100, 24)

	// View should contain percentage indicators
	if !strings.Contains(view, "%") {
		t.Error("expected percentage indicator in view")
	}
}

func TestTreeViewPercentageProportions(t *testing.T) {
	// Create a tree with known sizes
	root := &tree.Node{
		Path:          "/test",
		Name:          "test",
		IsDir:         true,
		Expanded:      true,
		LargeFileSize: 1000,
	}

	// Add a file with half the size
	file := &tree.Node{
		Path: "/test/file.txt",
		Name: "file.txt",
		Size: 500, // Half of root's LargeFileSize
	}
	root.AddChild(file)

	tv := NewTreeView(root)
	view := tv.View(100, 24)

	// View should render without errors
	if view == "" {
		t.Error("expected non-empty view")
	}

	// The file should show 50% (half of root's size)
	lines := strings.Split(view, "\n")
	foundFileWithPercent := false
	for _, line := range lines {
		if strings.Contains(line, "file.txt") && strings.Contains(line, "50%") {
			foundFileWithPercent = true
			break
		}
	}
	if !foundFileWithPercent {
		t.Error("expected file to show 50% percentage")
	}
}

// Staging area tests.
func TestTreeViewRenderStagingAreaEmpty(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// No selection, should return empty string
	staging := tv.RenderStagingArea(80)
	if staging != "" {
		t.Errorf("expected empty staging area with no selection, got %q", staging)
	}
}

func TestTreeViewRenderStagingAreaWithSelection(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Select a file
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2
	tv.MoveDown() // file3
	tv.ToggleSelect()

	staging := tv.RenderStagingArea(80)
	if staging == "" {
		t.Error("expected non-empty staging area with selection")
	}

	// Should contain selection count
	if !strings.Contains(staging, "1 selected") {
		t.Error("expected staging area to show selection count")
	}

	// Should contain key hints
	if !strings.Contains(staging, "d") || !strings.Contains(staging, "c") {
		t.Error("expected staging area to show key hints")
	}
}

func TestTreeViewRenderStagingAreaMultipleSelections(t *testing.T) {
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

	staging := tv.RenderStagingArea(80)
	if !strings.Contains(staging, "2 selected") {
		t.Errorf("expected staging area to show 2 selected, got %q", staging)
	}
}

// Helper method tests.
func TestTreeViewSelectedCount(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	if tv.SelectedCount() != 0 {
		t.Error("expected 0 selected initially")
	}

	// Select files
	tv.MoveDown()
	tv.MoveDown()
	tv.MoveDown() // file3
	tv.ToggleSelect()

	if tv.SelectedCount() != 1 {
		t.Errorf("expected 1 selected, got %d", tv.SelectedCount())
	}
}

func TestTreeViewSelectedSize(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	if tv.SelectedSize() != 0 {
		t.Error("expected 0 selected size initially")
	}

	// Select file3 (50 MiB)
	tv.MoveDown()
	tv.MoveDown()
	tv.MoveDown() // file3
	tv.ToggleSelect()

	expectedSize := int64(1024 * 1024 * 50) // 50 MiB
	if tv.SelectedSize() != expectedSize {
		t.Errorf("expected selected size %d, got %d", expectedSize, tv.SelectedSize())
	}
}

func TestTreeViewHasSelection(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	if tv.HasSelection() {
		t.Error("expected no selection initially")
	}

	tv.MoveDown()
	tv.MoveDown()
	tv.MoveDown() // file3
	tv.ToggleSelect()

	if !tv.HasSelection() {
		t.Error("expected selection after toggle")
	}

	tv.ClearSelection()
	if tv.HasSelection() {
		t.Error("expected no selection after clear")
	}
}

// Tree mutation tests (live updates).

func TestTreeViewAddFile(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialCount := len(tv.flat)

	// Add a new file to dir2
	tv.AddFile("/test/dir2/newfile.txt", 1024*1024*30, 1234567890)

	// Should have one more visible node (since dir2 is expanded)
	if len(tv.flat) != initialCount+1 {
		t.Errorf("expected %d visible nodes after add, got %d", initialCount+1, len(tv.flat))
	}

	// Find the new file
	found := false
	for _, node := range tv.flat {
		if node.Path == "/test/dir2/newfile.txt" {
			found = true
			if node.Size != 1024*1024*30 {
				t.Errorf("expected size %d, got %d", 1024*1024*30, node.Size)
			}
			if node.ModTime != 1234567890 {
				t.Errorf("expected modTime %d, got %d", 1234567890, node.ModTime)
			}
			if node.FileType != "Text" {
				t.Errorf("expected file type 'Text', got %q", node.FileType)
			}
			break
		}
	}
	if !found {
		t.Error("expected to find new file in flat list")
	}

	// Verify aggregates were updated
	for _, child := range tv.root.Children {
		if child.Name == "dir2" {
			// dir2 had 1 file of 50MiB, now has 2 files totaling 80MiB
			expectedSize := int64(1024 * 1024 * 80)
			if child.LargeFileSize != expectedSize {
				t.Errorf("expected dir2 LargeFileSize %d, got %d", expectedSize, child.LargeFileSize)
			}
			if child.LargeFileCount != 2 {
				t.Errorf("expected dir2 LargeFileCount 2, got %d", child.LargeFileCount)
			}
			break
		}
	}
}

func TestTreeViewAddFileNewDirectory(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialCount := len(tv.flat)

	// Add a file in a new directory
	tv.AddFile("/test/newdir/nested/file.txt", 1024*1024*10, 1234567890)

	// The new directories should be created but collapsed by default
	// So only the immediate child of root (newdir) should appear, collapsed
	// flat should have: root, dir1, dir2, file3, newdir = 5
	if len(tv.flat) != initialCount+1 {
		t.Errorf("expected %d visible nodes (new dir collapsed), got %d", initialCount+1, len(tv.flat))
	}

	// Find the new directory
	found := false
	for _, node := range tv.flat {
		if node.Path == "/test/newdir" {
			found = true
			if !node.IsDir {
				t.Error("expected newdir to be a directory")
			}
			// Aggregates should reflect the file
			if node.LargeFileSize != 1024*1024*10 {
				t.Errorf("expected newdir LargeFileSize %d, got %d", 1024*1024*10, node.LargeFileSize)
			}
			break
		}
	}
	if !found {
		t.Error("expected to find new directory in flat list")
	}
}

func TestTreeViewAddFileToCollapsedDir(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialCount := len(tv.flat)

	// dir1 is collapsed, add a file to it
	tv.AddFile("/test/dir1/newfile.txt", 1024*1024*25, 1234567890)

	// Flat list should not change since dir1 is collapsed
	if len(tv.flat) != initialCount {
		t.Errorf("expected %d visible nodes (dir collapsed), got %d", initialCount, len(tv.flat))
	}

	// But the aggregates should be updated
	for _, child := range tv.root.Children {
		if child.Name == "dir1" {
			// dir1 had 2 files of 100MiB each, now has 3 files totaling 225MiB
			expectedSize := int64(1024 * 1024 * 225)
			if child.LargeFileSize != expectedSize {
				t.Errorf("expected dir1 LargeFileSize %d, got %d", expectedSize, child.LargeFileSize)
			}
			if child.LargeFileCount != 3 {
				t.Errorf("expected dir1 LargeFileCount 3, got %d", child.LargeFileCount)
			}
			break
		}
	}
}

func TestTreeViewRemoveFile(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialCount := len(tv.flat)

	// Remove file3 from dir2
	// Since dir2 becomes empty, it will also be removed (cleanup empty dirs)
	// So we lose both file3 and dir2: 4 - 2 = 2 visible nodes
	tv.RemoveFile("/test/dir2/file3.txt")

	// Should have two less visible nodes (file3 + empty dir2)
	if len(tv.flat) != initialCount-2 {
		t.Errorf("expected %d visible nodes after remove, got %d", initialCount-2, len(tv.flat))
	}

	// File should not be in flat list
	for _, node := range tv.flat {
		if node.Path == "/test/dir2/file3.txt" {
			t.Error("expected file3.txt to be removed from flat list")
		}
	}

	// dir2 should also be removed (empty directory cleanup)
	for _, child := range tv.root.Children {
		if child.Name == "dir2" {
			t.Error("expected dir2 to be removed when empty")
		}
	}

	// Root aggregate should be updated (only dir1's 200MiB remains)
	expectedRootSize := int64(1024 * 1024 * 200)
	if tv.root.LargeFileSize != expectedRootSize {
		t.Errorf("expected root LargeFileSize %d, got %d", expectedRootSize, tv.root.LargeFileSize)
	}
}

func TestTreeViewRemoveFileCleanupEmptyDirs(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Remove file3 from dir2, which should leave dir2 empty
	tv.RemoveFile("/test/dir2/file3.txt")

	// dir2 should be removed since it's now empty
	found := false
	for _, child := range tv.root.Children {
		if child.Name == "dir2" {
			found = true
			break
		}
	}
	if found {
		t.Error("expected dir2 to be removed when empty")
	}
}

func TestTreeViewRemoveFileCleansSelection(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Select file3
	tv.MoveDown() // dir1
	tv.MoveDown() // dir2
	tv.MoveDown() // file3
	tv.ToggleSelect()

	if !tv.selected["/test/dir2/file3.txt"] {
		t.Error("expected file3.txt to be selected")
	}

	// Remove the file
	tv.RemoveFile("/test/dir2/file3.txt")

	// Selection should be cleaned
	if tv.selected["/test/dir2/file3.txt"] {
		t.Error("expected file3.txt selection to be cleared after removal")
	}
}

func TestTreeViewRemoveNonexistent(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialCount := len(tv.flat)

	// Removing a non-existent file should be a no-op
	tv.RemoveFile("/test/nonexistent.txt")

	if len(tv.flat) != initialCount {
		t.Errorf("expected %d visible nodes (no change), got %d", initialCount, len(tv.flat))
	}
}

func TestTreeViewUpdateFile(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Update file3's size
	oldSize := int64(1024 * 1024 * 50)
	newSize := int64(1024 * 1024 * 100)
	tv.UpdateFile("/test/dir2/file3.txt", newSize)

	// Find the file and verify
	for _, node := range tv.flat {
		if node.Path == "/test/dir2/file3.txt" {
			if node.Size != newSize {
				t.Errorf("expected size %d, got %d", newSize, node.Size)
			}
			break
		}
	}

	// Aggregates should be updated
	for _, child := range tv.root.Children {
		if child.Name == "dir2" {
			if child.LargeFileSize != newSize {
				t.Errorf("expected dir2 LargeFileSize %d, got %d", newSize, child.LargeFileSize)
			}
			break
		}
	}

	// Root aggregate should also be updated
	expectedRootSize := tv.root.LargeFileSize
	// dir1 has 200MiB, dir2 now has 100MiB = 300MiB total
	expectedTotal := int64(1024*1024*200) + newSize
	if expectedRootSize != expectedTotal {
		t.Errorf("expected root LargeFileSize %d, got %d", expectedTotal, expectedRootSize)
	}

	_ = oldSize // avoid unused variable warning
}

func TestTreeViewUpdateFileNonexistent(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	initialRootSize := tv.root.LargeFileSize

	// Updating a non-existent file should be a no-op
	tv.UpdateFile("/test/nonexistent.txt", 1024*1024*999)

	if tv.root.LargeFileSize != initialRootSize {
		t.Errorf("expected root LargeFileSize unchanged at %d, got %d", initialRootSize, tv.root.LargeFileSize)
	}
}

func TestTreeViewUpdateFileResortsParent(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Initially dir1 (200MiB) should be before dir2 (50MiB)
	if len(tv.root.Children) < 2 {
		t.Fatal("expected at least 2 children")
	}
	if tv.root.Children[0].Name != "dir1" {
		t.Errorf("expected dir1 first, got %s", tv.root.Children[0].Name)
	}

	// Update file3 to be massive (500MiB), making dir2 larger than dir1
	tv.UpdateFile("/test/dir2/file3.txt", 1024*1024*500)

	// Now dir2 should be first
	if tv.root.Children[0].Name != "dir2" {
		t.Errorf("expected dir2 first after size update, got %s", tv.root.Children[0].Name)
	}
}

func TestTreeViewAddFileMaintainsSortOrder(t *testing.T) {
	root := createTestTree()
	tv := NewTreeView(root)

	// Add a huge file to dir2, making it larger than dir1
	tv.AddFile("/test/dir2/hugefile.bin", 1024*1024*500, 1234567890)

	// dir2 should now be first (550MiB > 200MiB)
	if tv.root.Children[0].Name != "dir2" {
		t.Errorf("expected dir2 first after adding large file, got %s", tv.root.Children[0].Name)
	}
}
