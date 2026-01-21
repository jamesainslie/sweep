package indexer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon/indexer"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

func createTestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Create directory structure
	dirs := []string{"a", "b", "a/nested"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create files
	files := map[string]int{
		"a/small.txt":      100,
		"a/large.txt":      10000,
		"a/nested/big.dat": 50000,
		"b/medium.txt":     5000,
	}
	for name, size := range files {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestIndexerIndexPath(t *testing.T) {
	root := createTestTree(t)
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	idx := indexer.New(s)
	// Lower threshold for testing (test files are 100-50000 bytes)
	idx.MinLargeFileSize = 5000

	// Index the tree
	result, err := idx.Index(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	if result.FilesIndexed != 4 {
		t.Errorf("Expected 4 files, got %d", result.FilesIndexed)
	}

	if result.DirsIndexed < 3 {
		t.Errorf("Expected at least 3 dirs, got %d", result.DirsIndexed)
	}

	// Verify we can query large files
	large, err := s.GetLargeFiles(root, 5000, 10)
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}

	// Files >= 5000: a/large.txt (10000), a/nested/big.dat (50000), b/medium.txt (5000)
	if len(large) != 3 {
		t.Errorf("Expected 3 large files (>=5000), got %d", len(large))
	}
}

func TestIndexerProgress(t *testing.T) {
	root := createTestTree(t)
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	idx := indexer.New(s)

	var progressCalls int
	progress := func(_ indexer.Progress) {
		progressCalls++
	}

	_, err = idx.Index(context.Background(), root, progress)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	if progressCalls == 0 {
		t.Error("Expected progress callbacks")
	}
}

func TestAdditiveIndexing(t *testing.T) {
	// Create temp directories: tmpDir/Downloads, tmpDir/Desktop
	tmpDir := t.TempDir()
	downloads := filepath.Join(tmpDir, "Downloads")
	desktop := filepath.Join(tmpDir, "Desktop")

	if err := os.MkdirAll(downloads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(desktop, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some files in each
	if err := os.WriteFile(filepath.Join(downloads, "file1.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desktop, "file2.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	idx := indexer.New(s)
	ctx := context.Background()

	// Index Downloads first
	result1, err := idx.Index(ctx, downloads, nil)
	if err != nil {
		t.Fatalf("Index Downloads failed: %v", err)
	}
	if len(result1.SubsumedPaths) > 0 {
		t.Errorf("Expected no subsumed paths for first index, got %v", result1.SubsumedPaths)
	}

	// Index Desktop second
	result2, err := idx.Index(ctx, desktop, nil)
	if err != nil {
		t.Fatalf("Index Desktop failed: %v", err)
	}
	if len(result2.SubsumedPaths) > 0 {
		t.Errorf("Expected no subsumed paths for second index, got %v", result2.SubsumedPaths)
	}

	// Verify both are in indexed paths
	paths, err := s.GetIndexedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("Expected 2 indexed paths, got %d: %v", len(paths), paths)
	}

	// Index tmpDir (parent)
	result3, err := idx.Index(ctx, tmpDir, nil)
	if err != nil {
		t.Fatalf("Index tmpDir failed: %v", err)
	}

	// Verify children were subsumed
	if len(result3.SubsumedPaths) != 2 {
		t.Errorf("Expected 2 subsumed paths, got %d: %v", len(result3.SubsumedPaths), result3.SubsumedPaths)
	}

	// Verify only tmpDir remains
	paths, err = s.GetIndexedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Errorf("Expected 1 indexed path after subsumption, got %d: %v", len(paths), paths)
	}
	if len(paths) == 1 && paths[0] != tmpDir {
		t.Errorf("Expected indexed path to be %q, got %q", tmpDir, paths[0])
	}
}

func TestIndexingCoveredPath(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	childDir := filepath.Join(tmpDir, "child")

	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some files
	if err := os.WriteFile(filepath.Join(childDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	idx := indexer.New(s)
	ctx := context.Background()

	// Index parent path first
	_, err = idx.Index(ctx, tmpDir, nil)
	if err != nil {
		t.Fatalf("Index parent failed: %v", err)
	}

	// Try to index child path
	result, err := idx.Index(ctx, childDir, nil)
	if err != nil {
		t.Fatalf("Index child failed: %v", err)
	}

	// Verify it returns early with Cached: true and CoveredBy set
	if !result.Cached {
		t.Error("Expected Cached to be true for covered path")
	}
	if result.CoveredBy != tmpDir {
		t.Errorf("Expected CoveredBy to be %q, got %q", tmpDir, result.CoveredBy)
	}

	// Verify indexed paths are unchanged (still just the parent)
	paths, err := s.GetIndexedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Errorf("Expected 1 indexed path, got %d: %v", len(paths), paths)
	}
}
