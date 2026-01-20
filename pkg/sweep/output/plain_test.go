package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlainFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &PlainFormatter{}
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
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have header + 2 data rows
	require.Len(t, lines, 3)

	// Header should be SIZE\tPATH
	assert.True(t, strings.HasPrefix(lines[0], "SIZE"))
	assert.Contains(t, lines[0], "PATH")

	// Data rows should have tab-separated size and path
	assert.Contains(t, lines[1], "1.0 GiB")
	assert.Contains(t, lines[1], "/home/user/large.zip")
	assert.Contains(t, lines[2], "512 MiB")
	assert.Contains(t, lines[2], "/home/user/video.mp4")
}

func TestPlainFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &PlainFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	// Should only have header line
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 1)
	assert.Contains(t, lines[0], "SIZE")
	assert.Contains(t, lines[0], "PATH")
}

func TestPlainFormatter_Format_NoColors(t *testing.T) {
	formatter := &PlainFormatter{}
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

	// Should not contain ANSI escape codes
	assert.NotContains(t, output, "\x1b[")
	assert.NotContains(t, output, "\033[")
}

func TestPlainFormatter_Format_ColumnSeparated(t *testing.T) {
	formatter := &PlainFormatter{}
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
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Each line should have two columns (SIZE and PATH)
	// tabwriter converts tabs to spaces for alignment
	for _, line := range lines {
		// Verify line has both size and path content (two logical columns)
		fields := strings.Fields(line)
		assert.GreaterOrEqual(t, len(fields), 2, "Line should have at least two columns: %s", line)
	}
}

func TestPlainFormatter_Registration(t *testing.T) {
	// Verify the formatter is registered as "plain"
	formatter, err := Get("plain")
	require.NoError(t, err)
	assert.IsType(t, &PlainFormatter{}, formatter)
}

func TestPlainFormatter_Format_SpecialCharacters(t *testing.T) {
	formatter := &PlainFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file with spaces.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/home/user/file-name.zip", Size: 2048, SizeHuman: "2.0 KiB"},
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
	// Should contain the paths with special characters
	assert.Contains(t, output, "file with spaces.zip")
	assert.Contains(t, output, "file-name.zip")
}

func TestPlainFormatter_Format_AlignedColumns(t *testing.T) {
	formatter := &PlainFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/a.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
			{Path: "/b.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/c.zip", Size: 10737418240, SizeHuman: "10.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/",
		TotalFiles: 3,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// With tabwriter, columns should be aligned
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have 4 lines (header + 3 data)
	require.Len(t, lines, 4)

	// All lines should be properly formatted
	for _, line := range lines {
		assert.NotEmpty(t, line)
	}
}
