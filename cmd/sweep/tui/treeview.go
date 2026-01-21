// Package tui provides an interactive terminal user interface for the sweep disk analyzer.
package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/daemon/tree"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Tree view icons using Unicode symbols.
// Fill indicates selection, direction indicates expand state.
const (
	// Directories: filled = selected, outline = unselected
	iconDirExpandedSelected   = "\u25BC" // ▼ Black down-pointing triangle
	iconDirCollapsedSelected  = "\u25B6" // ▶ Black right-pointing triangle
	iconDirExpandedUnselected = "\u25BD" // ▽ White down-pointing triangle
	iconDirCollapsedUnselected = "\u25B7" // ▷ White right-pointing triangle
	// Files: filled = selected, outline = unselected
	iconFileSelected   = "\u25CF" // ● Black circle (filled)
	iconFileUnselected = "\u25CB" // ○ White circle (outline)
)

// TreeView displays a hierarchical tree of directories and files
// with expand/collapse, selection, and scrolling support.
type TreeView struct {
	root     *tree.Node
	flat     []*tree.Node    // Flattened visible nodes
	cursor   int             // Index in flat slice
	offset   int             // Scroll offset
	selected map[string]bool // Selected file paths
}

// NewTreeView creates a new TreeView with the given root node.
func NewTreeView(root *tree.Node) *TreeView {
	tv := &TreeView{
		root:     root,
		cursor:   0,
		offset:   0,
		selected: make(map[string]bool),
	}
	tv.refresh()
	return tv
}

// refresh rebuilds the flat list from the current tree state.
func (tv *TreeView) refresh() {
	if tv.root == nil {
		tv.flat = nil
		return
	}
	tv.flat = tv.root.Flatten()

	// Ensure cursor is in bounds
	if tv.cursor >= len(tv.flat) {
		tv.cursor = len(tv.flat) - 1
	}
	if tv.cursor < 0 {
		tv.cursor = 0
	}
}

// MoveUp moves the cursor up one position.
func (tv *TreeView) MoveUp() {
	if len(tv.flat) == 0 {
		return
	}
	if tv.cursor > 0 {
		tv.cursor--
		tv.ensureVisible()
	}
}

// MoveDown moves the cursor down one position.
func (tv *TreeView) MoveDown() {
	if len(tv.flat) == 0 {
		return
	}
	if tv.cursor < len(tv.flat)-1 {
		tv.cursor++
		tv.ensureVisible()
	}
}

// ensureVisible adjusts offset to keep cursor visible.
func (tv *TreeView) ensureVisible() {
	// This will be calculated based on view height during rendering
	// For now, use a reasonable default visible area
	visible := 20
	if tv.cursor < tv.offset {
		tv.offset = tv.cursor
	} else if tv.cursor >= tv.offset+visible {
		tv.offset = tv.cursor - visible + 1
	}
	if tv.offset < 0 {
		tv.offset = 0
	}
}

// Toggle expands/collapses a directory or toggles file selection.
func (tv *TreeView) Toggle() {
	node := tv.Selected()
	if node == nil {
		return
	}

	if node.IsDir {
		node.Toggle()
		tv.refresh()
	} else {
		// For files, toggle selection
		tv.ToggleSelect()
	}
}

// ToggleSelect toggles selection of the current node (file or directory).
func (tv *TreeView) ToggleSelect() {
	node := tv.Selected()
	if node == nil {
		return
	}

	if tv.selected[node.Path] {
		delete(tv.selected, node.Path)
	} else {
		tv.selected[node.Path] = true
	}
}

// Selected returns the currently highlighted node.
func (tv *TreeView) Selected() *tree.Node {
	if len(tv.flat) == 0 || tv.cursor < 0 || tv.cursor >= len(tv.flat) {
		return nil
	}
	return tv.flat[tv.cursor]
}

// GetSelectedFiles returns all selected nodes (files and directories).
func (tv *TreeView) GetSelectedFiles() []*tree.Node {
	var result []*tree.Node
	for _, node := range tv.flat {
		if tv.selected[node.Path] {
			result = append(result, node)
		}
	}
	return result
}

