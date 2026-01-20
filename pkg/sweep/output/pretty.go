package output

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
)

// PrettyFormatter formats output with colors and styling using lipgloss.
// It produces a visually appealing output suitable for terminal display.
type PrettyFormatter struct{}

// Format writes the formatted output to the buffer.
func (f *PrettyFormatter) Format(w *bytes.Buffer, r *Result) error {
	// Build header
	header := f.formatHeader(r)
	w.WriteString(header)
	w.WriteString("\n")

	// Build file table
	table := f.formatTable(r)
	w.WriteString(table)

	// Build footer
	footer := f.formatFooter(r)
	w.WriteString(footer)

	// Add warnings if any
	if len(r.Warnings) > 0 {
		warnings := f.formatWarnings(r.Warnings)
		w.WriteString("\n")
		w.WriteString(warnings)
	}

	return nil
}

// formatHeader builds the header box with scan metadata.
func (f *PrettyFormatter) formatHeader(r *Result) string {
	var lines []string

	// Source line
	sourceLabel := LabelStyle.Render("Source:")
	sourceValue := ValueStyle.Render(r.Source)
	lines = append(lines, fmt.Sprintf("%s %s", sourceLabel, sourceValue))

	// Index age and scan info line
	var infoParts []string

	if r.IndexAge > 0 {
		indexAgeLabel := LabelStyle.Render("Index:")
		indexAgeValue := MutedStyle.Render(formatDuration(r.IndexAge) + " old")
		infoParts = append(infoParts, fmt.Sprintf("%s %s", indexAgeLabel, indexAgeValue))
	}

	scannedLabel := LabelStyle.Render("Scanned:")
	scannedValue := ValueStyle.Render(fmt.Sprintf("%d files in %s",
		r.Stats.FilesScanned, formatDuration(r.Stats.Duration)))
	infoParts = append(infoParts, fmt.Sprintf("%s %s", scannedLabel, scannedValue))

	// Daemon status
	daemonStatus := f.formatDaemonStatus(r.DaemonUp, r.WatchActive)
	infoParts = append(infoParts, daemonStatus)

	lines = append(lines, strings.Join(infoParts, "  "))

	// Interrupted notice
	if r.Interrupted {
		interruptedStyle := WarningStyle.Bold(true)
		lines = append(lines, interruptedStyle.Render("Scan interrupted by user"))
	}

	content := strings.Join(lines, "\n")
	return HeaderBox.Render(content)
}

// formatDaemonStatus returns a styled string indicating daemon status.
func (f *PrettyFormatter) formatDaemonStatus(daemonUp, watchActive bool) string {
	if !daemonUp {
		return MutedStyle.Render("daemon: off")
	}

	if watchActive {
		return SuccessStyle.Render("daemon: watching")
	}

	return LabelStyle.Render("daemon: ") + ValueStyle.Render("up")
}

// formatTable builds the file table with SIZE and PATH columns.
func (f *PrettyFormatter) formatTable(r *Result) string {
	if len(r.Files) == 0 {
		return MutedStyle.Render("  No files found matching criteria\n")
	}

	var sb strings.Builder

	// Column headers
	sizeHeader := TableHeaderStyle.Render("SIZE")
	pathHeader := TableHeaderStyle.Render("PATH")
	sb.WriteString(fmt.Sprintf("  %s  %s\n", sizeHeader, pathHeader))

	// Calculate max size width for alignment
	maxSizeWidth := 0
	for _, file := range r.Files {
		if len(file.SizeHuman) > maxSizeWidth {
			maxSizeWidth = len(file.SizeHuman)
		}
	}
	if maxSizeWidth < 8 {
		maxSizeWidth = 8 // Minimum width
	}

	// File rows
	for _, file := range r.Files {
		sizeStr := SizeStyle.Render(padLeft(file.SizeHuman, maxSizeWidth))
		pathStr := PathStyle.Render(file.Path)
		sb.WriteString(fmt.Sprintf("  %s  %s\n", sizeStr, pathStr))
	}

	return sb.String()
}

// formatFooter builds the footer box with summary information.
func (f *PrettyFormatter) formatFooter(r *Result) string {
	var parts []string

	// File count
	fileCountLabel := LabelStyle.Render("Files:")
	fileCountValue := ValueStyle.Render(fmt.Sprintf("%d", r.TotalFiles))
	parts = append(parts, fmt.Sprintf("%s %s", fileCountLabel, fileCountValue))

	// Total size
	totalSize := r.TotalSize()
	totalSizeLabel := LabelStyle.Render("Total:")
	totalSizeValue := SizeStyle.Render(humanize.IBytes(uint64(totalSize)))
	parts = append(parts, fmt.Sprintf("%s %s", totalSizeLabel, totalSizeValue))

	// Hints
	hint := MutedStyle.Render("Use -o plain for unformatted output")
	parts = append(parts, hint)

	content := strings.Join(parts, "  ")
	return FooterBox.Render(content)
}

// formatWarnings builds a warning block.
func (f *PrettyFormatter) formatWarnings(warnings []string) string {
	var sb strings.Builder

	titleStyle := WarningStyle.Bold(true)
	sb.WriteString(titleStyle.Render("Warnings:"))
	sb.WriteString("\n")

	for _, warning := range warnings {
		sb.WriteString(WarningStyle.Render("  " + warning))
		sb.WriteString("\n")
	}

	return sb.String()
}

// padLeft pads a string with spaces on the left to achieve the desired width.
func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// formatDuration formats a duration in a human-friendly way.
func formatDuration(d interface{}) string {
	switch v := d.(type) {
	case int:
		return formatDurationSeconds(v)
	case interface{ Seconds() float64 }:
		return formatDurationFromDuration(v)
	default:
		return fmt.Sprintf("%v", d)
	}
}

// formatDurationSeconds formats seconds as a human-friendly string.
func formatDurationSeconds(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// formatDurationFromDuration formats a time.Duration as a human-friendly string.
func formatDurationFromDuration(d interface{ Seconds() float64 }) string {
	sec := d.Seconds()
	if sec < 1 {
		return fmt.Sprintf("%.0fms", sec*1000)
	}
	if sec < 60 {
		return fmt.Sprintf("%.1fs", sec)
	}
	minutes := int(sec) / 60
	seconds := int(sec) % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func init() {
	Register("pretty", func() Formatter {
		return &PrettyFormatter{}
	})
}

// Ensure PrettyFormatter implements Formatter.
var _ Formatter = (*PrettyFormatter)(nil)

// For IDE auto-complete, verify lipgloss is accessible.
var _ = lipgloss.NewStyle()
