package logging

import "sync"

// DefaultBufferSize is the default number of log entries to keep in the buffer.
const DefaultBufferSize = 100

// LogBuffer holds recent log entries in a ring buffer for TUI display.
type LogBuffer struct {
	entries []LogEntry
	maxSize int
	start   int // Index of oldest entry
	count   int // Number of entries in buffer
	mu      sync.RWMutex
}

// NewLogBuffer creates a new log buffer with the given maximum size.
func NewLogBuffer(maxSize int) *LogBuffer {
	if maxSize <= 0 {
		maxSize = DefaultBufferSize
	}
	return &LogBuffer{
		entries: make([]LogEntry, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a log entry to the buffer.
// If the buffer is full, the oldest entry is overwritten.
func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Calculate the index where the new entry goes
	idx := (b.start + b.count) % b.maxSize
	b.entries[idx] = entry

	if b.count < b.maxSize {
		b.count++
	} else {
		// Buffer is full, advance start to overwrite oldest
		b.start = (b.start + 1) % b.maxSize
	}
}

// Entries returns all entries in the buffer, oldest first.
// The returned slice is a copy and safe to modify.
func (b *LogBuffer) Entries() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]LogEntry, b.count)
	for i := 0; i < b.count; i++ {
		idx := (b.start + i) % b.maxSize
		result[i] = b.entries[idx]
	}
	return result
}

// Last returns the most recent n entries, newest last.
// If n is greater than the number of entries, all entries are returned.
func (b *LogBuffer) Last(n int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n > b.count {
		n = b.count
	}

	result := make([]LogEntry, n)
	startOffset := b.count - n
	for i := 0; i < n; i++ {
		idx := (b.start + startOffset + i) % b.maxSize
		result[i] = b.entries[idx]
	}
	return result
}

// Len returns the number of entries currently in the buffer.
func (b *LogBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// Clear removes all entries from the buffer.
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.start = 0
	b.count = 0
}
