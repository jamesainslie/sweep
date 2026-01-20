package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
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
	files         []types.FileInfo
	cursor        int
	selected      map[int]bool
	offset        int // scroll offset
	width         int
	height        int
	metrics       ScanMetrics
	lastFreedSize int64 // Size freed in last delete operation
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
		m.cursor -= m.visibleRows()
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()
	case "pgdown":
		m.cursor += m.visibleRows()
		if m.cursor >= len(m.files) {
			m.cursor = len(m.files) - 1
		}
		m.ensureVisible()
	}
	return nil
}

// ensureVisible adjusts offset to keep cursor visible.
func (m *ResultModel) ensureVisible() {
	visible := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// View renders the result model.
func (m ResultModel) View() string {
	if len(m.files) == 0 {
		return m.renderEmpty()
	}

	var b strings.Builder

	// Calculate dimensions
	// Outer box has border (2 chars) but no padding, so content width is m.width - 4
	// (m.width - 2 for outer box width, minus 2 for border)
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
	// Icon and app name
	icon := "🧹"
	appName := titleStyle.Bold(true).Render("SWEEP")

	// Stats in muted style
	fileCount := fmt.Sprintf("%d files", len(m.files))
	totalSize := types.FormatSize(m.TotalSize())
	stats := mutedTextStyle.Render(fmt.Sprintf("  %s  •  %s", fileCount, totalSize))

	header := fmt.Sprintf(" %s %s%s", icon, appName, stats)

	// Show freed size if any
	if m.lastFreedSize > 0 {
		freedStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
		freed := freedStyle.Render(fmt.Sprintf("  ✓ Freed %s", types.FormatSize(m.lastFreedSize)))
		header = header + freed
	}

	return header
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

// Styles for file list rendering.
var (
	// Row highlight style - light pink background spanning full width
	rowHighlightStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#4A2040")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	// Normal row style
	rowNormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC"))

	// Checkbox styles - explicitly pad to 3 chars with centered indicator
	checkboxChecked   = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render("●") + " "
	checkboxUnchecked = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("○") + " "

	// Size style
	sizeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00AAFF")).Width(10).Align(lipgloss.Right)
)

// renderFileList renders the scrollable file list with full-width highlighting.
func (m ResultModel) renderFileList(width int) string {
	var b strings.Builder

	// Header row
	header := fmt.Sprintf("   %s  %s", padLeft("Size", 10), "File")
	b.WriteString(mutedTextStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(renderDivider(width))
	b.WriteString("\n")

	visible := m.visibleRows()
	for i := m.offset; i < m.offset+visible && i < len(m.files); i++ {
		file := m.files[i]
		isCursor := i == m.cursor
		isSelected := m.selected[i]

		filename := filepath.Base(file.Path)

		// Calculate available width for filename
		// Layout: checkbox(3) + size(10) + "  " + filename = 3 + 10 + 2 = 15 chars before filename
		filenameWidth := width - 15
		if filenameWidth < 20 {
			filenameWidth = 20
		}
		if len(filename) > filenameWidth {
			filename = filename[:filenameWidth-3] + "..."
		}

		// For highlighted row, use plain text so background spans full width
		// For normal rows, use styled checkbox and size
		if isCursor {
			checkbox := " ✓ "
			if !isSelected {
				checkbox = " ○ "
			}
			size := padLeft(types.FormatSize(file.Size), 10)
			row := fmt.Sprintf("%s%s  %s", checkbox, size, filename)
			b.WriteString(rowHighlightStyle.Width(width).Render(row))
		} else {
			var checkbox string
			if isSelected {
				checkbox = checkboxChecked
			} else {
				checkbox = checkboxUnchecked
			}
			size := sizeStyle.Render(types.FormatSize(file.Size))
			row := fmt.Sprintf("%s%s  %s", checkbox, size, filename)
			b.WriteString(rowNormalStyle.Width(width).Render(row))
		}
		b.WriteString("\n")
	}

	// Pad remaining rows
	rendered := m.offset + visible
	if rendered > len(m.files) {
		rendered = len(m.files)
	}
	for i := rendered - m.offset; i < visible; i++ {
		b.WriteString("\n")
	}

	// Detail panel for selected file
	if m.cursor >= 0 && m.cursor < len(m.files) {
		b.WriteString(m.renderDetailPanel(m.files[m.cursor], width))
	}

	return b.String()
}

// renderDetailPanel renders a clean detail panel for the selected file.
func (m ResultModel) renderDetailPanel(file types.FileInfo, width int) string {
	var b strings.Builder

	// Divider
	b.WriteString(renderDivider(width))
	b.WriteString("\n")

	// Path line - show full path with home shortened
	fullPath := file.Path
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(fullPath, home) {
		fullPath = "~" + fullPath[len(home):]
	}

	// Truncate path if needed, keeping the end
	maxPathLen := width - 10
	if maxPathLen < 40 {
		maxPathLen = 40
	}
	if len(fullPath) > maxPathLen {
		fullPath = "…" + fullPath[len(fullPath)-(maxPathLen-1):]
	}

	pathLine := "  Path: " + fullPath
	b.WriteString(mutedTextStyle.Render(pathLine))
	b.WriteString("\n")

	// Metadata line
	modTime := file.ModTime.Format("2006-01-02 15:04")
	ext := filepath.Ext(file.Path)
	if ext == "" {
		ext = "none"
	} else {
		ext = ext[1:] // Remove leading dot
	}

	metaLine := fmt.Sprintf("  Modified: %s  |  Type: %s", modTime, ext)
	if file.Owner != "" && file.Owner != "unknown" {
		metaLine += "  |  Owner: " + file.Owner
	}
	b.WriteString(mutedTextStyle.Render(metaLine))
	b.WriteString("\n")

	return b.String()
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
	// Outer box uses Height(m.height - 2), giving us m.height - 2 lines of content space.
	// Fixed overhead lines:
	//   View(): top margin(1) + header(1) + metrics(1) + divider(1) + help(1) + divider(1) = 6
	//   View(): footer divider(1) + footer(1) = 2
	//   renderFileList(): column header(1) + divider(1) = 2
	//   renderFileList(): detail panel divider(1) + path(1) + metadata(1) = 3
	// Total overhead: 6 + 2 + 2 + 3 = 13 lines
	// Plus outer box border reduction: 2 lines
	// Available for file rows: m.height - 2 - 13 = m.height - 15
	available := m.height - 15
	if available < 3 {
		available = 3
	}
	return available
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

// SetLastFreedSize sets the size freed in the last delete operation.
func (m *ResultModel) SetLastFreedSize(size int64) {
	m.lastFreedSize = size
}

// LastFreedSize returns the size freed in the last delete operation.
func (m ResultModel) LastFreedSize() int64 {
	return m.lastFreedSize
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

// SetFiles replaces all files at once, sorting by size descending.
// This is O(n log n) vs O(n²) for calling AddFile repeatedly.
// Use this for batch loading (e.g., from daemon).
func (m *ResultModel) SetFiles(files []types.FileInfo) {
	// Sort by size descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].Size > files[j].Size
	})
	m.files = files
	m.selected = make(map[int]bool)
	m.cursor = 0
	m.offset = 0
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
}

// ViewWithProgress renders the results with scan progress information in the footer.
func (m ResultModel) ViewWithProgress(progress ScanProgress) string {
	return m.ViewWithProgressAndNotifications(progress, nil, false, nil)
}

// ViewWithProgressAndNotifications renders the results with progress, notifications, live status,
// and status hints.
func (m ResultModel) ViewWithProgressAndNotifications(
	progress ScanProgress,
	notifications []Notification,
	liveWatching bool,
	statusHint *logging.LogEntry,
) string {
	// Show empty state only when scan is complete and no files found
	if len(m.files) == 0 && !progress.Scanning {
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

	// Footer with progress and status hint.
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n")
	b.WriteString(m.renderFooterWithProgressAndHint(contentWidth, progress, statusHint))

	content := b.String()
	return outerBoxStyle.Width(m.width - 2).Height(m.height - 2).Render(content)
}

// renderHeaderWithLive renders the header with an optional live indicator.
func (m ResultModel) renderHeaderWithLive(_ int, liveWatching bool) string {
	// Icon and app name
	icon := "🧹"
	appName := titleStyle.Bold(true).Render("SWEEP")

	// Stats in muted style
	fileCount := fmt.Sprintf("%d files", len(m.files))
	totalSize := types.FormatSize(m.TotalSize())
	stats := mutedTextStyle.Render(fmt.Sprintf("  %s  •  %s", fileCount, totalSize))

	header := fmt.Sprintf(" %s %s%s", icon, appName, stats)

	// Show freed size if any
	if m.lastFreedSize > 0 {
		freedStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
		freed := freedStyle.Render(fmt.Sprintf("  ✓ Freed %s", types.FormatSize(m.lastFreedSize)))
		header = header + freed
	}

	if liveWatching {
		liveIndicator := successTextStyle.Render("  ● LIVE")
		header = header + liveIndicator
	}

	return header
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

// renderFooterWithProgressAndHint renders the footer with selection summary, scan status, and status hint.
func (m ResultModel) renderFooterWithProgressAndHint(width int, progress ScanProgress, statusHint *logging.LogEntry) string {
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

	// If we have a status hint, show it instead of navigation hint
	var right string
	if statusHint != nil {
		right = renderStatusHint(statusHint, width-lipgloss.Width(left)-4)
	} else {
		right = mutedTextStyle.Render("[" + string(rune(0x2191)) + string(rune(0x2193)) + "] Navigate")
	}

	spacing := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if spacing < 1 {
		spacing = 1
	}

	return left + strings.Repeat(" ", spacing) + right
}

// renderStatusHint formats a log entry as a status hint with color coding.
func renderStatusHint(entry *logging.LogEntry, maxWidth int) string {
	if entry == nil {
		return ""
	}

	// Format: ● [Component] Message
	hint := fmt.Sprintf("[%s] %s", entry.Component, entry.Message)

	// Truncate if too long
	if len(hint) > maxWidth-2 { // -2 for "● "
		if maxWidth > 5 {
			hint = hint[:maxWidth-5] + "..."
		} else {
			hint = hint[:maxWidth-2]
		}
	}

	// Select style based on level
	var style lipgloss.Style
	switch entry.Level {
	case logging.LevelDebug:
		style = statusHintDebugStyle
	case logging.LevelInfo:
		style = statusHintInfoStyle
	case logging.LevelWarn:
		style = statusHintWarnStyle
	case logging.LevelError:
		style = statusHintErrorStyle
	default:
		style = statusHintInfoStyle
	}

	return style.Render("● " + hint)
}
