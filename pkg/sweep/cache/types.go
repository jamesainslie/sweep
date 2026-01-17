package cache

import (
	"bytes"
	"encoding/gob"
)

// CacheVersion is incremented when the cache format changes.
const CacheVersion = 1

// KeySeparator separates root from relative path in cache keys.
const KeySeparator = '\x00'

// CachedEntry represents a cached filesystem entry.
type CachedEntry struct {
	IsDir    bool
	Size     int64    // File size in bytes (0 for directories)
	Mtime    int64    // Modification time as UnixNano
	Children []string // Child names for directories, nil for files
}

// Encode serializes the entry to bytes using gob.
func (e *CachedEntry) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode deserializes bytes into the entry using gob.
func (e *CachedEntry) Decode(data []byte) error {
	return gob.NewDecoder(bytes.NewReader(data)).Decode(e)
}

// MakeKey creates a cache key from root and relative path.
// Format: <root>\x00<relative_path>
func MakeKey(root, relPath string) []byte {
	if relPath == "" {
		return []byte(root + string(KeySeparator))
	}
	return []byte(root + string(KeySeparator) + relPath)
}

// ParseKey extracts root and relative path from a cache key.
func ParseKey(key []byte) (root, relPath string) {
	idx := bytes.IndexByte(key, KeySeparator)
	if idx == -1 {
		return string(key), ""
	}
	return string(key[:idx]), string(key[idx+1:])
}

// MakeKeyPrefix returns the prefix for all keys under a root.
func MakeKeyPrefix(root string) []byte {
	return []byte(root + string(KeySeparator))
}
