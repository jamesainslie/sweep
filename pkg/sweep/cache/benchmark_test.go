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
	root, _ := os.MkdirTemp("", "bench-*")

	// Create files distributed across directories
	for i := 0; i < numFiles; i++ {
		dir := filepath.Join(root, "dir"+string(rune('a'+i%26)))
		os.MkdirAll(dir, 0755)

		size := 100 // small
		if i%10 == 0 {
			size = 10 * 1024 // large (10KB)
		}

		os.WriteFile(
			filepath.Join(dir, "file"+string(rune('0'+i%10))+".txt"),
			make([]byte, size),
			0644,
		)
	}

	return root
}

func BenchmarkScanCold(b *testing.B) {
	root := createBenchTree(b, 1000)
	defer os.RemoveAll(root)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   nil, // No cache
		})
		s.Scan(context.Background())
	}
}

func BenchmarkScanWarm(b *testing.B) {
	root := createBenchTree(b, 1000)
	defer os.RemoveAll(root)

	cacheDir, _ := os.MkdirTemp("", "cache-bench-*")
	defer os.RemoveAll(cacheDir)

	c, _ := cache.Open(filepath.Join(cacheDir, "cache"))
	defer c.Close()

	// Warm up cache with first scan
	s := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})
	s.Scan(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   c,
		})
		s.Scan(context.Background())
	}
}

func BenchmarkScanCold_Large(b *testing.B) {
	root := createBenchTree(b, 5000)
	defer os.RemoveAll(root)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   nil,
		})
		s.Scan(context.Background())
	}
}

func BenchmarkScanWarm_Large(b *testing.B) {
	root := createBenchTree(b, 5000)
	defer os.RemoveAll(root)

	cacheDir, _ := os.MkdirTemp("", "cache-bench-*")
	defer os.RemoveAll(cacheDir)

	c, _ := cache.Open(filepath.Join(cacheDir, "cache"))
	defer c.Close()

	// Warm up cache
	s := scanner.New(scanner.Options{
		Root:    root,
		MinSize: 1024,
		Cache:   c,
	})
	s.Scan(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := scanner.New(scanner.Options{
			Root:    root,
			MinSize: 1024,
			Cache:   c,
		})
		s.Scan(context.Background())
	}
}
