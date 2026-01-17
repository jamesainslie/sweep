package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheOpenClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "cache-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cache, err := Open(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestCacheValidateAndGetStale(t *testing.T) {
	// Create test filesystem
	root := createTestTree(t)
	defer os.RemoveAll(root)

	// Create cache
	cacheDir, err := os.MkdirTemp("", "cache-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	cache, err := Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// First call - empty cache, should return root as stale
	validFiles, staleDirs, err := cache.ValidateAndGetStale(root)
	if err != nil {
		t.Fatalf("ValidateAndGetStale failed: %v", err)
	}

	if len(staleDirs) == 0 {
		t.Error("expected stale dirs for empty cache")
	}
	if len(validFiles) != 0 {
		t.Error("expected no valid files for empty cache")
	}
}

func TestCacheUpdate(t *testing.T) {
	root := createTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	cache, err := Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Build entries from filesystem
	entries := make(map[string]*CachedEntry)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if relPath == "." {
			relPath = ""
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		entry := &CachedEntry{
			IsDir: d.IsDir(),
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		}
		if d.IsDir() {
			dirEntries, dirErr := os.ReadDir(path)
			if dirErr != nil {
				return dirErr
			}
			for _, e := range dirEntries {
				entry.Children = append(entry.Children, e.Name())
			}
		}
		entries[relPath] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir failed: %v", err)
	}

	// Update cache
	if err := cache.Update(root, entries); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Now validate should show no stale dirs
	validFiles, staleDirs, err := cache.ValidateAndGetStale(root)
	if err != nil {
		t.Fatalf("ValidateAndGetStale after update failed: %v", err)
	}

	if len(staleDirs) != 0 {
		t.Errorf("expected no stale dirs after update, got %v", staleDirs)
	}
	if len(validFiles) == 0 {
		t.Error("expected valid files after update")
	}
}

func TestCacheGetLargeFiles(t *testing.T) {
	root := createTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	cache, err := Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Populate cache
	entries := make(map[string]*CachedEntry)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if relPath == "." {
			relPath = ""
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		entry := &CachedEntry{
			IsDir: d.IsDir(),
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		}
		if d.IsDir() {
			dirEntries, dirErr := os.ReadDir(path)
			if dirErr != nil {
				return dirErr
			}
			for _, e := range dirEntries {
				entry.Children = append(entry.Children, e.Name())
			}
		}
		entries[relPath] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir failed: %v", err)
	}
	if err := cache.Update(root, entries); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Get files over 1500 bytes (should get file2.txt which is 2KB)
	files, err := cache.GetLargeFiles(root, 1500)
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 large file, got %d", len(files))
	}
}
