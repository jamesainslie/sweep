package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// renderAppHeader renders the shared application header with title and stats.
// Parameters:
//   - fileCount: number of large files found
//   - totalSize: total size of large files
//   - freedSize: size freed in last delete operation (0 if none)
//   - liveWatching: whether live file watching is active
func renderAppHeader(fileCount int, totalSize int64, freedSize int64, liveWatching bool) string {
	// Icon and app name
	icon := "🧹"
	appName := titleStyle.Bold(true).Render("SWEEP")

	// Stats in muted style
	fileCountStr := fmt.Sprintf("%d files", fileCount)
	totalSizeStr := types.FormatSize(totalSize)
	stats := mutedTextStyle.Render(fmt.Sprintf("  %s  •  %s", fileCountStr, totalSizeStr))

	header := fmt.Sprintf(" %s %s%s", icon, appName, stats)

	// Show freed size if any
	if freedSize > 0 {
		freedStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
		freed := freedStyle.Render(fmt.Sprintf("  ✓ Freed %s", types.FormatSize(freedSize)))
		header = header + freed
	}

	// Show live indicator if watching
	if liveWatching {
		liveIndicator := successTextStyle.Render("  ● LIVE")
		header = header + liveIndicator
	}

	return header
}

// renderScanMetrics renders the scan metrics line showing directories/files scanned and elapsed time.
// Parameters:
//   - dirsScanned: number of directories scanned
//   - filesScanned: number of files scanned
//   - elapsed: elapsed time of the scan
//
// Returns an empty string if there are no metrics to display.
func renderScanMetrics(dirsScanned, filesScanned int64, elapsed time.Duration) string {
	var parts []string

	// Dirs and files scanned
	if dirsScanned > 0 || filesScanned > 0 {
		parts = append(parts, fmt.Sprintf("Scanned: %s dirs, %s files",
			humanize.Comma(dirsScanned),
			humanize.Comma(filesScanned)))
	}

	// Elapsed time
	if elapsed > 0 {
		parts = append(parts, fmt.Sprintf("Time: %v", elapsed.Round(time.Millisecond)))
	}

	if len(parts) == 0 {
		return ""
	}

	return mutedTextStyle.Render("  " + strings.Join(parts, "  |  "))
}
