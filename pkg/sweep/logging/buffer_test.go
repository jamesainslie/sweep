package logging

import (
	"testing"
	"time"
)

func TestLogBuffer_AddAndEntries(t *testing.T) {
	buf := NewLogBuffer(3)

	// Add entries
	for i := 0; i < 3; i++ {
		buf.Add(LogEntry{
			Time:      time.Now(),
			Level:     LevelInfo,
			Component: "test",
			Message:   string(rune('A' + i)),
		})
	}

	entries := buf.Entries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify order (oldest first)
	for i, e := range entries {
		want := string(rune('A' + i))
		if e.Message != want {
			t.Errorf("entry %d: got %q, want %q", i, e.Message, want)
		}
	}
}

func TestLogBuffer_Overflow(t *testing.T) {
	buf := NewLogBuffer(3)

	// Add 5 entries to a buffer of size 3
	for i := 0; i < 5; i++ {
		buf.Add(LogEntry{
			Time:      time.Now(),
			Level:     LevelInfo,
			Component: "test",
			Message:   string(rune('A' + i)),
		})
	}

	entries := buf.Entries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Should have C, D, E (oldest 2 overwritten)
	want := []string{"C", "D", "E"}
	for i, e := range entries {
		if e.Message != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, e.Message, want[i])
		}
	}
}

func TestLogBuffer_Last(t *testing.T) {
	buf := NewLogBuffer(5)

	for i := 0; i < 5; i++ {
		buf.Add(LogEntry{
			Message: string(rune('A' + i)),
		})
	}

	// Get last 2
	last := buf.Last(2)
	if len(last) != 2 {
		t.Errorf("expected 2 entries, got %d", len(last))
	}

	// Should be D, E
	want := []string{"D", "E"}
	for i, e := range last {
		if e.Message != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, e.Message, want[i])
		}
	}
}

func TestLogBuffer_LastMoreThanCount(t *testing.T) {
	buf := NewLogBuffer(10)

	buf.Add(LogEntry{Message: "A"})
	buf.Add(LogEntry{Message: "B"})

	// Request more than available
	last := buf.Last(100)
	if len(last) != 2 {
		t.Errorf("expected 2 entries, got %d", len(last))
	}
}

func TestLogBuffer_Len(t *testing.T) {
	buf := NewLogBuffer(5)

	if buf.Len() != 0 {
		t.Errorf("new buffer should have len 0, got %d", buf.Len())
	}

	buf.Add(LogEntry{Message: "A"})
	buf.Add(LogEntry{Message: "B"})

	if buf.Len() != 2 {
		t.Errorf("expected len 2, got %d", buf.Len())
	}
}

func TestLogBuffer_Clear(t *testing.T) {
	buf := NewLogBuffer(5)

	buf.Add(LogEntry{Message: "A"})
	buf.Add(LogEntry{Message: "B"})
	buf.Clear()

	if buf.Len() != 0 {
		t.Errorf("cleared buffer should have len 0, got %d", buf.Len())
	}

	entries := buf.Entries()
	if len(entries) != 0 {
		t.Errorf("cleared buffer should return empty entries, got %d", len(entries))
	}
}

func TestNewLogBuffer_InvalidSize(t *testing.T) {
	buf := NewLogBuffer(0)
	if buf.maxSize != DefaultBufferSize {
		t.Errorf("expected default size %d, got %d", DefaultBufferSize, buf.maxSize)
	}

	buf = NewLogBuffer(-5)
	if buf.maxSize != DefaultBufferSize {
		t.Errorf("expected default size %d, got %d", DefaultBufferSize, buf.maxSize)
	}
}
