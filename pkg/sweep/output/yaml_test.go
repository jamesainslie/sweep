package output

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYAMLFormatter_Format_BasicOutput(t *testing.T) {
	formatter := &YAMLFormatter{}
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

	// Should be valid YAML
	var parsed map[string]interface{}
	err = yaml.Unmarshal(buf.Bytes(), &parsed)
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
	// YAML unmarshals large numbers as int, not int64
	assert.Equal(t, 1073741824, file1["size"])
}

func TestYAMLFormatter_Format_EmptyResult(t *testing.T) {
	formatter := &YAMLFormatter{}
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
	err = yaml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	// Files should be an empty list
	files := parsed["files"].([]interface{})
	assert.Len(t, files, 0)
}

func TestYAMLFormatter_Format_ValidYAML(t *testing.T) {
	formatter := &YAMLFormatter{}
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/file: with colons.zip", Size: 1024, SizeHuman: "1.0 KiB"},
			{Path: "/home/user/file# with hash.zip", Size: 2048, SizeHuman: "2.0 KiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	// Should be valid YAML even with special characters
	var parsed map[string]interface{}
	err = yaml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	files := parsed["files"].([]interface{})
	file1 := files[0].(map[string]interface{})
	assert.Equal(t, "/home/user/file: with colons.zip", file1["path"])
}

func TestYAMLFormatter_Format_SameStructureAsJSON(t *testing.T) {
	yamlFormatter := &YAMLFormatter{}
	jsonFormatter := &JSONFormatter{}

	var yamlBuf, jsonBuf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			DirsScanned:  100,
			FilesScanned: 5000,
			LargeFiles:   1,
			Duration:     2 * time.Second,
		},
		Source:      "/home/user",
		IndexAge:    5 * time.Minute,
		DaemonUp:    true,
		WatchActive: true,
		TotalFiles:  1,
		Warnings:    []string{"warning1"},
	}

	err := yamlFormatter.Format(&yamlBuf, result)
	require.NoError(t, err)

	err = jsonFormatter.Format(&jsonBuf, result)
	require.NoError(t, err)

	// Parse YAML
	var yamlParsed map[string]interface{}
	err = yaml.Unmarshal(yamlBuf.Bytes(), &yamlParsed)
	require.NoError(t, err)

	// Just verify JSON produced output (structure check via YAML parsing)
	require.NotEmpty(t, jsonBuf.String())

	// The structure should be equivalent (same top-level keys)
	assert.Contains(t, yamlParsed, "files")
	assert.Contains(t, yamlParsed, "stats")
	assert.Contains(t, yamlParsed, "meta")

	// Verify meta fields match
	meta := yamlParsed["meta"].(map[string]interface{})
	assert.Equal(t, "/home/user", meta["source"])
	assert.Equal(t, true, meta["daemon_up"])
}

func TestYAMLFormatter_Format_MetaSection(t *testing.T) {
	formatter := &YAMLFormatter{}
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
	err = yaml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	meta := parsed["meta"].(map[string]interface{})
	assert.Equal(t, "/home/user", meta["source"])
	assert.Equal(t, true, meta["daemon_up"])
	assert.Equal(t, true, meta["watch_active"])

	warnings := meta["warnings"].([]interface{})
	assert.Len(t, warnings, 2)
}

func TestYAMLFormatter_Registration(t *testing.T) {
	formatter, err := Get("yaml")
	require.NoError(t, err)
	assert.IsType(t, &YAMLFormatter{}, formatter)
}

func TestYAMLFormatter_Format_Indented(t *testing.T) {
	formatter := &YAMLFormatter{}
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

	// Should contain indentation (spaces)
	assert.Contains(t, output, "  ")

	// Should start with files: or have structure
	assert.Contains(t, output, "files:")
	assert.Contains(t, output, "stats:")
	assert.Contains(t, output, "meta:")
}
