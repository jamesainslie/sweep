package store

import (
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Schema versions:
// 1 - Initial version (entries only)
// 2 - Added large files index (l:) and metadata (m:)
const CurrentSchemaVersion = 2

const schemaKey = "m:__schema__"

// Schema holds database schema information.
type Schema struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetSchema returns the current schema version, or nil if not set.
func (s *Store) GetSchema() *Schema {
	var schema *Schema

	_ = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(schemaKey))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			schema = &Schema{}
			return json.Unmarshal(val, schema)
		})
	})

	return schema
}

// SetSchema stores the schema version.
func (s *Store) SetSchema(schema *Schema) error {
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(schemaKey), data)
	})
}

// NeedsMigration returns true if the database needs migration.
func (s *Store) NeedsMigration() bool {
	schema := s.GetSchema()
	if schema == nil {
		// No schema = old database, check if it has any data
		return s.hasAnyEntries()
	}
	return schema.Version < CurrentSchemaVersion
}

// hasAnyEntries checks if the store has any entries (indicating an old database).
func (s *Store) hasAnyEntries() bool {
	var found bool
	_ = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		it.Rewind()
		if it.Valid() {
			// Skip schema key if it's the first
			key := it.Item().Key()
			if string(key) != schemaKey {
				found = true
			} else {
				it.Next()
				found = it.Valid()
			}
		}
		return nil
	})
	return found
}
