package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/sweep/pkg/sweep/scanner"
)

func createBenchTree(b *testing.B, numFiles int) string {
	b.Helper()
	root, err := os.MkdirTemp("", "bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}

	// Create files distributed across directories
	for i := range numFiles {
		dir := filepath.Join(root, "dir"+string(rune('a'+i%26)))
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatalf("failed to create dir: %v", err)
		}

		size := 100 // small
		if i%10 == 0 {
			size = 10 * 1024 // large (10KB)
		}

		if err := os.WriteFile(
			filepath.Join(dir, "file"+string(rune('0'+i%10))+".txt"),
			make([]byte, size),
			0644,
		); err != nil {
			b.Fatalf("failed to write file: %v", err)
		}
	}

	return root
}

func BenchmarkScan(b *testing.B) {
	root := createBenchTree(b, 1000)
	defer os.RemoveAll(root)

	b.ResetTimer()
	for b.Loop() {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
		})
		if _, err := s.Scan(context.Background()); err != nil {
			b.Fatalf("scan failed: %v", err)
		}
	}
}

func BenchmarkScan_Large(b *testing.B) {
	root := createBenchTree(b, 5000)
	defer os.RemoveAll(root)

	b.ResetTimer()
	for b.Loop() {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
		})
		if _, err := s.Scan(context.Background()); err != nil {
			b.Fatalf("scan failed: %v", err)
		}
	}
}
