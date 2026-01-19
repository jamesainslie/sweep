package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/scanner"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// AppState represents the current state of the application.
type AppState int

const (
	StateResults AppState = iota
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
	NoDaemon    bool
	Cache       *cache.Cache
}

// ScanProgress tracks the progress of a scan for the TUI.
type ScanProgress struct {
	DirsScanned  int64
	FilesScanned int64
	CacheHits    int64
	CacheMisses  int64
	Scanning     bool
	StartTime    time.Time
}

// NotificationType represents the type of notification.
type NotificationType int

const (
	NotificationAdded NotificationType = iota
	NotificationRemoved
	NotificationModified
)

// Notification represents a temporary notification message.
type Notification struct {
	Type    NotificationType
	Message string
	Expires time.Time
}

// Model is the main Bubble Tea model for the sweep TUI.
type Model struct {
	state       AppState
	resultModel ResultModel
	options     Options

	// Scanning state
	ctx          context.Context
	cancel       context.CancelFunc
	scanDone     bool
	scanProgress ScanProgress
	fileChan     chan types.FileInfo
	progressChan chan types.ScanProgress

	// Live file events state
	liveEventChan <-chan client.FileEvent
	liveWatching  bool

	// Notifications for live events
	notifications []Notification

	// Confirmation dialog state
	confirmFocused int // 0 = cancel, 1 = delete

	// Deleting state
	deleteSpinner      spinner.Model
	deleteProgress     int
	deleteTotal        int
	deleteErrors       []string
	deleteProgressChan chan deleteProgressMsg
	lastFreedSize      int64 // Size freed in last delete operation

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
		state:       StateResults,
		resultModel: NewResultModel(nil), // Start with empty results
		options:     opts,
		ctx:         ctx,
		cancel:      cancel,
		scanProgress: ScanProgress{
			Scanning:  true,
			StartTime: time.Now(),
		},
		width:          80,
		height:         24,
		confirmFocused: 0,
		deleteSpinner:  s,
		fileChan:       make(chan types.FileInfo, 100),
		progressChan:   make(chan types.ScanProgress, 100),
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startStreamingScan(),
		m.listenForFiles(),
		m.listenForProgress(),
		m.tickUI(),
	)
}

// tickUIMsg triggers a UI refresh.
type tickUIMsg struct{}

// tickUI returns a command that periodically triggers UI updates.
func (m Model) tickUI() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return tickUIMsg{}
	})
}

// FileFoundMsg is sent when a new file is found during scanning.
type FileFoundMsg struct {
	File types.FileInfo
}

// ScanDoneMsg is sent when scanning completes.
type ScanDoneMsg struct {
	Err error
}

// LiveFileEventMsg is sent when a live file event is received from the daemon.
type LiveFileEventMsg struct {
	Event client.FileEvent
}

// LiveWatchStartedMsg is sent when live file watching starts successfully.
type LiveWatchStartedMsg struct {
	EventChan <-chan client.FileEvent
}

