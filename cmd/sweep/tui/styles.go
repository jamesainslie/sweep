// Package tui provides an interactive terminal user interface for the sweep disk analyzer.
// It uses Charmbracelet's Bubble Tea, Lip Gloss, and Bubbles for a beautiful terminal UI.
package tui

import "github.com/charmbracelet/lipgloss"

// Color palette for the TUI.
var (
	// Primary colors
	primaryColor = lipgloss.Color("#7D56F4")

	// Status colors
	successColor = lipgloss.Color("#28A745")
	warningColor = lipgloss.Color("#FFC107")
	dangerColor  = lipgloss.Color("#DC3545")

	// Neutral colors
	mutedColor  = lipgloss.Color("#666666")
	subtleColor = lipgloss.Color("#444444")
	borderColor = lipgloss.Color("#333333")
)

// Box styles for containers.
var (
	// outerBoxStyle is the main container style.
	// No horizontal padding so highlighted rows can span edge-to-edge.
	outerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor)

	// dividerStyle creates horizontal dividers.
	dividerStyle = lipgloss.NewStyle().
			Foreground(borderColor)
)

// Text styles.
var (
	// titleStyle for main titles.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	// mutedTextStyle for less important text.
	mutedTextStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// errorTextStyle for error messages.
	errorTextStyle = lipgloss.NewStyle().
			Foreground(dangerColor)

	// successTextStyle for success messages.
	successTextStyle = lipgloss.NewStyle().
				Foreground(successColor)

	// warningTextStyle for warning messages.
	warningTextStyle = lipgloss.NewStyle().
				Foreground(warningColor)
)

// File list styles are now handled by the bubbles/table component.

// Progress bar styles.
var (
	// progressFillStyle for the filled portion.
	progressFillStyle = lipgloss.NewStyle().
				Foreground(successColor)

	// progressEmptyStyle for the empty portion.
	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(subtleColor)
)

// Stats box styles.
var (
	// statsBoxStyle for the stats container.
	statsBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(0, 2)

	// statsLabelStyle for stat labels.
	statsLabelStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// statsValueStyle for stat values.
	statsValueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))
)

// Key hint styles.
var (
	// keyStyle for keyboard key hints.
	keyStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	// keyDescStyle for key descriptions.
	keyDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)

// Confirmation dialog styles.
var (
	// dialogBoxStyle for modal dialogs.
	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(warningColor).
			Padding(1, 2).
			Width(50)

	// dialogTitleStyle for dialog titles.
	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(warningColor).
				Align(lipgloss.Center)

	// dialogTextStyle for dialog content.
	dialogTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Align(lipgloss.Center)

	// activeButtonStyle for the active/focused button.
	activeButtonStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Margin(0, 1).
				Background(dangerColor).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	// inactiveButtonStyle for inactive buttons.
	inactiveButtonStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Margin(0, 1).
				Background(subtleColor).
				Foreground(lipgloss.Color("#CCCCCC"))
)

// renderDivider creates a horizontal divider line.
func renderDivider(width int) string {
	return dividerStyle.Render(repeatChar('─', width))
}

// repeatChar repeats a character n times.
func repeatChar(char rune, n int) string {
	if n <= 0 {
		return ""
	}
	result := make([]rune, n)
	for i := range result {
		result[i] = char
	}
	return string(result)
}

// truncatePath truncates a path to fit within maxLen, preserving the end.
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	if maxLen <= 3 {
		return path[:maxLen]
	}
	return "..." + path[len(path)-(maxLen-3):]
}

// padLeft pads a string to the left to reach the target width.
func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return repeatChar(' ', width-len(s)) + s
}

// center centers a string within the given width.
func center(s string, width int) string {
	if len(s) >= width {
		return s
	}
	leftPad := (width - len(s)) / 2
	rightPad := width - len(s) - leftPad
	return repeatChar(' ', leftPad) + s + repeatChar(' ', rightPad)
}

// Notification styles for live file events.
var (
	// notificationAddedStyle for file added notifications.
	notificationAddedStyle = lipgloss.NewStyle().
				Foreground(successColor).
				Padding(0, 1)

	// notificationRemovedStyle for file removed notifications.
	notificationRemovedStyle = lipgloss.NewStyle().
					Foreground(dangerColor).
					Padding(0, 1)

	// notificationModifiedStyle for file modified notifications.
	notificationModifiedStyle = lipgloss.NewStyle().
					Foreground(warningColor).
					Padding(0, 1)
)
