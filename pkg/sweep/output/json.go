package output

import (
	"bytes"
	"encoding/json"
)

// JSONFormatter formats output as a single indented JSON object.
// It produces a complete JSON document with files, stats, and meta sections.
type JSONFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *JSONFormatter) Format(w *bytes.Buffer, r *Result) error {
	output := BuildStructuredOutput(r)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
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
		sf := StructuredFile{
			Path:      file.Path,
			Name:      file.Name,
			Dir:       file.Dir,
			Ext:       file.Ext,
			Size:      file.Size,
			SizeHuman: file.SizeHuman,
			ModTime:   file.ModTime,
			Age:       FormatDurationString(file.Age),
			Perms:     file.Perms,
			Owner:     file.Owner,
			Depth:     file.Depth,
		}

		data, err := json.Marshal(sf)
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
