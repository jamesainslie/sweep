package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("creates manifest with valid directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		m, err := New(dir)
		if err != nil {
			t.Fatalf("New() error = %v, want nil", err)
		}
		if m == nil {
			t.Fatal("New() returned nil")
		}
	})

	t.Run("returns error for empty directory", func(t *testing.T) {
		t.Parallel()

		_, err := New("")
		if err == nil {
			t.Fatal("New() error = nil, want error for empty directory")
		}
	})
}

func TestManifest_EnsureDir(t *testing.T) {
	t.Parallel()

	t.Run("creates directory if not exists", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		manifestDir := filepath.Join(tmpDir, "manifests")

		m, err := New(manifestDir)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		if err := m.EnsureDir(); err != nil {
			t.Fatalf("EnsureDir() error = %v", err)
		}

		info, err := os.Stat(manifestDir)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("path is not a directory")
		}
	})

	t.Run("succeeds if directory already exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		m, err := New(dir)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		if err := m.EnsureDir(); err != nil {
			t.Fatalf("EnsureDir() error = %v", err)
		}
	})
}

func TestManifest_LogScan(t *testing.T) {
	t.Parallel()

	t.Run("logs scan operation successfully", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		files := []FileRecord{
			{Path: "/tmp/file1.txt", Size: 100, ModTime: time.Now()},
			{Path: "/tmp/file2.txt", Size: 200, ModTime: time.Now()},
		}

		entry, err := m.LogScan(files)
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		if entry.Operation != OpScan {
			t.Errorf("Operation = %v, want %v", entry.Operation, OpScan)
		}
		if entry.Summary.TotalFiles != 2 {
			t.Errorf("TotalFiles = %v, want 2", entry.Summary.TotalFiles)
		}
		if entry.Summary.TotalBytes != 300 {
			t.Errorf("TotalBytes = %v, want 300", entry.Summary.TotalBytes)
		}
		if len(entry.Files) != 2 {
			t.Errorf("len(Files) = %v, want 2", len(entry.Files))
		}
	})

	t.Run("generates unique ID with scan prefix", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		entry, err := m.LogScan([]FileRecord{})
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		if len(entry.ID) == 0 {
			t.Error("ID is empty")
		}
		// ID should start with "scan-"
		if len(entry.ID) < 5 || entry.ID[:5] != "scan-" {
			t.Errorf("ID = %v, want prefix 'scan-'", entry.ID)
		}
	})

	t.Run("persists entry to file", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		files := []FileRecord{
			{Path: "/tmp/test.txt", Size: 50, ModTime: time.Now()},
		}

		entry, err := m.LogScan(files)
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		// Verify file was created
		retrieved, err := m.Get(entry.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if retrieved.ID != entry.ID {
			t.Errorf("retrieved ID = %v, want %v", retrieved.ID, entry.ID)
		}
	})
}

func TestManifest_LogDelete(t *testing.T) {
	t.Parallel()

	t.Run("logs delete operation successfully", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		now := time.Now()
		files := []FileRecord{
			{Path: "/tmp/deleted.txt", Size: 500, ModTime: now, DeletedAt: now},
		}

		entry, err := m.LogDelete(files)
		if err != nil {
			t.Fatalf("LogDelete() error = %v", err)
		}

		if entry.Operation != OpDelete {
			t.Errorf("Operation = %v, want %v", entry.Operation, OpDelete)
		}
		if entry.Summary.TotalFiles != 1 {
			t.Errorf("TotalFiles = %v, want 1", entry.Summary.TotalFiles)
		}
		if entry.Summary.TotalBytes != 500 {
			t.Errorf("TotalBytes = %v, want 500", entry.Summary.TotalBytes)
		}
	})

	t.Run("generates unique ID with delete prefix", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		entry, err := m.LogDelete([]FileRecord{})
		if err != nil {
			t.Fatalf("LogDelete() error = %v", err)
		}

		if len(entry.ID) < 7 || entry.ID[:7] != "delete-" {
			t.Errorf("ID = %v, want prefix 'delete-'", entry.ID)
		}
	})
}

