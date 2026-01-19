package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// ScanModel represents the scanning phase of the TUI.
type ScanModel struct {
	progress    types.ScanProgress
	spinner     spinner.Model
	currentPath string
	startTime   time.Time
	width       int
	height      int
	rootPath    string
	minSize     int64
	done        bool
	err         error
}

// ProgressMsg is sent when scan progress is updated.
type ProgressMsg types.ScanProgress

// ScanCompleteMsg is sent when the scan is complete.
type ScanCompleteMsg struct {
	Files        []types.FileInfo
	Err          error
	DirsScanned  int64
	FilesScanned int64
	CacheHits    int64
	CacheMisses  int64
	Elapsed      time.Duration
}

// NewScanModel creates a new scanning model.
func NewScanModel(rootPath string, minSize int64) ScanModel {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	return ScanModel{
		spinner:   s,
		startTime: time.Now(),
		width:     80,
		height:    24,
		rootPath:  rootPath,
		minSize:   minSize,
	}
}

// Init initializes the scanning model.
func (m ScanModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages for the scanning model.
func (m ScanModel) Update(msg tea.Msg) (ScanModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case ProgressMsg:
		m.progress = types.ScanProgress(msg)
		m.currentPath = msg.CurrentPath
		return m, nil

	case ScanCompleteMsg:
		m.done = true
		m.err = msg.Err
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the scanning model.
func (m ScanModel) View() string {
	var b strings.Builder

	// Calculate content width (accounting for border padding)
	contentWidth := m.width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Add top margin for visual spacing
	b.WriteString("\n")

	// Header
	header := m.renderHeader(contentWidth)
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n\n")

	// Scanning status
	if m.done {
		if m.err != nil {
			b.WriteString(errorTextStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		} else {
			b.WriteString(successTextStyle.Render("  Scan complete!"))
		}
	} else {
		scanningText := fmt.Sprintf("  %s Scanning: %s",
			m.spinner.View(),
			truncatePath(m.currentPath, contentWidth-20))
		b.WriteString(scanningText)
	}
	b.WriteString("\n")

	// Progress bar
	b.WriteString("\n")
	b.WriteString(m.renderProgressBar(contentWidth))
	b.WriteString("\n\n")

	// Stats boxes
	b.WriteString(m.renderStats(contentWidth))
	b.WriteString("\n")

	// Build content and calculate padding needed to fill screen
	content := b.String()
	contentLines := strings.Count(content, "\n") + 1

	// Account for outer box border (2 lines: top and bottom)
	availableLines := m.height - 2
	if availableLines > contentLines {
		padding := availableLines - contentLines
		content += strings.Repeat("\n", padding)
	}

	// Wrap in outer box with full height
	return outerBoxStyle.Width(m.width - 2).Height(m.height - 2).Render(content)
}

// renderHeader renders the header section.
func (m ScanModel) renderHeader(width int) string {
	title := titleStyle.Render("  sweep v1.0.0")
	hint := mutedTextStyle.Render("[Ctrl+C to stop]")

	// Calculate spacing
	spacing := width - lipgloss.Width(title) - lipgloss.Width(hint)
	if spacing < 1 {
		spacing = 1
	}

	return title + strings.Repeat(" ", spacing) + hint
}

// renderProgressBar renders the progress bar.
// Since we don't know total files upfront, we use an animated indeterminate progress bar.
func (m ScanModel) renderProgressBar(width int) string {
	barWidth := width - 4
	if barWidth < 10 {
		barWidth = 10
	}

	// Create an indeterminate progress animation
	elapsed := time.Since(m.startTime)
	position := int(elapsed.Seconds()*2) % (barWidth * 2)
	if position > barWidth {
		position = barWidth*2 - position
	}

	// Build the progress bar
	var bar strings.Builder
	bar.WriteString("  ")

	pulseWidth := barWidth / 5
	if pulseWidth < 3 {
		pulseWidth = 3
	}

	for i := range barWidth {
		dist := i - position
		if dist < 0 {
			dist = -dist
		}
		if dist < pulseWidth {
			bar.WriteString(progressFillStyle.Render("█"))
		} else {
			bar.WriteString(progressEmptyStyle.Render("░"))
		}
	}

	return bar.String()
}

// renderStats renders the statistics boxes.
func (m ScanModel) renderStats(totalWidth int) string {
	// Calculate box width (5 boxes with spacing)
	boxWidth := (totalWidth - 12) / 5
	if boxWidth < 10 {
		boxWidth = 10
	}

	// Format values
	dirsVal := humanize.Comma(m.progress.DirsScanned)
	filesVal := humanize.Comma(m.progress.FilesScanned)
	largeVal := humanize.Comma(m.progress.LargeFiles)
	elapsed := time.Since(m.startTime)
	elapsedVal := formatDuration(elapsed)

	// Cache stats
	cacheHits := m.progress.CacheHits
	cacheMisses := m.progress.CacheMisses
	var cacheVal string
	if cacheHits+cacheMisses > 0 {
		hitRate := float64(cacheHits) / float64(cacheHits+cacheMisses) * 100
		cacheVal = fmt.Sprintf("%.0f%%", hitRate)
	} else {
		cacheVal = "-"
	}

	// Create stats boxes
	dirsBox := m.renderStatBox("Dirs", dirsVal, boxWidth)
	filesBox := m.renderStatBox("Files", filesVal, boxWidth)
	largeBox := m.renderStatBox("Large", largeVal, boxWidth)
	cacheBox := m.renderStatBox("Cache", cacheVal, boxWidth)
	elapsedBox := m.renderStatBox("Time", elapsedVal, boxWidth)

	// Join horizontally
	return lipgloss.JoinHorizontal(lipgloss.Top,
		"  ", dirsBox, " ", filesBox, " ", largeBox, " ", cacheBox, " ", elapsedBox)
}

// renderStatBox renders a single stat box.
func (m ScanModel) renderStatBox(label, value string, width int) string {
	labelStr := statsLabelStyle.Render(label)
	valueStr := statsValueStyle.Render(value)

	content := lipgloss.JoinVertical(lipgloss.Center,
		center(labelStr, width-4),
		center(valueStr, width-4))

	return statsBoxStyle.Width(width).Render(content)
}

// formatDuration formats a duration as M:SS.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	return fmt.Sprintf("%d:%02d", m, s)
}

// SetProgress updates the progress.
func (m *ScanModel) SetProgress(p types.ScanProgress) {
	m.progress = p
	m.currentPath = p.CurrentPath
}

// SetDone marks the scan as complete.
func (m *ScanModel) SetDone(err error) {
	m.done = true
	m.err = err
}

// IsDone returns true if the scan is complete.
func (m ScanModel) IsDone() bool {
	return m.done
}

// Error returns any error from the scan.
func (m ScanModel) Error() error {
	return m.err
}
