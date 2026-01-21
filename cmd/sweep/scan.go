package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/jamesainslie/sweep/cmd/sweep/tui"
	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/filter"
	"github.com/jamesainslie/sweep/pkg/sweep/output"
	"github.com/jamesainslie/sweep/pkg/sweep/scanner"
	"github.com/jamesainslie/sweep/pkg/sweep/tuner"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runScan is the main scan command handler.
func runScan(_ *cobra.Command, args []string) error {
	// Determine scan path
	scanPath := "."
	if len(args) > 0 {
		scanPath = args[0]
	} else if defaultPath := viper.GetString("default_path"); defaultPath != "" {
		scanPath = defaultPath
	}

	// Expand ~ in path
	expandedPath, err := config.ExpandPath(scanPath)
	if err != nil {
		return fmt.Errorf("failed to expand path: %w", err)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify path exists and is accessible
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", absPath)
		}
		return fmt.Errorf("cannot access path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Parse minimum size
	minSizeStr := viper.GetString("min_size")
	if minSizeStr == "" {
		minSizeStr = config.DefaultMinSize
	}

	minSize, err := types.ParseSize(minSizeStr)
	if err != nil {
		return fmt.Errorf("invalid minimum size %q: %w", minSizeStr, err)
	}

	// Get worker configuration
	workers := viper.GetInt("workers")

	// Detect system resources
	resources, err := tuner.Detect()
	if err != nil {
		printVerbose("Failed to detect system resources, using defaults: %v", err)
		// Use conservative defaults
		resources = tuner.SystemResources{
			CPUCores:     4,
			TotalRAM:     8 * types.GiB,
			AvailableRAM: 4 * types.GiB,
		}
	}

	// Calculate optimal configuration
	var optConfig tuner.OptimalConfig
	if workers > 0 {
		optConfig = tuner.CalculateWithOverrides(resources, workers)
	} else {
		optConfig = tuner.Calculate(resources)
	}

	printVerbose("System: %d CPUs, %s RAM, %s available",
		resources.CPUCores,
		types.FormatSize(resources.TotalRAM),
		types.FormatSize(resources.AvailableRAM))
	printVerbose("Config: %d dir workers, %d file workers, queue size %d",
		optConfig.DirWorkers, optConfig.FileWorkers, optConfig.DirQueueSize)

	// Get exclusion patterns
	exclude := viper.GetStringSlice("exclude")

	// Build scan options
	opts := types.ScanOptions{
		Root:        absPath,
		MinSize:     minSize,
		Exclude:     exclude,
		DirWorkers:  optConfig.DirWorkers,
		FileWorkers: optConfig.FileWorkers,
	}

	// Determine output mode
	noInteractive := viper.GetBool("no_interactive")
	outFormat := viper.GetString("output")

	// If output format is explicitly set (not default), force non-interactive mode
	if outFormat != "" && outFormat != "pretty" {
		noInteractive = true
	}

	// Run scan
	if noInteractive {
		return runNonInteractiveScan(opts)
	}

	// Interactive TUI mode
	return runInteractiveTUI(opts)
}

// runInteractiveTUI runs the TUI application.
func runInteractiveTUI(opts types.ScanOptions) error {
	dryRun := viper.GetBool("dry_run")
	noDaemon := viper.GetBool("no_daemon")

	// Re-initialize logging for TUI mode (enables log buffer, disables console)
	if err := initTUILogging(); err != nil {
		return fmt.Errorf("failed to initialize TUI logging: %w", err)
	}

	// Build filter from CLI flags
	f, err := buildFilter()
	if err != nil {
		return fmt.Errorf("failed to build filter: %w", err)
	}

	tuiOpts := tui.Options{
		Root:        opts.Root,
		MinSize:     opts.MinSize,
		Exclude:     opts.Exclude,
		DirWorkers:  opts.DirWorkers,
		FileWorkers: opts.FileWorkers,
		DryRun:      dryRun,
		NoDaemon:    noDaemon,
		Filter:      f,
	}

	return tui.Run(tuiOpts)
}

// scanResult holds the results of a scan for internal use.
type scanResult struct {
	Files        []types.FileInfo `json:"files"`
	DirsScanned  int64            `json:"dirs_scanned"`
	FilesScanned int64            `json:"files_scanned"`
	TotalSize    int64            `json:"total_size"`
	Elapsed      time.Duration    `json:"elapsed"`
	Errors       []scanError      `json:"errors,omitempty"`
}

type scanError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// runNonInteractiveScan runs the scan in non-interactive mode.
func runNonInteractiveScan(opts types.ScanOptions) error {
	// Build filter from CLI flags
	f, err := buildFilter()
	if err != nil {
		return fmt.Errorf("failed to build filter: %w", err)
	}

	// Get output formatter
	outFormat := viper.GetString("output")
	if outFormat == "" {
		outFormat = "pretty"
	}

	var formatter output.Formatter
	if outFormat == "template" {
		// Handle custom template format
		tmplStr := viper.GetString("template")
		if tmplStr == "" {
			return fmt.Errorf("--template is required when using -o template")
		}
		formatter = output.NewTemplateFormatter(tmplStr)
	} else {
		formatter, err = output.Get(outFormat)
		if err != nil {
			return fmt.Errorf("unknown output format %q: available formats are %v", outFormat, output.Available())
		}
	}

	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	interrupted := false
	go func() {
		<-sigChan
		printInfo("\nInterrupted, stopping scan...")
		interrupted = true
		cancel()
	}()

	startTime := time.Now()

	// Determine daemon usage
	noDaemon := viper.GetBool("no_daemon")
	forceScn := viper.GetBool("force_scan")
	forceDmn := viper.GetBool("force_daemon")

	// force-scan overrides force-daemon
	if forceScn {
		noDaemon = true
	}

	var internalResult *scanResult
	usedDaemon := false

	// Try daemon first if available
	if !noDaemon {
		internalResult, usedDaemon = tryDaemonScan(ctx, opts, f)
	}

	// Handle force-daemon failure
	if forceDmn && !usedDaemon {
		return fmt.Errorf("daemon unavailable but --force-daemon was specified")
	}

	// Fallback to direct scan if daemon not used
	if !usedDaemon {
		if !getQuiet() {
			printInfo("Scanning %s for files >= %s...", opts.Root, types.FormatSize(opts.MinSize))
		}

		// Run the scan using the fast scanner
		internalResult, err = performScan(ctx, opts)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				printInfo("Scan cancelled")
				return nil
			}
			return fmt.Errorf("scan failed: %w", err)
		}
	}

	elapsed := time.Since(startTime)
	internalResult.Elapsed = elapsed

	// Convert to output.Result and apply filter
	result := convertToOutputResult(internalResult, f, opts.Root, usedDaemon, interrupted)

	// Output results
	var buf bytes.Buffer
	if err := formatter.Format(&buf, result); err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}
	fmt.Print(buf.String())

	return nil
}