func TestManifest_List(t *testing.T) {
	t.Parallel()

	t.Run("returns entries sorted by timestamp descending", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		// Create entries with slight delays to ensure different timestamps
		_, err := m.LogScan([]FileRecord{{Path: "/first", Size: 1}})
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}
		time.Sleep(10 * time.Millisecond)

		_, err = m.LogDelete([]FileRecord{{Path: "/second", Size: 2}})
		if err != nil {
			t.Fatalf("LogDelete() error = %v", err)
		}
		time.Sleep(10 * time.Millisecond)

		_, err = m.LogScan([]FileRecord{{Path: "/third", Size: 3}})
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		entries, err := m.List(0) // 0 means no limit
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		if len(entries) != 3 {
			t.Fatalf("len(entries) = %v, want 3", len(entries))
		}

		// Newest first
		for i := 0; i < len(entries)-1; i++ {
			if entries[i].Timestamp.Before(entries[i+1].Timestamp) {
				t.Errorf("entries not sorted: %v before %v", entries[i].Timestamp, entries[i+1].Timestamp)
			}
		}
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		for i := 0; i < 5; i++ {
			_, err := m.LogScan([]FileRecord{{Path: "/test", Size: int64(i)}})
			if err != nil {
				t.Fatalf("LogScan() error = %v", err)
			}
			time.Sleep(5 * time.Millisecond)
		}

		entries, err := m.List(2)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		if len(entries) != 2 {
			t.Errorf("len(entries) = %v, want 2", len(entries))
		}
	})

	t.Run("returns empty slice for empty directory", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		entries, err := m.List(0)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		if entries == nil {
			t.Error("List() returned nil, want empty slice")
		}
		if len(entries) != 0 {
			t.Errorf("len(entries) = %v, want 0", len(entries))
		}
	})
}

func TestManifest_Get(t *testing.T) {
	t.Parallel()

	t.Run("retrieves existing entry", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		files := []FileRecord{
			{Path: "/tmp/gettest.txt", Size: 999, ModTime: time.Now()},
		}

		original, err := m.LogScan(files)
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		retrieved, err := m.Get(original.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if retrieved.ID != original.ID {
			t.Errorf("ID = %v, want %v", retrieved.ID, original.ID)
		}
		if retrieved.Operation != original.Operation {
			t.Errorf("Operation = %v, want %v", retrieved.Operation, original.Operation)
		}
		if len(retrieved.Files) != len(original.Files) {
			t.Errorf("len(Files) = %v, want %v", len(retrieved.Files), len(original.Files))
		}
	})

	t.Run("returns error for non-existent entry", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		_, err := m.Get("nonexistent-id")
		if err == nil {
			t.Fatal("Get() error = nil, want error for non-existent entry")
		}
	})

	t.Run("returns error for empty ID", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		_, err := m.Get("")
		if err == nil {
			t.Fatal("Get() error = nil, want error for empty ID")
		}
	})
}

func TestManifest_Cleanup(t *testing.T) {
	t.Parallel()

	t.Run("removes entries older than retention days", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		// Create an entry
		entry, err := m.LogScan([]FileRecord{{Path: "/test", Size: 1}})
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		// Manually modify the file timestamp to make it old
		files, err := os.ReadDir(m.dir)
		if err != nil {
			t.Fatalf("ReadDir() error = %v", err)
		}

		for _, f := range files {
			filePath := filepath.Join(m.dir, f.Name())
			// Set mod time to 10 days ago
			oldTime := time.Now().AddDate(0, 0, -10)
			if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
				t.Fatalf("Chtimes() error = %v", err)
			}
		}

		// Cleanup entries older than 5 days
		if err := m.Cleanup(5); err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}

		// Entry should be gone
		_, err = m.Get(entry.ID)
		if err == nil {
			t.Error("Get() should return error after cleanup")
		}
	})

	t.Run("keeps entries newer than retention days", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		entry, err := m.LogScan([]FileRecord{{Path: "/test", Size: 1}})
		if err != nil {
			t.Fatalf("LogScan() error = %v", err)
		}

		// Cleanup entries older than 30 days (entry was just created)
		if err := m.Cleanup(30); err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}

		// Entry should still exist
		_, err = m.Get(entry.ID)
		if err != nil {
			t.Errorf("Get() error = %v, entry should still exist", err)
		}
	})

	t.Run("handles empty directory", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		if err := m.Cleanup(7); err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}
	})
}

