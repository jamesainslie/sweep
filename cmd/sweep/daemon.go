package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the sweepd daemon",
	Long: `Manage the sweepd daemon for background indexing and fast queries.

The daemon maintains an index of file metadata for faster queries.
Start it in the background for instant results on repeat scans.`,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the sweepd daemon",
	Long:  `Start the sweepd daemon in the background.`,
	RunE:  runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the sweepd daemon",
	Long:  `Stop the sweepd daemon gracefully.`,
	RunE:  runDaemonStop,
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the sweepd daemon",
	Long:  `Stop and start the sweepd daemon.`,
	RunE:  runDaemonRestart,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	Long:  `Show the current status of the sweepd daemon.`,
	RunE:  runDaemonStatus,
}

var daemonIndexCmd = &cobra.Command{
	Use:   "index [path]",
	Short: "Trigger indexing of a path",
	Long:  `Trigger the daemon to index a specific path.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDaemonIndex,
}

var daemonClearCmd = &cobra.Command{
	Use:   "clear [path]",
	Short: "Clear cache for a path",
	Long:  `Clear the daemon's cache for a specific path, or all caches if no path specified.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDaemonClear,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonIndexCmd)
	daemonCmd.AddCommand(daemonClearCmd)

	// Flags for index command
	daemonIndexCmd.Flags().BoolP("force", "f", false, "Force re-indexing even if already indexed")
}

func runDaemonStart(_ *cobra.Command, _ []string) error {
	printVerbose("starting daemon...")
	if err := client.StartDaemon(); err != nil {
		printVerbose("start failed: %v", err)
		return err
	}
	printVerbose("daemon started successfully")
	printInfo("Daemon started")
	return nil
}

func runDaemonStop(_ *cobra.Command, _ []string) error {
	pidPath := client.DefaultPIDPath()
	socketPath := client.DefaultSocketPath()

	printVerbose("checking PID file: %s", pidPath)
	printVerbose("socket path: %s", socketPath)

	// Check if running
	if !client.IsDaemonRunning(pidPath) {
		printVerbose("daemon not running (PID check failed)")
		return errors.New("daemon is not running")
	}
	printVerbose("daemon is running")

	// Connect and send shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	printVerbose("connecting to daemon...")
	daemonClient, err := client.ConnectWithContext(ctx, socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer daemonClient.Close()
	printVerbose("connected, sending shutdown request...")

	if err := daemonClient.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}
	printVerbose("shutdown request sent, waiting for daemon to stop...")

	// Wait for the daemon to stop
	for i := range 20 {
		time.Sleep(250 * time.Millisecond)
		if !client.IsDaemonRunning(pidPath) {
			printVerbose("daemon stopped after %d checks", i+1)
			printInfo("Daemon stopped")
			return nil
		}
		printVerbose("still running (check %d/20)", i+1)
	}

	return errors.New("daemon did not stop in time")
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	pidPath := client.DefaultPIDPath()

	// Stop if running
	if client.IsDaemonRunning(pidPath) {
		if err := runDaemonStop(cmd, args); err != nil {
			return fmt.Errorf("failed to stop daemon: %w", err)
		}
	}

	// Start daemon
	if err := runDaemonStart(cmd, args); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	return nil
}

func runDaemonStatus(_ *cobra.Command, _ []string) error {
	pidPath := client.DefaultPIDPath()
	socketPath := client.DefaultSocketPath()

	// Check if running
	if !client.IsDaemonRunning(pidPath) {
		printInfo("Daemon status: not running")
		return nil
	}

	// Connect and get status
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	daemonClient, err := client.ConnectWithContext(ctx, socketPath)
	if err != nil {
		printInfo("Daemon status: running (but not responding)")
		return nil
	}
	defer daemonClient.Close()

	status, err := daemonClient.GetDaemonStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get daemon status: %w", err)
	}

	printInfo("Daemon status: running")
	printInfo("  Uptime: %s", formatDuration(time.Duration(status.UptimeSeconds)*time.Second))
	printInfo("  Memory: %s", types.FormatSize(status.MemoryBytes))
	printInfo("  Cache size: %s", types.FormatSize(status.CacheSizeBytes))
	printInfo("  Files indexed: %d", status.TotalFilesIndexed)

	if len(status.WatchedPaths) > 0 {
		printInfo("  Watched paths:")
		for _, p := range status.WatchedPaths {
			printInfo("    - %s", p)
		}
	}

	return nil
}

func runDaemonIndex(cmd *cobra.Command, args []string) error {
	pidPath := client.DefaultPIDPath()
	socketPath := client.DefaultSocketPath()

	// Check if running
	if !client.IsDaemonRunning(pidPath) {
		return errors.New("daemon is not running (start with: sweep daemon start)")
	}

	// Determine path
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Connect and trigger index
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	daemonClient, err := client.ConnectWithContext(ctx, socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer daemonClient.Close()

	force, _ := cmd.Flags().GetBool("force")
	if err := daemonClient.TriggerIndex(ctx, absPath, force); err != nil {
		return fmt.Errorf("failed to trigger indexing: %w", err)
	}

	printInfo("Indexing started for %s", absPath)
	return nil
}

func runDaemonClear(_ *cobra.Command, args []string) error {
	pidPath := client.DefaultPIDPath()
	socketPath := client.DefaultSocketPath()

	// Check if running
	if !client.IsDaemonRunning(pidPath) {
		return errors.New("daemon is not running")
	}

	// Determine path (empty means all)
	path := ""
	if len(args) > 0 {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve path: %w", err)
		}
		path = absPath
	}

	// Connect and clear cache
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	daemonClient, err := client.ConnectWithContext(ctx, socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer daemonClient.Close()

	cleared, err := daemonClient.ClearCache(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	if path == "" {
		printInfo("Cleared all cache entries (%d entries)", cleared)
	} else {
		printInfo("Cleared cache for %s (%d entries)", path, cleared)
	}

	return nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}
