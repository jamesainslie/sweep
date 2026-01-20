package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/adrg/xdg"
	"github.com/jamesainslie/sweep/pkg/daemon"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

func main() {
	// Ensure XDG directories exist
	if err := config.EnsureDataDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data dir: %v\n", err)
		os.Exit(1)
	}
	if err := config.EnsureStateDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create state dir: %v\n", err)
		os.Exit(1)
	}

	// Load config for logging settings
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging
	logPath := cfg.Logging.Path
	if logPath == "" {
		logPath = config.DefaultLogPath()
	}

	// Parse max_size (e.g., "10MB") to bytes
	maxSize := int64(10 * 1024 * 1024) // default 10MB
	if cfg.Logging.Rotation.MaxSize != "" {
		if parsed, parseErr := parseSize(cfg.Logging.Rotation.MaxSize); parseErr == nil {
			maxSize = parsed
		}
	}

	logCfg := logging.Config{
		Level: cfg.Logging.Level,
		Path:  logPath,
		Rotation: logging.RotationConfig{
			MaxSize:    maxSize,
			MaxAge:     cfg.Logging.Rotation.MaxAge,
			MaxBackups: cfg.Logging.Rotation.MaxBackups,
			Daily:      cfg.Logging.Rotation.Daily,
		},
		Components: cfg.Logging.Components,
	}
	if err := logging.Init(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logging.Close()

	log := logging.Get("daemon")

	// Default paths
	dataDir := filepath.Join(xdg.DataHome, "sweep")
	socketPath := filepath.Join(dataDir, "sweep.sock")
	pidPath := filepath.Join(dataDir, "sweep.pid")

	// Check if already running
	if daemon.IsDaemonRunning(pidPath) {
		fmt.Fprintln(os.Stderr, "sweepd is already running")
		os.Exit(1)
	}

	// Create server
	srvCfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    dataDir,
	}

	srv, err := daemon.NewServer(srvCfg)
	if err != nil {
		log.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	// Write PID file
	if err := daemon.WritePIDFile(pidPath); err != nil {
		log.Error("failed to write PID file", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := daemon.RemovePIDFile(pidPath); err != nil {
			log.Warn("failed to remove PID file", "error", err)
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("shutting down")
		if err := srv.Close(); err != nil {
			log.Warn("error during shutdown", "error", err)
		}
	}()

	log.Info("daemon starting", "socket", socketPath)

	// Start serving
	if err := srv.Serve(); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}

// parseSize parses size strings like "10MB" to bytes.
func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = s[:len(s)-2]
	}
	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return val * multiplier, nil
}
