package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/sweep/pkg/sweep/cache"
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

func BenchmarkScanCold(b *testing.B) {
	root := createBenchTree(b, 1000)
	defer os.RemoveAll(root)

	b.ResetTimer()
	for b.Loop() {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   nil, // No cache
		})
		if _, err := s.Scan(context.Background()); err != nil {
			b.Fatalf("scan failed: %v", err)
		}
	}
}

func BenchmarkScanWarm(b *testing.B) {
	root := createBenchTree(b, 1000)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-bench-*")
	if err != nil {
		b.Fatalf("failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	c, err := cache.Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		b.Fatalf("failed to open cache: %v", err)
	}
	defer c.Close()

	// Warm up cache with first scan
	s := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})
	if _, err := s.Scan(context.Background()); err != nil {
		b.Fatalf("warmup scan failed: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   c,
		})
		if _, err := s.Scan(context.Background()); err != nil {
			b.Fatalf("scan failed: %v", err)
		}
	}
}

func BenchmarkScanCold_Large(b *testing.B) {
	root := createBenchTree(b, 5000)
	defer os.RemoveAll(root)

	b.ResetTimer()
	for b.Loop() {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   nil,
		})
		if _, err := s.Scan(context.Background()); err != nil {
			b.Fatalf("scan failed: %v", err)
		}
	}
}

func BenchmarkScanWarm_Large(b *testing.B) {
	root := createBenchTree(b, 5000)
	defer os.RemoveAll(root)

	cacheDir, err := os.MkdirTemp("", "cache-bench-*")
	if err != nil {
		b.Fatalf("failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	c, err := cache.Open(filepath.Join(cacheDir, "cache"))
	if err != nil {
		b.Fatalf("failed to open cache: %v", err)
	}
	defer c.Close()

	// Warm up cache
	s := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})
	if _, err := s.Scan(context.Background()); err != nil {
		b.Fatalf("warmup scan failed: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   c,
		})
		if _, err := s.Scan(context.Background()); err != nil {
			b.Fatalf("scan failed: %v", err)
		}
	}
}