func TestManifest_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	t.Run("handles concurrent log operations", func(t *testing.T) {
		t.Parallel()
		m := setupTestManifest(t)

		var wg sync.WaitGroup
		errCh := make(chan error, 20)

		// Spawn 10 concurrent scan operations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				files := []FileRecord{
					{Path: "/tmp/concurrent-" + string(rune('A'+idx)), Size: int64(idx * 100)},
				}
				_, err := m.LogScan(files)
				if err != nil {
					errCh <- err
				}
			}(i)
		}

		// Spawn 10 concurrent delete operations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				files := []FileRecord{
					{Path: "/tmp/delete-" + string(rune('A'+idx)), Size: int64(idx * 50)},
				}
				_, err := m.LogDelete(files)
				if err != nil {
					errCh <- err
				}
			}(i)
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("concurrent operation error: %v", err)
		}

		// Verify all entries were created
		entries, err := m.List(0)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		if len(entries) != 20 {
			t.Errorf("len(entries) = %v, want 20", len(entries))
		}
	})
}

func TestGenerateID(t *testing.T) {
	t.Parallel()

	t.Run("generates ID with operation prefix", func(t *testing.T) {
		t.Parallel()

		scanID := generateID(OpScan)
		if len(scanID) < 5 || scanID[:5] != "scan-" {
			t.Errorf("scan ID = %v, want prefix 'scan-'", scanID)
		}

		deleteID := generateID(OpDelete)
		if len(deleteID) < 7 || deleteID[:7] != "delete-" {
			t.Errorf("delete ID = %v, want prefix 'delete-'", deleteID)
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		t.Parallel()

		ids := make(map[string]struct{})
		for i := 0; i < 100; i++ {
			id := generateID(OpScan)
			if _, exists := ids[id]; exists {
				t.Errorf("duplicate ID generated: %v", id)
			}
			ids[id] = struct{}{}
		}
	})
}

func TestEntry_JSONSerialization(t *testing.T) {
	t.Parallel()

	t.Run("serializes and deserializes correctly", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC().Truncate(time.Second)
		entry := Entry{
			ID:        "scan-2024-06-15T10-30-00-abc123",
			Timestamp: now,
			Operation: OpScan,
			Files: []FileRecord{
				{
					Path:    "/tmp/test.txt",
					Size:    1024,
					ModTime: now,
					SHA256:  "abcdef123456",
				},
			},
			Summary: Summary{
				TotalFiles: 1,
				TotalBytes: 1024,
			},
		}

		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		var decoded Entry
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		if decoded.ID != entry.ID {
			t.Errorf("ID = %v, want %v", decoded.ID, entry.ID)
		}
		if decoded.Operation != entry.Operation {
			t.Errorf("Operation = %v, want %v", decoded.Operation, entry.Operation)
		}
		if len(decoded.Files) != len(entry.Files) {
			t.Errorf("len(Files) = %v, want %v", len(decoded.Files), len(entry.Files))
		}
	})
}

// setupTestManifest creates a manifest with a temporary directory for testing.
func setupTestManifest(t *testing.T) *Manifest {
	t.Helper()
	dir := t.TempDir()

	m, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := m.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}

	return m
}
