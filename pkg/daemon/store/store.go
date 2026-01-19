// Package store provides Badger DB-backed storage for the file index.
package store

import (
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

// Key prefixes for different data types
const (
	prefixEntry     = "e:" // Regular file/dir entries
	prefixLargeFile = "l:" // Large files index (for fast queries)
	prefixMeta      = "m:" // Metadata (counts, etc.)
)

// Entry represents a file or directory in the index.
type Entry struct {
	Path     string   `json:"path"`
	Size     int64    `json:"size"`
	ModTime  int64    `json:"mod_time"`
	IsDir    bool     `json:"is_dir"`
	Children []string `json:"children,omitempty"`
}

// Store is the index storage backed by Badger DB.
type Store struct {
	db *badger.DB
}

// Open opens or creates a store at the given path.
func Open(path string) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // Disable logging

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// Put stores an entry.
func (s *Store) Put(entry *Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(entry.Path), data)
	})
}

// PutBatch stores multiple entries efficiently.
func (s *Store) PutBatch(entries []*Entry) error {
	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := wb.Set([]byte(entry.Path), data); err != nil {
			return err
		}
	}

	return wb.Flush()
}

// Get retrieves an entry by path.
func (s *Store) Get(path string) (*Entry, error) {
	var entry Entry

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(path))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &entry)
		})
	})

	if err != nil {
		return nil, err
	}

	return &entry, nil
}

// GetLargeFiles returns files >= minSize under the given root path.
// Uses the pre-computed large files index for fast queries.
func (s *Store) GetLargeFiles(root string, minSize int64, limit int) ([]*Entry, error) {
	var results []*Entry

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		// Use large files index: l:<root>/<path> -> size (8 bytes)
		prefix := []byte(prefixLargeFile + root)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if limit > 0 && len(results) >= limit {
				break
			}

			item := it.Item()
			key := item.Key()
			// Extract path from key (remove "l:" prefix)
			path := string(key[len(prefixLargeFile):])

			err := item.Value(func(val []byte) error {
				if len(val) < 16 {
					return nil // Invalid entry
				}
				size := int64(binary.BigEndian.Uint64(val[0:8]))
				modTime := int64(binary.BigEndian.Uint64(val[8:16]))

				if size >= minSize {
					results = append(results, &Entry{
						Path:    path,
						Size:    size,
						ModTime: modTime,
						IsDir:   false,
					})
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Sort by size descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Size > results[j].Size
	})

	return results, err
}

// AddLargeFile adds a file to the large files index for fast queries.
// Call this during indexing for files that meet the size threshold.
func (s *Store) AddLargeFile(path string, size, modTime int64) error {
	key := []byte(prefixLargeFile + path)
	val := make([]byte, 16)
	binary.BigEndian.PutUint64(val[0:8], uint64(size))
	binary.BigEndian.PutUint64(val[8:16], uint64(modTime))

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, val)
	})
}

// AddLargeFileBatch adds multiple files to the large files index efficiently.
func (s *Store) AddLargeFileBatch(files []*Entry) error {
	if len(files) == 0 {
		return nil
	}

	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	for _, f := range files {
		key := []byte(prefixLargeFile + f.Path)
		val := make([]byte, 16)
		binary.BigEndian.PutUint64(val[0:8], uint64(f.Size))
		binary.BigEndian.PutUint64(val[8:16], uint64(f.ModTime))
		if err := wb.Set(key, val); err != nil {
			return err
		}
	}

	return wb.Flush()
}

// RemoveLargeFile removes a file from the large files index.
func (s *Store) RemoveLargeFile(path string) error {
	key := []byte(prefixLargeFile + path)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// RebuildLargeFilesIndex rebuilds the large files index from existing entries.
// This is used for migration when upgrading from older versions.
func (s *Store) RebuildLargeFilesIndex(root string, minSize int64) (int, error) {
	var largeFiles []*Entry
	var count int

	// Scan all entries and collect large files
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(root)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Skip entries that are already in the large files index
			key := it.Item().Key()
			if len(key) > 2 && string(key[:2]) == prefixLargeFile {
				continue
			}

			err := it.Item().Value(func(val []byte) error {
				var entry Entry
				if err := json.Unmarshal(val, &entry); err != nil {
					return nil // Skip invalid entries
				}
				if !entry.IsDir && entry.Size >= minSize {
					largeFiles = append(largeFiles, &entry)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Write to large files index
	if len(largeFiles) > 0 {
		if err := s.AddLargeFileBatch(largeFiles); err != nil {
			return 0, err
		}
		count = len(largeFiles)
	}

	return count, nil
}

// HasLargeFilesIndex checks if the large files index exists for a root.
func (s *Store) HasLargeFilesIndex(root string) bool {
	var found bool
	_ = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(prefixLargeFile + root)
		it.Seek(prefix)
		found = it.ValidForPrefix(prefix)
		return nil
	})
	return found
}

// Delete removes an entry.
func (s *Store) Delete(path string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(path))
	})
}

// DeletePrefix removes all entries with the given path prefix.
func (s *Store) DeletePrefix(prefix string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		var keysToDelete [][]byte
		prefixBytes := []byte(prefix)

		for it.Seek(prefixBytes); it.ValidForPrefix(prefixBytes); it.Next() {
			key := it.Item().KeyCopy(nil)
			keysToDelete = append(keysToDelete, key)
		}

		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetIndexedRoot returns the root path if it exists in the index.
func (s *Store) GetIndexedRoot(root string) (*Entry, error) {
	return s.Get(root)
}

// CountEntries returns the number of entries under a path.
func (s *Store) CountEntries(prefix string) (files, dirs int64, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		prefixBytes := []byte(prefix)
		for it.Seek(prefixBytes); it.ValidForPrefix(prefixBytes); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				var entry Entry
				if err := json.Unmarshal(val, &entry); err != nil {
					return err
				}
				if entry.IsDir {
					dirs++
				} else {
					files++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return files, dirs, err
}

// HasIndex checks if a path has been indexed.
func (s *Store) HasIndex(root string) bool {
	_, err := s.Get(root)
	return err == nil
}

// IsPathUnderRoot checks if path is under root.
func IsPathUnderRoot(path, root string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	return strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) || cleanPath == cleanRoot
}
