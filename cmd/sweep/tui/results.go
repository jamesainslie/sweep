package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// ResultModel represents the results phase of the TUI.
type ResultModel struct {
	files    []types.FileInfo
	cursor   int
	selected map[int]bool
	offset   int // scroll offset
	width    int
	height   int
}

// NewResultModel creates a new result model with the given files.
func NewResultModel(files []types.FileInfo) ResultModel {
	return ResultModel{
		files:    files,
		cursor:   0,
		selected: make(map[int]bool),
		offset:   0,
		width:    80,
		height:   24,
	}
}

// Init initializes the result model.
func (m ResultModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the result model.
func (m ResultModel) Update(msg tea.Msg) (ResultModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	return m, nil
}

// HandleKey handles key input for the result model.
func (m *ResultModel) HandleKey(key string) tea.Cmd {
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}

	case "down", "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
			m.ensureVisible()
		}

	case " ":
		m.Toggle(m.cursor)

	case "a":
		m.SelectAll()

	case "n":
		m.SelectNone()

	case "home", "g":
		m.cursor = 0
		m.offset = 0

	case "end", "G":
		if len(m.files) > 0 {
			m.cursor = len(m.files) - 1
			m.ensureVisible()
		}

	case "pgup":
		visibleRows := m.visibleRows()
		m.cursor -= visibleRows
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()

	case "pgdown":
		visibleRows := m.visibleRows()
		m.cursor += visibleRows
		if m.cursor >= len(m.files) {
			m.cursor = len(m.files) - 1
		}
		m.ensureVisible()
	}

	return nil
}

// View renders the result model.
func (m ResultModel) View() string {
	if len(m.files) == 0 {
		return m.renderEmpty()
	}

	var b strings.Builder

	// Calculate dimensions
	contentWidth := m.width - 4
	if contentWidth < 60 {
		contentWidth = 60
	}

	// Header
	b.WriteString(m.renderHeader(contentWidth))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")

	// Help bar
	b.WriteString(m.renderHelpBar(contentWidth))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")

	// File list
	b.WriteString(m.renderFileList(contentWidth))

	// Footer
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")
	b.WriteString(m.renderFooter(contentWidth))

	content := b.String()
	return outerBoxStyle.Width(m.width - 2).Render(content)
}

// renderEmpty renders the empty state view.
func (m ResultModel) renderEmpty() string {
	contentWidth := m.width - 4

	var b strings.Builder
	b.WriteString(m.renderHeader(contentWidth))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n\n")
	b.WriteString(center(mutedTextStyle.Render("No large files found matching your criteria."), contentWidth))
	b.WriteString("\n\n")
	b.WriteString(center(mutedTextStyle.Render("Try reducing the minimum size threshold with -s flag."), contentWidth))
	b.WriteString("\n\n")
	b.WriteString(center(keyStyle.Render("[q]")+" "+keyDescStyle.Render("Quit"), contentWidth))
	b.WriteString("\n")

	return outerBoxStyle.Width(m.width - 2).Render(b.String())
}

// renderHeader renders the header.
func (m ResultModel) renderHeader(width int) string {
	title := fmt.Sprintf("  sweep - %d files over threshold (Total: %s)",
		len(m.files), types.FormatSize(m.TotalSize()))

	return titleStyle.Render(title)
}

// renderHelpBar renders the help bar with key hints.
func (m ResultModel) renderHelpBar(width int) string {
	hints := []struct {
		key  string
		desc string
	}{
		{"Space", "Toggle"},
		{"a", "All"},
		{"n", "None"},
		{"Enter", "Delete"},
		{"q", "Quit"},
	}

	var parts []string
	for _, h := range hints {
		parts = append(parts, keyStyle.Render("["+h.key+"]")+" "+keyDescStyle.Render(h.desc))
	}

	return "  " + strings.Join(parts, "  ")
}

// renderFileList renders the scrollable file list.
func (m ResultModel) renderFileList(width int) string {
	var b strings.Builder

	visibleRows := m.visibleRows()
	pathWidth := width - 18 // checkbox + size + padding

	// Render visible files
	for i := m.offset; i < m.offset+visibleRows && i < len(m.files); i++ {
		file := m.files[i]
		isSelected := m.selected[i]
		isCursor := i == m.cursor

		// Build the line
		line := m.renderFileLine(file, i, isSelected, isCursor, pathWidth)
		b.WriteString(line)
		b.WriteString("\n")

		// Show details for cursor item
		if isCursor {
			details := m.renderFileDetails(file, width)
			b.WriteString(details)
			b.WriteString("\n")
		}
	}

	// Pad remaining rows
	rendered := m.offset + visibleRows
	if rendered > len(m.files) {
		rendered = len(m.files)
	}
	// Account for detail lines
	lineCount := 0
	for i := m.offset; i < rendered; i++ {
		lineCount++ // file line
		if i == m.cursor {
			lineCount++ // detail line
		}
	}
	for lineCount < visibleRows*2 {
		b.WriteString("\n")
		lineCount++
	}

	return b.String()
}

