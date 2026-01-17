package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jamesainslie/sweep/cmd/sweep/tui"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
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
		return runNonInteractiveScan(opts, jsonOutput)
	}

	// Interactive TUI mode
	return runInteractiveTUI(opts)
}

// runInteractiveTUI runs the TUI application.
func runInteractiveTUI(opts types.ScanOptions) error {
	dryRun := viper.GetBool("dry_run")

	tuiOpts := tui.Options{
		Root:        opts.Root,
		MinSize:     opts.MinSize,
		Exclude:     opts.Exclude,
		DirWorkers:  opts.DirWorkers,
		FileWorkers: opts.FileWorkers,
		DryRun:      dryRun,
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
func runNonInteractiveScan(opts types.ScanOptions, jsonOutput bool) error {
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

	if !jsonOutput && !getQuiet() {
		printInfo("Scanning %s for files >= %s...", opts.Root, types.FormatSize(opts.MinSize))
	}

	startTime := time.Now()

	// Run the scan
	result, err := performScan(ctx, opts)
	if err != nil {
		if ctx.Err() == context.Canceled {
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

// performScan executes the directory scan with the given options.
func performScan(ctx context.Context, opts types.ScanOptions) (*scanResult, error) {
	result := &scanResult{
		Files:  []types.FileInfo{},
		Errors: []scanError{},
	}

	var dirsScanned, filesScanned int64
	var mu sync.Mutex
	var files []types.FileInfo
	var scanErrors []scanError

	// Create a channel for work items
	workChan := make(chan string, opts.DirWorkers*100)
	resultChan := make(chan types.FileInfo, opts.FileWorkers*100)
	errorChan := make(chan scanError, 100)
	doneChan := make(chan struct{})

	// Start result collector
	go func() {
		for {
			select {
			case file, ok := <-resultChan:
				if !ok {
					return
				}
				mu.Lock()
				files = append(files, file)
				mu.Unlock()
			case err, ok := <-errorChan:
				if !ok {
					return
				}
				mu.Lock()
				scanErrors = append(scanErrors, err)
				mu.Unlock()
			case <-doneChan:
				return
			}
		}
	}()

	// Compile exclusion patterns
	excludePatterns := make([]string, len(opts.Exclude))
	copy(excludePatterns, opts.Exclude)

	// Walk the directory tree
	err := filepath.WalkDir(opts.Root, func(path string, d fs.DirEntry, err error) error {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			errorChan <- scanError{Path: path, Error: err.Error()}
			return nil // Continue walking despite errors
		}

		// Check exclusions
		for _, pattern := range excludePatterns {
			if matched, _ := filepath.Match(pattern, path); matched {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Also check if path contains the pattern (for absolute paths like /proc)
			if strings.HasPrefix(path, pattern) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			atomic.AddInt64(&dirsScanned, 1)
			return nil
		}

		atomic.AddInt64(&filesScanned, 1)

		// Get file info
		info, err := d.Info()
		if err != nil {
			errorChan <- scanError{Path: path, Error: err.Error()}
			return nil
		}

		// Check size threshold
		if info.Size() >= opts.MinSize {
			fileInfo := types.FileInfo{
				Path:    path,
				Size:    info.Size(),
				ModTime: info.ModTime(),
				Mode:    info.Mode(),
			}
			resultChan <- fileInfo
		}

		return nil
	})

	// Close channels and wait for collector
	close(workChan)
	close(resultChan)
	close(errorChan)
	close(doneChan)

	if err != nil && err != context.Canceled {
		return nil, err
	}

	// Sort files by size (largest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Size > files[j].Size
	})

	// Calculate total size
	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}

	result.Files = files
	result.DirsScanned = atomic.LoadInt64(&dirsScanned)
	result.FilesScanned = atomic.LoadInt64(&filesScanned)
	result.TotalSize = totalSize
	result.Errors = scanErrors

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
