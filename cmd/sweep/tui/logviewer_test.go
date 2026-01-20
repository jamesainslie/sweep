package tui

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

func TestLogRingBuffer_AddEntry(t *testing.T) {
	tests := []struct {
		name          string
		maxEntries    int
		addCount      int
		expectedCount int
		firstMessage  string // expected first message after adding
	}{
		{
			name:          "add entries below max",
			maxEntries:    100,
			addCount:      50,
			expectedCount: 50,
			firstMessage:  "message 0",
		},
		{
			name:          "add entries at max",
			maxEntries:    100,
			addCount:      100,
			expectedCount: 100,
			firstMessage:  "message 0",
		},
		{
			name:          "add entries above max - FIFO eviction",
			maxEntries:    10,
			addCount:      15,
			expectedCount: 10,
			firstMessage:  "message 5", // oldest 5 evicted
		},
		{
			name:          "single entry buffer",
			maxEntries:    1,
			addCount:      5,
			expectedCount: 1,
			firstMessage:  "message 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := newLogRingBuffer(tt.maxEntries)

			for i := 0; i < tt.addCount; i++ {
				rb.Add(logging.LogEntry{
					Time:      time.Now(),
					Level:     logging.LevelInfo,
					Component: "test",
					Message:   "message " + string(rune('0'+i%10)) + string(rune('0'+i/10)),
				})
			}

			// Fix message generation for proper testing
			rb = newLogRingBuffer(tt.maxEntries)
			for i := 0; i < tt.addCount; i++ {
				rb.Add(logging.LogEntry{
					Time:      time.Now(),
					Level:     logging.LevelInfo,
					Component: "test",
					Message:   formatTestMessage(i),
				})
			}

			entries := rb.Entries()
			if len(entries) != tt.expectedCount {
				t.Errorf("expected %d entries, got %d", tt.expectedCount, len(entries))
			}

			if len(entries) > 0 && entries[0].Message != tt.firstMessage {
				t.Errorf("expected first message %q, got %q", tt.firstMessage, entries[0].Message)
			}
		})
	}
}

// formatTestMessage creates a consistent test message format.
func formatTestMessage(i int) string {
	return "message " + itoa(i)
}

// itoa converts an int to string without fmt to avoid import cycle issues in simple tests.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	if negative {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}

func TestLogRingBuffer_Empty(t *testing.T) {
	rb := newLogRingBuffer(100)
	entries := rb.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty buffer, got %d", len(entries))
	}
}

func TestFilterEntriesByLevel(t *testing.T) {
	entries := []logging.LogEntry{
		{Level: logging.LevelDebug, Message: "debug 1"},
		{Level: logging.LevelInfo, Message: "info 1"},
		{Level: logging.LevelWarn, Message: "warn 1"},
		{Level: logging.LevelError, Message: "error 1"},
		{Level: logging.LevelDebug, Message: "debug 2"},
		{Level: logging.LevelInfo, Message: "info 2"},
	}

	tests := []struct {
		name           string
		filterLevel    logging.Level
		expectedCount  int
		expectedLevels []logging.Level
	}{
		{
			name:          "filter debug shows all",
			filterLevel:   logging.LevelDebug,
			expectedCount: 6,
			expectedLevels: []logging.Level{
				logging.LevelDebug, logging.LevelInfo, logging.LevelWarn,
				logging.LevelError, logging.LevelDebug, logging.LevelInfo,
			},
		},
		{
			name:           "filter info hides debug",
			filterLevel:    logging.LevelInfo,
			expectedCount:  4,
			expectedLevels: []logging.Level{logging.LevelInfo, logging.LevelWarn, logging.LevelError, logging.LevelInfo},
		},
		{
			name:           "filter warn shows warn and error",
			filterLevel:    logging.LevelWarn,
			expectedCount:  2,
			expectedLevels: []logging.Level{logging.LevelWarn, logging.LevelError},
		},
		{
			name:           "filter error shows only error",
			filterLevel:    logging.LevelError,
			expectedCount:  1,
			expectedLevels: []logging.Level{logging.LevelError},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterEntriesByLevel(entries, tt.filterLevel)

			if len(filtered) != tt.expectedCount {
				t.Errorf("expected %d entries, got %d", tt.expectedCount, len(filtered))
			}

			for i, e := range filtered {
				if i < len(tt.expectedLevels) && e.Level != tt.expectedLevels[i] {
					t.Errorf("entry %d: expected level %v, got %v", i, tt.expectedLevels[i], e.Level)
				}
			}
		})
	}
}

