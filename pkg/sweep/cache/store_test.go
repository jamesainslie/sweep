package cache

import (
	"os"
	"testing"
	"time"
)

func TestStoreOpenClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestStoreGetPut(t *testing.T) {
	dir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	root := "/Users/test"
	entry := &CachedEntry{
		IsDir:    true,
		Mtime:    time.Now().UnixNano(),
		Children: []string{"a", "b", "c"},
	}

	// Put
	if err := store.Put(root, "", entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	got, err := store.Get(root, "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.IsDir != entry.IsDir {
		t.Errorf("IsDir mismatch")
	}
	if len(got.Children) != len(entry.Children) {
		t.Errorf("Children mismatch: got %d, want %d", len(got.Children), len(entry.Children))
	}
}

func TestStoreGetNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.Get("/nonexistent", "path")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreDelete(t *testing.T) {
	dir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	root := "/Users/test"
	entry := &CachedEntry{IsDir: false, Size: 100, Mtime: time.Now().UnixNano()}

	if err := store.Put(root, "file.txt", entry); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(root, "file.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(root, "file.txt")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStoreListChildren(t *testing.T) {
	dir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	root := "/Users/test"

	// Add root with children
	rootEntry := &CachedEntry{
		IsDir:    true,
		Mtime:    time.Now().UnixNano(),
		Children: []string{"a.txt", "b.txt", "subdir"},
	}
	if err := store.Put(root, "", rootEntry); err != nil {
		t.Fatal(err)
	}

	// Get and verify children
	entry, err := store.Get(root, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(entry.Children) != 3 {
		t.Errorf("expected 3 children, got %d", len(entry.Children))
	}
}
