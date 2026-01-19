package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createTestTree(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "validator-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Create structure:
	// root/
	//   file1.txt (1KB)
	//   subdir/
	//     file2.txt (2KB)

	if err := os.WriteFile(filepath.Join(root, "file1.txt"), make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(root, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(subdir, "file2.txt"), make([]byte, 2048), 0644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestValidatorEmptyCache(t *testing.T) {
	root := createTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	store, err := OpenStore(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	v := NewValidator(store)
	result, err := v.Validate(root)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Empty cache means root is stale
	if len(result.StaleDirs) != 1 || result.StaleDirs[0] != root {
		t.Errorf("expected root in stale dirs, got %v", result.StaleDirs)
	}
}

func TestValidatorFullyCached(t *testing.T) {
	root := createTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	store, err := OpenStore(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Populate cache to match filesystem
	populateCacheFromFS(t, store, root)

	v := NewValidator(store)
	result, err := v.Validate(root)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Fully cached and unchanged = no stale dirs
	if len(result.StaleDirs) != 0 {
		t.Errorf("expected no stale dirs, got %v", result.StaleDirs)
	}

	// Should have cached files
	if len(result.ValidFiles) == 0 {
		t.Error("expected valid files from cache")
	}
}

func TestValidatorDetectsNewFile(t *testing.T) {
	root := createTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	store, err := OpenStore(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Populate cache
	populateCacheFromFS(t, store, root)

	// Add a new file (this changes the directory mtime)
	time.Sleep(10 * time.Millisecond)
	newFile := filepath.Join(root, "newfile.txt")
	if err := os.WriteFile(newFile, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewValidator(store)
	result, err := v.Validate(root)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Root should be stale because its mtime changed when newfile was added
	if len(result.StaleDirs) == 0 {
		t.Error("expected stale dirs after adding new file")
	}
}

func TestValidatorDetectsDeletedFile(t *testing.T) {
	root := createTestTree(t)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cacheDir)

	store, err := OpenStore(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Populate cache
	populateCacheFromFS(t, store, root)

	// Delete a file
	if err := os.Remove(filepath.Join(root, "file1.txt")); err != nil {
		t.Fatal(err)
	}

	v := NewValidator(store)
	result, err := v.Validate(root)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Deleting a file changes the parent directory's mtime,
	// so the directory should be marked as stale for rescan
	if len(result.StaleDirs) == 0 && len(result.DeletedPaths) == 0 {
		t.Error("expected stale dirs or deleted paths after file deletion")
	}
}

// populateCacheFromFS is a helper to populate cache from filesystem.
func populateCacheFromFS(t *testing.T, store *Store, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			relPath = ""
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		entry := &CachedEntry{
			IsDir: d.IsDir(),
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		}

		if d.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, e := range entries {
				entry.Children = append(entry.Children, e.Name())
			}
		}

		return store.Put(root, relPath, entry)
	})

	if err != nil {
		t.Fatalf("populateCacheFromFS failed: %v", err)
	}
}
