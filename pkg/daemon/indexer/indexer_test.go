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