// tryDaemonScan attempts to use the daemon for scanning.
// Returns the result and a boolean indicating if the daemon was used.
func tryDaemonScan(ctx context.Context, opts types.ScanOptions, f *filter.Filter) (*scanResult, bool) {
	// Check if daemon is running
	pidPath := client.DefaultPIDPath()
	if !client.IsDaemonRunning(pidPath) {
		printVerbose("Daemon not running, using direct scan")
		return nil, false
	}

	// Try to connect to daemon
	socketPath := client.DefaultSocketPath()
	daemonClient, err := client.ConnectWithContext(ctx, socketPath)
	if err != nil {
		printVerbose("Failed to connect to daemon: %v", err)
		return nil, false
	}
	defer daemonClient.Close()

	// Check if index is ready for this path
	ready, err := daemonClient.IsIndexReady(ctx, opts.Root)
	if err != nil {
		printVerbose("Failed to check index status: %v", err)
		// Trigger indexing in background for next time (uses fresh context)
		go triggerBackgroundIndexing(opts.Root) //nolint:contextcheck // intentionally uses fresh context for background work
		return nil, false
	}

	// Check max-age if specified
	maxAgeStr := viper.GetString("max_age")
	if maxAgeStr != "" {
		maxAgeDur, parseErr := filter.ParseDuration(maxAgeStr)
		if parseErr == nil {
			status, _ := daemonClient.GetIndexStatus(ctx, opts.Root)
			if status != nil && !status.LastUpdated.IsZero() {
				indexAge := time.Since(status.LastUpdated)
				if indexAge > maxAgeDur {
					printVerbose("Index too old (%v > %v), triggering background indexing", indexAge, maxAgeDur)
					go triggerBackgroundIndexing(opts.Root) //nolint:contextcheck // intentionally uses fresh context for background work
					return nil, false
				}
			}
		}
	}

	if !ready {
		printVerbose("Index not ready for %s, triggering background indexing", opts.Root)
		// Trigger indexing in background for next time (uses fresh context)
		go triggerBackgroundIndexing(opts.Root) //nolint:contextcheck // intentionally uses fresh context for background work
		return nil, false
	}

	// Index is ready, query the daemon
	printVerbose("Using daemon index for %s", opts.Root)
	// Pass filter limit to daemon for server-side limiting
	limit := 0
	if f != nil && f.Limit > 0 {
		// Request more than needed since we'll filter client-side
		// The daemon only filters by min-size and exclude patterns
		limit = f.Limit * 10 // Request extra for client-side filtering
		if limit > 10000 {
			limit = 10000 // Cap at reasonable limit
		}
	}
	files, err := daemonClient.GetLargeFiles(ctx, opts.Root, opts.MinSize, opts.Exclude, limit)
	if err != nil {
		printVerbose("Failed to query daemon: %v", err)
		return nil, false
	}

	// Sort files by size (largest first) - should already be sorted but ensure consistency
	sort.Slice(files, func(i, j int) bool {
		return files[i].Size > files[j].Size
	})

	// Calculate total size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}

	// Get index status for statistics
	status, err := daemonClient.GetIndexStatus(ctx, opts.Root)
	if err != nil {
		printVerbose("Failed to get index status: %v", err)
	}

	result := &scanResult{
		Files:        files,
		DirsScanned:  0,
		FilesScanned: 0,
		TotalSize:    totalSize,
	}

	if status != nil {
		result.DirsScanned = status.DirsIndexed
		result.FilesScanned = status.FilesIndexed
	}

	return result, true
}

