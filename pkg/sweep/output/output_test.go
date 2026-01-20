package output

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileInfo(t *testing.T) {
	fi := FileInfo{
		Path:      "/home/user/large.zip",
		Name:      "large.zip",
		Dir:       "/home/user",
		Ext:       ".zip",
		Size:      1073741824, // 1 GiB
		SizeHuman: "1.0 GiB",
		ModTime:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Age:       30 * 24 * time.Hour,
		Perms:     "-rw-r--r--",
		Mode:      os.FileMode(0644),
		Owner:     "user",
		Depth:     2,
	}

	assert.Equal(t, "/home/user/large.zip", fi.Path)
	assert.Equal(t, "large.zip", fi.Name)
	assert.Equal(t, "/home/user", fi.Dir)
	assert.Equal(t, ".zip", fi.Ext)
	assert.Equal(t, int64(1073741824), fi.Size)
	assert.Equal(t, "1.0 GiB", fi.SizeHuman)
	assert.Equal(t, 2024, fi.ModTime.Year())
	assert.Equal(t, 30*24*time.Hour, fi.Age)
	assert.Equal(t, "-rw-r--r--", fi.Perms)
	assert.Equal(t, os.FileMode(0644), fi.Mode)
	assert.Equal(t, "user", fi.Owner)
	assert.Equal(t, 2, fi.Depth)
}

func TestScanStats(t *testing.T) {
	stats := ScanStats{
		DirsScanned:  100,
		FilesScanned: 5000,
		LargeFiles:   42,
		Duration:     2 * time.Second,
	}

	assert.Equal(t, int64(100), stats.DirsScanned)
	assert.Equal(t, int64(5000), stats.FilesScanned)
	assert.Equal(t, int64(42), stats.LargeFiles)
	assert.Equal(t, 2*time.Second, stats.Duration)
}

func TestResult(t *testing.T) {
	now := time.Now()
	result := Result{
		Files: []FileInfo{
			{Path: "/a.txt", Size: 1000},
			{Path: "/b.txt", Size: 2000},
			{Path: "/c.txt", Size: 3000},
		},
		Stats: ScanStats{
			DirsScanned:  10,
			FilesScanned: 100,
			LargeFiles:   3,
			Duration:     time.Second,
		},
		Source:      "/home/user",
		IndexAge:    5 * time.Minute,
		DaemonUp:    true,
		WatchActive: true,
		TotalFiles:  3,
		Warnings:    []string{"permission denied: /root"},
		Interrupted: false,
	}

	assert.Len(t, result.Files, 3)
	assert.Equal(t, "/home/user", result.Source)
	assert.Equal(t, 5*time.Minute, result.IndexAge)
	assert.True(t, result.DaemonUp)
	assert.True(t, result.WatchActive)
	assert.Equal(t, 3, result.TotalFiles)
	assert.Len(t, result.Warnings, 1)
	assert.False(t, result.Interrupted)

	_ = now // silence unused
}

func TestResult_TotalSize(t *testing.T) {
	tests := []struct {
		name     string
		files    []FileInfo
		expected int64
	}{
		{
			name:     "empty files",
			files:    []FileInfo{},
			expected: 0,
		},
		{
			name: "single file",
			files: []FileInfo{
				{Path: "/a.txt", Size: 1000},
			},
			expected: 1000,
		},
		{
			name: "multiple files",
			files: []FileInfo{
				{Path: "/a.txt", Size: 1000},
				{Path: "/b.txt", Size: 2000},
				{Path: "/c.txt", Size: 3000},
			},
			expected: 6000,
		},
		{
			name: "large files",
			files: []FileInfo{
				{Path: "/a.bin", Size: 1073741824},  // 1 GiB
				{Path: "/b.bin", Size: 2147483648},  // 2 GiB
				{Path: "/c.bin", Size: 10737418240}, // 10 GiB
			},
			expected: 13958643712, // 13 GiB total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Result{Files: tt.files}
			assert.Equal(t, tt.expected, result.TotalSize())
		})
	}
}

// mockFormatter is a simple formatter for testing the registry
type mockFormatter struct {
	formatCalled bool
}

func (m *mockFormatter) Format(w *bytes.Buffer, r *Result) error {
	m.formatCalled = true
	w.WriteString("mock output")
	return nil
}

func TestFormatterInterface(t *testing.T) {
	var f Formatter = &mockFormatter{}
	var buf bytes.Buffer
	result := &Result{}

	err := f.Format(&buf, result)
	require.NoError(t, err)
	assert.Equal(t, "mock output", buf.String())
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	// Create a fresh registry for testing
	reg := NewRegistry()

	// Register a formatter factory
	mockFactory := func() Formatter {
		return &mockFormatter{}
	}
	reg.Register("mock", mockFactory)

	// Get the formatter
	formatter, err := reg.Get("mock")
	require.NoError(t, err)
	assert.NotNil(t, formatter)

	// Verify it works
	var buf bytes.Buffer
	err = formatter.Format(&buf, &Result{})
	require.NoError(t, err)
	assert.Equal(t, "mock output", buf.String())
}

func TestRegistry_GetUnknown(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Get("unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestRegistry_Available(t *testing.T) {
	reg := NewRegistry()

	mockFactory := func() Formatter {
		return &mockFormatter{}
	}

	reg.Register("alpha", mockFactory)
	reg.Register("beta", mockFactory)
	reg.Register("gamma", mockFactory)

	available := reg.Available()
	assert.Contains(t, available, "alpha")
	assert.Contains(t, available, "beta")
	assert.Contains(t, available, "gamma")
	assert.Len(t, available, 3)
}

func TestRegistry_Available_Sorted(t *testing.T) {
	reg := NewRegistry()

	mockFactory := func() Formatter {
		return &mockFormatter{}
	}

	// Register in non-alphabetical order
	reg.Register("zeta", mockFactory)
	reg.Register("alpha", mockFactory)
	reg.Register("beta", mockFactory)

	available := reg.Available()
	// Should be sorted alphabetically
	assert.Equal(t, []string{"alpha", "beta", "zeta"}, available)
}

func TestGlobalRegistry(t *testing.T) {
	// Test that the global registry functions work
	available := Available()
	assert.NotNil(t, available)
	// At minimum, it should return a slice (may be empty if no formatters registered yet)
}
