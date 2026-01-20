package output

import "github.com/charmbracelet/lipgloss"

// Color constants using ANSI 256-color palette.
// These provide a consistent color scheme across all formatters.
const (
	// ColorPrimary is used for primary elements like headers (bright blue).
	ColorPrimary = lipgloss.Color("39")

	// ColorSuccess is used for positive status indicators (green).
	ColorSuccess = lipgloss.Color("42")

	// ColorWarning is used for warning messages (orange/yellow).
	ColorWarning = lipgloss.Color("214")

	// ColorDanger is used for errors and critical information (red).
	ColorDanger = lipgloss.Color("196")

	// ColorMuted is used for less important or secondary text (gray).
	ColorMuted = lipgloss.Color("245")
)

// Box styles for containing grouped content.
var (
	// HeaderBox is the style for the header section containing scan info.
	HeaderBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1).
			MarginBottom(1)

	// FooterBox is the style for the footer section containing summary info.
	FooterBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1).
			MarginTop(1)

	// ErrorBox is the style for error messages.
	ErrorBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDanger).
			Padding(0, 1)
)

// Text styles for various content types.
var (
	// TitleStyle is used for major section titles.
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	// LabelStyle is used for field labels (e.g., "Source:", "Files:").
	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// ValueStyle is used for field values.
	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	// SuccessStyle is used for positive status text.
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	// WarningStyle is used for warning text.
	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorWarning)

	// ErrorStyle is used for error text.
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorDanger)

	// MutedStyle is used for less important text.
	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// PathStyle is used for file paths.
	PathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	// SizeStyle is used for file sizes.
	SizeStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
)

// Table styles for tabular data display.
var (
	// TableHeaderStyle is used for table column headers.
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorMuted).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorMuted).
				PaddingRight(2)

	// TableRowStyle is used for table data rows.
	TableRowStyle = lipgloss.NewStyle().
			PaddingRight(2)
)
