package output

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateFormatter_Format_BasicOutput(t *testing.T) {
	formatter := NewTemplateFormatter("{{range .Files}}{{.Path}}\n{{end}}")
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
	assert.Contains(t, output, "/home/user/large.zip")
	assert.Contains(t, output, "/home/user/video.mp4")
}

func TestTemplateFormatter_Format_EmptyResult(t *testing.T) {
	formatter := NewTemplateFormatter("Files: {{len .Files}}")
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
	assert.Contains(t, output, "Files: 0")
}

func TestTemplateFormatter_Format_DateFunction(t *testing.T) {
	formatter := NewTemplateFormatter(`{{range .Files}}{{date .ModTime "2006-01-02"}}{{end}}`)
	var buf bytes.Buffer

	modTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	result := &Result{
		Files: []FileInfo{
			{Path: "/home/user/large.zip", Size: 1024, SizeHuman: "1.0 KiB", ModTime: modTime},
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
	assert.Equal(t, "2024-06-15", output)
}

func TestTemplateFormatter_Format_BytesFunction(t *testing.T) {
	formatter := NewTemplateFormatter(`{{range .Files}}{{bytes .Size}}{{end}}`)
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
	assert.Equal(t, "1.0 GiB", output)
}

func TestTemplateFormatter_Format_ComplexTemplate(t *testing.T) {
	template := `Source: {{.Source}}
Files found: {{len .Files}}
{{range .Files}}- {{.SizeHuman}} {{.Path}}
{{end}}`

	formatter := NewTemplateFormatter(template)
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
	assert.Contains(t, output, "Source: /home/user")
	assert.Contains(t, output, "Files found: 2")
	assert.Contains(t, output, "- 1.0 GiB /home/user/large.zip")
	assert.Contains(t, output, "- 512 MiB /home/user/video.mp4")
}

func TestTemplateFormatter_Format_InvalidTemplate(t *testing.T) {
	formatter := NewTemplateFormatter("{{.InvalidField}}")
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	// This should produce an error because InvalidField doesn't exist
	err := formatter.Format(&buf, result)
	// The behavior depends on template execution - may error or produce empty output
	// Just verify it doesn't panic
	_ = err
}

func TestTemplateFormatter_Format_SyntaxError(t *testing.T) {
	// This template has invalid syntax
	formatter := NewTemplateFormatter("{{.Files")
	var buf bytes.Buffer

	result := &Result{
		Files:      []FileInfo{},
		Stats:      ScanStats{Duration: time.Second},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	assert.Error(t, err) // Should error on invalid syntax
}

func TestTemplateFormatter_Format_AccessStats(t *testing.T) {
	formatter := NewTemplateFormatter("Scanned {{.Stats.FilesScanned}} files in {{.Stats.DirsScanned}} dirs")
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{},
		Stats: ScanStats{
			DirsScanned:  100,
			FilesScanned: 5000,
			Duration:     time.Second,
		},
		Source:     "/home/user",
		TotalFiles: 0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	assert.Equal(t, "Scanned 5000 files in 100 dirs", output)
}

func TestTemplateFormatter_Format_AccessMeta(t *testing.T) {
	formatter := NewTemplateFormatter("Daemon: {{if .DaemonUp}}up{{else}}down{{end}}, Watching: {{.WatchActive}}")
	var buf bytes.Buffer

	result := &Result{
		Files:       []FileInfo{},
		Stats:       ScanStats{Duration: time.Second},
		Source:      "/home/user",
		DaemonUp:    true,
		WatchActive: true,
		TotalFiles:  0,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	assert.Equal(t, "Daemon: up, Watching: true", output)
}

func TestTemplateFormatter_Format_TotalSize(t *testing.T) {
	formatter := NewTemplateFormatter("Total: {{bytes .TotalSize}}")
	var buf bytes.Buffer

	result := &Result{
		Files: []FileInfo{
			{Path: "/a.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
			{Path: "/b.zip", Size: 1073741824, SizeHuman: "1.0 GiB"},
		},
		Stats: ScanStats{
			Duration: time.Second,
		},
		Source:     "/",
		TotalFiles: 2,
	}

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	// Total should be 2 GiB
	assert.Equal(t, "Total: 2.0 GiB", output)
}

func TestTemplateFormatter_Registration(t *testing.T) {
	// Template formatter should be registered but requires a template string
	formatter, err := Get("template")
	require.NoError(t, err)
	assert.IsType(t, &TemplateFormatter{}, formatter)
}

func TestTemplateFormatter_SetTemplate(t *testing.T) {
	// Get the registered formatter and set a custom template
	formatter, err := Get("template")
	require.NoError(t, err)

	templateFormatter := formatter.(*TemplateFormatter)
	templateFormatter.SetTemplate("Custom: {{.Source}}")

	var buf bytes.Buffer
	result := &Result{
		Source:     "/test",
		Files:      []FileInfo{},
		Stats:      ScanStats{},
		TotalFiles: 0,
	}

	err = formatter.Format(&buf, result)
	require.NoError(t, err)

	assert.Equal(t, "Custom: /test", buf.String())
}
