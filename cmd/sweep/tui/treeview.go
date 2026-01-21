// Package tui provides an interactive terminal user interface for the sweep disk analyzer.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/daemon/tree"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Tree view icons using Unicode symbols.
const (
	iconExpanded   = "\u25BC" // Black down-pointing triangle
	iconCollapsed  = "\u25B6" // Black right-pointing triangle
	iconSelected   = "\u25CF" // Black circle (filled)
	iconUnselected = "\u25CB" // White circle (outline)
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

// ToggleSelect toggles selection of the current file.
// Has no effect on directories.
func (tv *TreeView) ToggleSelect() {
	node := tv.Selected()
	if node == nil || node.IsDir {
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

// GetSelectedFiles returns all selected file nodes.
func (tv *TreeView) GetSelectedFiles() []*tree.Node {
	var result []*tree.Node
	for _, node := range tv.flat {
		if !node.IsDir && tv.selected[node.Path] {
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

// renderNode renders a single node row.
func (tv *TreeView) renderNode(node *tree.Node, width int, isCursor, isSelected bool) string {
	// Calculate indentation based on depth
	depth := node.Depth()
	indent := strings.Repeat("  ", depth)

	// Build the row content
	var content strings.Builder

	// Indentation
	content.WriteString(indent)

	// Icon for dirs or selection indicator for files
	if node.IsDir {
		if node.Expanded {
			content.WriteString(iconExpanded)
		} else {
			content.WriteString(iconCollapsed)
		}
		content.WriteString(" ")
	} else {
		// File selection indicator
		if isSelected {
			content.WriteString(iconSelected)
		} else {
			content.WriteString(iconUnselected)
		}
		content.WriteString(" ")
	}

	// Name
	content.WriteString(node.Name)

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

	// Calculate padding for right alignment
	contentLen := lipgloss.Width(content.String())
	sizeLen := lipgloss.Width(sizeStr)
	padding := width - contentLen - sizeLen - 1
	if padding < 1 {
		padding = 1
	}

	row := content.String() + strings.Repeat(" ", padding) + sizeStr

	// Apply styling
	if isCursor {
		return treeRowHighlightStyle.Width(width).Render(row)
	}

	// Color the selection indicator for files
	if !node.IsDir {
		// Re-render with colored selection indicator
		var styled strings.Builder
		styled.WriteString(indent)
		if isSelected {
			styled.WriteString(lipgloss.NewStyle().Foreground(treeSelectedColor).Render(iconSelected))
		} else {
			styled.WriteString(lipgloss.NewStyle().Foreground(treeUnselectedColor).Render(iconUnselected))
		}
		styled.WriteString(" ")
		styled.WriteString(node.Name)
		styled.WriteString(strings.Repeat(" ", padding))
		styled.WriteString(lipgloss.NewStyle().Foreground(treeSizeColor).Render(sizeStr))
		return treeRowNormalStyle.Width(width).Render(styled.String())
	}

	return treeRowNormalStyle.Width(width).Render(row)
}

// formatSize formats a size in bytes as a human-readable string.
func formatSize(bytes int64) string {
	return types.FormatSize(bytes)
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
)
