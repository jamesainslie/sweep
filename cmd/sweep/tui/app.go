package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/scanner"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// AppState represents the current state of the application.
type AppState int

const (
	StateScanning AppState = iota
	StateResults
	StateConfirm
	StateDeleting
	StateComplete
)

// Options configures the TUI application.
type Options struct {
	Root        string
	MinSize     int64
	Exclude     []string
	DirWorkers  int
	FileWorkers int
	DryRun      bool
	Cache       *cache.Cache
}

// Model is the main Bubble Tea model for the sweep TUI.
type Model struct {
	state       AppState
	scanModel   ScanModel
	resultModel ResultModel
	options     Options

	// Scanning state
	ctx       context.Context
	cancel    context.CancelFunc
	scanDone  bool
	scanErr   error
	scanFiles []types.FileInfo

	// Confirmation dialog state
	confirmFocused int // 0 = cancel, 1 = delete

	// Deleting state
	deleteSpinner  spinner.Model
	deleteProgress int
	deleteTotal    int
	deleteErrors   []string

	// Window dimensions
	width  int
	height int
}

// NewModel creates a new TUI model with the given options.
func NewModel(opts Options) Model {
	ctx, cancel := context.WithCancel(context.Background())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(dangerColor)

	return Model{
		state:          StateScanning,
		scanModel:      NewScanModel(opts.Root, opts.MinSize),
		options:        opts,
		ctx:            ctx,
		cancel:         cancel,
		width:          80,
		height:         24,
		confirmFocused: 0,
		deleteSpinner:  s,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.scanModel.Init(),
		m.startScan(),
		m.tickProgress(),
	)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scanModel.width = msg.Width
		m.scanModel.height = msg.Height
		m.resultModel.SetDimensions(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case ProgressMsg:
		m.scanModel.SetProgress(types.ScanProgress(msg))
		return m, nil

	case ScanCompleteMsg:
		m.scanDone = true
		m.scanErr = msg.Err
		m.scanFiles = msg.Files
		m.scanModel.SetDone(msg.Err)

		if msg.Err == nil {
			// Transition to results
			m.state = StateResults
			m.resultModel = NewResultModel(msg.Files)
			m.resultModel.SetDimensions(m.width, m.height)
		}
		return m, nil

	case progressTickMsg:
		if m.state == StateScanning && !m.scanDone {
			return m, m.tickProgress()
		}
		return m, nil

	case spinner.TickMsg:
		switch m.state {
		case StateScanning:
			var cmd tea.Cmd
			m.scanModel.spinner, cmd = m.scanModel.spinner.Update(msg)
			cmds = append(cmds, cmd)
		case StateDeleting:
			var cmd tea.Cmd
			m.deleteSpinner, cmd = m.deleteSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case deleteProgressMsg:
		m.deleteProgress = msg.current
		if msg.err != nil {
			m.deleteErrors = append(m.deleteErrors, msg.err.Error())
		}
		if msg.done {
			m.state = StateComplete
		}
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// handleKey handles keyboard input.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "ctrl+c":
		m.cancel()
		return m, tea.Quit
	}

	// State-specific keys
	switch m.state {
	case StateScanning:
		if key == "q" || key == "esc" {
			m.cancel()
			return m, tea.Quit
		}

	case StateResults:
		switch key {
		case "q", "esc":
			return m, tea.Quit
		case "enter":
			if m.resultModel.HasSelection() {
				m.state = StateConfirm
				m.confirmFocused = 0 // Default to cancel
			}
		default:
			m.resultModel.HandleKey(key)
		}

	case StateConfirm:
		switch key {
		case "q", "esc", "n":
			m.state = StateResults
		case "left", "h":
			m.confirmFocused = 0
		case "right", "l":
			m.confirmFocused = 1
		case "tab":
			m.confirmFocused = (m.confirmFocused + 1) % 2
		case "enter":
			if m.confirmFocused == 1 {
				// Delete confirmed
				return m.startDelete()
			}
			m.state = StateResults
		case "y":
			// Shortcut for yes
			return m.startDelete()
		}

	case StateDeleting:
		// No key handling during delete

	case StateComplete:
		if key == "q" || key == "enter" || key == "esc" {
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the current state.
func (m Model) View() string {
	switch m.state {
	case StateScanning:
		return m.scanModel.View()
	case StateResults:
		return m.resultModel.View()
	case StateConfirm:
		return m.renderConfirmDialog()
	case StateDeleting:
		return m.renderDeleting()
	case StateComplete:
		return m.renderComplete()
	}
	return ""
}

// renderConfirmDialog renders the deletion confirmation dialog.
func (m Model) renderConfirmDialog() string {
	// Background is the results view
	bg := m.resultModel.View()

	// Build dialog content
	selectedCount := m.resultModel.SelectedCount()
	selectedSize := m.resultModel.SelectedSize()

	var dialogContent strings.Builder
	dialogContent.WriteString(dialogTitleStyle.Render("Confirm Deletion"))
	dialogContent.WriteString("\n\n")
	dialogContent.WriteString(dialogTextStyle.Render(
		fmt.Sprintf("Delete %d files (%s)?", selectedCount, types.FormatSize(selectedSize))))
	dialogContent.WriteString("\n")

	if m.options.DryRun {
		dialogContent.WriteString(warningTextStyle.Render("(Dry run - no files will be deleted)"))
		dialogContent.WriteString("\n")
	}

	dialogContent.WriteString("\n")

	// Buttons
	cancelBtn := inactiveButtonStyle.Render("Cancel")
	deleteBtn := inactiveButtonStyle.Render("Delete")

	if m.confirmFocused == 0 {
		cancelBtn = activeButtonStyle.Render("Cancel")
	} else {
		deleteBtn = activeButtonStyle.Background(dangerColor).Render("Delete")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, cancelBtn, "  ", deleteBtn)
	dialogContent.WriteString(center(buttons, 46))

	// Render dialog box
	dialog := dialogBoxStyle.Render(dialogContent.String())

	// Center dialog over background
	return m.overlayDialog(bg, dialog)
}

// renderDeleting renders the deletion progress view.
func (m Model) renderDeleting() string {
	contentWidth := m.width - 4

	var b strings.Builder
	b.WriteString(titleStyle.Render("  Deleting files..."))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n\n")

	// Progress
	b.WriteString(fmt.Sprintf("  %s Deleting: %d / %d files",
		m.deleteSpinner.View(), m.deleteProgress, m.deleteTotal))
	b.WriteString("\n\n")

	// Progress bar
	if m.deleteTotal > 0 {
		pct := float64(m.deleteProgress) / float64(m.deleteTotal)
		barWidth := contentWidth - 4
		filled := int(pct * float64(barWidth))
		empty := barWidth - filled

		bar := "  " + progressFillStyle.Render(strings.Repeat("█", filled)) +
			progressEmptyStyle.Render(strings.Repeat("░", empty))
		b.WriteString(bar)
		b.WriteString(fmt.Sprintf(" %d%%", int(pct*100)))
		b.WriteString("\n")
	}

	// Errors
	if len(m.deleteErrors) > 0 {
		b.WriteString("\n")
		b.WriteString(errorTextStyle.Render(fmt.Sprintf("  %d errors:", len(m.deleteErrors))))
		b.WriteString("\n")
		for _, e := range m.deleteErrors {
			b.WriteString(errorTextStyle.Render("    - " + truncatePath(e, contentWidth-6)))
			b.WriteString("\n")
		}
	}

	return outerBoxStyle.Width(m.width - 2).Render(b.String())
}

// renderComplete renders the completion view.
func (m Model) renderComplete() string {
	contentWidth := m.width - 4

	var b strings.Builder
	b.WriteString(successTextStyle.Render("  Deletion Complete"))
	b.WriteString("\n")
	b.WriteString(renderDivider(contentWidth))
	b.WriteString("\n\n")

	deleted := m.deleteProgress - len(m.deleteErrors)
	if m.options.DryRun {
		b.WriteString(fmt.Sprintf("  Would have deleted: %d files\n", m.deleteTotal))
	} else {
		b.WriteString(fmt.Sprintf("  Successfully deleted: %d files\n", deleted))
	}

	if len(m.deleteErrors) > 0 {
		b.WriteString(errorTextStyle.Render(fmt.Sprintf("  Failed: %d files\n", len(m.deleteErrors))))
		b.WriteString("\n")
		b.WriteString(errorTextStyle.Render("  Errors:"))
		b.WriteString("\n")
		maxErrors := 5
		for i, e := range m.deleteErrors {
			if i >= maxErrors {
				b.WriteString(errorTextStyle.Render(fmt.Sprintf("    ... and %d more", len(m.deleteErrors)-maxErrors)))
				b.WriteString("\n")
				break
			}
			b.WriteString(errorTextStyle.Render("    - " + truncatePath(e, contentWidth-6)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(center(keyStyle.Render("[Enter]")+" "+keyDescStyle.Render("Exit"), contentWidth))
	b.WriteString("\n")

	return outerBoxStyle.Width(m.width - 2).Render(b.String())
}

// overlayDialog centers a dialog over a background view.
func (m Model) overlayDialog(bg, dialog string) string {
	// For simplicity, just replace the view with dialog centered
	// In a real implementation, you'd composite them

	dialogLines := strings.Split(dialog, "\n")
	bgLines := strings.Split(bg, "\n")

	// Calculate vertical position
	dialogHeight := len(dialogLines)
	startRow := (m.height - dialogHeight) / 2
	if startRow < 0 {
		startRow = 0
	}

	// Calculate horizontal position
	dialogWidth := lipgloss.Width(dialog)
	startCol := (m.width - dialogWidth) / 2
	if startCol < 0 {
		startCol = 0
	}

	// Build output
	var result []string
	for i := range max(len(bgLines), startRow+dialogHeight) {
		if i < startRow || i >= startRow+dialogHeight {
			if i < len(bgLines) {
				result = append(result, bgLines[i])
			} else {
				result = append(result, "")
			}
		} else {
			dialogLine := dialogLines[i-startRow]
			// Dim the background line and overlay dialog
			if i < len(bgLines) {
				bgLine := bgLines[i]
				// Simple overlay: pad left then append dialog
				if startCol > len(bgLine) {
					result = append(result, strings.Repeat(" ", startCol)+dialogLine)
				} else {
					// Overlay dialog on background
					line := bgLine[:min(startCol, len(bgLine))] + dialogLine
					result = append(result, line)
				}
			} else {
				result = append(result, strings.Repeat(" ", startCol)+dialogLine)
			}
		}
	}

	return strings.Join(result, "\n")
}

// startScan starts the scanning process.
func (m Model) startScan() tea.Cmd {
	return func() tea.Msg {
		opts := scanner.Options{
			Root:        m.options.Root,
			MinSize:     m.options.MinSize,
			Exclude:     m.options.Exclude,
			DirWorkers:  m.options.DirWorkers,
			FileWorkers: m.options.FileWorkers,
			OnProgress:  nil, // Will be set separately
			Cache:       m.options.Cache,
		}

		s := scanner.New(opts)
		result, err := s.Scan(m.ctx)

		if err != nil {
			return ScanCompleteMsg{Err: err}
		}

		// Sort by size descending
		files := result.Files
		sort.Slice(files, func(i, j int) bool {
			return files[i].Size > files[j].Size
		})

		return ScanCompleteMsg{Files: files}
	}
}

// progressTickMsg is sent periodically to update progress display.
type progressTickMsg struct{}

// tickProgress returns a command that ticks progress updates.
func (m Model) tickProgress() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return progressTickMsg{}
	})
}

// deleteProgressMsg reports deletion progress.
type deleteProgressMsg struct {
	current int
	done    bool
	err     error
}

// startDelete begins the deletion process.
func (m Model) startDelete() (tea.Model, tea.Cmd) {
	m.state = StateDeleting
	m.deleteTotal = m.resultModel.SelectedCount()
	m.deleteProgress = 0
	m.deleteErrors = nil

	files := m.resultModel.SelectedFiles()
	dryRun := m.options.DryRun

	cmd := func() tea.Msg {
		for i, file := range files {
			var err error
			if !dryRun {
				err = os.Remove(file.Path)
			}

			// Send progress update through program
			// Note: In real implementation, use program.Send()
			// For now, we'll batch at the end

			if err != nil {
				m.deleteErrors = append(m.deleteErrors, fmt.Sprintf("%s: %v", file.Path, err))
			}

			// Small delay for visual feedback
			time.Sleep(10 * time.Millisecond)
			_ = i // Progress tracking would go here
		}

		return deleteProgressMsg{
			current: len(files),
			done:    true,
		}
	}

	return m, tea.Batch(m.deleteSpinner.Tick, cmd)
}

// Run starts the TUI application.
func Run(opts Options) error {
	model := NewModel(opts)

	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := p.Run()
	return err
}
