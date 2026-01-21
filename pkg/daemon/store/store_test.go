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

func TestIndexedPaths(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initially no indexed paths
	paths, err := s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths failed: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("Expected 0 indexed paths initially, got %d", len(paths))
	}

	// Add some indexed paths
	testPaths := []string{
		"/Users/test/projects",
		"/Users/test/documents",
		"/var/data",
	}

	for _, p := range testPaths {
		if err := s.AddIndexedPath(p); err != nil {
			t.Fatalf("AddIndexedPath(%q) failed: %v", p, err)
		}
	}

	// Verify all paths are tracked
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths failed: %v", err)
	}
	if len(paths) != len(testPaths) {
		t.Errorf("Expected %d indexed paths, got %d", len(testPaths), len(paths))
	}

	// Verify each path is present
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	for _, expected := range testPaths {
		if !pathSet[expected] {
			t.Errorf("Expected path %q to be in indexed paths", expected)
		}
	}

	// Remove one path
	if err := s.RemoveIndexedPath("/Users/test/documents"); err != nil {
		t.Fatalf("RemoveIndexedPath failed: %v", err)
	}

	// Verify removal
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths after removal failed: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("Expected 2 indexed paths after removal, got %d", len(paths))
	}

	// Verify the removed path is gone
	pathSet = make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	if pathSet["/Users/test/documents"] {
		t.Error("Expected /Users/test/documents to be removed")
	}
	if !pathSet["/Users/test/projects"] {
		t.Error("Expected /Users/test/projects to still exist")
	}
	if !pathSet["/var/data"] {
		t.Error("Expected /var/data to still exist")
	}

	// Test idempotent add (adding same path twice should not duplicate)
	if err := s.AddIndexedPath("/Users/test/projects"); err != nil {
		t.Fatalf("AddIndexedPath (duplicate) failed: %v", err)
	}
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths after duplicate add failed: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("Expected 2 indexed paths after duplicate add, got %d", len(paths))
	}

	// Test removing non-existent path (should not error)
	if err := s.RemoveIndexedPath("/nonexistent/path"); err != nil {
		t.Fatalf("RemoveIndexedPath (non-existent) failed: %v", err)
	}
}

func TestPathSubsumption(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Add child paths first
	childPaths := []string{
		"/Users/james/Downloads",
		"/Users/james/Desktop",
		"/Users/james/Documents/work",
	}

	for _, p := range childPaths {
		if err := s.AddIndexedPath(p); err != nil {
			t.Fatalf("AddIndexedPath(%q) failed: %v", p, err)
		}
	}

	// Verify child paths exist
	paths, err := s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths failed: %v", err)
	}
	if len(paths) != 3 {
		t.Errorf("Expected 3 indexed paths, got %d", len(paths))
	}

	// Add parent path with subsumption - should remove all children
	subsumed, err := s.AddIndexedPathWithSubsumption("/Users/james")
	if err != nil {
		t.Fatalf("AddIndexedPathWithSubsumption failed: %v", err)
	}

	// Verify subsumed paths are returned
	if len(subsumed) != 3 {
		t.Errorf("Expected 3 subsumed paths, got %d: %v", len(subsumed), subsumed)
	}

	// Verify subsumed paths contain expected values
	subsumedSet := make(map[string]bool)
	for _, p := range subsumed {
		subsumedSet[p] = true
	}
	for _, expected := range childPaths {
		if !subsumedSet[expected] {
			t.Errorf("Expected %q to be in subsumed paths", expected)
		}
	}

	// Verify only parent remains in indexed paths
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths after subsumption failed: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("Expected 1 indexed path after subsumption, got %d: %v", len(paths), paths)
	}
	if len(paths) > 0 && paths[0] != "/Users/james" {
		t.Errorf("Expected /Users/james to be the only indexed path, got %q", paths[0])
	}

	// Test adding a path that has no children to subsume
	subsumed, err = s.AddIndexedPathWithSubsumption("/var/log")
	if err != nil {
		t.Fatalf("AddIndexedPathWithSubsumption(/var/log) failed: %v", err)
	}
	if len(subsumed) != 0 {
		t.Errorf("Expected 0 subsumed paths for /var/log, got %d", len(subsumed))
	}

	// Verify both paths now exist
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths failed: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("Expected 2 indexed paths, got %d", len(paths))
	}
}