// renderFileLine renders a single file line.
func (m ResultModel) renderFileLine(file types.FileInfo, index int, isSelected, isCursor bool, pathWidth int) string {
	// Checkbox
	var checkbox string
	if isSelected {
		checkbox = checkedStyle.Render("[x]")
	} else {
		checkbox = uncheckedStyle.Render("[ ]")
	}

	// Size
	size := fileSizeStyle.Render(padLeft(types.FormatSize(file.Size), 9))

	// Path (truncated)
	path := truncatePath(file.Path, pathWidth)

	// Cursor indicator
	var cursor string
	if isCursor {
		cursor = cursorStyle.Render(">")
	} else {
		cursor = " "
	}

	// Combine parts
	line := fmt.Sprintf("  %s %s %s  %s", checkbox, size, cursor, path)

	// Apply style based on cursor position
	if isCursor {
		return selectedItemStyle.Width(pathWidth + 20).Render(line)
	}
	return normalItemStyle.Render(line)
}

// renderFileDetails renders the file detail line.
func (m ResultModel) renderFileDetails(file types.FileInfo, width int) string {
	modTime := file.ModTime.Format("2006-01-02")
	owner := file.Owner
	if owner == "" {
		owner = "unknown"
	}

	details := fmt.Sprintf("Modified: %s  Owner: %s", modTime, owner)
	return fileDetailStyle.Render(details)
}

// renderFooter renders the footer with selection summary.
func (m ResultModel) renderFooter(width int) string {
	selectedCount := len(m.selected)
	selectedSize := m.SelectedSize()

	left := fmt.Sprintf("  Selected: %d files (%s)", selectedCount, types.FormatSize(selectedSize))
	right := mutedTextStyle.Render("[↑↓] Navigate")

	spacing := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if spacing < 1 {
		spacing = 1
	}

	return left + strings.Repeat(" ", spacing) + right
}

// visibleRows returns the number of visible rows for the file list.
func (m ResultModel) visibleRows() int {
	// Account for header, help, dividers, footer
	// Each file takes 1-2 lines (2 when selected)
	available := m.height - 12
	if available < 5 {
		available = 5
	}
	// Divide by 2 since cursor item shows details
	return available / 2
}

// ensureVisible adjusts offset to keep cursor visible.
func (m *ResultModel) ensureVisible() {
	visibleRows := m.visibleRows()

	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+visibleRows {
		m.offset = m.cursor - visibleRows + 1
	}

	if m.offset < 0 {
		m.offset = 0
	}
}

// Toggle toggles selection of the file at the given index.
func (m *ResultModel) Toggle(index int) {
	if index < 0 || index >= len(m.files) {
		return
	}
	if m.selected[index] {
		delete(m.selected, index)
	} else {
		m.selected[index] = true
	}
}

// SelectAll selects all files.
func (m *ResultModel) SelectAll() {
	for i := range m.files {
		m.selected[i] = true
	}
}

// SelectNone deselects all files.
func (m *ResultModel) SelectNone() {
	m.selected = make(map[int]bool)
}

// SelectedFiles returns the list of selected files.
func (m ResultModel) SelectedFiles() []types.FileInfo {
	var result []types.FileInfo
	for i, selected := range m.selected {
		if selected && i < len(m.files) {
			result = append(result, m.files[i])
		}
	}
	return result
}

// SelectedSize returns the total size of selected files.
func (m ResultModel) SelectedSize() int64 {
	var total int64
	for i, selected := range m.selected {
		if selected && i < len(m.files) {
			total += m.files[i].Size
		}
	}
	return total
}

// SelectedCount returns the number of selected files.
func (m ResultModel) SelectedCount() int {
	return len(m.selected)
}

// TotalSize returns the total size of all files.
func (m ResultModel) TotalSize() int64 {
	var total int64
	for _, f := range m.files {
		total += f.Size
	}
	return total
}

// Files returns the list of files.
func (m ResultModel) Files() []types.FileInfo {
	return m.files
}

// Cursor returns the current cursor position.
func (m ResultModel) Cursor() int {
	return m.cursor
}

// HasSelection returns true if any files are selected.
func (m ResultModel) HasSelection() bool {
	return len(m.selected) > 0
}

// SetDimensions updates the width and height.
func (m *ResultModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}
