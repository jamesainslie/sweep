package manifest

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manifest manages operation logging to the filesystem.
type Manifest struct {
	dir string
	mu  sync.Mutex
}

// New creates a new Manifest with the given directory.
// The directory is not created until EnsureDir is called.
func New(dir string) (*Manifest, error) {
	if dir == "" {
		return nil, errors.New("manifest directory cannot be empty")
	}
	return &Manifest{dir: dir}, nil
}

// EnsureDir creates the manifest directory if it does not exist.
func (m *Manifest) EnsureDir() error {
	return os.MkdirAll(m.dir, 0o755)
}

// LogScan logs a scan operation and returns the created entry.
func (m *Manifest) LogScan(files []FileRecord) (*Entry, error) {
	return m.log(OpScan, files)
}

// LogDelete logs a delete operation and returns the created entry.
func (m *Manifest) LogDelete(files []FileRecord) (*Entry, error) {
	return m.log(OpDelete, files)
}

// log creates and persists a manifest entry for the given operation.
func (m *Manifest) log(op OperationType, files []FileRecord) (*Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	id := generateID(op)

	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}

	entry := &Entry{
		ID:        id,
		Timestamp: now,
		Operation: op,
		Files:     files,
		Summary: Summary{
			TotalFiles: int64(len(files)),
			TotalBytes: totalBytes,
		},
	}

	if err := m.writeEntry(entry); err != nil {
		return nil, fmt.Errorf("failed to write manifest entry: %w", err)
	}

	return entry, nil
}

// writeEntry writes an entry to a JSON file in the manifest directory.
func (m *Manifest) writeEntry(entry *Entry) error {
	filename := m.entryFilename(entry)
	filePath := filepath.Join(m.dir, filename)

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	// Write atomically using a temp file and rename
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		// Cleanup temp file on rename failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// entryFilename generates a filename for an entry based on its ID.
// The ID already contains timestamp and operation, ensuring unique filenames.
func (m *Manifest) entryFilename(entry *Entry) string {
	return fmt.Sprintf("%s.json", entry.ID)
}

// List returns all manifest entries sorted by timestamp descending (newest first).
// If limit is 0 or negative, all entries are returned.
func (m *Manifest) List(limit int) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("failed to read manifest directory: %w", err)
	}

	var entries []Entry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		entry, err := m.readEntryFile(f.Name())
		if err != nil {
			// Skip files that can't be parsed
			continue
		}
		entries = append(entries, *entry)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	// Apply limit
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	// Ensure we return an empty slice, not nil
	if entries == nil {
		entries = []Entry{}
	}

	return entries, nil
}

// Get retrieves a specific entry by ID.
func (m *Manifest) Get(id string) (*Entry, error) {
	if id == "" {
		return nil, errors.New("entry ID cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		entry, err := m.readEntryFile(f.Name())
		if err != nil {
			continue
		}

		if entry.ID == id {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("entry not found: %s", id)
}

// readEntryFile reads and parses a manifest entry from a JSON file.
func (m *Manifest) readEntryFile(filename string) (*Entry, error) {
	filePath := filepath.Join(m.dir, filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
	}

	return &entry, nil
}

// Cleanup removes entries older than retentionDays.
func (m *Manifest) Cleanup(retentionDays int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	files, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read manifest directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(m.dir, f.Name())

		info, err := f.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				// Log error but continue cleanup
				continue
			}
		}
	}

	return nil
}

// generateID creates a unique ID like "scan-2024-06-15T10-30-00-abc123".
func generateID(op OperationType) string {
	ts := time.Now().UTC().Format("2006-01-02T15-04-05")

	// Add random suffix for uniqueness
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		// Fallback to nanoseconds if crypto/rand fails
		suffix = []byte(fmt.Sprintf("%06d", time.Now().Nanosecond()%1000000))
	}

	return fmt.Sprintf("%s-%s-%s", op, ts, hex.EncodeToString(suffix))
}
