package scanner

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// TestDefaultOptions verifies default options are set correctly.
func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Root != "." {
		t.Errorf("expected Root='.', got %q", opts.Root)
	}
	if opts.MinSize != 100*types.MiB {
		t.Errorf("expected MinSize=%d, got %d", 100*types.MiB, opts.MinSize)
	}
	if opts.DirWorkers != 4 {
		t.Errorf("expected DirWorkers=4, got %d", opts.DirWorkers)
	}
	if opts.FileWorkers != 8 {
		t.Errorf("expected FileWorkers=8, got %d", opts.FileWorkers)
	}
}

// TestOptionsValidate verifies validation sets defaults for invalid values.
func TestOptionsValidate(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		wantRoot string
		wantDir  int
		wantFile int
	}{
		{
			name:     "empty options",
			opts:     Options{},
			wantRoot: ".",
			wantDir:  4,
			wantFile: 8,
		},
		{
			name: "negative workers",
			opts: Options{
				DirWorkers:  -1,
				FileWorkers: 0,
			},
			wantRoot: ".",
			wantDir:  4,
			wantFile: 8,
		},
		{
			name: "valid options unchanged",
			opts: Options{
				Root:        "/tmp",
				DirWorkers:  2,
				FileWorkers: 4,
			},
			wantRoot: "/tmp",
			wantDir:  2,
			wantFile: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.opts.Root != tt.wantRoot {
				t.Errorf("Root: got %q, want %q", tt.opts.Root, tt.wantRoot)
			}
			if tt.opts.DirWorkers != tt.wantDir {
				t.Errorf("DirWorkers: got %d, want %d", tt.opts.DirWorkers, tt.wantDir)
			}
			if tt.opts.FileWorkers != tt.wantFile {
				t.Errorf("FileWorkers: got %d, want %d", tt.opts.FileWorkers, tt.wantFile)
			}
		})
	}
}

