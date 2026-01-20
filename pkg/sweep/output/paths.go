package output

import (
	"bytes"
)

// PathsFormatter formats output as one file path per line.
// It produces a simple list of paths suitable for piping to other tools.
// Only the paths are output, without size or other metadata.
type PathsFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *PathsFormatter) Format(w *bytes.Buffer, r *Result) error {
	for _, file := range r.Files {
		w.WriteString(file.Path)
		w.WriteByte('\n')
	}
	return nil
}

func init() {
	Register("paths", func() Formatter {
		return &PathsFormatter{}
	})
}

// Ensure PathsFormatter implements Formatter.
var _ Formatter = (*PathsFormatter)(nil)

// NullFormatter formats output as null-delimited paths.
// It produces paths separated by null bytes (0x00), suitable for use with
// xargs -0 or other tools that support null-delimited input.
// This format safely handles paths containing spaces, newlines, or other
// special characters.
type NullFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *NullFormatter) Format(w *bytes.Buffer, r *Result) error {
	for _, file := range r.Files {
		w.WriteString(file.Path)
		w.WriteByte(0) // Null byte delimiter
	}
	return nil
}

func init() {
	Register("null", func() Formatter {
		return &NullFormatter{}
	})
}

// Ensure NullFormatter implements Formatter.
var _ Formatter = (*NullFormatter)(nil)
