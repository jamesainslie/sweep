package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/filter"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
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
	Filter      *filter.Filter // Optional filter for pre-filtering views
}

// ScanProgress tracks the progress of a scan for the TUI.
type ScanProgress struct {
	DirsScanned  int64
	FilesScanned int64
	CacheHits    int64
	CacheMisses  int64
	Scanning     bool
	StartTime    time.Time
	// WalkCompleteElapsed is the frozen elapsed time when directory traversal completes.
	// If non-zero, this is used for display instead of continuing to count from StartTime.
	WalkCompleteElapsed time.Duration
}

// NotificationType represents the type of notification.
type NotificationType int

const (
	NotificationAdded NotificationType = iota
	NotificationRemoved
	NotificationModified
	NotificationRenamed
)

// Notification represents a temporary notification message.
type Notification struct {
	Type      NotificationType
	Message   string
	Expires   time.Time
	CreatedAt time.Time
}

// pendingRenameEvent tracks a rename event waiting for its matching create.
type pendingRenameEvent struct {
	OldPath   string
	Timestamp time.Time
}

// renameCorrelationWindow is how long to wait for a create after a rename.
const renameCorrelationWindow = 100 * time.Millisecond

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

	// Pending rename tracking (to correlate rename + create events)
	pendingRename *pendingRenameEvent

	// Status bar hint state
	statusHint       *logging.LogEntry // Current hint to display (nil if none)
	statusHintExpiry time.Time         // When to hide the hint
	logEntryChan     <-chan logging.LogEntry

	// Log viewer pane state
	logViewer *LogViewerState

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
	log := logging.Get("tui")
	log.Info("TUI starting", "root", opts.Root, "minSize", types.FormatSize(opts.MinSize))

	ctx, cancel := context.WithCancel(context.Background())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(dangerColor)

	// Subscribe to log entries for status bar hints
	logEntryChan := logging.Subscribe()

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
		logEntryChan:   logEntryChan,
		logViewer:      NewLogViewerState(),
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startStreamingScan(),
		m.listenForFiles(),
		m.listenForProgress(),
		m.listenForLogEntries(),
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

