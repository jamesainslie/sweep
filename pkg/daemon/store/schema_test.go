package store_test

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

func TestSchemaGetSet(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Initially no schema
	if schema := s.GetSchema(); schema != nil {
		t.Errorf("Expected nil schema initially, got %+v", schema)
	}

	// Set schema
	now := time.Now()
	err = s.SetSchema(&store.Schema{
		Version:   2,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("SetSchema failed: %v", err)
	}

	// Get it back
	schema := s.GetSchema()
	if schema == nil {
		t.Fatal("Expected schema to exist")
	}
	if schema.Version != 2 {
		t.Errorf("Expected version 2, got %d", schema.Version)
	}
}

func TestNeedsMigration(t *testing.T) {
	t.Run("empty database", func(t *testing.T) {
		s, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer s.Close()

		// Empty database doesn't need migration
		if s.NeedsMigration() {
			t.Error("Empty database should not need migration")
		}
	})

	t.Run("old database without schema", func(t *testing.T) {
		s, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer s.Close()

		// Add an entry to simulate old database
		err = s.Put(&store.Entry{Path: "/test/file.txt", Size: 100})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// Should need migration
		if !s.NeedsMigration() {
			t.Error("Database with entries but no schema should need migration")
		}
	})

	t.Run("database with current schema", func(t *testing.T) {
		s, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer s.Close()

		// Set current schema
		err = s.SetSchema(&store.Schema{Version: store.CurrentSchemaVersion})
		if err != nil {
			t.Fatalf("SetSchema failed: %v", err)
		}

		// Should not need migration
		if s.NeedsMigration() {
			t.Error("Database with current schema should not need migration")
		}
	})

	t.Run("database with old schema", func(t *testing.T) {
		s, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer s.Close()

		// Set old schema
		err = s.SetSchema(&store.Schema{Version: 1})
		if err != nil {
			t.Fatalf("SetSchema failed: %v", err)
		}

		// Should need migration
		if !s.NeedsMigration() {
			t.Error("Database with old schema should need migration")
		}
	})
}
