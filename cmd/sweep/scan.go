package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/jamesainslie/sweep/cmd/sweep/tui"
	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/cache"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/scanner"
	"github.com/jamesainslie/sweep/pkg/sweep/tuner"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runScan is the main scan command handler.
func runScan(cmd *cobra.Command, args []string) error {
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

	// Initialize cache if not disabled
	var c *cache.Cache
	noCache := viper.GetBool("no_cache")
	if !noCache {
		cachePath := filepath.Join(xdg.CacheHome, "sweep", "metadata")
		var err error
		c, err = cache.Open(cachePath)
		if err != nil {
			// Log warning, continue without cache
			printVerbose("Warning: cache unavailable: %v", err)
		} else {
			defer c.Close()
			printVerbose("Using cache at %s", cachePath)
		}
	} else {
		printVerbose("Cache disabled via --no-cache flag")
	}

	// Build scan options
	opts := types.ScanOptions{
		Root:        absPath,
		MinSize:     minSize,
		Exclude:     exclude,
		DirWorkers:  optConfig.DirWorkers,
		FileWorkers: optConfig.FileWorkers,
	}

	// Determine output mode
	jsonOutput := viper.GetBool("json")
	noInteractive := viper.GetBool("no_interactive")

	// If JSON output is requested, force non-interactive mode
	if jsonOutput {
		noInteractive = true
	}

	// Run scan
	if noInteractive {
		return runNonInteractiveScan(opts, c, jsonOutput)
	}

	// Interactive TUI mode
	return runInteractiveTUI(opts, c)
}

// runInteractiveTUI runs the TUI application.
func runInteractiveTUI(opts types.ScanOptions, c *cache.Cache) error {
	dryRun := viper.GetBool("dry_run")

	tuiOpts := tui.Options{
		Root:        opts.Root,
		MinSize:     opts.MinSize,
		Exclude:     opts.Exclude,
		DirWorkers:  opts.DirWorkers,
		FileWorkers: opts.FileWorkers,
		DryRun:      dryRun,
		Cache:       c,
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
	CacheHits    int64            `json:"cache_hits"`
	CacheMisses  int64            `json:"cache_misses"`
	Errors       []scanError      `json:"errors,omitempty"`
}

type scanError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// runNonInteractiveScan runs the scan in non-interactive mode.
func runNonInteractiveScan(opts types.ScanOptions, c *cache.Cache, jsonOutput bool) error {
	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		printInfo("\nInterrupted, stopping scan...")
		cancel()
	}()

	startTime := time.Now()

	// Try daemon first if available
	noDaemon := viper.GetBool("no_daemon")
	if !noDaemon {
		result, usedDaemon := tryDaemonScan(ctx, opts, jsonOutput)
		if usedDaemon {
			result.Elapsed = time.Since(startTime)
			if jsonOutput {
				return outputJSON(result)
			}
			return outputTable(result)
		}
	}

	// Fallback to direct scan
	if !jsonOutput && !getQuiet() {
		printInfo("Scanning %s for files >= %s...", opts.Root, types.FormatSize(opts.MinSize))
	}

	// Run the scan using the fast scanner
	result, err := performScan(ctx, opts, c)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			printInfo("Scan cancelled")
			return nil
		}
		return fmt.Errorf("scan failed: %w", err)
	}

	result.Elapsed = time.Since(startTime)

	// Output results
	if jsonOutput {
		return outputJSON(result)
	}
	return outputTable(result)
}

// tryDaemonScan attempts to use the daemon for scanning.
// Returns the result and a boolean indicating if the daemon was used.
func tryDaemonScan(ctx context.Context, opts types.ScanOptions, _ bool) (*scanResult, bool) {
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

	if !ready {
		printVerbose("Index not ready for %s, triggering background indexing", opts.Root)
		// Trigger indexing in background for next time (uses fresh context)
		go triggerBackgroundIndexing(opts.Root) //nolint:contextcheck // intentionally uses fresh context for background work
		return nil, false
	}

	// Index is ready, query the daemon
	printVerbose("Using daemon index for %s", opts.Root)
	files, err := daemonClient.GetLargeFiles(ctx, opts.Root, opts.MinSize, opts.Exclude, 0)
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
	for _, f := range files {
		totalSize += f.Size
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
func performScan(ctx context.Context, opts types.ScanOptions, c *cache.Cache) (*scanResult, error) {
	// Create scanner with fastwalk-based implementation
	s := scanner.New(scanner.Options{
		Root:        opts.Root,
		MinSize:     opts.MinSize,
		Exclude:     opts.Exclude,
		DirWorkers:  opts.DirWorkers,
		FileWorkers: opts.FileWorkers,
		Cache:       c,
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
		CacheHits:    scanRes.CacheHits,
		CacheMisses:  scanRes.CacheMisses,
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

// outputJSON outputs the scan result as JSON.
func outputJSON(result *scanResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputTable outputs the scan result as a formatted table.
func outputTable(result *scanResult) error {
	if len(result.Files) == 0 {
		printInfo("No files found matching criteria.")
		printInfo("\nScanned %d directories, %d files in %v",
			result.DirsScanned, result.FilesScanned, result.Elapsed.Round(time.Millisecond))
		// Print cache metrics if available
		totalCacheOps := result.CacheHits + result.CacheMisses
		if totalCacheOps > 0 {
			hitRate := float64(result.CacheHits) / float64(totalCacheOps) * 100
			printInfo("Cache: %d hits, %d misses (%.1f%% hit rate)",
				result.CacheHits, result.CacheMisses, hitRate)
		}
		return nil
	}

	// Print header
	fmt.Printf("\n%-12s  %s\n", "SIZE", "PATH")
	fmt.Println(strings.Repeat("-", 80))

	// Print files (limit to 50 for readability)
	limit := 50
	if len(result.Files) < limit {
		limit = len(result.Files)
	}

	for i := 0; i < limit; i++ {
		file := result.Files[i]
		fmt.Printf("%-12s  %s\n", types.FormatSize(file.Size), file.Path)
	}

	if len(result.Files) > limit {
		fmt.Printf("\n... and %d more files\n", len(result.Files)-limit)
	}

	// Print summary
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("\nFound %d large files totaling %s\n",
		len(result.Files), types.FormatSize(result.TotalSize))
	fmt.Printf("Scanned %d directories, %d files in %v\n",
		result.DirsScanned, result.FilesScanned, result.Elapsed.Round(time.Millisecond))

	// Print cache metrics if available
	totalCacheOps := result.CacheHits + result.CacheMisses
	if totalCacheOps > 0 {
		hitRate := float64(result.CacheHits) / float64(totalCacheOps) * 100
		fmt.Printf("Cache: %d hits, %d misses (%.1f%% hit rate)\n",
			result.CacheHits, result.CacheMisses, hitRate)
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n%d errors encountered during scan (use --verbose for details)\n", len(result.Errors))
		if getVerbose() {
			fmt.Println("\nErrors:")
			for _, e := range result.Errors {
				fmt.Printf("  %s: %s\n", e.Path, e.Error)
			}
		}
	}

	return nil
}