// DaemonFilesMsg is sent when daemon returns all files at once.
type DaemonFilesMsg struct {
	Files        []types.FileInfo
	DirsScanned  int64
	FilesScanned int64
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

// LogEntryMsg is sent when a log entry is received from the logging system.
type LogEntryMsg struct {
	Entry logging.LogEntry
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

		// Clear expired status hint
		if m.statusHint != nil && now.After(m.statusHintExpiry) {
			m.statusHint = nil
		}

		// Keep UI refreshing during scanning, live watching, notifications, or status hint
		if !m.scanDone || m.liveWatching || len(m.notifications) > 0 || m.statusHint != nil {
			return m, m.tickUI()
		}
		return m, nil

	case ProgressMsg:
		m.scanProgress.DirsScanned = msg.DirsScanned
		m.scanProgress.FilesScanned = msg.FilesScanned
		m.scanProgress.CacheHits = msg.CacheHits
		m.scanProgress.CacheMisses = msg.CacheMisses
		// Freeze elapsed time when walk completes (cache flush may continue)
		if msg.WalkComplete && m.scanProgress.WalkCompleteElapsed == 0 {
			m.scanProgress.WalkCompleteElapsed = time.Since(m.scanProgress.StartTime)
		}
		// Keep listening for more progress
		return m, m.listenForProgress()

	case FileFoundMsg:
		// Add file to results as it's found (if it passes the filter)
		if m.filePassesFilter(msg.File) {
			m.resultModel.AddFile(msg.File)
		}
		// Keep listening for more files
		return m, m.listenForFiles()

	case DaemonFilesMsg:
		// Daemon returned all files at once - apply filter and add them
		filteredFiles := m.applyFilterToFiles(msg.Files)
		for _, f := range filteredFiles {
			m.resultModel.AddFile(f)
		}
		// Update progress
		m.scanProgress.DirsScanned = msg.DirsScanned
		m.scanProgress.FilesScanned = msg.FilesScanned
		m.scanProgress.CacheHits = msg.FilesScanned
		m.scanProgress.CacheMisses = 0
		// Mark scan as done
		m.scanDone = true
		m.scanProgress.Scanning = false
		elapsed := time.Since(m.scanProgress.StartTime)
		m.resultModel.metrics = ScanMetrics{
			DirsScanned:  msg.DirsScanned,
			FilesScanned: msg.FilesScanned,
			CacheHits:    msg.FilesScanned,
			CacheMisses:  0,
			Elapsed:      elapsed,
		}
		logging.Get("tui").Info("scan completed via daemon",
			"files", len(filteredFiles),
			"filtered_from", len(msg.Files),
			"elapsed", elapsed.Round(time.Millisecond))
		// Start live file watching
		if !m.options.NoDaemon {
			return m, m.startLiveWatch()
		}
		return m, nil

	case ScanDoneMsg:
		m.scanDone = true
		m.scanProgress.Scanning = false
		elapsed := time.Since(m.scanProgress.StartTime)
		// Update metrics in result model
		m.resultModel.metrics = ScanMetrics{
			DirsScanned:  m.scanProgress.DirsScanned,
			FilesScanned: m.scanProgress.FilesScanned,
			CacheHits:    m.scanProgress.CacheHits,
			CacheMisses:  m.scanProgress.CacheMisses,
			Elapsed:      elapsed,
		}
		logging.Get("tui").Info("scan completed",
			"files", len(m.resultModel.files),
			"dirs", m.scanProgress.DirsScanned,
			"elapsed", elapsed.Round(time.Millisecond))
		// Start live file watching if daemon is available
		if !m.options.NoDaemon {
			return m, m.startLiveWatch()
		}
		return m, nil

	case LiveWatchStartedMsg:
		m.liveWatching = true
		m.liveEventChan = msg.EventChan
		logging.Get("tui").Debug("live watch started")
		return m, m.listenForLiveEvents()

	case LiveWatchErrorMsg:
		// Live watching failed, continue without it
		m.liveWatching = false
		return m, nil

	case LiveFileEventMsg:
		now := time.Now()

		// Check for stale pending rename (timed out without matching create)
		if m.pendingRename != nil && now.Sub(m.pendingRename.Timestamp) > renameCorrelationWindow {
			// Emit as a removal since we didn't see a matching create
			m.notifications = append(m.notifications, Notification{
				Type:      NotificationRemoved,
				Message:   truncateFilename(m.pendingRename.OldPath, 40),
				Expires:   now.Add(3 * time.Second),
				CreatedAt: m.pendingRename.Timestamp,
			})
			m.pendingRename = nil
		}

		// Handle rename correlation
		if msg.Event.Type == "renamed" {
			// Store for correlation with upcoming create
			m.pendingRename = &pendingRenameEvent{
				OldPath:   msg.Event.Path,
				Timestamp: now,
			}
			// Remove from results immediately
			m.resultModel.RemoveFile(msg.Event.Path)
			// Don't emit notification yet - wait for create
			return m, m.listenForLiveEvents()
		}

		if msg.Event.Type == "created" && m.pendingRename != nil &&
			now.Sub(m.pendingRename.Timestamp) <= renameCorrelationWindow {
			// This create likely matches the pending rename
			oldPath := m.pendingRename.OldPath
			m.pendingRename = nil

			// Add the file with new path
			fi := types.FileInfo{
				Path:    msg.Event.Path,
				Size:    msg.Event.Size,
				ModTime: time.Unix(msg.Event.ModTime, 0),
			}
			if m.options.Filter == nil || m.options.Filter.Match(toFilterFileInfo(fi)) {
				m.resultModel.AddFile(fi)
			}

			// Emit rename notification with old → new
			m.notifications = append(m.notifications, Notification{
				Type:      NotificationRenamed,
				Message:   fmt.Sprintf("%s → %s", truncateFilename(oldPath, 18), truncateFilename(msg.Event.Path, 18)),
				Expires:   now.Add(3 * time.Second),
				CreatedAt: now,
			})
			return m, m.listenForLiveEvents()
		}

		// Normal event handling
		notification := handleLiveFileEvent(&m.resultModel, msg.Event, m.options.Filter)
		if notification != nil {
			m.notifications = append(m.notifications, *notification)
		}
		// Keep listening for more events
		return m, m.listenForLiveEvents()

	case LogEntryMsg:
		// Add to log viewer buffer
		m.logViewer.AddEntry(msg.Entry)

		// Only show info/warn/error level hints (filter out debug)
		if msg.Entry.Level >= logging.LevelInfo {
			m.statusHint = &msg.Entry
			m.statusHintExpiry = time.Now().Add(3 * time.Second)
		}
		// Keep listening for more log entries
		return m, m.listenForLogEntries()

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
		// Log viewer takes priority when open
		if m.logViewer.Open {
			switch key {
			case "L":
				m.logViewer.Toggle()
			case "esc":
				m.logViewer.Open = false
			case "1":
				m.logViewer.SetFilterLevel(logging.LevelDebug)
			case "2":
				m.logViewer.SetFilterLevel(logging.LevelInfo)
			case "3":
				m.logViewer.SetFilterLevel(logging.LevelWarn)
			case "4":
				m.logViewer.SetFilterLevel(logging.LevelError)
			case "up", "k":
				m.logViewer.ScrollUp()
			case "down", "j":
				m.logViewer.ScrollDown(m.logViewerVisibleRows())
			case "q":
				return m, tea.Quit
			}
			return m, nil
		}

		switch key {
		case "q", "esc":
			return m, tea.Quit
		case "L":
			m.logViewer.Toggle()
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
		return m.renderResultsWithLogViewer()
	case StateConfirm:
		return m.renderConfirmDialog()
	case StateDeleting:
		return m.renderDeleting()
	case StateComplete:
		return m.renderComplete()
	}
	return ""
}

// renderResultsWithLogViewer renders the results view, optionally with the log viewer pane.
func (m Model) renderResultsWithLogViewer() string {
	if !m.logViewer.Open {
		return m.resultModel.ViewWithProgressAndNotifications(m.scanProgress, m.notifications, m.liveWatching, m.statusHint)
	}

	// Calculate heights: log viewer takes bottom 1/3 of screen
	logViewerHeight := m.height / 3
	if logViewerHeight < 5 {
		logViewerHeight = 5
	}
	resultsHeight := m.height - logViewerHeight

	// Render results with reduced height
	m.resultModel.SetDimensions(m.width, resultsHeight)
	resultsView := m.resultModel.ViewWithProgressAndNotifications(m.scanProgress, m.notifications, m.liveWatching, m.statusHint)

	// Render log viewer pane
	logViewerView := m.renderLogViewerPane(logViewerHeight)

	// Stack them vertically
	return resultsView + "\n" + logViewerView
}

// renderLogViewerPane renders the collapsible log viewer pane.
func (m Model) renderLogViewerPane(height int) string {
	contentWidth := m.width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	entries := m.logViewer.Buffer.Entries()
	return renderLogViewer(entries, m.logViewer.FilterLevel, m.logViewer.ScrollOffset, contentWidth, height)
}

// logViewerVisibleRows returns the number of visible rows in the log viewer.
func (m Model) logViewerVisibleRows() int {
	logViewerHeight := m.height / 3
	if logViewerHeight < 5 {
		logViewerHeight = 5
	}
	// Subtract 2 for title bar and divider
	return logViewerHeight - 2
}

// renderConfirmDialog renders the deletion confirmation dialog.
func (m Model) renderConfirmDialog() string {
	// Background is the results view
	bg := m.resultModel.View()

	selectedCount := m.resultModel.SelectedCount()
	selectedSize := m.resultModel.SelectedSize()

	var dialogContent strings.Builder

	// Summary with clear formatting
	dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render("Delete "))
	dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true).Render(fmt.Sprintf("%d files", selectedCount)))
	dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(" ("))
	dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true).Render(types.FormatSize(selectedSize)))
	dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render(")?"))
	dialogContent.WriteString("\n")

	if m.options.DryRun {
		dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107")).Italic(true).Render("(dry run)"))
		dialogContent.WriteString("\n")
	}

	dialogContent.WriteString("\n")

	// Clear button options
	if m.confirmFocused == 0 {
		dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render("[n] Cancel"))
		dialogContent.WriteString("   ")
		dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("[y] Delete"))
	} else {
		dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("[n] Cancel"))
		dialogContent.WriteString("   ")
		dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true).Render("[y] Delete"))
	}

	// Minimal dialog box
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#666666")).
		Padding(1, 3)

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
	sizeStyle := lipgloss.NewStyle().Foreground(successColor)

	if m.options.DryRun {
		dialogContent.WriteString(fmt.Sprintf("Would free %s (%d files)", sizeStyle.Render(types.FormatSize(m.lastFreedSize)), m.deleteTotal))
	} else {
		dialogContent.WriteString(fmt.Sprintf("Freed %s (%d files)", sizeStyle.Render(types.FormatSize(m.lastFreedSize)), deleted))
	}

	if len(m.deleteErrors) > 0 {
		errorStyle := lipgloss.NewStyle().Foreground(dangerColor)
		dialogContent.WriteString(errorStyle.Render(fmt.Sprintf(", %d failed", len(m.deleteErrors))))
	}

	dialogContent.WriteString("\n\n")
	dialogContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("[Enter] Continue  [q] Quit"))

	// Minimal dialog box
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 2)

	dialog := dialogStyle.Render(dialogContent.String())

	// Center dialog over background
	return m.overlayDialog(bg, dialog)
}

