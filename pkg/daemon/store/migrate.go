package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// MigrationProgress reports migration progress.
type MigrationProgress struct {
	FromVersion  int
	ToVersion    int
	EntriesTotal int64
	EntriesDone  int64
	CurrentPath  string
}

// MigrationProgressFunc is called with progress updates during migration.
type MigrationProgressFunc func(MigrationProgress)

// Migrate runs any pending migrations to bring the database up to current schema.
// Returns the number of migrations run, or an error.
func (s *Store) Migrate(ctx context.Context, largeFileThreshold int64, onProgress MigrationProgressFunc) (int, error) {
	schema := s.GetSchema()
	fromVersion := 0
	if schema != nil {
		fromVersion = schema.Version
	} else if s.hasAnyEntries() {
		// Database has entries but no schema = v1 (original format)
		fromVersion = 1
	}

	if fromVersion >= CurrentSchemaVersion {
		return 0, nil // Already up to date
	}

	migrationsRun := 0

	// Run migrations in order
	for version := fromVersion + 1; version <= CurrentSchemaVersion; version++ {
		select {
		case <-ctx.Done():
			return migrationsRun, ctx.Err()
		default:
		}

		var err error
		switch version {
		case 2:
			err = s.migrateToV2(ctx, largeFileThreshold, onProgress)
		}

		if err != nil {
			return migrationsRun, err
		}

		// Update schema version after each successful migration
		if err := s.SetSchema(&Schema{
			Version:   version,
			UpdatedAt: time.Now(),
		}); err != nil {
			return migrationsRun, err
		}

		migrationsRun++
	}

	return migrationsRun, nil
}

// migrateToV2 builds the large files index and metadata from existing entries.
func (s *Store) migrateToV2(ctx context.Context, largeFileThreshold int64, onProgress MigrationProgressFunc) error {
	// First, count total entries for progress reporting
	var totalEntries int64
	if onProgress != nil {
		totalEntries = s.countAllEntries()
	}

	// Track stats as we go
	var entriesDone int64
	var filesCount, dirsCount int64
	var largeFiles []*Entry
	roots := make(map[string]*IndexMeta)

	// Scan all entries
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			item := it.Item()
			key := item.Key()

			// Skip non-entry keys (metadata, large files index, schema)
			keyStr := string(key)
			if len(keyStr) >= 2 {
				prefix := keyStr[:2]
				if prefix == prefixLargeFile || prefix == prefixMeta {
					continue
				}
			}

			err := item.Value(func(val []byte) error {
				var entry Entry
				if err := json.Unmarshal(val, &entry); err != nil {
					return nil //nolint:nilerr // intentionally skip malformed entries
				}

				if entry.IsDir {
					dirsCount++
				} else {
					filesCount++
					// Check if this is a large file
					if entry.Size >= largeFileThreshold {
						largeFiles = append(largeFiles, &Entry{
							Path:    entry.Path,
							Size:    entry.Size,
							ModTime: entry.ModTime,
						})
					}
				}

				entriesDone++

				// Report progress periodically
				if onProgress != nil && entriesDone%10000 == 0 {
					onProgress(MigrationProgress{
						FromVersion:  1,
						ToVersion:    2,
						EntriesTotal: totalEntries,
						EntriesDone:  entriesDone,
						CurrentPath:  entry.Path,
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
	if err != nil {
		return err
	}

	// Write large files index
	if len(largeFiles) > 0 {
		if err := s.AddLargeFileBatch(largeFiles); err != nil {
			return err
		}
	}

	// For now, we store a single root metadata entry
	// In the future, we could track multiple indexed roots
	if filesCount > 0 || dirsCount > 0 {
		// Find the common root from entries
		// For simplicity, we'll store under empty key which means "all"
		roots[""] = &IndexMeta{
			Files: filesCount,
			Dirs:  dirsCount,
		}

		for root, meta := range roots {
			if err := s.SetIndexMeta(root, meta); err != nil {
				return err
			}
		}
	}

	// Final progress update
	if onProgress != nil {
		onProgress(MigrationProgress{
			FromVersion:  1,
			ToVersion:    2,
			EntriesTotal: totalEntries,
			EntriesDone:  entriesDone,
		})
	}

	return nil
}

// countAllEntries counts all entries in the store (for progress reporting).
func (s *Store) countAllEntries() int64 {
	var count int64
	_ = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			keyStr := string(key)
			// Only count entry keys
			if len(keyStr) >= 2 {
				prefix := keyStr[:2]
				if prefix == prefixLargeFile || prefix == prefixMeta {
					continue
				}
			}
			count++
		}
		return nil
	})
	return count
}
