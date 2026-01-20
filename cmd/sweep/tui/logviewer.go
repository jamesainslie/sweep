package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

// logRingBuffer is a thread-safe ring buffer for log entries.
// It maintains a fixed-size buffer with FIFO eviction.
type logRingBuffer struct {
	mu         sync.RWMutex
	entries    []logging.LogEntry
	maxEntries int
}

// newLogRingBuffer creates a new ring buffer with the specified max size.
func newLogRingBuffer(maxEntries int) *logRingBuffer {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &logRingBuffer{
		entries:    make([]logging.LogEntry, 0, maxEntries),
		maxEntries: maxEntries,
	}
}

// Add appends an entry to the buffer, evicting the oldest if at capacity.
func (rb *logRingBuffer) Add(entry logging.LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.entries) >= rb.maxEntries {
		// Remove oldest entry (FIFO)
		rb.entries = rb.entries[1:]
	}
	rb.entries = append(rb.entries, entry)
}

// Entries returns a copy of all entries in chronological order.
func (rb *logRingBuffer) Entries() []logging.LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]logging.LogEntry, len(rb.entries))
	copy(result, rb.entries)
	return result
}

// Len returns the number of entries in the buffer.
func (rb *logRingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return len(rb.entries)
}

// filterEntriesByLevel returns entries at or above the specified level.
func filterEntriesByLevel(entries []logging.LogEntry, minLevel logging.Level) []logging.LogEntry {
	result := make([]logging.LogEntry, 0, len(entries))
	for _, e := range entries {
		if e.Level >= minLevel {
			result = append(result, e)
		}
	}
	return result
}