// ClearSelection removes all selections.
func (tv *TreeView) ClearSelection() {
	tv.selected = make(map[string]bool)
}

// View renders the tree view within the given dimensions.
func (tv *TreeView) View(width, height int) string {
	if len(tv.flat) == 0 {
		return tv.renderEmpty(width, height)
	}

	var b strings.Builder

	// Calculate visible rows (leave room for header/footer)
	visibleRows := height - 2
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Adjust ensure visible with actual height
	tv.ensureVisibleWithHeight(visibleRows)

	// Render visible nodes
	for i := tv.offset; i < tv.offset+visibleRows && i < len(tv.flat); i++ {
		node := tv.flat[i]
		isCursor := i == tv.cursor
		isSelected := tv.selected[node.Path]

		row := tv.renderNode(node, width, isCursor, isSelected)
		b.WriteString(row)
		b.WriteString("\n")
	}

	// Pad remaining rows
	rendered := tv.offset + visibleRows
	if rendered > len(tv.flat) {
		rendered = len(tv.flat)
	}
	for i := rendered - tv.offset; i < visibleRows; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// ensureVisibleWithHeight adjusts offset with the actual visible height.
func (tv *TreeView) ensureVisibleWithHeight(visible int) {
	if tv.cursor < tv.offset {
		tv.offset = tv.cursor
	} else if tv.cursor >= tv.offset+visible {
		tv.offset = tv.cursor - visible + 1
	}
	if tv.offset < 0 {
		tv.offset = 0
	}
}

// renderEmpty renders the empty tree state.
func (tv *TreeView) renderEmpty(width, _ int) string {
	msg := mutedTextStyle.Render("No files to display")
	return center(msg, width) + "\n"
}

// sizeBarWidth is the maximum width for the proportional size bar.
const sizeBarWidth = 15

// renderNode renders a single node row.
func (tv *TreeView) renderNode(node *tree.Node, width int, isCursor, isSelected bool) string {
	// Calculate indentation based on depth
	depth := node.Depth()
	indent := strings.Repeat("  ", depth)

	// Build the row content
	var content strings.Builder

	// Indentation
	content.WriteString(indent)

	// Single icon combining selection (fill) and expand state (direction)
	var icon string
	if node.IsDir {
		if isSelected {
			if node.Expanded {
				icon = iconDirExpandedSelected // ▼
			} else {
				icon = iconDirCollapsedSelected // ▶
			}
		} else {
			if node.Expanded {
				icon = iconDirExpandedUnselected // ▽
			} else {
				icon = iconDirCollapsedUnselected // ▷
			}
		}
	} else {
		if isSelected {
			icon = iconFileSelected // ●
		} else {
			icon = iconFileUnselected // ○
		}
	}
	content.WriteString(icon)
	content.WriteString(" ")

	// Name
	content.WriteString(node.Name)

	// Calculate proportional size bar
	var sizeRatio float64
	var nodeSize int64
	if tv.root != nil && tv.root.LargeFileSize > 0 {
		if node.IsDir {
			nodeSize = node.LargeFileSize
		} else {
			nodeSize = node.Size
		}
		sizeRatio = float64(nodeSize) / float64(tv.root.LargeFileSize)
	}

	barWidth := int(sizeRatio * sizeBarWidth)
	if barWidth > sizeBarWidth {
		barWidth = sizeBarWidth
	}
	var bar string
	switch {
	case barWidth > 0:
		bar = strings.Repeat("█", barWidth) + strings.Repeat("░", sizeBarWidth-barWidth)
	case nodeSize > 0:
		// Show at least a minimal bar for non-zero sizes
		bar = "▏" + strings.Repeat("░", sizeBarWidth-1)
	default:
		bar = strings.Repeat("░", sizeBarWidth)
	}

	// Size (right-aligned)
	var sizeStr string
	if node.IsDir {
		if node.LargeFileCount > 0 {
			sizeStr = fmt.Sprintf("(%d files, %s)",
				node.LargeFileCount,
				formatSize(node.LargeFileSize))
		}
	} else {
		sizeStr = formatSize(node.Size)
	}

	// Calculate padding for right alignment (bar + space + size)
	contentLen := lipgloss.Width(content.String())
	barLen := sizeBarWidth
	sizeLen := lipgloss.Width(sizeStr)
	padding := width - contentLen - barLen - sizeLen - 3 // 3 = spaces between elements
	if padding < 1 {
		padding = 1
	}

	// Build full row: content + padding + bar + space + size
	row := content.String() + strings.Repeat(" ", padding) + bar + " " + sizeStr

	// Apply styling
	if isCursor {
		return treeRowHighlightStyle.Width(width).Render(row)
	}

	// Re-render with colored icon and styled bar
	var styled strings.Builder
	styled.WriteString(indent)
	if isSelected {
		styled.WriteString(lipgloss.NewStyle().Foreground(treeSelectedColor).Render(icon))
	} else {
		styled.WriteString(lipgloss.NewStyle().Foreground(treeUnselectedColor).Render(icon))
	}
	styled.WriteString(" ")
	styled.WriteString(node.Name)
	styled.WriteString(strings.Repeat(" ", padding))
	styled.WriteString(treeBarStyle.Render(bar))
	styled.WriteString(" ")
	styled.WriteString(lipgloss.NewStyle().Foreground(treeSizeColor).Render(sizeStr))

	return treeRowNormalStyle.Width(width).Render(styled.String())
}

// formatSize formats a size in bytes as a human-readable string.
func formatSize(bytes int64) string {
	return types.FormatSize(bytes)
}

// RenderStagingArea renders the selection staging area showing selected file count and actions.
// Returns an empty string if no files are selected.
func (tv *TreeView) RenderStagingArea(width int) string {
	selected := tv.GetSelectedFiles()
	if len(selected) == 0 {
		return ""
	}

	var totalSize int64
	for _, f := range selected {
		totalSize += f.Size
	}

	// Build staging area content
	content := fmt.Sprintf("  %d selected  -  %s                   ",
		len(selected), formatSize(totalSize))

	// Add key hints
	deleteKey := treeStagingKeyStyle.Render("[d]")
	clearKey := treeStagingKeyStyle.Render("[c]")
	content += deleteKey + "elete  " + clearKey + "lear"

	// Apply styling and ensure it spans the full width
	return treeStagingStyle.Width(width).Render(content)
}

// SelectedCount returns the number of selected files.
func (tv *TreeView) SelectedCount() int {
	return len(tv.selected)
}

// SelectedSize returns the total size of selected nodes.
// For directories, uses LargeFileSize (sum of large files underneath).
func (tv *TreeView) SelectedSize() int64 {
	var total int64
	for _, node := range tv.flat {
		if tv.selected[node.Path] {
			if node.IsDir {
				total += node.LargeFileSize
			} else {
				total += node.Size
			}
		}
	}
	return total
}

// HasSelection returns true if any files are selected.
func (tv *TreeView) HasSelection() bool {
	return len(tv.selected) > 0
}

// AddFile adds a new file to the tree.
// Creates intermediate directories as needed.
// Updates aggregates up to root and resorts affected directories.
func (tv *TreeView) AddFile(path string, size int64, modTime int64) {
	if tv.root == nil {
		return
	}

	// Create the file node
	fileNode := &tree.Node{
		Path:     path,
		Name:     filepath.Base(path),
		IsDir:    false,
		Size:     size,
		ModTime:  modTime,
		FileType: detectFileType(path),
	}

	// Ensure parent directories exist and get the immediate parent
	parentPath := filepath.Dir(path)
	parent := tv.ensureParentDirs(parentPath)
	if parent == nil {
		return
	}

	// Add file to parent
	parent.AddChild(fileNode)

	// Update aggregates up the tree
	tv.updateAncestorAggregates(parent, size, 1)
	parent.LargeFileSize += size
	parent.LargeFileCount++

	// Resort the parent's children
	tv.sortNodeChildren(parent)

	// Resort parent directories up to root (size changes may affect sort order)
	for p := parent.Parent; p != nil; p = p.Parent {
		tv.sortNodeChildren(p)
	}

	// Refresh the flat list
	tv.refresh()
}

// RemoveFile removes a file from the tree by path.
// It also removes the file from the selection map, cleans up empty directories,
// and refreshes the flat list.
func (tv *TreeView) RemoveFile(path string) {
	// Remove from selection
	delete(tv.selected, path)

	// Find and remove the node from the tree
	if tv.root != nil {
		tv.removeNodeByPath(tv.root, path)
	}

	// Clean up empty directories
	tv.cleanupEmptyDirs(tv.root)

	// Refresh the flat list
	tv.refresh()
}

// UpdateFile updates a file's size in the tree.
// Recalculates aggregates up to root and resorts affected directories.
func (tv *TreeView) UpdateFile(path string, newSize int64) {
	if tv.root == nil {
		return
	}

	// Find the node
	node := tv.findNodeByPath(tv.root, path)
	if node == nil || node.IsDir {
		return
	}

	// Calculate size delta
	oldSize := node.Size
	sizeDelta := newSize - oldSize

	// Update the node's size
	node.Size = newSize

	// Update aggregates up the tree
	if node.Parent != nil {
		node.Parent.LargeFileSize += sizeDelta
		tv.updateAncestorAggregates(node.Parent, sizeDelta, 0)

		// Resort parent directories (size changes may affect sort order)
		for p := node.Parent; p != nil; p = p.Parent {
			tv.sortNodeChildren(p)
		}
	}

	// Refresh the flat list
	tv.refresh()
}

// ensureParentDirs ensures all parent directories exist from root to the given path.
// Returns the immediate parent node.
func (tv *TreeView) ensureParentDirs(parentPath string) *tree.Node {
	// If parent is the root, return root
	if parentPath == tv.root.Path {
		return tv.root
	}

	// Check if parent already exists
	existing := tv.findNodeByPath(tv.root, parentPath)
	if existing != nil {
		return existing
	}

	// Build list of directories to create (from target up to root)
	var dirsToCreate []string
	current := parentPath
	for current != tv.root.Path && current != "/" && current != "." {
		if tv.findNodeByPath(tv.root, current) == nil {
			dirsToCreate = append(dirsToCreate, current)
		}
		current = filepath.Dir(current)
	}

	// Create directories in reverse order (from root down)
	for i := len(dirsToCreate) - 1; i >= 0; i-- {
		dirPath := dirsToCreate[i]
		dirNode := &tree.Node{
			Path:     dirPath,
			Name:     filepath.Base(dirPath),
			IsDir:    true,
			Expanded: false, // New directories are collapsed by default
		}

		// Find parent and add
		dirParentPath := filepath.Dir(dirPath)
		var dirParent *tree.Node
		if dirParentPath == tv.root.Path {
			dirParent = tv.root
		} else {
			dirParent = tv.findNodeByPath(tv.root, dirParentPath)
		}

		if dirParent != nil {
			dirParent.AddChild(dirNode)
		}
	}

	// Return the immediate parent
	return tv.findNodeByPath(tv.root, parentPath)
}

// removeNodeByPath recursively searches for and removes a node with the given path.
func (tv *TreeView) removeNodeByPath(node *tree.Node, path string) bool {
	for i, child := range node.Children {
		if child.Path == path {
			// Found it - remove from parent's children
			node.Children = append(node.Children[:i], node.Children[i+1:]...)

			// Update parent's aggregates for large files
			if node.IsDir && !child.IsDir {
				node.LargeFileCount--
				node.LargeFileSize -= child.Size
				// Propagate up to ancestors
				tv.updateAncestorAggregates(node, -child.Size, -1)
			}
			return true
		}
		// Recurse into directories
		if child.IsDir && tv.removeNodeByPath(child, path) {
			return true
		}
	}
	return false
}

// findNodeByPath recursively searches for a node with the given path.
func (tv *TreeView) findNodeByPath(node *tree.Node, path string) *tree.Node {
	if node.Path == path {
		return node
	}
	for _, child := range node.Children {
		if found := tv.findNodeByPath(child, path); found != nil {
			return found
		}
	}
	return nil
}

// updateAncestorAggregates updates LargeFileSize and LargeFileCount up the tree.
func (tv *TreeView) updateAncestorAggregates(node *tree.Node, sizeDelta int64, countDelta int) {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		parent.LargeFileSize += sizeDelta
		parent.LargeFileCount += countDelta
	}
}

