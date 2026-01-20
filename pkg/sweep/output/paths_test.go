package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Paths Formatter Tests

func TestPathsFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &PathsFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
			{Path: "/home/user/video.mp4", Size: 536870912, SizeHuman: "512 MiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Should have exactly 2 lines (one per file)
	require.Len(t, lines, 2)

	// Each line should be just the path
	assert.Equal(t, "/home/user/large.zip", lines[0])
	assert.Equal(t, "/home/user/video.mp4", lines[1])
}

func TestPathsFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &PathsFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should be empty
	assert.Empty(t, buf.String())
}

func TestPathsFormatter_Format_OnePathPerLine(t *testing.T) {
	formatter := &PathsFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/a.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/b.zip", Size: 2048, SizeHuman: "2.0 KiB"},
			{Path: "/c.zip", Size: 3072, SizeHuman: "3.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/",
		TotalFiles: 3,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Each path should be on its own line
	assert.Contains(t, output, "/a.zip\n")
	assert.Contains(t, output, "/b.zip\n")
	assert.Contains(t, output, "/c.zip\n")

	// Should not contain size information
	assert.NotContains(t, output, "KiB")
	assert.NotContains(t, output, "GiB")
}

func TestPathsFormatter_Format_NoExtraData(t *testing.T) {
	formatter := &PathsFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 1,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Should only contain path and newline
	assert.Equal(t, "/home/user/large.zip\n", output)
}

func TestPathsFormatter_Registration(t *testing.T) {
	formatter, err := Get("paths")
	require.NoError(t, err)
	assert.IsType(t, &PathsFormatter{}, formatter)
}

// Null Formatter Tests

func TestNullFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &NullFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
			{Path: "/home/user/video.mp4", Size: 536870912, SizeHuman: "512 MiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.Bytes()

	// Should have null bytes as delimiters
	assert.Contains(t, string(output), "\x00")

	// Split by null byte
	parts := bytes.Split(output, []byte{0})

	// Should have 2 paths (with trailing empty from final null)
	// Remove empty parts
	var paths []string
	for _, p := range parts {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	assert.Len(t, paths, 2)
	assert.Equal(t, "/home/user/large.zip", paths[0])
	assert.Equal(t, "/home/user/video.mp4", paths[1])
}

func TestNullFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &NullFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should be empty
	assert.Empty(t, buf.String())
}

func TestNullFormatter_Format_NullDelimited(t *testing.T) {
	formatter := &NullFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/a.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/b.zip", Size: 2048, SizeHuman: "2.0 KiB"},
			{Path: "/c.zip", Size: 3072, SizeHuman: "3.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/",
		TotalFiles: 3,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.Bytes()

	// Count null bytes - should be 3 (one after each path)
	nullCount := bytes.Count(output, []byte{0})
	assert.Equal(t, 3, nullCount)
}

func TestNullFormatter_Format_NoNewlines(t *testing.T) {
	formatter := &NullFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 1,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Should not contain newlines (uses null byte instead)
	assert.NotContains(t, output, "\n")
}

func TestNullFormatter_Format_XargsCompatible(t *testing.T) {
	formatter := &NullFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file with spaces.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/home/user/file\nwith\nnewlines.zip", Size: 2048, SizeHuman: "2.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.Bytes()

	// Split by null byte and verify paths are preserved
	parts := bytes.Split(output, []byte{0})
	var paths []string
	for _, p := range parts {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}

	assert.Len(t, paths, 2)
	assert.Equal(t, "/home/user/file with spaces.zip", paths[0])
	assert.Equal(t, "/home/user/file\nwith\nnewlines.zip", paths[1])
}

func TestNullFormatter_Registration(t *testing.T) {
	formatter, err := Get("null")
	require.NoError(t, err)
	assert.IsType(t, &NullFormatter{}, formatter)
}