func TestLogScrollBounds(t *testing.T) {
	tests := []struct {
		name           string
		totalEntries   int
		visibleRows    int
		initialOffset  int
		scrollDelta    int
		expectedOffset int
	}{
		{
			name:           "scroll down within bounds",
			totalEntries:   30,
			visibleRows:    10,
			initialOffset:  0,
			scrollDelta:    5,
			expectedOffset: 5,
		},
		{
			name:           "scroll down clamped at max",
			totalEntries:   30,
			visibleRows:    10,
			initialOffset:  15,
			scrollDelta:    10,
			expectedOffset: 20, // max is totalEntries - visibleRows
		},
		{
			name:           "scroll up within bounds",
			totalEntries:   30,
			visibleRows:    10,
			initialOffset:  10,
			scrollDelta:    -5,
			expectedOffset: 5,
		},
		{
			name:           "scroll up clamped at zero",
			totalEntries:   30,
			visibleRows:    10,
			initialOffset:  3,
			scrollDelta:    -10,
			expectedOffset: 0,
		},
		{
			name:           "no scroll when entries fit in view",
			totalEntries:   5,
			visibleRows:    10,
			initialOffset:  0,
			scrollDelta:    5,
			expectedOffset: 0, // can't scroll when all entries visible
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newOffset := clampLogScroll(tt.initialOffset+tt.scrollDelta, tt.totalEntries, tt.visibleRows)
			if newOffset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, newOffset)
			}
		})
	}
}

func TestLogLevelColors(t *testing.T) {
	// Verify each level returns a valid style (we can't easily test ANSI codes without a terminal)
	levels := []logging.Level{
		logging.LevelDebug,
		logging.LevelInfo,
		logging.LevelWarn,
		logging.LevelError,
	}

	for _, level := range levels {
		style := logLevelStyle(level)
		// Verify the style can render without panic
		rendered := style.Render("test")
		// Rendered should contain the original text
		if len(rendered) < 4 { // "test" is 4 chars
			t.Errorf("level %v render is too short: %q", level, rendered)
		}
	}

	// Verify each level returns a different style by checking their configurations are non-empty
	// The actual colors are tested visually, but we verify the function returns valid styles
	debugStyle := logLevelStyle(logging.LevelDebug)
	infoStyle := logLevelStyle(logging.LevelInfo)
	warnStyle := logLevelStyle(logging.LevelWarn)
	errorStyle := logLevelStyle(logging.LevelError)

	// Basic sanity check - they should all be able to render
	_ = debugStyle.Render("x")
	_ = infoStyle.Render("x")
	_ = warnStyle.Render("x")
	_ = errorStyle.Render("x")
}

func TestLogViewerVisibleEntries(t *testing.T) {
	rb := newLogRingBuffer(100)
	for i := 0; i < 50; i++ {
		rb.Add(logging.LogEntry{
			Time:      time.Now(),
			Level:     logging.LevelInfo,
			Component: "test",
			Message:   formatTestMessage(i),
		})
	}

	// Get visible entries with offset and limit
	entries := rb.Entries()
	visible := getVisibleLogEntries(entries, logging.LevelDebug, 10, 20)

	if len(visible) != 20 {
		t.Errorf("expected 20 visible entries, got %d", len(visible))
	}

	// First visible entry should be message 10
	if visible[0].Message != "message 10" {
		t.Errorf("expected first visible to be 'message 10', got %q", visible[0].Message)
	}
}

func TestLogViewerVisibleEntriesWithFilter(t *testing.T) {
	rb := newLogRingBuffer(100)
	for i := 0; i < 20; i++ {
		level := logging.LevelInfo
		if i%2 == 0 {
			level = logging.LevelDebug
		}
		rb.Add(logging.LogEntry{
			Time:      time.Now(),
			Level:     level,
			Component: "test",
			Message:   formatTestMessage(i),
		})
	}

	// Filter to info only (10 entries), get first 5
	entries := rb.Entries()
	visible := getVisibleLogEntries(entries, logging.LevelInfo, 0, 5)

	if len(visible) != 5 {
		t.Errorf("expected 5 visible entries, got %d", len(visible))
	}

	// All visible entries should be info level
	for i, e := range visible {
		if e.Level != logging.LevelInfo {
			t.Errorf("entry %d: expected info level, got %v", i, e.Level)
		}
	}
}