func TestPathSubsumption_EdgeCases(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Test: Adding path that is already indexed (should not error, no subsumption)
	if err := s.AddIndexedPath("/Users/james"); err != nil {
		t.Fatalf("AddIndexedPath failed: %v", err)
	}
	subsumed, err := s.AddIndexedPathWithSubsumption("/Users/james")
	if err != nil {
		t.Fatalf("AddIndexedPathWithSubsumption (same path) failed: %v", err)
	}
	if len(subsumed) != 0 {
		t.Errorf("Expected 0 subsumed paths when adding same path, got %d", len(subsumed))
	}

	// Test: Sibling paths should not be subsumed
	// Add /Users/alice, then add /Users/james (should not subsume /Users/alice)
	if err := s.AddIndexedPath("/Users/alice"); err != nil {
		t.Fatalf("AddIndexedPath(/Users/alice) failed: %v", err)
	}
	subsumed, err = s.AddIndexedPathWithSubsumption("/Users/james")
	if err != nil {
		t.Fatalf("AddIndexedPathWithSubsumption failed: %v", err)
	}
	if len(subsumed) != 0 {
		t.Errorf("Expected 0 subsumed paths (sibling), got %d: %v", len(subsumed), subsumed)
	}

	// Verify both paths exist
	paths, err := s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths failed: %v", err)
	}
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	if !pathSet["/Users/alice"] {
		t.Error("Expected /Users/alice to still exist")
	}
	if !pathSet["/Users/james"] {
		t.Error("Expected /Users/james to exist")
	}

	// Test: Path that looks like child but isn't (e.g., /Users/jamesbond vs /Users/james)
	// This tests proper path boundary handling
	if err := s.AddIndexedPath("/Users/jamesbond"); err != nil {
		t.Fatalf("AddIndexedPath(/Users/jamesbond) failed: %v", err)
	}
	subsumed, err = s.AddIndexedPathWithSubsumption("/Users/james")
	if err != nil {
		t.Fatalf("AddIndexedPathWithSubsumption failed: %v", err)
	}
	// /Users/jamesbond should NOT be subsumed by /Users/james
	for _, p := range subsumed {
		if p == "/Users/jamesbond" {
			t.Error("/Users/jamesbond should NOT be subsumed by /Users/james")
		}
	}

	// Verify /Users/jamesbond still exists
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatalf("GetIndexedPaths failed: %v", err)
	}
	pathSet = make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	if !pathSet["/Users/jamesbond"] {
		t.Error("Expected /Users/jamesbond to still exist (not a child)")
	}
}

func TestIsPathCovered(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Add /Users/james as indexed
	if err := s.AddIndexedPath("/Users/james"); err != nil {
		t.Fatalf("AddIndexedPath failed: %v", err)
	}

	tests := []struct {
		name             string
		path             string
		expectCovered    bool
		expectCoveringBy string
	}{
		{
			name:             "child path is covered",
			path:             "/Users/james/Downloads",
			expectCovered:    true,
			expectCoveringBy: "/Users/james",
		},
		{
			name:             "nested child path is covered",
			path:             "/Users/james/Documents/work/projects",
			expectCovered:    true,
			expectCoveringBy: "/Users/james",
		},
		{
			name:             "exact match is covered",
			path:             "/Users/james",
			expectCovered:    true,
			expectCoveringBy: "/Users/james",
		},
		{
			name:             "unrelated path is not covered",
			path:             "/var/log",
			expectCovered:    false,
			expectCoveringBy: "",
		},
		{
			name:             "parent path is not covered",
			path:             "/Users",
			expectCovered:    false,
			expectCoveringBy: "",
		},
		{
			name:             "sibling path is not covered",
			path:             "/Users/alice",
			expectCovered:    false,
			expectCoveringBy: "",
		},
		{
			name:             "similar prefix but not child is not covered",
			path:             "/Users/jamesbond",
			expectCovered:    false,
			expectCoveringBy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			covered, coveringPath := s.IsPathCovered(tt.path)
			if covered != tt.expectCovered {
				t.Errorf("IsPathCovered(%q) = %v, want %v", tt.path, covered, tt.expectCovered)
			}
			if coveringPath != tt.expectCoveringBy {
				t.Errorf("IsPathCovered(%q) coveringPath = %q, want %q", tt.path, coveringPath, tt.expectCoveringBy)
			}
		})
	}
}

func TestIsPathCovered_MultipleIndexedPaths(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Add multiple indexed paths
	indexedPaths := []string{
		"/Users/james/projects",
		"/var/log",
		"/opt/data",
	}

	for _, p := range indexedPaths {
		if err := s.AddIndexedPath(p); err != nil {
			t.Fatalf("AddIndexedPath(%q) failed: %v", p, err)
		}
	}

	// Test coverage with multiple indexed paths
	tests := []struct {
		path             string
		expectCovered    bool
		expectCoveringBy string
	}{
		{"/Users/james/projects/sweep", true, "/Users/james/projects"},
		{"/var/log/system.log", true, "/var/log"},
		{"/opt/data/cache", true, "/opt/data"},
		{"/Users/james/documents", false, ""},
		{"/etc/config", false, ""},
	}

	for _, tt := range tests {
		covered, coveringPath := s.IsPathCovered(tt.path)
		if covered != tt.expectCovered {
			t.Errorf("IsPathCovered(%q) = %v, want %v", tt.path, covered, tt.expectCovered)
		}
		if coveringPath != tt.expectCoveringBy {
			t.Errorf("IsPathCovered(%q) coveringPath = %q, want %q", tt.path, coveringPath, tt.expectCoveringBy)
		}
	}
}
