package store_test

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

func TestStoreBasicOperations(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Put a file entry
	entry := &store.Entry{
		Path:    "/Users/test/file.txt",
		Size:    1024,
		ModTime: time.Now().Unix(),
		IsDir:   false,
	}

	if err := s.Put(entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get it back
	got, err := s.Get("/Users/test/file.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Size != 1024 {
		t.Errorf("Expected size 1024, got %d", got.Size)
	}
}

func TestStoreGetLargeFiles(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Add files to the large files index (used for fast queries)
	largeFiles := []*store.Entry{
		{Path: "/a/medium.txt", Size: 1000, ModTime: 1000, IsDir: false},
		{Path: "/a/large.txt", Size: 10000, ModTime: 2000, IsDir: false},
		{Path: "/b/huge.txt", Size: 100000, ModTime: 3000, IsDir: false},
	}

	if err := s.AddLargeFileBatch(largeFiles); err != nil {
		t.Fatalf("AddLargeFileBatch failed: %v", err)
	}

	// Query for files >= 1000 bytes under /a
	results, err := s.GetLargeFiles("/a", 1000, 10)
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Verify sorting by size descending
	if len(results) >= 2 && results[0].Size < results[1].Size {
		t.Errorf("Results should be sorted by size descending")
	}
}

func TestStorePutBatch(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	entries := []*store.Entry{
		{Path: "/batch/file1.txt", Size: 100, IsDir: false},
		{Path: "/batch/file2.txt", Size: 200, IsDir: false},
		{Path: "/batch/file3.txt", Size: 300, IsDir: false},
	}

	if err := s.PutBatch(entries); err != nil {
		t.Fatalf("PutBatch failed: %v", err)
	}

	// Verify all entries were stored
	for _, e := range entries {
		got, err := s.Get(e.Path)
		if err != nil {
			t.Fatalf("Get failed for %s: %v", e.Path, err)
		}
		if got.Size != e.Size {
			t.Errorf("Expected size %d for %s, got %d", e.Size, e.Path, got.Size)
		}
	}
}

func TestStoreDelete(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	entry := &store.Entry{
		Path:  "/delete/file.txt",
		Size:  100,
		IsDir: false,
	}

	if err := s.Put(entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify it exists
	_, err = s.Get("/delete/file.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Delete it
	if err := s.Delete("/delete/file.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, err = s.Get("/delete/file.txt")
	if err == nil {
		t.Error("Expected error after delete, got nil")
	}
}

func TestStoreDeletePrefix(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	entries := []*store.Entry{
		{Path: "/prefix/a/file1.txt", Size: 100, IsDir: false},
		{Path: "/prefix/a/file2.txt", Size: 200, IsDir: false},
		{Path: "/prefix/b/file3.txt", Size: 300, IsDir: false},
		{Path: "/other/file4.txt", Size: 400, IsDir: false},
	}

	for _, e := range entries {
		if err := s.Put(e); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Delete everything under /prefix/a
	if err := s.DeletePrefix("/prefix/a"); err != nil {
		t.Fatalf("DeletePrefix failed: %v", err)
	}

	// Verify /prefix/a files are gone
	_, err = s.Get("/prefix/a/file1.txt")
	if err == nil {
		t.Error("Expected /prefix/a/file1.txt to be deleted")
	}
	_, err = s.Get("/prefix/a/file2.txt")
	if err == nil {
		t.Error("Expected /prefix/a/file2.txt to be deleted")
	}

	// Verify other files still exist
	_, err = s.Get("/prefix/b/file3.txt")
	if err != nil {
		t.Errorf("Expected /prefix/b/file3.txt to exist: %v", err)
	}
	_, err = s.Get("/other/file4.txt")
	if err != nil {
		t.Errorf("Expected /other/file4.txt to exist: %v", err)
	}
}

func TestStoreCountEntries(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	entries := []*store.Entry{
		{Path: "/count", IsDir: true},
		{Path: "/count/dir1", IsDir: true},
		{Path: "/count/dir1/file1.txt", Size: 100, IsDir: false},
		{Path: "/count/dir1/file2.txt", Size: 200, IsDir: false},
		{Path: "/count/file3.txt", Size: 300, IsDir: false},
	}

	for _, e := range entries {
		if err := s.Put(e); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	files, dirs, err := s.CountEntries("/count")
	if err != nil {
		t.Fatalf("CountEntries failed: %v", err)
	}

	if files != 3 {
		t.Errorf("Expected 3 files, got %d", files)
	}
	if dirs != 2 {
		t.Errorf("Expected 2 dirs, got %d", dirs)
	}
}

func TestStoreHasIndex(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initially no index
	if s.HasIndex("/test/path") {
		t.Error("Expected HasIndex to return false for non-existent path")
	}

	// Add an entry
	entry := &store.Entry{
		Path:  "/test/path",
		IsDir: true,
	}
	if err := s.Put(entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Now it should exist
	if !s.HasIndex("/test/path") {
		t.Error("Expected HasIndex to return true for existing path")
	}
}

func TestIsPathUnderRoot(t *testing.T) {
	tests := []struct {
		path     string
		root     string
		expected bool
	}{
		{"/a/b/c", "/a", true},
		{"/a/b/c", "/a/b", true},
		{"/a/b/c", "/a/b/c", true},
		{"/a/b/c", "/a/b/c/d", false},
		{"/other/path", "/a", false},
		{"/abc", "/a", false}, // /abc is not under /a (no separator)
	}

	for _, tt := range tests {
		got := store.IsPathUnderRoot(tt.path, tt.root)
		if got != tt.expected {
			t.Errorf("IsPathUnderRoot(%q, %q) = %v, expected %v",
				tt.path, tt.root, got, tt.expected)
		}
	}
}

func TestLargeFilesIndex(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initially no large files index
	if s.HasLargeFilesIndex("/test") {
		t.Error("Expected HasLargeFilesIndex to return false initially")
	}

	// Add a large file
	if err := s.AddLargeFile("/test/big.bin", 100000, 1234567890); err != nil {
		t.Fatalf("AddLargeFile failed: %v", err)
	}

	// Now it should have the index
	if !s.HasLargeFilesIndex("/test") {
		t.Error("Expected HasLargeFilesIndex to return true after adding file")
	}

	// Query it back
	results, err := s.GetLargeFiles("/test", 0, 10)
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].Size != 100000 {
		t.Errorf("Expected size 100000, got %d", results[0].Size)
	}

	// Remove the large file
	if err := s.RemoveLargeFile("/test/big.bin"); err != nil {
		t.Fatalf("RemoveLargeFile failed: %v", err)
	}

	// Should be empty now
	results, err = s.GetLargeFiles("/test", 0, 10)
	if err != nil {
		t.Fatalf("GetLargeFiles after remove failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results after removal, got %d", len(results))
	}
}

func TestRebuildLargeFilesIndex(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Add entries using Put (simulating old data without large files index)
	entries := []*store.Entry{
		{Path: "/root/small.txt", Size: 100, ModTime: 1000, IsDir: false},
		{Path: "/root/large1.bin", Size: 15000000, ModTime: 2000, IsDir: false},
		{Path: "/root/large2.bin", Size: 20000000, ModTime: 3000, IsDir: false},
		{Path: "/root/dir", Size: 0, ModTime: 4000, IsDir: true},
	}

	for _, e := range entries {
		if err := s.Put(e); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// No large files index yet
	if s.HasLargeFilesIndex("/root") {
		t.Error("Expected no large files index before rebuild")
	}

	// Rebuild the index (10MB threshold)
	count, err := s.RebuildLargeFilesIndex("/root", 10*1024*1024)
	if err != nil {
		t.Fatalf("RebuildLargeFilesIndex failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 large files indexed, got %d", count)
	}

	// Now query should work
	results, err := s.GetLargeFiles("/root", 10*1024*1024, 10)
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}
