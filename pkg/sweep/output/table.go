package output

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// TSVFormatter formats output as tab-separated values.
// It produces a simple table with a header row followed by data rows.
type TSVFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *TSVFormatter) Format(w *bytes.Buffer, r *Result) error {
	// Write header
	w.WriteString("SIZE\tPATH\n")

	// Write data rows
	for _, file := range r.Files {
		fmt.Fprintf(w, "%s\t%s\n", file.SizeHuman, file.Path)
	}

	return nil
}

func init() {
	Register("tsv", func() Formatter {
		return &TSVFormatter{}
	})
}

// Ensure TSVFormatter implements Formatter.
var _ Formatter = (*TSVFormatter)(nil)

// CSVFormatter formats output as comma-separated values with proper quoting.
// It uses encoding/csv for RFC 4180 compliant output.
type CSVFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *CSVFormatter) Format(w *bytes.Buffer, r *Result) error {
	writer := csv.NewWriter(w)

	// Write header
	if err := writer.Write([]string{"SIZE", "PATH"}); err != nil {
		return err
	}

	// Write data rows
	for _, file := range r.Files {
		if err := writer.Write([]string{file.SizeHuman, file.Path}); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

func init() {
	Register("csv", func() Formatter {
		return &CSVFormatter{}
	})
}

// Ensure CSVFormatter implements Formatter.
var _ Formatter = (*CSVFormatter)(nil)

// MarkdownFormatter formats output as a GitHub-flavored Markdown table.
// It produces a table with header, separator, and data rows using | delimiters.
type MarkdownFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *MarkdownFormatter) Format(w *bytes.Buffer, r *Result) error {
	// Write header
	w.WriteString("| SIZE | PATH |\n")

	// Write separator
	w.WriteString("|------|------|\n")

	// Write data rows
	for _, file := range r.Files {
		// Escape pipes in the path
		escapedPath := escapeMarkdownPipe(file.Path)
		escapedSize := escapeMarkdownPipe(file.SizeHuman)
		fmt.Fprintf(w, "| %s | %s |\n", escapedSize, escapedPath)
	}

	return nil
}

// escapeMarkdownPipe escapes pipe characters in a string for Markdown tables.
func escapeMarkdownPipe(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

func init() {
	Register("markdown", func() Formatter {
		return &MarkdownFormatter{}
	})
}

// Ensure MarkdownFormatter implements Formatter.
var _ Formatter = (*MarkdownFormatter)(nil)
