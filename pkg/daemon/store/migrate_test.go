package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

func TestMigrateFromV1ToV2(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Simulate a v1 database (entries without large files index)
	entries := []*store.Entry{
		{Path: "/root", Size: 0, IsDir: true},
		{Path: "/root/small.txt", Size: 100, ModTime: 1000, IsDir: false},
		{Path: "/root/medium.txt", Size: 5000, ModTime: 2000, IsDir: false},
		{Path: "/root/large.bin", Size: 15000000, ModTime: 3000, IsDir: false},  // 15 MB
		{Path: "/root/huge.bin", Size: 50000000, ModTime: 4000, IsDir: false},   // 50 MB
	}

	for _, e := range entries {
		if err := s.Put(e); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Verify no large files index yet
	if s.HasLargeFilesIndex("/root") {
		t.Error("Should not have large files index before migration")
	}

	// Run migration with 10 MB threshold
	threshold := int64(10 * 1024 * 1024)
	var progressCalls int
	onProgress := func(p store.MigrationProgress) {
		progressCalls++
	}

	count, err := s.Migrate(context.Background(), threshold, onProgress)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 migration, got %d", count)
	}

	// Verify schema is updated
	schema := s.GetSchema()
	if schema == nil {
		t.Fatal("Expected schema to exist after migration")
	}
	if schema.Version != store.CurrentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", store.CurrentSchemaVersion, schema.Version)
	}

	// Verify large files index was created
	if !s.HasLargeFilesIndex("/root") {
		t.Error("Should have large files index after migration")
	}

	// Query large files
	files, err := s.GetLargeFiles("/root", threshold, 10)
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 large files, got %d", len(files))
	}

	// Verify progress was reported
	if progressCalls == 0 {
		t.Error("Expected progress callbacks during migration")
	}
}

func TestMigrateIdempotent(t *testing.T) {
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

	// Migration should be a no-op
	count, err := s.Migrate(context.Background(), 10*1024*1024, nil)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 migrations (already up to date), got %d", count)
	}
}

func TestMigrateCancellation(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// Add many entries to make migration take longer
	for i := 0; i < 100; i++ {
		err := s.Put(&store.Entry{
			Path:  "/root/file" + string(rune('0'+i%10)) + ".txt",
			Size:  int64(i * 100),
			IsDir: false,
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.Migrate(ctx, 10*1024*1024, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}
