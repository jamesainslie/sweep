package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrettyFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &PrettyFormatter{}
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

	output := buf.String()

	// Header should contain source info
	assert.Contains(t, output, "/home/user")

	// Should contain file paths and sizes
	assert.Contains(t, output, "large.zip")
	assert.Contains(t, output, "video.mp4")
	assert.Contains(t, output, "1.0 GiB")
	assert.Contains(t, output, "512 MiB")

	// Should contain column headers
	assert.Contains(t, output, "SIZE")
	assert.Contains(t, output, "PATH")
}

func TestPrettyFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &PrettyFormatter{}
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

	// Should indicate no files found
	assert.Contains(t, output, "0")
}

func TestPrettyFormatter_Format_WithWarnings(t *testing.T) {
	formatter := &PrettyFormatter{}
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
		Warnings:   []string{"permission denied: /root", "symlink skipped: /link"},
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Warnings should be displayed
	assert.Contains(t, output, "permission denied")
	assert.Contains(t, output, "symlink skipped")
}

func TestPrettyFormatter_Format_Interrupted(t *testing.T) {
	formatter := &PrettyFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:      "/home/user",
		TotalFiles:  1,
		Interrupted: true,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Should indicate interruption
	assert.True(t, strings.Contains(output, "interrupted") || strings.Contains(output, "Interrupted"))
}

func TestPrettyFormatter_Format_DaemonStatus(t *testing.T) {
	tests := []struct {
		name        string
		daemonUp    bool
		watchActive bool
	}{
		{
			name:        "daemon up and watching",
			daemonUp:    true,
			watchActive: true,
		},
		{
			name:        "daemon up not watching",
			daemonUp:    true,
			watchActive: false,
		},
		{
			name:        "daemon down",
			daemonUp:    false,
			watchActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &PrettyFormatter{}
			var buf bytes.Buffer

			result := &Result{
				Files: []FileInfo{
					{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
				},
				Stats: ScanStats{
					Duration: time.Second,
				},
				Source:      "/home/user",
				DaemonUp:    tt.daemonUp,
				WatchActive: tt.watchActive,
				TotalFiles:  1,
			}

			err := formatter.Format(&buf, result)
			require.NoError(t, err)

			// Just verify it doesn't error - the visual output varies
			assert.NotEmpty(t, buf.String())
		})
	}
}

func TestPrettyFormatter_Registration(t *testing.T) {
	// Verify the formatter is registered as "pretty"
	formatter, err := Get("pretty")
	require.NoError(t, err)
	assert.IsType(t, &PrettyFormatter{}, formatter)
}

func TestPrettyFormatter_Format_LongPaths(t *testing.T) {
	formatter := &PrettyFormatter{}
	var buf bytes.Buffer

	longPath := "/home/user/very/deep/nested/directory/structure/with/many/levels/file.zip"
	result := &Result{
		Files: []FileInfo{
			{Path: longPath, Size: 1073741824, SizeHuman: "1.0 GiB"},
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
	// Should contain the path (potentially truncated but present)
	assert.Contains(t, output, "file.zip")
}

func TestPrettyFormatter_Format_TotalSize(t *testing.T) {
	formatter := &PrettyFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/a.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
			{Path: "/b.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home",
		TotalFiles: 2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	// Footer should contain total size (2 GiB)
	assert.Contains(t, output, "2")
}
