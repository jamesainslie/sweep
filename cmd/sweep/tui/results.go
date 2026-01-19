package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// ScanMetrics holds scan statistics for display.
type ScanMetrics struct {
	DirsScanned  int64
	FilesScanned int64
	CacheHits    int64
	CacheMisses  int64
	Elapsed      time.Duration
}

// ResultModel represents the results phase of the TUI.
type ResultModel struct {
	files    []types.FileInfo
	cursor   int
	selected map[int]bool
	offset   int // scroll offset
	width    int
	height   int
	metrics  ScanMetrics
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

// NewResultModelWithMetrics creates a new result model with files and scan metrics.
func NewResultModelWithMetrics(files []types.FileInfo, metrics ScanMetrics) ResultModel {
	return ResultModel{
		files:    files,
		cursor:   0,
		selected: make(map[int]bool),
		offset:   0,
		width:    80,
		height:   24,
		metrics:  metrics,
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

	// Add top margin for visual spacing
	b.WriteString("\n")

	// Header
	b.WriteString(m.renderHeader(contentWidth))
	b.WriteString("\n")

	// Metrics line (if available)
	metricsLine := m.renderMetrics(contentWidth)
	if metricsLine != "" {
		b.WriteString(metricsLine)
		b.WriteString("\n")
	}

	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")

	// Help bar
	b.WriteString(m.renderHelpBar(contentWidth))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")

	// File list
	b.WriteString(m.renderFileList(contentWidth))

	// Calculate padding needed to push footer to bottom
	// Available height minus border (2), header area (4 lines), footer area (2 lines)
	contentSoFar := strings.Count(b.String(), "\n")
	availableHeight := m.height - 4 // Account for outer box borders and padding
	footerLines := 2                // divider + footer
	neededPadding := availableHeight - contentSoFar - footerLines
	if neededPadding > 0 {
		b.WriteString(strings.Repeat("\n", neededPadding))
	}

	// Footer
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")
	b.WriteString(m.renderFooter(contentWidth))

	content := b.String()
	return outerBoxStyle.Width(m.width - 2).Height(m.height - 2).Render(content)
}

// renderEmpty renders the empty state view.
func (m ResultModel) renderEmpty() string {
	contentWidth := m.width - 4

	var b strings.Builder
	// Add top margin for visual spacing
	b.WriteString("\n")
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

	return outerBoxStyle.Width(m.width - 2).Height(m.height - 2).Render(b.String())
}

// renderHeader renders the header.
func (m ResultModel) renderHeader(_ int) string {
	title := fmt.Sprintf("  sweep - %d files over threshold (Total: %s)",
		len(m.files), types.FormatSize(m.TotalSize()))

	return titleStyle.Render(title)
}

// renderMetrics renders the scan metrics line.
func (m ResultModel) renderMetrics(_ int) string {
	var parts []string

	// Dirs and files scanned
	if m.metrics.DirsScanned > 0 || m.metrics.FilesScanned > 0 {
		parts = append(parts, fmt.Sprintf("Scanned: %s dirs, %s files",
			humanize.Comma(m.metrics.DirsScanned),
			humanize.Comma(m.metrics.FilesScanned)))
	}

	// Cache stats
	total := m.metrics.CacheHits + m.metrics.CacheMisses
	if total > 0 {
		hitRate := float64(m.metrics.CacheHits) / float64(total) * 100
		parts = append(parts, fmt.Sprintf("Cache: %s hits, %s misses (%.0f%%)",
			humanize.Comma(m.metrics.CacheHits),
			humanize.Comma(m.metrics.CacheMisses),
			hitRate))
	}

	// Elapsed time
	if m.metrics.Elapsed > 0 {
		parts = append(parts, fmt.Sprintf("Time: %v", m.metrics.Elapsed.Round(time.Millisecond)))
	}

	if len(parts) == 0 {
		return ""
	}

	return mutedTextStyle.Render("  " + strings.Join(parts, "  |  "))
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
	// Layout: checkbox(3) + size(9) + type(8) + cursor(2) + filename(rest)
	filenameWidth := width - 26
	if filenameWidth < 20 {
		filenameWidth = 20
	}

	// Render visible files
	for i := m.offset; i < m.offset+visibleRows && i < len(m.files); i++ {
		file := m.files[i]
		isSelected := m.selected[i]
		isCursor := i == m.cursor

		// Build the line
		line := m.renderFileLine(file, i, isSelected, isCursor, filenameWidth)
		b.WriteString(line)
		b.WriteString("\n")

		// Show details for cursor item (includes full path)
		if isCursor {
			details := m.renderFileDetails(file, width)
			b.WriteString(details)
			b.WriteString("\n")
		}
	}

	// Pad remaining rows to maintain consistent list height
	// Target height = visibleRows + 1 (for cursor detail line)
	rendered := m.offset + visibleRows
	if rendered > len(m.files) {
		rendered = len(m.files)
	}
	// Count actual lines rendered (files + cursor detail)
	lineCount := 0
	for i := m.offset; i < rendered; i++ {
		lineCount++ // file line
		if i == m.cursor {
			lineCount++ // detail line
		}
	}
	targetLines := visibleRows + 1 // +1 for cursor detail line
	for lineCount < targetLines {
		b.WriteString("\n")
		lineCount++
	}

	return b.String()
}

// renderFileLine renders a single file line.
func (m ResultModel) renderFileLine(file types.FileInfo, _ int, isSelected, isCursor bool, filenameWidth int) string {
	// Checkbox
	var checkbox string
	if isSelected {
		checkbox = checkedStyle.Render("[x]")
	} else {
		checkbox = uncheckedStyle.Render("[ ]")
	}

	// Size
	size := fileSizeStyle.Render(padLeft(types.FormatSize(file.Size), 9))

	// File type (extension)
	ext := filepath.Ext(file.Path)
	if ext == "" {
		ext = "-"
	} else {
		ext = ext[1:] // Remove leading dot
	}
	if len(ext) > 6 {
		ext = ext[:6]
	}
	fileType := mutedTextStyle.Render(padLeft(ext, 6))

	// Filename only (not full path)
	filename := filepath.Base(file.Path)
	if len(filename) > filenameWidth {
		filename = filename[:filenameWidth-3] + "..."
	}

	// Cursor indicator
	var cursor string
	if isCursor {
		cursor = cursorStyle.Render(">")
	} else {
		cursor = " "
	}

	// Combine parts: checkbox + size + type + cursor + filename
	line := fmt.Sprintf("  %s %s %s %s %s", checkbox, size, fileType, cursor, filename)

	// Apply style based on cursor position
	if isCursor {
		return selectedItemStyle.Width(filenameWidth + 30).Render(line)
	}
	return normalItemStyle.Render(line)
}

// renderFileDetails renders the file detail line showing full path and metadata.
func (m ResultModel) renderFileDetails(file types.FileInfo, width int) string {
	modTime := file.ModTime.Format("2006-01-02")
	owner := file.Owner
	if owner == "" {
		owner = "unknown"
	}

	// Show directory path (parent of the file)
	dir := filepath.Dir(file.Path)
	maxDirLen := width - 40 // Leave room for metadata
	if maxDirLen < 20 {
		maxDirLen = 20
	}
	if len(dir) > maxDirLen {
		dir = "..." + dir[len(dir)-(maxDirLen-3):]
	}

	details := fmt.Sprintf("    %s  (Modified: %s, Owner: %s)", dir, modTime, owner)
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
	// Account for header (2), metrics (1), help (2), dividers (3), footer (2) = 10 lines overhead
	// Plus 1 extra line for cursor item details
	available := m.height - 11
	if available < 5 {
		available = 5
	}
	return available
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

// AddFile inserts a file in sorted position (by size descending).
// This method is used for streaming results as files are found.
func (m *ResultModel) AddFile(file types.FileInfo) {
	// Find insertion point using binary search (largest first).
	idx := sort.Search(len(m.files), func(i int) bool {
		return m.files[i].Size <= file.Size
	})

	// Insert at the found position.
	m.files = append(m.files, types.FileInfo{})
	copy(m.files[idx+1:], m.files[idx:])
	m.files[idx] = file

	// Update selected indices for files that shifted.
	newSelected := make(map[int]bool)
	for i, selected := range m.selected {
		if selected {
			if i >= idx {
				newSelected[i+1] = true
			} else {
				newSelected[i] = true
			}
		}
	}
	m.selected = newSelected

	// Keep cursor visible after insertion.
	if m.cursor >= idx {
		m.cursor++
	}
}

// UpdateFile updates a file's size and mod time, re-sorting if needed.
// If the file is not found, it's added. If the new size is below min threshold,
// the file is removed.
func (m *ResultModel) UpdateFile(path string, newSize int64, modTime time.Time) {
	// Find the file by path.
	idx := -1
	for i, f := range m.files {
		if f.Path == path {
			idx = i
			break
		}
	}

	if idx == -1 {
		// File not found, add it.
		m.AddFile(types.FileInfo{
			Path:    path,
			Size:    newSize,
			ModTime: modTime,
		})
		return
	}

	// Check if size changed significantly enough to require re-sorting.
	oldSize := m.files[idx].Size
	m.files[idx].Size = newSize
	m.files[idx].ModTime = modTime

	if oldSize == newSize {
		return // No size change, no need to re-sort.
	}

	// Re-sort by removing and re-adding.
	// First, remove from current position.
	wasSelected := m.selected[idx]
	m.removeFileAtIndex(idx)

	// Re-add with new size.
	m.AddFile(types.FileInfo{
		Path:    path,
		Size:    newSize,
		ModTime: modTime,
	})

	// Restore selection if it was selected.
	if wasSelected {
		// Find new index.
		for i, f := range m.files {
			if f.Path == path {
				m.selected[i] = true
				break
			}
		}
	}
}

// RemoveFile removes a file from the results by path.
func (m *ResultModel) RemoveFile(path string) {
	// Find the file by path.
	idx := -1
	for i, f := range m.files {
		if f.Path == path {
			idx = i
			break
		}
	}

	if idx == -1 {
		return // File not found, nothing to do.
	}

	m.removeFileAtIndex(idx)
}

// removeFileAtIndex removes a file at the specified index.
func (m *ResultModel) removeFileAtIndex(idx int) {
	if idx < 0 || idx >= len(m.files) {
		return
	}

	// Remove from files slice.
	m.files = append(m.files[:idx], m.files[idx+1:]...)

	// Update selected indices for files that shifted.
	newSelected := make(map[int]bool)
	for i, selected := range m.selected {
		if selected {
			if i < idx {
				newSelected[i] = true
			} else if i > idx {
				newSelected[i-1] = true
			}
			// If i == idx, it's being removed so don't add to newSelected.
		}
	}
	m.selected = newSelected

	// Adjust cursor if needed.
	if m.cursor > idx {
		m.cursor--
	} else if m.cursor == idx && m.cursor >= len(m.files) && len(m.files) > 0 {
		m.cursor = len(m.files) - 1
	}

	// Ensure offset is still valid.
	m.ensureVisible()
}

// ViewWithProgress renders the results with scan progress information in the footer.
func (m ResultModel) ViewWithProgress(progress ScanProgress) string {
	return m.ViewWithProgressAndNotifications(progress, nil, false)
}

// ViewWithProgressAndNotifications renders the results with progress, notifications, and live status.
func (m ResultModel) ViewWithProgressAndNotifications(progress ScanProgress, notifications []Notification, liveWatching bool) string {
	if len(m.files) == 0 && progress.Scanning {
		return m.renderScanning(progress)
	}

	if len(m.files) == 0 {
		return m.renderEmpty()
	}

	var b strings.Builder

	// Calculate dimensions.
	contentWidth := m.width - 4
	if contentWidth < 60 {
		contentWidth = 60
	}

	// Add top margin for visual spacing.
	b.WriteString("\n")

	// Header with live indicator.
	b.WriteString(m.renderHeaderWithLive(contentWidth, liveWatching))
	b.WriteString("\n")

	// Metrics line (if available or scanning).
	metricsLine := m.renderMetricsWithProgress(contentWidth, progress)
	if metricsLine != "" {
		b.WriteString(metricsLine)
		b.WriteString("\n")
	}

	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")

	// Help bar.
	b.WriteString(m.renderHelpBar(contentWidth))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")

	// File list.
	b.WriteString(m.renderFileList(contentWidth))

	// Calculate padding needed to push footer to bottom.
	contentSoFar := strings.Count(b.String(), "\n")
	availableHeight := m.height - 4 // Account for outer box borders and padding.
	notificationLines := len(notifications)
	if notificationLines > 3 {
		notificationLines = 3 // Max 3 notifications shown
	}
	footerLines := 2 + notificationLines // divider + footer + notifications
	neededPadding := availableHeight - contentSoFar - footerLines
	if neededPadding > 0 {
		b.WriteString(strings.Repeat("\n", neededPadding))
	}

	// Notifications above footer
	if len(notifications) > 0 {
		b.WriteString(m.renderNotifications(contentWidth, notifications))
	}

	// Footer with progress.
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")
	b.WriteString(m.renderFooterWithProgress(contentWidth, progress))

	content := b.String()
	return outerBoxStyle.Width(m.width - 2).Height(m.height - 2).Render(content)
}

// renderHeaderWithLive renders the header with an optional live indicator.
func (m ResultModel) renderHeaderWithLive(width int, liveWatching bool) string {
	title := fmt.Sprintf("  sweep - %d files over threshold (Total: %s)",
		len(m.files), types.FormatSize(m.TotalSize()))

	if liveWatching {
		liveIndicator := notificationAddedStyle.Render("● LIVE")
		title = title + "  " + liveIndicator
	}

	_ = width // Suppress unused warning
	return titleStyle.Render(title)
}

// renderNotifications renders the notification area.
func (m ResultModel) renderNotifications(width int, notifications []Notification) string {
	var b strings.Builder

	// Show at most 3 notifications (newest first)
	maxNotifications := 3
	start := 0
	if len(notifications) > maxNotifications {
		start = len(notifications) - maxNotifications
	}

	for i := len(notifications) - 1; i >= start; i-- {
		n := notifications[i]
		var styled string
		switch n.Type {
		case NotificationAdded:
			styled = notificationAddedStyle.Render(n.Message)
		case NotificationRemoved:
			styled = notificationRemovedStyle.Render(n.Message)
		case NotificationModified:
			styled = notificationModifiedStyle.Render(n.Message)
		default:
			styled = n.Message
		}

		// Right-align notifications
		padding := width - len(n.Message) - 4
		if padding < 0 {
			padding = 0
		}
		b.WriteString(strings.Repeat(" ", padding) + styled + "\n")
	}

	return b.String()
}

// renderScanning renders the initial scanning state before any files are found.
func (m ResultModel) renderScanning(progress ScanProgress) string {
	contentWidth := m.width - 4
	if contentWidth < 60 {
		contentWidth = 60
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  sweep - scanning..."))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n\n")

	// Progress stats.
	elapsed := time.Since(progress.StartTime).Round(time.Millisecond)
	stats := fmt.Sprintf("  Dirs: %s  Files: %s  Time: %v",
		humanize.Comma(progress.DirsScanned),
		humanize.Comma(progress.FilesScanned),
		elapsed)
	b.WriteString(mutedTextStyle.Render(stats))
	b.WriteString("\n\n")

	b.WriteString(center(mutedTextStyle.Render("Searching for large files..."), contentWidth))
	b.WriteString("\n")

	return outerBoxStyle.Width(m.width - 2).Height(m.height - 2).Render(b.String())
}

// renderMetricsWithProgress renders metrics including real-time progress.
func (m ResultModel) renderMetricsWithProgress(width int, progress ScanProgress) string {
	var parts []string

	// Dirs and files scanned (prefer progress over final metrics during scanning).
	dirsScanned := m.metrics.DirsScanned
	filesScanned := m.metrics.FilesScanned
	if progress.Scanning {
		dirsScanned = progress.DirsScanned
		filesScanned = progress.FilesScanned
	}

	if dirsScanned > 0 || filesScanned > 0 {
		parts = append(parts, fmt.Sprintf("Scanned: %s dirs, %s files",
			humanize.Comma(dirsScanned),
			humanize.Comma(filesScanned)))
	}

	// Cache stats.
	cacheHits := m.metrics.CacheHits
	cacheMisses := m.metrics.CacheMisses
	if progress.Scanning {
		cacheHits = progress.CacheHits
		cacheMisses = progress.CacheMisses
	}

	total := cacheHits + cacheMisses
	if total > 0 {
		hitRate := float64(cacheHits) / float64(total) * 100
		parts = append(parts, fmt.Sprintf("Cache: %s hits, %s misses (%.0f%%)",
			humanize.Comma(cacheHits),
			humanize.Comma(cacheMisses),
			hitRate))
	}

	// Elapsed time.
	var elapsed time.Duration
	if progress.Scanning {
		elapsed = time.Since(progress.StartTime)
	} else {
		elapsed = m.metrics.Elapsed
	}
	if elapsed > 0 {
		parts = append(parts, fmt.Sprintf("Time: %v", elapsed.Round(time.Millisecond)))
	}

	if len(parts) == 0 {
		return ""
	}

	_ = width // Suppress unused warning, parameter kept for consistency.
	return mutedTextStyle.Render("  " + strings.Join(parts, "  |  "))
}

// renderFooterWithProgress renders the footer with selection summary and scan status.
func (m ResultModel) renderFooterWithProgress(width int, progress ScanProgress) string {
	selectedCount := len(m.selected)
	selectedSize := m.SelectedSize()

	var left string
	if progress.Scanning {
		left = fmt.Sprintf("  Scanning... Found: %d files (%s) | Selected: %d (%s)",
			len(m.files), types.FormatSize(m.TotalSize()),
			selectedCount, types.FormatSize(selectedSize))
	} else {
		left = fmt.Sprintf("  Selected: %d files (%s)", selectedCount, types.FormatSize(selectedSize))
	}

	right := mutedTextStyle.Render("[" + string(rune(0x2191)) + string(rune(0x2193)) + "] Navigate")

	spacing := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if spacing < 1 {
		spacing = 1
	}

	return left + strings.Repeat(" ", spacing) + right
}
