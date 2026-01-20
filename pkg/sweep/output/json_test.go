package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &JSONFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
			{Path: "/home/user/video.mp4", Size: 536870912, SizeHuman: "512 MiB"},
		},
		Stats: ScanStats{
			DirsScanned:  100,
			FilesScanned: 5000,
			LargeFiles:   2,
			Duration:     2 * time.Second,
		},
		Source:      "/home/user",
		IndexAge:    5 * time.Minute,
		DaemonUp:    true,
		WatchActive: true,
		TotalFiles:  2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should be valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	// Should have files, stats, and meta sections
	assert.Contains(t, parsed, "files")
	assert.Contains(t, parsed, "stats")
	assert.Contains(t, parsed, "meta")

	// Verify files
	files := parsed["files"].([]interface{})
	assert.Len(t, files, 2)

	file1 := files[0].(map[string]interface{})
	assert.Equal(t, "/home/user/large.zip", file1["path"])
	assert.Equal(t, float64(1073741824), file1["size"])
}

func TestJSONFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &JSONFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	files := parsed["files"].([]interface{})
	assert.Len(t, files, 0)
}

func TestJSONFormatter_Format_ValidJSON(t *testing.T) {
	formatter := &JSONFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file\"with\"quotes.zip", Size: 1024, SizeHuman: "1.0 KiB"},
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

	// Should be valid JSON even with special characters
	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
}

func TestJSONFormatter_Format_Indented(t *testing.T) {
	formatter := &JSONFormatter{}
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
	// Should be indented (contains newlines after opening braces)
	assert.Contains(t, output, "{\n")
}

func TestJSONFormatter_Registration(t *testing.T) {
	formatter, err := Get("json")
	require.NoError(t, err)
	assert.IsType(t, &JSONFormatter{}, formatter)
}

func TestJSONFormatter_Format_MetaSection(t *testing.T) {
	formatter := &JSONFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:      "/home/user",
		IndexAge:    5 * time.Minute,
		DaemonUp:    true,
		WatchActive: true,
		TotalFiles:  1,
		Warnings:    []string{"warning1", "warning2"},
		Interrupted: false,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	meta := parsed["meta"].(map[string]interface{})
	assert.Equal(t, "/home/user", meta["source"])
	assert.Equal(t, true, meta["daemon_up"])
	assert.Equal(t, true, meta["watch_active"])

	warnings := meta["warnings"].([]interface{})
	assert.Len(t, warnings, 2)
}

// JSONL Formatter Tests

func TestJSONLFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &JSONLFormatter{}
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

	// Should have one JSON object per line
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 2)

	// Each line should be valid JSON
	for _, line := range lines {
		var parsed map[string]interface{}
		err := json.Unmarshal([]byte(line), &parsed)
		require.NoError(t, err, "Line should be valid JSON: %s", line)
		assert.Contains(t, parsed, "path")
		assert.Contains(t, parsed, "size")
	}
}

func TestJSONLFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &JSONLFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should be empty (no lines)
	assert.Empty(t, strings.TrimSpace(buf.String()))
}

func TestJSONLFormatter_Format_OneLinePerFile(t *testing.T) {
	formatter := &JSONLFormatter{}
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

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 3)

	// Verify each file is on its own line
	var file1, file2, file3 map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &file1))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &file2))
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &file3))

	assert.Equal(t, "/a.zip", file1["path"])
	assert.Equal(t, "/b.zip", file2["path"])
	assert.Equal(t, "/c.zip", file3["path"])
}

func TestJSONLFormatter_Format_NoIndentation(t *testing.T) {
	formatter := &JSONLFormatter{}
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

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Each line should be a single compact JSON object (no internal newlines)
	for _, line := range lines {
		assert.NotContains(t, line, "\n")
		// Should not have leading spaces (would indicate indentation)
		assert.NotRegexp(t, `^\s`, line)
	}
}

func TestJSONLFormatter_Registration(t *testing.T) {
	formatter, err := Get("jsonl")
	require.NoError(t, err)
	assert.IsType(t, &JSONLFormatter{}, formatter)
}
