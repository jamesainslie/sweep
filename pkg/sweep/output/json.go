package output

import (
	"bytes"
	"encoding/json"
	"time"
)

// jsonOutput represents the full JSON output structure.
type jsonOutput struct {
	Files []jsonFile `json:"files"`
	Stats jsonStats  `json:"stats"`
	Meta  jsonMeta   `json:"meta"`
}

// jsonFile represents a file in JSON output.
type jsonFile struct {
	Path      string    `json:"path"`
	Name      string    `json:"name,omitempty"`
	Dir       string    `json:"dir,omitempty"`
	Ext       string    `json:"ext,omitempty"`
	Size      int64     `json:"size"`
	SizeHuman string    `json:"size_human"`
	ModTime   time.Time `json:"mod_time,omitempty"`
	Age       string    `json:"age,omitempty"`
	Perms     string    `json:"perms,omitempty"`
	Owner     string    `json:"owner,omitempty"`
	Depth     int       `json:"depth,omitempty"`
}

// jsonStats represents scan statistics in JSON output.
type jsonStats struct {
	DirsScanned  int64  `json:"dirs_scanned"`
	FilesScanned int64  `json:"files_scanned"`
	LargeFiles   int64  `json:"large_files"`
	Duration     string `json:"duration"`
}

// jsonMeta represents metadata in JSON output.
type jsonMeta struct {
	Source      string   `json:"source"`
	IndexAge    string   `json:"index_age,omitempty"`
	DaemonUp    bool     `json:"daemon_up"`
	WatchActive bool     `json:"watch_active"`
	TotalFiles  int      `json:"total_files"`
	TotalSize   int64    `json:"total_size"`
	Warnings    []string `json:"warnings,omitempty"`
	Interrupted bool     `json:"interrupted"`
}

// JSONFormatter formats output as a single indented JSON object.
// It produces a complete JSON document with files, stats, and meta sections.
type JSONFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *JSONFormatter) Format(w *bytes.Buffer, r *Result) error {
	output := f.buildOutput(r)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// buildOutput converts Result to the JSON output structure.
func (f *JSONFormatter) buildOutput(r *Result) jsonOutput {
	files := make([]jsonFile, len(r.Files))
	for i, file := range r.Files {
		files[i] = jsonFile{
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

	stats := jsonStats{
		DirsScanned:  r.Stats.DirsScanned,
		FilesScanned: r.Stats.FilesScanned,
		LargeFiles:   r.Stats.LargeFiles,
		Duration:     formatDurationString(r.Stats.Duration),
	}

	meta := jsonMeta{
		Source:      r.Source,
		IndexAge:    formatDurationString(r.IndexAge),
		DaemonUp:    r.DaemonUp,
		WatchActive: r.WatchActive,
		TotalFiles:  r.TotalFiles,
		TotalSize:   r.TotalSize(),
		Warnings:    r.Warnings,
		Interrupted: r.Interrupted,
	}

	return jsonOutput{
		Files: files,
		Stats: stats,
		Meta:  meta,
	}
}

// formatDurationString formats a duration as a string for JSON output.
func formatDurationString(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

func init() {
	Register("json", func() Formatter {
		return &JSONFormatter{}
	})
}

// Ensure JSONFormatter implements Formatter.
var _ Formatter = (*JSONFormatter)(nil)

// JSONLFormatter formats output as newline-delimited JSON (one object per line).
// Each file is written as a compact JSON object on its own line.
// This format is suitable for streaming processing with tools like jq.
type JSONLFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *JSONLFormatter) Format(w *bytes.Buffer, r *Result) error {
	for _, file := range r.Files {
		jf := jsonFile{
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

		data, err := json.Marshal(jf)
		if err != nil {
			return err
		}
		w.Write(data)
		w.WriteByte('\n')
	}
	return nil
}

func init() {
	Register("jsonl", func() Formatter {
		return &JSONLFormatter{}
	})
}

// Ensure JSONLFormatter implements Formatter.
var _ Formatter = (*JSONLFormatter)(nil)