// LiveWatchErrorMsg is sent when live file watching encounters an error.
type LiveWatchErrorMsg struct {
	Err error
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resultModel.SetDimensions(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickUIMsg:
		// Clear expired notifications
		now := time.Now()
		var activeNotifications []Notification
		for _, n := range m.notifications {
			if now.Before(n.Expires) {
				activeNotifications = append(activeNotifications, n)
			}
		}
		m.notifications = activeNotifications

		// Keep UI refreshing during scanning or if we have notifications
		if !m.scanDone || m.liveWatching || len(m.notifications) > 0 {
			return m, m.tickUI()
		}
		return m, nil

	case ProgressMsg:
		m.scanProgress.DirsScanned = msg.DirsScanned
		m.scanProgress.FilesScanned = msg.FilesScanned
		m.scanProgress.CacheHits = msg.CacheHits
		m.scanProgress.CacheMisses = msg.CacheMisses
		// Keep listening for more progress
		return m, m.listenForProgress()

	case FileFoundMsg:
		// Add file to results as it's found
		m.resultModel.AddFile(msg.File)
		// Keep listening for more files
		return m, m.listenForFiles()

	case ScanDoneMsg:
		m.scanDone = true
		m.scanProgress.Scanning = false
		// Update metrics in result model
		m.resultModel.metrics = ScanMetrics{
			DirsScanned:  m.scanProgress.DirsScanned,
			FilesScanned: m.scanProgress.FilesScanned,
			CacheHits:    m.scanProgress.CacheHits,
			CacheMisses:  m.scanProgress.CacheMisses,
			Elapsed:      time.Since(m.scanProgress.StartTime),
		}
		// Start live file watching if daemon is available
		if !m.options.NoDaemon {
			return m, m.startLiveWatch()
		}
		return m, nil

	case LiveWatchStartedMsg:
		m.liveWatching = true
		m.liveEventChan = msg.EventChan
		return m, m.listenForLiveEvents()

	case LiveWatchErrorMsg:
		// Live watching failed, continue without it
		m.liveWatching = false
		return m, nil

	case LiveFileEventMsg:
		notification := handleLiveFileEvent(&m.resultModel, msg.Event)
		if notification != nil {
			m.notifications = append(m.notifications, *notification)
		}
		// Keep listening for more events
		return m, m.listenForLiveEvents()

	case spinner.TickMsg:
		if m.state == StateDeleting {
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
			return m, nil
		}
		// Keep listening for more progress
		return m, m.listenForDeleteProgress()
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
		if key == "enter" || key == "esc" {
			// Remove successfully deleted files from results and return to list
			m.removeDeletedFiles()
			m.state = StateResults
			return m, nil
		}
		if key == "q" {
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the current state.
func (m Model) View() string {
	switch m.state {
	case StateResults:
		return m.resultModel.ViewWithProgressAndNotifications(m.scanProgress, m.notifications, m.liveWatching)
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

	selectedCount := m.resultModel.SelectedCount()
	selectedSize := m.resultModel.SelectedSize()

	var dialogContent strings.Builder

	// Warning icon and title
	warningIcon := "⚠️"
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF6B6B")).Render("DELETE FILES")
	dialogContent.WriteString(fmt.Sprintf("  %s  %s  %s\n", warningIcon, title, warningIcon))
	dialogContent.WriteString("\n")

	// Stats in a nice format
	countStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF6B6B"))

	dialogContent.WriteString(fmt.Sprintf("       Files:  %s\n", countStyle.Render(fmt.Sprintf("%d", selectedCount))))
	dialogContent.WriteString(fmt.Sprintf("       Size:   %s\n", sizeStyle.Render(types.FormatSize(selectedSize))))
	dialogContent.WriteString("\n")

	if m.options.DryRun {
		dryRunStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107")).Italic(true)
		dialogContent.WriteString(center(dryRunStyle.Render("(Dry run - no files will be deleted)"), 44))
		dialogContent.WriteString("\n\n")
	}

	// Question
	dialogContent.WriteString(center(dialogTextStyle.Render("This action cannot be undone."), 44))
	dialogContent.WriteString("\n\n")

	// Buttons with better styling
	cancelBtnStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Margin(0, 1).
		Background(lipgloss.Color("#444444")).
		Foreground(lipgloss.Color("#CCCCCC"))

	deleteBtnStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Margin(0, 1).
		Background(lipgloss.Color("#444444")).
		Foreground(lipgloss.Color("#CCCCCC"))

	if m.confirmFocused == 0 {
		cancelBtnStyle = cancelBtnStyle.
			Background(lipgloss.Color("#666666")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)
	} else {
		deleteBtnStyle = deleteBtnStyle.
			Background(lipgloss.Color("#DC3545")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)
	}

	cancelBtn := cancelBtnStyle.Render("Cancel")
	deleteBtn := deleteBtnStyle.Render("Delete")

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, cancelBtn, "    ", deleteBtn)
	dialogContent.WriteString(center(buttons, 44))

	// Render dialog box with updated style
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF6B6B")).
		Padding(1, 2).
		Width(48)

	dialog := dialogStyle.Render(dialogContent.String())

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
	// Background is the results view
	bg := m.resultModel.View()

	var dialogContent strings.Builder

	deleted := m.deleteProgress - len(m.deleteErrors)

	if len(m.deleteErrors) == 0 {
		// Success
		successIcon := "✅"
		title := lipgloss.NewStyle().Bold(true).Foreground(successColor).Render("COMPLETE")
		dialogContent.WriteString(fmt.Sprintf("     %s  %s  %s\n", successIcon, title, successIcon))
	} else {
		// Partial success
		warnIcon := "⚠️"
		title := lipgloss.NewStyle().Bold(true).Foreground(warningColor).Render("COMPLETED WITH ERRORS")
		dialogContent.WriteString(fmt.Sprintf("  %s  %s  %s\n", warnIcon, title, warnIcon))
	}
	dialogContent.WriteString("\n")

	// Stats
	countStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	sizeStyle := lipgloss.NewStyle().Bold(true).Foreground(successColor)

	if m.options.DryRun {
		dialogContent.WriteString(fmt.Sprintf("    Would delete:  %s files\n", countStyle.Render(fmt.Sprintf("%d", m.deleteTotal))))
		dialogContent.WriteString(fmt.Sprintf("    Would free:    %s\n", sizeStyle.Render(types.FormatSize(m.lastFreedSize))))
	} else {
		dialogContent.WriteString(fmt.Sprintf("    Deleted:  %s files\n", countStyle.Render(fmt.Sprintf("%d", deleted))))
		dialogContent.WriteString(fmt.Sprintf("    Freed:    %s\n", sizeStyle.Render(types.FormatSize(m.lastFreedSize))))
	}

	if len(m.deleteErrors) > 0 {
		errorStyle := lipgloss.NewStyle().Foreground(dangerColor)
		dialogContent.WriteString(fmt.Sprintf("    Failed:   %s\n", errorStyle.Render(fmt.Sprintf("%d files", len(m.deleteErrors)))))
	}

	dialogContent.WriteString("\n")

	// Instructions
	enterKey := keyStyle.Render("[Enter]")
	qKey := keyStyle.Render("[q]")
	dialogContent.WriteString(center(enterKey+" "+keyDescStyle.Render("Continue")+"    "+qKey+" "+keyDescStyle.Render("Quit"), 44))

	// Render dialog box
	borderColor := successColor
	if len(m.deleteErrors) > 0 {
		borderColor = warningColor
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(48)

	dialog := dialogStyle.Render(dialogContent.String())

	// Center dialog over background
	return m.overlayDialog(bg, dialog)
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

// startStreamingScan starts the scanning process and streams files as they're found.
func (m Model) startStreamingScan() tea.Cmd {
	fileChan := m.fileChan
	progressChan := m.progressChan
	return func() tea.Msg {
		// Try daemon first if not disabled
		if !m.options.NoDaemon {
			if m.tryDaemonStreamingScan(fileChan, progressChan) {
				close(fileChan)
				close(progressChan)
				return ScanDoneMsg{}
			}
		}

		// Fall back to direct scan
		opts := scanner.Options{
			Root:        m.options.Root,
			MinSize:     m.options.MinSize,
			Exclude:     m.options.Exclude,
			DirWorkers:  m.options.DirWorkers,
			FileWorkers: m.options.FileWorkers,
			OnProgress: func(p types.ScanProgress) {
				select {
				case progressChan <- p:
				default:
					// Channel full, skip this update
				}
			},
			OnFile: func(f types.FileInfo) {
				select {
				case fileChan <- f:
				default:
					// Channel full, skip this file (shouldn't happen)
				}
			},
			Cache: m.options.Cache,
		}

		s := scanner.New(opts)
		_, err := s.Scan(m.ctx)

		// Close channels when scan completes
		close(fileChan)
		close(progressChan)

		if err != nil {
			return ScanDoneMsg{Err: err}
		}

		return ScanDoneMsg{}
	}
}

// listenForFiles returns a command that waits for files from the scanner.
func (m Model) listenForFiles() tea.Cmd {
	fileChan := m.fileChan
	return func() tea.Msg {
		f, ok := <-fileChan
		if !ok {
			// Channel closed, scan is done
			return nil
		}
		return FileFoundMsg{File: f}
	}
}

// tryDaemonStreamingScan attempts to get results from the daemon and stream them.
// Returns true if the daemon was used, false otherwise.
func (m Model) tryDaemonStreamingScan(fileChan chan types.FileInfo, progressChan chan types.ScanProgress) bool {
	// Check if daemon is running
	pidPath := client.DefaultPIDPath()
	if !client.IsDaemonRunning(pidPath) {
		return false
	}

	// Try to connect to daemon
	socketPath := client.DefaultSocketPath()
	daemonClient, err := client.ConnectWithContext(m.ctx, socketPath)
	if err != nil {
		return false
	}
	defer daemonClient.Close()

	// Check if index is ready for this path
	ready, err := daemonClient.IsIndexReady(m.ctx, m.options.Root)
	if err != nil || !ready {
		return false
	}

	// Query the daemon
	files, err := daemonClient.GetLargeFiles(m.ctx, m.options.Root, m.options.MinSize, m.options.Exclude, 0)
	if err != nil {
		return false
	}

	// Get index status for statistics
	var dirsIndexed, filesIndexed int64
	if status, err := daemonClient.GetIndexStatus(m.ctx, m.options.Root); err == nil && status != nil {
		dirsIndexed = status.DirsIndexed
		filesIndexed = status.FilesIndexed
	}

	// Send progress
	select {
	case progressChan <- types.ScanProgress{
		DirsScanned:  dirsIndexed,
		FilesScanned: filesIndexed,
		CacheHits:    filesIndexed,
		CacheMisses:  0,
	}:
	default:
	}

	// Stream files one at a time (they come sorted from daemon)
	for _, f := range files {
		select {
		case fileChan <- f:
		case <-m.ctx.Done():
			return true
		}
	}

	return true
}

// listenForProgress returns a command that waits for progress updates.
func (m Model) listenForProgress() tea.Cmd {
	progressChan := m.progressChan
	return func() tea.Msg {
		p, ok := <-progressChan
		if !ok {
			// Channel closed, scan is done
			return nil
		}
		return ProgressMsg(p)
	}
}

// startLiveWatch starts watching for live file events from the daemon.
func (m Model) startLiveWatch() tea.Cmd {
	ctx := m.ctx
	root := m.options.Root
	minSize := m.options.MinSize
	exclude := m.options.Exclude

	return func() tea.Msg {
		// Check if daemon is running
		pidPath := client.DefaultPIDPath()
		if !client.IsDaemonRunning(pidPath) {
			return LiveWatchErrorMsg{Err: errors.New("daemon not running")}
		}

		// Connect to daemon
		socketPath := client.DefaultSocketPath()
		daemonClient, err := client.ConnectWithContext(ctx, socketPath)
		if err != nil {
			return LiveWatchErrorMsg{Err: err}
		}

		// Start watching for file events
		eventChan, err := daemonClient.WatchLargeFiles(ctx, root, minSize, exclude)
		if err != nil {
			daemonClient.Close()
			return LiveWatchErrorMsg{Err: err}
		}

		// Note: We don't close daemonClient here because the stream needs it to stay open.
		// The connection will be closed when the context is cancelled.

		return LiveWatchStartedMsg{EventChan: eventChan}
	}
}

// listenForLiveEvents returns a command that waits for live file events.
func (m Model) listenForLiveEvents() tea.Cmd {
	eventChan := m.liveEventChan
	return func() tea.Msg {
		if eventChan == nil {
			return nil
		}
		event, ok := <-eventChan
		if !ok {
			// Channel closed, watching stopped
			return LiveWatchErrorMsg{Err: errors.New("live watch stream closed")}
		}
		return LiveFileEventMsg{Event: event}
	}
}

// handleLiveFileEvent processes a live file event and updates the results.
// Returns a notification if one should be shown.
func handleLiveFileEvent(resultModel *ResultModel, event client.FileEvent) *Notification {
	const notificationDuration = 3 * time.Second
	expires := time.Now().Add(notificationDuration)

	switch event.Type {
	case "created":
		// Add the new file to results
		fi := types.FileInfo{
			Path:    event.Path,
			Size:    event.Size,
			ModTime: time.Unix(event.ModTime, 0),
		}
		resultModel.AddFile(fi)
		return &Notification{
			Type:    NotificationAdded,
			Message: fmt.Sprintf("+ %s (%s)", truncateFilename(event.Path, 30), types.FormatSize(event.Size)),
			Expires: expires,
		}

	case "modified":
		// Update the file in results
		resultModel.UpdateFile(event.Path, event.Size, time.Unix(event.ModTime, 0))
		return &Notification{
			Type:    NotificationModified,
			Message: fmt.Sprintf("~ %s (%s)", truncateFilename(event.Path, 30), types.FormatSize(event.Size)),
			Expires: expires,
		}

	case "deleted":
		// Remove the file from results
		resultModel.RemoveFile(event.Path)
		return &Notification{
			Type:    NotificationRemoved,
			Message: "- " + truncateFilename(event.Path, 40),
			Expires: expires,
		}

	case "renamed":
		// Treat rename as delete - the new name will trigger a create event
		resultModel.RemoveFile(event.Path)
		return &Notification{
			Type:    NotificationRemoved,
			Message: "↻ " + truncateFilename(event.Path, 40),
			Expires: expires,
		}
	}
	return nil
}

// truncateFilename truncates a filename (not path) to fit within maxLen.
func truncateFilename(path string, maxLen int) string {
	// Extract just the filename
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			path = path[i+1:]
			break
		}
	}

	if len(path) <= maxLen {
		return path
	}
	if maxLen <= 3 {
		return path[:maxLen]
	}
	return path[:maxLen-3] + "..."
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
	m.lastFreedSize = m.resultModel.SelectedSize() // Track size being freed

	files := m.resultModel.SelectedFiles()
	dryRun := m.options.DryRun

	// Create channel for progress updates
	m.deleteProgressChan = make(chan deleteProgressMsg, 100)
	progressChan := m.deleteProgressChan

	// Start deletion in background
	go func() {
		for i, file := range files {
			var err error
			if !dryRun {
				err = os.Remove(file.Path)
			}

			// Send progress update (non-blocking)
			select {
			case progressChan <- deleteProgressMsg{current: i + 1, done: false, err: err}:
			default:
				// Channel full, skip this update
			}
		}

		// Send final completion message
		progressChan <- deleteProgressMsg{
			current: len(files),
			done:    true,
		}
		close(progressChan)
	}()

	return m, tea.Batch(m.deleteSpinner.Tick, m.listenForDeleteProgress())
}

// listenForDeleteProgress returns a command that waits for delete progress updates.
func (m Model) listenForDeleteProgress() tea.Cmd {
	progressChan := m.deleteProgressChan
	return func() tea.Msg {
		if progressChan == nil {
			return deleteProgressMsg{current: m.deleteTotal, done: true}
		}
		msg, ok := <-progressChan
		if !ok {
			return deleteProgressMsg{current: m.deleteTotal, done: true}
		}
		return msg
	}
}

// removeDeletedFiles removes successfully deleted files from the results.
func (m *Model) removeDeletedFiles() {
	// Get the files that were selected for deletion
	files := m.resultModel.SelectedFiles()

	// Build a set of paths that had errors (weren't deleted)
	errorPaths := make(map[string]bool)
	for _, errPath := range m.deleteErrors {
		errorPaths[errPath] = true
	}

	// Calculate actual freed size (excluding errors)
	var actualFreedSize int64
	for _, file := range files {
		if !errorPaths[file.Path] && !m.options.DryRun {
			actualFreedSize += file.Size
			m.resultModel.RemoveFile(file.Path)
		}
	}

	// Update the freed size (add to any previous freed size)
	currentFreed := m.resultModel.LastFreedSize()
	m.resultModel.SetLastFreedSize(currentFreed + actualFreedSize)

	// Clear selection
	m.resultModel.SelectNone()
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