// triggerBackgroundIndexing triggers indexing in the background.
func triggerBackgroundIndexing(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if daemon is running
	pidPath := client.DefaultPIDPath()
	if !client.IsDaemonRunning(pidPath) {
		return
	}

	socketPath := client.DefaultSocketPath()
	daemonClient, err := client.ConnectWithContext(ctx, socketPath)
	if err != nil {
		return
	}
	defer daemonClient.Close()

	// Trigger indexing (don't wait for completion)
	_ = daemonClient.TriggerIndex(ctx, path, false)
}

// performScan executes the directory scan with the given options using the fast scanner.
func performScan(ctx context.Context, opts types.ScanOptions) (*scanResult, error) {
	// Create scanner with fastwalk-based implementation
	s := scanner.New(scanner.Options{
		Root:        opts.Root,
		MinSize:     opts.MinSize,
		Exclude:     opts.Exclude,
		DirWorkers:  opts.DirWorkers,
		FileWorkers: opts.FileWorkers,
	})

	// Run the scan
	scanRes, err := s.Scan(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to internal result format
	result := &scanResult{
		Files:        scanRes.Files,
		DirsScanned:  scanRes.DirsScanned,
		FilesScanned: scanRes.FilesScanned,
		TotalSize:    scanRes.TotalSize,
		Errors:       make([]scanError, len(scanRes.Errors)),
	}

	for i, e := range scanRes.Errors {
		result.Errors[i] = scanError{
			Path:  e.Path,
			Error: e.Error,
		}
	}

	// Sort files by size (largest first)
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Size > result.Files[j].Size
	})

	return result, nil
}

// convertToOutputResult converts internal scanResult to output.Result and applies the filter.
func convertToOutputResult(r *scanResult, f *filter.Filter, source string, daemonUp, interrupted bool) *output.Result {
	// Convert types.FileInfo to filter.FileInfo for filtering
	filterFiles := make([]filter.FileInfo, len(r.Files))
	for i, file := range r.Files {
		filterFiles[i] = filter.FileInfo{
			Path:    file.Path,
			Name:    filepath.Base(file.Path),
			Dir:     filepath.Dir(file.Path),
			Ext:     filepath.Ext(file.Path),
			Size:    file.Size,
			ModTime: file.ModTime,
			Mode:    file.Mode,
			Owner:   file.Owner,
			Depth:   calculateDepth(file.Path, source),
		}
	}

	// Apply filter (match, sort, limit)
	filtered := f.Apply(filterFiles)

	// Convert to output.FileInfo
	outputFiles := make([]output.FileInfo, len(filtered))
	now := time.Now()
	for i, file := range filtered {
		outputFiles[i] = output.FileInfo{
			Path:      file.Path,
			Name:      file.Name,
			Dir:       file.Dir,
			Ext:       file.Ext,
			Size:      file.Size,
			SizeHuman: types.FormatSize(file.Size),
			ModTime:   file.ModTime,
			Age:       now.Sub(file.ModTime),
			Perms:     file.Mode.Perm().String(),
			Mode:      file.Mode,
			Owner:     file.Owner,
			Depth:     file.Depth,
		}
	}

	// Build warnings from errors
	var warnings []string
	for _, e := range r.Errors {
		warnings = append(warnings, fmt.Sprintf("%s: %s", e.Path, e.Error))
	}

	return &output.Result{
		Files: outputFiles,
		Stats: output.ScanStats{
			DirsScanned:  r.DirsScanned,
			FilesScanned: r.FilesScanned,
			LargeFiles:   int64(len(r.Files)),
			Duration:     r.Elapsed,
		},
		Source:      source,
		DaemonUp:    daemonUp,
		TotalFiles:  len(outputFiles),
		Warnings:    warnings,
		Interrupted: interrupted,
	}
}

// calculateDepth calculates the directory depth relative to the root.
func calculateDepth(path, root string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	// Count path separators
	depth := 0
	for _, c := range rel {
		if c == filepath.Separator {
			depth++
		}
	}
	return depth
}
