package main

import (
	"fmt"
	"strings"

	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/manifest"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View operation history",
	Long: `View the history of scan and delete operations.

The manifest stores a record of all operations performed by sweep,
including which files were scanned or deleted.`,
	RunE: runHistory,
}

var historyShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show details of a specific operation",
	Long:  `Display detailed information about a specific operation by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runHistoryShow,
}

var historyCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up old history entries",
	Long:  `Remove history entries older than the retention period.`,
	RunE:  runHistoryClean,
}

var (
	historyLimit int
)

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "l", 20, "maximum number of entries to show")

	historyCmd.AddCommand(historyShowCmd)
	historyCmd.AddCommand(historyCleanCmd)
	rootCmd.AddCommand(historyCmd)
}

// getManifest returns a manifest instance with the configured directory.
func getManifest() (*manifest.Manifest, error) {
	cfg, err := config.Load()
	if err != nil {
		// Use default manifest path if config fails to load
		manifestDir, dirErr := config.ManifestDir()
		if dirErr != nil {
			return nil, fmt.Errorf("failed to get manifest directory: %w", dirErr)
		}
		return manifest.New(manifestDir)
	}

	return manifest.New(cfg.Manifest.Path)
}

// runHistory lists recent operations.
func runHistory(cmd *cobra.Command, args []string) error {
	m, err := getManifest()
	if err != nil {
		return fmt.Errorf("failed to initialize manifest: %w", err)
	}

	entries, err := m.List(historyLimit)
	if err != nil {
		return fmt.Errorf("failed to list history: %w", err)
	}

	if len(entries) == 0 {
		printInfo("No history entries found.")
		printInfo("Run 'sweep [path]' to scan for large files.")
		return nil
	}

	// Print header
	fmt.Printf("\n%-40s  %-8s  %-10s  %-12s\n", "ID", "TYPE", "FILES", "SIZE")
	fmt.Println(strings.Repeat("-", 80))

	for _, entry := range entries {
		fmt.Printf("%-40s  %-8s  %-10d  %-12s\n",
			truncateString(entry.ID, 40),
			entry.Operation,
			entry.Summary.TotalFiles,
			types.FormatSize(entry.Summary.TotalBytes),
		)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("\nShowing %d of %d entries. Use --limit to see more.\n", len(entries), len(entries))
	fmt.Println("Use 'sweep history show <id>' for details on a specific entry.")

	return nil
}

// runHistoryShow displays details of a specific operation.
func runHistoryShow(cmd *cobra.Command, args []string) error {
	id := args[0]

	m, err := getManifest()
	if err != nil {
		return fmt.Errorf("failed to initialize manifest: %w", err)
	}

	entry, err := m.Get(id)
	if err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	// Display entry details
	fmt.Println("\nOperation Details")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("ID:         %s\n", entry.ID)
	fmt.Printf("Timestamp:  %s\n", entry.Timestamp.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Operation:  %s\n", entry.Operation)
	fmt.Printf("Files:      %d\n", entry.Summary.TotalFiles)
	fmt.Printf("Total Size: %s\n", types.FormatSize(entry.Summary.TotalBytes))

	if len(entry.Files) > 0 {
		fmt.Println("\nFiles:")
		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("%-12s  %s\n", "SIZE", "PATH")
		fmt.Println(strings.Repeat("-", 60))

		// Limit display to 50 files
		limit := 50
		if len(entry.Files) < limit {
			limit = len(entry.Files)
		}

		for i := 0; i < limit; i++ {
			file := entry.Files[i]
			fmt.Printf("%-12s  %s\n", types.FormatSize(file.Size), file.Path)
		}

		if len(entry.Files) > limit {
			fmt.Printf("\n... and %d more files\n", len(entry.Files)-limit)
		}
	}

	return nil
}

// runHistoryClean removes old history entries.
func runHistoryClean(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	m, err := manifest.New(cfg.Manifest.Path)
	if err != nil {
		return fmt.Errorf("failed to initialize manifest: %w", err)
	}

	retentionDays := cfg.Manifest.RetentionDays
	if retentionDays <= 0 {
		retentionDays = config.DefaultRetentionDays
	}

	printInfo("Cleaning history entries older than %d days...", retentionDays)

	if err := m.Cleanup(retentionDays); err != nil {
		return fmt.Errorf("failed to clean history: %w", err)
	}

	printInfo("History cleanup complete.")
	return nil
}

// truncateString truncates a string to maxLen, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