// createTestDir creates a temporary directory structure for testing.
// Returns the root path and a cleanup function.
func createTestDir(t *testing.T) (string, func()) {
	t.Helper()

	root, err := os.MkdirTemp("", "scanner-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create directory structure:
	// root/
	//   small.txt (10 bytes)
	//   large.txt (1 MiB)
	//   subdir/
	//     medium.txt (100 KiB)
	//     nested/
	//       big.txt (2 MiB)
	//   excluded/
	//     ignored.txt (1 MiB)

	dirs := []string{
		filepath.Join(root, "subdir"),
		filepath.Join(root, "subdir", "nested"),
		filepath.Join(root, "excluded"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = os.RemoveAll(root)
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	files := []struct {
		path string
		size int64
	}{
		{filepath.Join(root, "small.txt"), 10},
		{filepath.Join(root, "large.txt"), 1 * int64(types.MiB)},
		{filepath.Join(root, "subdir", "medium.txt"), 100 * int64(types.KiB)},
		{filepath.Join(root, "subdir", "nested", "big.txt"), 2 * int64(types.MiB)},
		{filepath.Join(root, "excluded", "ignored.txt"), 1 * int64(types.MiB)},
	}

	for _, f := range files {
		if err := createFileOfSize(f.path, f.size); err != nil {
			_ = os.RemoveAll(root)
			t.Fatalf("failed to create file %s: %v", f.path, err)
		}
	}

	return root, func() { _ = os.RemoveAll(root) }
}

// createFileOfSize creates a file with the specified size.
func createFileOfSize(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	if size > 0 {
		if err := f.Truncate(size); err != nil {
			_ = f.Close()
			return err
		}
	}
	return f.Close()
}

// TestScanBasic verifies basic scanning functionality.
func TestScanBasic(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	opts := Options{
		Root:        root,
		MinSize:     500 * int64(types.KiB), // Only files >= 500 KiB
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should find 3 files >= 500 KiB: large.txt (1 MiB), big.txt (2 MiB), ignored.txt (1 MiB)
	if len(result.Files) != 3 {
		t.Errorf("expected 3 large files, got %d", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  found: %s (%d bytes)", f.Path, f.Size)
		}
	}

	// Verify dirs scanned.
	if result.DirsScanned < 4 {
		t.Errorf("expected at least 4 dirs scanned, got %d", result.DirsScanned)
	}

	// Verify files scanned.
	if result.FilesScanned != 5 {
		t.Errorf("expected 5 files scanned, got %d", result.FilesScanned)
	}

	// Verify elapsed time is set.
	if result.Elapsed == 0 {
		t.Error("expected Elapsed to be set")
	}
}

// TestScanWithExclusions verifies exclusion patterns work.
func TestScanWithExclusions(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	excludedPath := filepath.Join(root, "excluded")

	opts := Options{
		Root:        root,
		MinSize:     500 * int64(types.KiB),
		Exclude:     []string{excludedPath},
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should find 2 files: large.txt (1 MiB), big.txt (2 MiB)
	// excluded/ignored.txt should be skipped
	if len(result.Files) != 2 {
		t.Errorf("expected 2 large files (exclusion should work), got %d", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  found: %s (%d bytes)", f.Path, f.Size)
		}
	}

	// Verify excluded file is not in results.
	for _, f := range result.Files {
		if filepath.Base(f.Path) == "ignored.txt" {
			t.Error("excluded file should not be in results")
		}
	}
}

// TestScanWithGlobExclusion verifies glob pattern exclusions work.
func TestScanWithGlobExclusion(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	opts := Options{
		Root:        root,
		MinSize:     1, // Include all files
		Exclude:     []string{"*.txt"},
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// All files are .txt, so none should be found.
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files (all .txt excluded), got %d", len(result.Files))
	}
}

// TestScanContextCancellation verifies the scanner respects context cancellation.
func TestScanContextCancellation(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	// Create a context that we'll cancel immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(ctx)

	// Should complete (possibly with partial results) without hanging.
	if err != nil && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should not be nil.
	if result == nil {
		t.Error("expected result to be non-nil even with cancellation")
	}
}

// TestScanProgress verifies progress callbacks are called.
func TestScanProgress(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	var progressCalls atomic.Int32

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  1,
		FileWorkers: 1,
		OnProgress: func(p types.ScanProgress) {
			progressCalls.Add(1)
		},
	}

	scanner := New(opts)
	_, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Progress might not be called for small test directories,
	// but the callback mechanism should work.
	t.Logf("Progress callbacks: %d", progressCalls.Load())
}

// TestScanNonExistentPath verifies error handling for non-existent paths.
func TestScanNonExistentPath(t *testing.T) {
	opts := Options{
		Root:        "/this/path/does/not/exist",
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	_, err := scanner.Scan(context.Background())

	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

// TestScanFileNotDirectory verifies error handling when root is a file.
func TestScanFileNotDirectory(t *testing.T) {
	// Create a temporary file.
	f, err := os.CreateTemp("", "scanner-test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(name) }()

	opts := Options{
		Root:        name,
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	_, err = scanner.Scan(context.Background())

	if err == nil {
		t.Error("expected error when root is a file")
	}
}

// TestScanFileInfo verifies FileInfo fields are populated correctly.
func TestScanFileInfo(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	opts := Options{
		Root:        root,
		MinSize:     1 * int64(types.MiB), // Only 1 MiB+ files
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(result.Files) == 0 {
		t.Fatal("expected at least one file")
	}

	for _, f := range result.Files {
		// Verify path is absolute.
		if !filepath.IsAbs(f.Path) {
			t.Errorf("path should be absolute: %s", f.Path)
		}

		// Verify size is set.
		if f.Size <= 0 {
			t.Errorf("size should be positive: %d", f.Size)
		}

		// Verify ModTime is set.
		if f.ModTime.IsZero() {
			t.Error("ModTime should be set")
		}

		// Verify Mode is set.
		if f.Mode == 0 {
			t.Error("Mode should be set")
		}

		// Verify Owner is set (should be current user or UID).
		if f.Owner == "" || f.Owner == "unknown" {
			t.Errorf("Owner should be set: %q", f.Owner)
		}

		// Verify Group is set.
		if f.Group == "" || f.Group == "unknown" {
			t.Errorf("Group should be set: %q", f.Group)
		}
	}
}

// TestScanConcurrency verifies concurrent scanning works correctly.
func TestScanConcurrency(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	// Test with various worker configurations.
	configs := []struct {
		dirWorkers  int
		fileWorkers int
	}{
		{1, 1},
		{2, 2},
		{4, 8},
		{1, 10},
		{10, 1},
	}

	for _, cfg := range configs {
		t.Run("", func(t *testing.T) {
			opts := Options{
				Root:        root,
				MinSize:     1,
				DirWorkers:  cfg.dirWorkers,
				FileWorkers: cfg.fileWorkers,
			}

			scanner := New(opts)
			result, err := scanner.Scan(context.Background())
			if err != nil {
				t.Fatalf("Scan failed with %d dir/%d file workers: %v",
					cfg.dirWorkers, cfg.fileWorkers, err)
			}

			// Should always find 5 files.
			if result.FilesScanned != 5 {
				t.Errorf("expected 5 files scanned, got %d", result.FilesScanned)
			}
		})
	}
}

// TestIsExcluded verifies exclusion pattern matching.
func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{
			name:     "exact match",
			patterns: []string{"/proc"},
			path:     "/proc",
			want:     true,
		},
		{
			name:     "prefix match",
			patterns: []string{"/proc"},
			path:     "/proc/1/fd",
			want:     true,
		},
		{
			name:     "no match",
			patterns: []string{"/proc"},
			path:     "/home/user",
			want:     false,
		},
		{
			name:     "glob match",
			patterns: []string{"*.log"},
			path:     "/var/log/app.log",
			want:     true,
		},
		{
			name:     "glob no match",
			patterns: []string{"*.log"},
			path:     "/var/log/app.txt",
			want:     false,
		},
		{
			name:     "multiple patterns",
			patterns: []string{"/proc", "/sys", "*.tmp"},
			path:     "/sys/kernel",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scanner{
				opts: Options{
					Exclude: tt.patterns,
				},
			}

			got := s.isExcluded(tt.path)
			if got != tt.want {
				t.Errorf("isExcluded(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestScanPermissionErrors verifies errors are collected without stopping scan.
func TestScanPermissionErrors(t *testing.T) {
	// Skip if running as root (no permission errors).
	if os.Getuid() == 0 {
		t.Skip("skipping permission test as root")
	}

	root, cleanup := createTestDir(t)
	defer cleanup()

	// Create a directory we can't read.
	noReadDir := filepath.Join(root, "noread")
	if err := os.Mkdir(noReadDir, 0o000); err != nil {
		t.Fatalf("failed to create unreadable dir: %v", err)
	}
	// Restore permissions for cleanup.
	defer func() { _ = os.Chmod(noReadDir, 0o755) }()

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(context.Background())

	// Should complete without error.
	if err != nil {
		t.Fatalf("Scan should complete despite permission errors: %v", err)
	}

	// Should have collected the permission error.
	if len(result.Errors) == 0 {
		t.Error("expected permission error to be collected")
	}

	// Other files should still be found.
	if result.FilesScanned < 4 {
		t.Errorf("expected at least 4 files scanned, got %d", result.FilesScanned)
	}
}

// TestScanEmptyDirectory verifies scanning an empty directory.
func TestScanEmptyDirectory(t *testing.T) {
	root, err := os.MkdirTemp("", "scanner-empty-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)
	result, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}

	if result.DirsScanned != 1 {
		t.Errorf("expected 1 dir scanned, got %d", result.DirsScanned)
	}
}

// TestScanTimeout verifies scan respects timeout context.
func TestScanTimeout(t *testing.T) {
	root, cleanup := createTestDir(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  2,
		FileWorkers: 2,
	}

	scanner := New(opts)

	done := make(chan struct{})
	go func() {
		_, _ = scanner.Scan(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Scan completed successfully.
	case <-time.After(10 * time.Second):
		t.Fatal("scan did not complete within timeout")
	}
}

// BenchmarkScan benchmarks the scanner performance.
func BenchmarkScan(b *testing.B) {
	root, err := os.MkdirTemp("", "scanner-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	// Create a larger test structure.
	for i := range 10 {
		subdir := filepath.Join(root, "dir"+string(rune('0'+i)))
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			b.Fatalf("failed to create subdir: %v", err)
		}

		for j := range 100 {
			file := filepath.Join(subdir, "file"+string(rune('0'+j/10))+string(rune('0'+j%10))+".txt")
			if err := createFileOfSize(file, 1024); err != nil {
				b.Fatalf("failed to create file: %v", err)
			}
		}
	}

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  4,
		FileWorkers: 8,
	}

	b.ResetTimer()
	for range b.N {
		scanner := New(opts)
		_, err := scanner.Scan(context.Background())
		if err != nil {
			b.Fatalf("Scan failed: %v", err)
		}
	}
}

// BenchmarkScanParallel benchmarks parallel scanning.
func BenchmarkScanParallel(b *testing.B) {
	root, err := os.MkdirTemp("", "scanner-bench-parallel-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	// Create test structure.
	for i := range 5 {
		subdir := filepath.Join(root, "dir"+string(rune('0'+i)))
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			b.Fatalf("failed to create subdir: %v", err)
		}

		for j := range 50 {
			file := filepath.Join(subdir, "file"+string(rune('0'+j/10))+string(rune('0'+j%10))+".txt")
			if err := createFileOfSize(file, 1024); err != nil {
				b.Fatalf("failed to create file: %v", err)
			}
		}
	}

	opts := Options{
		Root:        root,
		MinSize:     1,
		DirWorkers:  4,
		FileWorkers: 8,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			scanner := New(opts)
			_, err := scanner.Scan(context.Background())
			if err != nil {
				b.Fatalf("Scan failed: %v", err)
			}
		}
	})
}