// clampLogScroll ensures the scroll offset stays within valid bounds.
func clampLogScroll(offset, totalEntries, visibleRows int) int {
	if totalEntries <= visibleRows {
		return 0
	}
	maxOffset := totalEntries - visibleRows
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

// getVisibleLogEntries returns a slice of entries to display.
// It filters by level, then applies offset and limit.
func getVisibleLogEntries(entries []logging.LogEntry, minLevel logging.Level, offset, limit int) []logging.LogEntry {
	filtered := filterEntriesByLevel(entries, minLevel)

	if offset >= len(filtered) {
		return nil
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[offset:end]
}

// logLevelStyle returns the style for a log level.
func logLevelStyle(level logging.Level) lipgloss.Style {
	switch level {
	case logging.LevelDebug:
		return logDebugStyle
	case logging.LevelInfo:
		return logInfoStyle
	case logging.LevelWarn:
		return logWarnStyle
	case logging.LevelError:
		return logErrorStyle
	default:
		return logInfoStyle
	}
}

// logLevelChar returns a single character for the log level.
func logLevelChar(level logging.Level) string {
	switch level {
	case logging.LevelDebug:
		return "D"
	case logging.LevelInfo:
		return "I"
	case logging.LevelWarn:
		return "W"
	case logging.LevelError:
		return "E"
	default:
		return "?"
	}
}

// renderLogViewer renders the log viewer pane.
// width is the available width, height is the height for the log pane.
func renderLogViewer(entries []logging.LogEntry, filterLevel logging.Level, scrollOffset, width, height int) string {
	if height < 3 {
		return ""
	}

	var b strings.Builder

	// Title bar with filter level indicator
	filterName := filterLevel.String()
	title := fmt.Sprintf(" Logs [%s] ", filterName)
	filterHint := "[1-4] filter  [Esc] close"

	logTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor)

	titleBar := logTitleStyle.Render(title) + mutedTextStyle.Render(filterHint)
	b.WriteString(titleBar)
	b.WriteString("\n")

	// Divider
	b.WriteString(renderDivider(width))
	b.WriteString("\n")

	// Calculate visible rows for logs (height minus title bar and divider)
	visibleRows := height - 2
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Get filtered entries
	filtered := filterEntriesByLevel(entries, filterLevel)

	// Clamp scroll offset
	scrollOffset = clampLogScroll(scrollOffset, len(filtered), visibleRows)

	// Get visible entries
	visible := getVisibleLogEntries(entries, filterLevel, scrollOffset, visibleRows)

	// Render log entries
	for _, entry := range visible {
		b.WriteString(renderLogEntry(entry, width))
		b.WriteString("\n")
	}

	// Pad remaining rows
	for i := len(visible); i < visibleRows; i++ {
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(filtered) > visibleRows {
		scrollPct := 0
		if len(filtered)-visibleRows > 0 {
			scrollPct = scrollOffset * 100 / (len(filtered) - visibleRows)
		}
		scrollIndicator := mutedTextStyle.Render(fmt.Sprintf(" [%d/%d] %d%%", scrollOffset+1, len(filtered), scrollPct))
		// Right-align scroll indicator
		padding := width - lipgloss.Width(scrollIndicator)
		if padding > 0 {
			b.WriteString(strings.Repeat(" ", padding))
		}
		b.WriteString(scrollIndicator)
	}

	return b.String()
}

// renderLogEntry renders a single log entry.
func renderLogEntry(entry logging.LogEntry, width int) string {
	// Format: HH:MM:SS [L] component: message
	timeStr := entry.Time.Format("15:04:05")

	levelChar := logLevelChar(entry.Level)
	levelStyle := logLevelStyle(entry.Level)

	// Calculate available width for message
	// Time(8) + space(1) + [L](3) + space(1) + component(~10) + :(1) + space(1) = ~25
	componentWidth := 10
	if len(entry.Component) < componentWidth {
		componentWidth = len(entry.Component)
	}

	prefixWidth := 8 + 1 + 3 + 1 + componentWidth + 1 + 1 // time [L] comp:
	msgWidth := width - prefixWidth
	if msgWidth < 10 {
		msgWidth = 10
	}

	// Truncate message if needed
	msg := entry.Message
	if len(msg) > msgWidth {
		msg = msg[:msgWidth-3] + "..."
	}

	// Truncate component if needed
	comp := entry.Component
	if len(comp) > 10 {
		comp = comp[:10]
	}

	// Build the log line
	line := fmt.Sprintf("%s %s %s: %s",
		logTimeStyle.Render(timeStr),
		levelStyle.Render("["+levelChar+"]"),
		logComponentStyle.Render(comp),
		msg)

	return line
}

// LogViewerState holds the state for the log viewer pane.
type LogViewerState struct {
	Open         bool
	Buffer       *logRingBuffer
	FilterLevel  logging.Level
	ScrollOffset int
	Subscription <-chan logging.LogEntry
}

// NewLogViewerState creates a new log viewer state.
func NewLogViewerState() *LogViewerState {
	return &LogViewerState{
		Open:        false,
		Buffer:      newLogRingBuffer(100),
		FilterLevel: logging.LevelDebug, // Show all by default
	}
}

// Toggle toggles the log viewer open/closed.
func (s *LogViewerState) Toggle() {
	s.Open = !s.Open
}

// SetFilterLevel sets the filter level.
func (s *LogViewerState) SetFilterLevel(level logging.Level) {
	s.FilterLevel = level
	// Reset scroll when changing filter
	s.ScrollOffset = 0
}

// ScrollUp scrolls up by one line.
func (s *LogViewerState) ScrollUp() {
	if s.ScrollOffset > 0 {
		s.ScrollOffset--
	}
}

// ScrollDown scrolls down by one line.
func (s *LogViewerState) ScrollDown(visibleRows int) {
	filtered := filterEntriesByLevel(s.Buffer.Entries(), s.FilterLevel)
	maxOffset := len(filtered) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.ScrollOffset < maxOffset {
		s.ScrollOffset++
	}
}

// AddEntry adds a log entry to the buffer.
func (s *LogViewerState) AddEntry(entry logging.LogEntry) {
	s.Buffer.Add(entry)
}

// FilteredEntryCount returns the number of entries at or above the current filter level.
func (s *LogViewerState) FilteredEntryCount() int {
	return len(filterEntriesByLevel(s.Buffer.Entries(), s.FilterLevel))
}
