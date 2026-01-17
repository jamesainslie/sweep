package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the sweep cache",
	Long: `Commands for managing the sweep metadata cache.

The cache stores file metadata to speed up repeat scans of the same directories.
Cache data is stored in the XDG cache directory (typically ~/.cache/sweep/metadata).`,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all cached data",
	Long:  `Removes all cached metadata. The next scan will perform a full directory traversal.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cachePath := filepath.Join(xdg.CacheHome, "sweep", "metadata")

		// Check if cache exists
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			fmt.Println("Cache is already empty.")
			return nil
		}

		if err := os.RemoveAll(cachePath); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}

		fmt.Println("Cache cleared.")
		return nil
	},
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	Long:  `Displays information about the cache including its location, size, and last modified time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cachePath := filepath.Join(xdg.CacheHome, "sweep", "metadata")

		info, err := os.Stat(cachePath)
		if os.IsNotExist(err) {
			fmt.Println("Cache: empty (no cache file)")
			fmt.Printf("Cache location: %s\n", cachePath)
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to stat cache: %w", err)
		}

		// Get directory size
		var size int64
		var fileCount int
		err = filepath.Walk(cachePath, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				size += info.Size()
				fileCount++
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to calculate cache size: %w", err)
		}

		fmt.Printf("Cache location: %s\n", cachePath)
		fmt.Printf("Cache size: %.2f MB\n", float64(size)/1024/1024)
		fmt.Printf("Cache files: %d\n", fileCount)
		fmt.Printf("Last modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))

		return nil
	},
}

var cachePathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show cache location",
	Long:  `Prints the path to the cache directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		cachePath := filepath.Join(xdg.CacheHome, "sweep", "metadata")
		fmt.Println(cachePath)
	},
}

func init() {
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cachePathCmd)
	rootCmd.AddCommand(cacheCmd)
}
