package cache

import (
	"errors"

	"github.com/dgraph-io/badger/v4"
)

// ErrNotFound is returned when a cache entry doesn't exist.
var ErrNotFound = errors.New("cache entry not found")

// Store wraps Badger for cache operations.
type Store struct {
	db *badger.DB
}

// OpenStore opens or creates a cache store at the given path.
func OpenStore(path string) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // Disable badger logging

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

// Get retrieves a cached entry by root and relative path.
func (s *Store) Get(root, relPath string) (*CachedEntry, error) {
	key := MakeKey(root, relPath)
	var entry CachedEntry

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(entry.Decode)
	})

	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// Put stores a cached entry.
func (s *Store) Put(root, relPath string, entry *CachedEntry) error {
	key := MakeKey(root, relPath)
	value, err := entry.Encode()
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// Delete removes a cached entry.
func (s *Store) Delete(root, relPath string) error {
	key := MakeKey(root, relPath)

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// DeletePrefix removes all entries with the given root prefix.
func (s *Store) DeletePrefix(root string) error {
	prefix := MakeKeyPrefix(root)

	return s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := txn.Delete(it.Item().KeyCopy(nil)); err != nil {
				return err
			}
		}
		return nil
	})
}

// PutBatch stores multiple entries efficiently in a single transaction.
func (s *Store) PutBatch(root string, entries map[string]*CachedEntry) error {
	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	for relPath, entry := range entries {
		key := MakeKey(root, relPath)
		value, err := entry.Encode()
		if err != nil {
			return err
		}
		if err := wb.Set(key, value); err != nil {
			return err
		}
	}

	return wb.Flush()
}
