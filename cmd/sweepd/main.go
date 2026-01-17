package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/adrg/xdg"
	"github.com/jamesainslie/sweep/pkg/daemon"
)

func main() {
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
	cfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    dataDir,
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Write PID file
	if err := daemon.WritePIDFile(pidPath); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}
	defer func() {
		if err := daemon.RemovePIDFile(pidPath); err != nil {
			log.Printf("Warning: failed to remove PID file: %v", err)
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		if err := srv.Close(); err != nil {
			log.Printf("Warning: error during shutdown: %v", err)
		}
	}()

	log.Printf("sweepd starting on %s", socketPath)

	// Start serving
	if err := srv.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