// overlayDialog centers a dialog over a background view.
func (m Model) overlayDialog(bg, dialog string) string {
	dialogLines := strings.Split(dialog, "\n")

	// Calculate dialog dimensions
	dialogHeight := len(dialogLines)
	dialogWidth := lipgloss.Width(dialog)

	// Calculate centered position
	startRow := (m.height - dialogHeight) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (m.width - dialogWidth) / 2
	if startCol < 0 {
		startCol = 0
	}

	// Build the centered dialog with padding
	var result strings.Builder
	for i := 0; i < m.height; i++ {
		if i > 0 {
			result.WriteString("\n")
		}
		if i >= startRow && i < startRow+dialogHeight {
			// Dialog row - pad left to center
			result.WriteString(strings.Repeat(" ", startCol))
			result.WriteString(dialogLines[i-startRow])
		}
		// Non-dialog rows are left empty (alt screen clears them)
	}

	return result.String()
}

// startStreamingScan starts the scanning process and streams files as they're found.
func (m Model) startStreamingScan() tea.Cmd {
	fileChan := m.fileChan
	progressChan := m.progressChan
	return func() tea.Msg {
		// Try daemon first if not disabled - returns all files instantly
		if !m.options.NoDaemon {
			if msg := m.tryDaemonInstantLoad(); msg != nil {
				close(fileChan)
				close(progressChan)
				return *msg
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

// tryDaemonInstantLoad attempts to get all files from the daemon instantly.
// Returns a DaemonFilesMsg if successful, nil otherwise.
func (m Model) tryDaemonInstantLoad() *DaemonFilesMsg {
	// Check if daemon is running
	pidPath := client.DefaultPIDPath()
	if !client.IsDaemonRunning(pidPath) {
		return nil
	}

	// Try to connect to daemon
	socketPath := client.DefaultSocketPath()
	daemonClient, err := client.ConnectWithContext(m.ctx, socketPath)
	if err != nil {
		return nil
	}
	defer daemonClient.Close()

	// Resolve symlinks in root path to match daemon's indexed paths
	// (e.g., /Volumes/Development -> /Users/user/Development)
	root := m.options.Root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	// Check if index is ready for this path
	ready, err := daemonClient.IsIndexReady(m.ctx, root)
	if err != nil || !ready {
		return nil
	}

	// Query the daemon - get all files at once
	files, err := daemonClient.GetLargeFiles(m.ctx, root, m.options.MinSize, m.options.Exclude, 0)
	if err != nil {
		return nil
	}

	// Get index status for statistics
	var dirsIndexed, filesIndexed int64
	if status, err := daemonClient.GetIndexStatus(m.ctx, root); err == nil && status != nil {
		dirsIndexed = status.DirsIndexed
		filesIndexed = status.FilesIndexed
	}

	return &DaemonFilesMsg{
		Files:        files,
		DirsScanned:  dirsIndexed,
		FilesScanned: filesIndexed,
	}
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

// listenForLogEntries returns a command that waits for log entries.
func (m Model) listenForLogEntries() tea.Cmd {
	logEntryChan := m.logEntryChan
	return func() tea.Msg {
		if logEntryChan == nil {
			return nil
		}
		entry, ok := <-logEntryChan
		if !ok {
			// Channel closed
			return nil
		}
		return LogEntryMsg{Entry: entry}
	}
}

// startLiveWatch starts watching for live file events from the daemon.
func (m Model) startLiveWatch() tea.Cmd {
	ctx := m.ctx
	root := m.options.Root
	minSize := m.options.MinSize
	exclude := m.options.Exclude

	// Resolve symlinks to match daemon's indexed paths
	// (e.g., /Volumes/Development -> /Users/user/Development)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

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
// If a filter is provided, new/modified files are only added if they pass the filter.
func handleLiveFileEvent(resultModel *ResultModel, event client.FileEvent, f *filter.Filter) *Notification {
	const notificationDuration = 3 * time.Second
	now := time.Now()
	expires := now.Add(notificationDuration)

	switch event.Type {
	case "created":
		// Build the file info
		fi := types.FileInfo{
			Path:    event.Path,
			Size:    event.Size,
			ModTime: time.Unix(event.ModTime, 0),
		}
		// Check if it passes the filter
		if f != nil && !f.Match(toFilterFileInfo(fi)) {
			return nil // File doesn't pass filter, skip it
		}
		// Add the new file to results
		resultModel.AddFile(fi)
		return &Notification{
			Type:      NotificationAdded,
			Message:   fmt.Sprintf("%s (%s)", truncateFilename(event.Path, 30), types.FormatSize(event.Size)),
			Expires:   expires,
			CreatedAt: now,
		}

	case "modified":
		// Build the file info for filter check
		fi := types.FileInfo{
			Path:    event.Path,
			Size:    event.Size,
			ModTime: time.Unix(event.ModTime, 0),
		}
		// Check if it passes the filter
		if f != nil && !f.Match(toFilterFileInfo(fi)) {
			// File no longer passes filter, remove it
			resultModel.RemoveFile(event.Path)
			return &Notification{
				Type:      NotificationRemoved,
				Message:   truncateFilename(event.Path, 40) + " (filtered)",
				Expires:   expires,
				CreatedAt: now,
			}
		}
		// Update the file in results
		resultModel.UpdateFile(event.Path, event.Size, time.Unix(event.ModTime, 0))
		return &Notification{
			Type:      NotificationModified,
			Message:   fmt.Sprintf("%s (%s)", truncateFilename(event.Path, 30), types.FormatSize(event.Size)),
			Expires:   expires,
			CreatedAt: now,
		}

	case "deleted":
		// Remove the file from results
		resultModel.RemoveFile(event.Path)
		return &Notification{
			Type:      NotificationRemoved,
			Message:   truncateFilename(event.Path, 40),
			Expires:   expires,
			CreatedAt: now,
		}

	case "renamed":
		// Treat rename as delete - the new name will trigger a create event
		resultModel.RemoveFile(event.Path)
		return &Notification{
			Type:      NotificationRemoved,
			Message:   truncateFilename(event.Path, 40),
			Expires:   expires,
			CreatedAt: now,
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

	logging.Get("tui").Info("delete started",
		"count", m.deleteTotal,
		"size", types.FormatSize(m.lastFreedSize),
		"dryRun", dryRun)

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
	deletedCount := 0
	for _, file := range files {
		if !errorPaths[file.Path] && !m.options.DryRun {
			actualFreedSize += file.Size
			deletedCount++
			m.resultModel.RemoveFile(file.Path)
		}
	}

	logging.Get("tui").Info("delete completed",
		"deleted", deletedCount,
		"freed", types.FormatSize(actualFreedSize),
		"errors", len(m.deleteErrors))

	// Update the freed size (add to any previous freed size)
	currentFreed := m.resultModel.LastFreedSize()
	m.resultModel.SetLastFreedSize(currentFreed + actualFreedSize)

	// Clear selection
	m.resultModel.SelectNone()
}

// filePassesFilter checks if a file passes the configured filter.
// If no filter is configured, it returns true (backward compatibility).
func (m *Model) filePassesFilter(f types.FileInfo) bool {
	if m.options.Filter == nil {
		return true
	}
	return m.options.Filter.Match(toFilterFileInfo(f))
}

// applyFilterToFiles applies the configured filter to a slice of files.
// If no filter is configured, it returns the original slice unchanged.
func (m *Model) applyFilterToFiles(files []types.FileInfo) []types.FileInfo {
	if m.options.Filter == nil {
		return files
	}

	// Convert to filter.FileInfo slice
	filterInfos := make([]filter.FileInfo, len(files))
	for i, f := range files {
		filterInfos[i] = toFilterFileInfo(f)
	}

	// Apply filter
	filtered := m.options.Filter.Apply(filterInfos)

	// Convert back to types.FileInfo slice
	result := make([]types.FileInfo, len(filtered))
	for i, fi := range filtered {
		result[i] = fromFilterFileInfo(fi)
	}

	return result
}

// toFilterFileInfo converts types.FileInfo to filter.FileInfo.
func toFilterFileInfo(f types.FileInfo) filter.FileInfo {
	return filter.FileInfo{
		Path:    f.Path,
		Name:    filepath.Base(f.Path),
		Dir:     filepath.Dir(f.Path),
		Ext:     filepath.Ext(f.Path),
		Size:    f.Size,
		ModTime: f.ModTime,
		Mode:    f.Mode,
		Owner:   f.Owner,
	}
}

// fromFilterFileInfo converts filter.FileInfo back to types.FileInfo.
func fromFilterFileInfo(fi filter.FileInfo) types.FileInfo {
	return types.FileInfo{
		Path:    fi.Path,
		Size:    fi.Size,
		ModTime: fi.ModTime,
		Mode:    fi.Mode,
		Owner:   fi.Owner,
	}
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