// sortNodeChildren sorts a node's children by size descending.
// Directories come before files when sizes are equal.
func (tv *TreeView) sortNodeChildren(node *tree.Node) {
	if !node.IsDir || len(node.Children) == 0 {
		return
	}

	sort.Slice(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]

		// Get comparable size (LargeFileSize for dirs, Size for files)
		aSize := a.Size
		if a.IsDir {
			aSize = a.LargeFileSize
		}
		bSize := b.Size
		if b.IsDir {
			bSize = b.LargeFileSize
		}

		// Sort by size descending
		if aSize != bSize {
			return aSize > bSize
		}

		// Directories before files when same size
		if a.IsDir != b.IsDir {
			return a.IsDir
		}

		// Alphabetical as tiebreaker
		return a.Name < b.Name
	})
}

// cleanupEmptyDirs recursively removes empty directories from the tree.
// Returns true if the node itself should be removed (is empty).
func (tv *TreeView) cleanupEmptyDirs(node *tree.Node) bool {
	if node == nil {
		return false
	}

	// Process children first (bottom-up)
	var remaining []*tree.Node
	for _, child := range node.Children {
		if child.IsDir {
			// Recursively clean up this directory
			if !tv.cleanupEmptyDirs(child) {
				remaining = append(remaining, child)
			}
		} else {
			// Keep files
			remaining = append(remaining, child)
		}
	}
	node.Children = remaining

	// A directory is empty if it has no children (don't remove root)
	if node.IsDir && len(node.Children) == 0 && node.Parent != nil {
		return true
	}
	return false
}

