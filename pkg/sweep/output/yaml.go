package output

import (
	"bytes"
	"time"

	"gopkg.in/yaml.v3"
)

// yamlOutput represents the full YAML output structure.
type yamlOutput struct {
	Files []yamlFile `yaml:"files"`
	Stats yamlStats  `yaml:"stats"`
	Meta  yamlMeta   `yaml:"meta"`
}

// yamlFile represents a file in YAML output.
type yamlFile struct {
	Path      string    `yaml:"path"`
	Name      string    `yaml:"name,omitempty"`
	Dir       string    `yaml:"dir,omitempty"`
	Ext       string    `yaml:"ext,omitempty"`
	Size      int64     `yaml:"size"`
	SizeHuman string    `yaml:"size_human"`
	ModTime   time.Time `yaml:"mod_time,omitempty"`
	Age       string    `yaml:"age,omitempty"`
	Perms     string    `yaml:"perms,omitempty"`
	Owner     string    `yaml:"owner,omitempty"`
	Depth     int       `yaml:"depth,omitempty"`
}

// yamlStats represents scan statistics in YAML output.
type yamlStats struct {
	DirsScanned  int64  `yaml:"dirs_scanned"`
	FilesScanned int64  `yaml:"files_scanned"`
	LargeFiles   int64  `yaml:"large_files"`
	Duration     string `yaml:"duration"`
}

// yamlMeta represents metadata in YAML output.
type yamlMeta struct {
	Source      string   `yaml:"source"`
	IndexAge    string   `yaml:"index_age,omitempty"`
	DaemonUp    bool     `yaml:"daemon_up"`
	WatchActive bool     `yaml:"watch_active"`
	TotalFiles  int      `yaml:"total_files"`
	TotalSize   int64    `yaml:"total_size"`
	Warnings    []string `yaml:"warnings,omitempty"`
	Interrupted bool     `yaml:"interrupted"`
}

// YAMLFormatter formats output as YAML.
// It produces the same structure as JSONFormatter but in YAML format.
type YAMLFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *YAMLFormatter) Format(w *bytes.Buffer, r *Result) error {
	output := f.buildOutput(r)

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(output); err != nil {
		return err
	}
	return encoder.Close()
}

// buildOutput converts Result to the YAML output structure.
func (f *YAMLFormatter) buildOutput(r *Result) yamlOutput {
	files := make([]yamlFile, len(r.Files))
	for i, file := range r.Files {
		files[i] = yamlFile{
			Path:      file.Path,
			Name:      file.Name,
			Dir:       file.Dir,
			Ext:       file.Ext,
			Size:      file.Size,
			SizeHuman: file.SizeHuman,
			ModTime:   file.ModTime,
			Age:       formatDurationString(file.Age),
			Perms:     file.Perms,
			Owner:     file.Owner,
			Depth:     file.Depth,
		}
	}

	stats := yamlStats{
		DirsScanned:  r.Stats.DirsScanned,
		FilesScanned: r.Stats.FilesScanned,
		LargeFiles:   r.Stats.LargeFiles,
		Duration:     formatDurationString(r.Stats.Duration),
	}

	meta := yamlMeta{
		Source:      r.Source,
		IndexAge:    formatDurationString(r.IndexAge),
		DaemonUp:    r.DaemonUp,
		WatchActive: r.WatchActive,
		TotalFiles:  r.TotalFiles,
		TotalSize:   r.TotalSize(),
		Warnings:    r.Warnings,
		Interrupted: r.Interrupted,
	}

	return yamlOutput{
		Files: files,
		Stats: stats,
		Meta:  meta,
	}
}

func init() {
	Register("yaml", func() Formatter {
		return &YAMLFormatter{}
	})
}

// Ensure YAMLFormatter implements Formatter.
var _ Formatter = (*YAMLFormatter)(nil)
