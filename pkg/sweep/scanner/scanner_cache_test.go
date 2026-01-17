package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/scanner"
)

func createLargeTestTree(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "scanner-cache-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Create files of varying sizes
	// Small files (under threshold)
	for i := range 5 {
		path := filepath.Join(root, "small", "file"+string(rune('a'+i))+".txt")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			_ = os.RemoveAll(root)
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, 100), 0644); err != nil {
			_ = os.RemoveAll(root)
			t.Fatal(err)
		}
	}

	// Large files (over threshold)
	for i := range 3 {
		path := filepath.Join(root, "large", "file"+string(rune('a'+i))+".dat")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			_ = os.RemoveAll(root)
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, 10*1024), 0644); err != nil { // 10KB
			_ = os.RemoveAll(root)
			t.Fatal(err)
		}
	}

	return root
}

func TestScannerWithCache(t *testing.T) {
	root := createLargeTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "scanner-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	c, err := cache.Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// First scan - populates cache
	s1 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024, // 1KB threshold
		Cache:   c,
	})

	result1, err := s1.Scan(context.Background())
	if err != nil {
		t.Fatalf("First scan failed: %v", err)
	}

	if len(result1.Files) != 3 {
		t.Errorf("Expected 3 large files, got %d", len(result1.Files))
		for _, f := range result1.Files {
			t.Logf("  found: %s (%d bytes)", f.Path, f.Size)
		}
	}

	// Second scan - should use cache
	s2 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})

	start := time.Now()
	result2, err := s2.Scan(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}

	if len(result2.Files) != 3 {
		t.Errorf("Expected 3 large files from cache, got %d", len(result2.Files))
	}

	// Second scan should be faster (cached)
	t.Logf("Cached scan took %v", elapsed)
}

func TestScannerCacheDetectsChanges(t *testing.T) {
	root := createLargeTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "scanner-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	c, err := cache.Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// First scan
	s1 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})
	result1, err := s1.Scan(context.Background())
	if err != nil {
		t.Fatalf("First scan failed: %v", err)
	}
	t.Logf("First scan found %d files", len(result1.Files))

	// Add a new large file - use a different method to ensure mtime changes
	time.Sleep(100 * time.Millisecond) // Ensure mtime changes (some filesystems have low resolution)
	newFile := filepath.Join(root, "large", "newfile.dat")
	if err := os.WriteFile(newFile, make([]byte, 20*1024), 0644); err != nil { // 20KB
		t.Fatal(err)
	}

	// Explicitly touch the parent directory to ensure its mtime is updated
	// This is necessary because some filesystems may not update parent mtime immediately
	now := time.Now()
	if err := os.Chtimes(filepath.Join(root, "large"), now, now); err != nil {
		t.Logf("Warning: could not update parent dir mtime: %v", err)
	}
	// Also update root mtime since we changed a child
	if err := os.Chtimes(root, now, now); err != nil {
		t.Logf("Warning: could not update root mtime: %v", err)
	}

	// Second scan should detect the new file
	s2 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})

	result2, err := s2.Scan(context.Background())
	if err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}

	if len(result2.Files) != 4 {
		t.Errorf("Expected 4 large files after adding one, got %d", len(result2.Files))
		for _, f := range result2.Files {
			t.Logf("  found: %s (%d bytes)", f.Path, f.Size)
		}
	}
}

func TestScannerWithoutCache(t *testing.T) {
	root := createLargeTestTree(t)
	defer os.RemoveAll(root)

	// Scan without cache (Cache: nil)
	s := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   nil, // No cache
	})

	result, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(result.Files) != 3 {
		t.Errorf("Expected 3 large files, got %d", len(result.Files))
	}
}

func TestScannerCacheWithExclusions(t *testing.T) {
	root := createLargeTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "scanner-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	c, err := cache.Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	excludedPath := filepath.Join(root, "large")

	// First scan with exclusions
	s1 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Exclude: []string{excludedPath},
		Cache:   c,
	})

	result1, err := s1.Scan(context.Background())
	if err != nil {
		t.Fatalf("First scan failed: %v", err)
	}

	// Should find 0 files since small files are below threshold
	// and large files are excluded
	if len(result1.Files) != 0 {
		t.Errorf("Expected 0 large files with exclusion, got %d", len(result1.Files))
	}

	// Second scan with same exclusions should also work
	s2 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Exclude: []string{excludedPath},
		Cache:   c,
	})

	result2, err := s2.Scan(context.Background())
	if err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}

	if len(result2.Files) != 0 {
		t.Errorf("Expected 0 large files from cached scan with exclusion, got %d", len(result2.Files))
	}
}

func TestScannerCachePartialStale(t *testing.T) {
	root := createLargeTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "scanner-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	c, err := cache.Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// First scan
	s1 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})
	_, err = s1.Scan(context.Background())
	if err != nil {
		t.Fatalf("First scan failed: %v", err)
	}

	// Create a new subdirectory with large files
	time.Sleep(10 * time.Millisecond)
	newDir := filepath.Join(root, "newdir")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(newDir, "bigfile.dat")
	if err := os.WriteFile(newFile, make([]byte, 15*1024), 0644); err != nil {
		t.Fatal(err)
	}

	// Second scan should detect the new directory and file
	s2 := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})

	result2, err := s2.Scan(context.Background())
	if err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}

	// Should find original 3 + 1 new = 4 files
	if len(result2.Files) != 4 {
		t.Errorf("Expected 4 large files after adding new directory, got %d", len(result2.Files))
		for _, f := range result2.Files {
			t.Logf("  found: %s (%d bytes)", f.Path, f.Size)
		}
	}
}
