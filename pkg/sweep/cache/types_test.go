package cache

import (
	"bytes"
	"testing"
	"time"
)

func TestCachedEntryEncodeDecode(t *testing.T) {
	original := CachedEntry{
		IsDir:    true,
		Size:     0,
		Mtime:    time.Now().UnixNano(),
		Children: []string{"file1.txt", "subdir"},
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	var decoded CachedEntry
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.IsDir != original.IsDir {
		t.Errorf("IsDir mismatch: got %v, want %v", decoded.IsDir, original.IsDir)
	}
	if decoded.Mtime != original.Mtime {
		t.Errorf("Mtime mismatch: got %v, want %v", decoded.Mtime, original.Mtime)
	}
	if len(decoded.Children) != len(original.Children) {
		t.Errorf("Children length mismatch: got %v, want %v", len(decoded.Children), len(original.Children))
	}
}

func TestCachedEntryFile(t *testing.T) {
	entry := CachedEntry{
		IsDir: false,
		Size:  1024 * 1024,
		Mtime: time.Now().UnixNano(),
	}

	encoded, err := entry.Encode()
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	var decoded CachedEntry
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Size != entry.Size {
		t.Errorf("Size mismatch: got %v, want %v", decoded.Size, entry.Size)
	}
}

func TestMakeKey(t *testing.T) {
	tests := []struct {
		root     string
		relPath  string
		expected string
	}{
		{"/Users/james", "", "/Users/james\x00"},
		{"/Users/james", "Documents", "/Users/james\x00Documents"},
		{"/Users/james", "Documents/file.txt", "/Users/james\x00Documents/file.txt"},
	}

	for _, tt := range tests {
		key := MakeKey(tt.root, tt.relPath)
		if !bytes.Equal(key, []byte(tt.expected)) {
			t.Errorf("MakeKey(%q, %q) = %q, want %q", tt.root, tt.relPath, key, tt.expected)
		}
	}
}

func TestParseKey(t *testing.T) {
	key := MakeKey("/Users/james", "Documents/file.txt")
	root, relPath := ParseKey(key)

	if root != "/Users/james" {
		t.Errorf("root = %q, want %q", root, "/Users/james")
	}
	if relPath != "Documents/file.txt" {
		t.Errorf("relPath = %q, want %q", relPath, "Documents/file.txt")
	}
}
