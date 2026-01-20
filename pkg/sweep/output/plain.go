package output

import (
	"bytes"
	"text/tabwriter"
)

// PlainFormatter formats output as a simple tab-separated table.
// It produces plain text output suitable for scripting and piping.
// No colors or styling are applied.
type PlainFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *PlainFormatter) Format(w *bytes.Buffer, r *Result) error {
	// Use tabwriter for aligned columns
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)

	// Write header
	_, err := tw.Write([]byte("SIZE\tPATH\n"))
	if err != nil {
		return err
	}

	// Write data rows
	for _, file := range r.Files {
		_, err := tw.Write([]byte(file.SizeHuman + "\t" + file.Path + "\n"))
		if err != nil {
			return err
		}
	}

	// Flush tabwriter to buffer
	return tw.Flush()
}

func init() {
	Register("plain", func() Formatter {
		return &PlainFormatter{}
	})
}

// Ensure PlainFormatter implements Formatter.
var _ Formatter = (*PlainFormatter)(nil)
