package output

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TSV Formatter Tests

func TestTSVFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &TSVFormatter{}
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

	// Header should be tab-separated
	assert.Contains(t, lines[0], "SIZE\t")
	assert.Contains(t, lines[0], "\tPATH")

	// Data rows should be tab-separated
	assert.Contains(t, lines[1], "1.0 GiB\t")
	assert.Contains(t, lines[1], "/home/user/large.zip")
}

func TestTSVFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &TSVFormatter{}
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
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should only have header
	assert.Len(t, lines, 1)
	assert.Contains(t, lines[0], "SIZE")
}

func TestTSVFormatter_Format_TabsInFields(t *testing.T) {
	formatter := &TSVFormatter{}
	var buf bytes.Buffer

	// Path contains a tab character - this is unusual but should be handled
	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file\twith\ttab.zip", Size: 1024, SizeHuman: "1.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 1,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should still produce output (tabs in path get escaped or included as-is)
	assert.NotEmpty(t, buf.String())
}

func TestTSVFormatter_Registration(t *testing.T) {
	formatter, err := Get("tsv")
	require.NoError(t, err)
	assert.IsType(t, &TSVFormatter{}, formatter)
}

// CSV Formatter Tests

func TestCSVFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &CSVFormatter{}
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

	// Should be valid CSV
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// Header + 2 data rows
	require.Len(t, records, 3)

	// Verify header
	assert.Equal(t, "SIZE", records[0][0])
	assert.Equal(t, "PATH", records[0][1])

	// Verify data
	assert.Equal(t, "1.0 GiB", records[1][0])
	assert.Equal(t, "/home/user/large.zip", records[1][1])
}

func TestCSVFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &CSVFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// Should only have header
	assert.Len(t, records, 1)
}

func TestCSVFormatter_Format_QuotedFields(t *testing.T) {
	formatter := &CSVFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file,with,commas.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/home/user/file\"with\"quotes.zip", Size: 2048, SizeHuman: "2.0 KiB"},
			{Path: "/home/user/file\nwith\nnewlines.zip", Size: 3072, SizeHuman: "3.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 3,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should be valid CSV with proper quoting
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	require.Len(t, records, 4) // header + 3 data

	// Verify special characters are preserved
	assert.Equal(t, "/home/user/file,with,commas.zip", records[1][1])
	assert.Equal(t, "/home/user/file\"with\"quotes.zip", records[2][1])
	assert.Equal(t, "/home/user/file\nwith\nnewlines.zip", records[3][1])
}

func TestCSVFormatter_Registration(t *testing.T) {
	formatter, err := Get("csv")
	require.NoError(t, err)
	assert.IsType(t, &CSVFormatter{}, formatter)
}

// Markdown Formatter Tests

func TestMarkdownFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &MarkdownFormatter{}
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

	// Should have header, separator, and 2 data rows
	require.Len(t, lines, 4)

	// Header with pipes
	assert.Contains(t, lines[0], "|")
	assert.Contains(t, lines[0], "SIZE")
	assert.Contains(t, lines[0], "PATH")

	// Separator row with dashes
	assert.Contains(t, lines[1], "---")
	assert.Contains(t, lines[1], "|")

	// Data rows with pipes
	assert.Contains(t, lines[2], "|")
	assert.Contains(t, lines[2], "1.0 GiB")
	assert.Contains(t, lines[2], "/home/user/large.zip")
}

func TestMarkdownFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &MarkdownFormatter{}
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
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have header and separator, but no data rows
	assert.Len(t, lines, 2)
}

func TestMarkdownFormatter_Format_PipeEscaping(t *testing.T) {
	formatter := &MarkdownFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file|with|pipes.zip", Size: 1024, SizeHuman: "1.0 KiB"},
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
	// Pipes in content should be escaped
	assert.Contains(t, output, `\|`)
}

func TestMarkdownFormatter_Format_GFMStyle(t *testing.T) {
	formatter := &MarkdownFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/a.zip", Size: 1024, SizeHuman: "1.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/",
		TotalFiles: 1,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// First line should start with |
	assert.True(t, strings.HasPrefix(lines[0], "|"))

	// Second line should be separator with --- patterns
	assert.Regexp(t, `\|[\s-]+\|`, lines[1])
}

func TestMarkdownFormatter_Registration(t *testing.T) {
	formatter, err := Get("markdown")
	require.NoError(t, err)
	assert.IsType(t, &MarkdownFormatter{}, formatter)
}