// detectFileType returns a human-readable file type based on the file extension.
func detectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	// Programming languages
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".rs":
		return "Rust"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".java":
		return "Java"
	case ".rb":
		return "Ruby"
	case ".sh", ".bash", ".zsh":
		return "Shell"

	// Data/Config
	case ".json":
		return "JSON"
	case ".yaml", ".yml":
		return "YAML"
	case ".toml":
		return "TOML"
	case ".xml":
		return "XML"

	// Documentation
	case ".md", ".markdown":
		return "Markdown"
	case ".txt":
		return "Text"
	case ".pdf":
		return "PDF"

	// Images
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico":
		return "Image"

	// Video
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "Video"

	// Audio
	case ".mp3", ".wav", ".ogg", ".flac", ".aac":
		return "Audio"

	// Archives
	case ".zip", ".tar", ".gz", ".rar", ".7z", ".bz2", ".xz":
		return "Archive"

	// Executables and libraries
	case ".exe":
		return "Executable"
	case ".dll", ".so", ".dylib", ".a":
		return "Library"
	case ".wasm":
		return "WebAssembly"

	// Database
	case ".db", ".sqlite", ".sqlite3":
		return "Database"

	// Binary
	case ".bin":
		return "Binary"

	default:
		return "File"
	}
}

// Tree view styles (following existing styles.go patterns).
var (
	// Row styles
	treeRowHighlightStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#4A2040")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	treeRowNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC"))

	// Selection indicator colors
	treeSelectedColor   = lipgloss.Color("#00FF00")
	treeUnselectedColor = lipgloss.Color("#666666")

	// Size color
	treeSizeColor = lipgloss.Color("#00AAFF")

	// Size bar style
	treeBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4"))

	// Staging area styles
	treeStagingStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#2A1A30")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Padding(0, 1)

	treeStagingKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7D56F4")).
				Bold(true)
)
