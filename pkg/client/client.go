// Package client provides a client for connecting to the sweepd daemon.
// It wraps the gRPC client with convenience methods and type conversions.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Client connects to the sweepd daemon via gRPC.
type Client struct {
	conn   *grpc.ClientConn
	client sweepv1.SweepDaemonClient
}

// IndexStatus represents the indexing status of a path.
type IndexStatus struct {
	Path         string
	State        string
	FilesIndexed int64
	DirsIndexed  int64
	TotalSize    int64
	LastUpdated  time.Time
	Progress     float32
}

// DaemonStatus represents the daemon's current status.
type DaemonStatus struct {
	Running           bool
	UptimeSeconds     int64
	MemoryBytes       int64
	WatchedPaths      []string
	CacheSizeBytes    int64
	TotalFilesIndexed int64
}

// DefaultSocketPath returns the default Unix socket path for sweepd.
func DefaultSocketPath() string {
	return filepath.Join(xdg.DataHome, "sweep", "sweep.sock")
}

// DefaultPIDPath returns the default PID file path for sweepd.
func DefaultPIDPath() string {
	return filepath.Join(xdg.DataHome, "sweep", "sweep.pid")
}

// Connect establishes a connection to the sweepd daemon.
// Uses a default timeout of 5 seconds.
func Connect(socketPath string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return ConnectWithContext(ctx, socketPath)
}

// ConnectWithContext establishes a connection to the sweepd daemon with a custom context.
func ConnectWithContext(ctx context.Context, socketPath string) (*Client, error) {
	// Check if socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("daemon socket not found at %s", socketPath)
	}

	target := "unix://" + socketPath

	// Use DialContext with block option to ensure connection is established
	//nolint:staticcheck // grpc.DialContext is deprecated but NewClient doesn't support blocking
	conn, err := grpc.DialContext(
		ctx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}

	return &Client{
		conn:   conn,
		client: sweepv1.NewSweepDaemonClient(conn),
	}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetLargeFiles queries the daemon for files matching the criteria.
// Returns files sorted by size (largest first).
func (c *Client) GetLargeFiles(ctx context.Context, path string, minSize int64, exclude []string, limit int) ([]types.FileInfo, error) {
	req := &sweepv1.GetLargeFilesRequest{
		Path:    path,
		MinSize: minSize,
		Exclude: exclude,
		Limit:   int32(limit),
	}

	stream, err := c.client.GetLargeFiles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GetLargeFiles RPC failed: %w", err)
	}

	var files []types.FileInfo
	for {
		fileInfo, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error receiving file: %w", err)
		}
		files = append(files, protoToFileInfo(fileInfo))
	}

	return files, nil
}

// IsIndexReady checks if the index for the given path is ready for queries.
func (c *Client) IsIndexReady(ctx context.Context, path string) (bool, error) {
	status, err := c.client.GetIndexStatus(ctx, &sweepv1.GetIndexStatusRequest{
		Path: path,
	})
	if err != nil {
		return false, fmt.Errorf("GetIndexStatus RPC failed: %w", err)
	}

	return status.GetState() == sweepv1.IndexState_INDEX_STATE_READY, nil
}

// GetIndexStatus returns the full index status for a path.
func (c *Client) GetIndexStatus(ctx context.Context, path string) (*IndexStatus, error) {
	status, err := c.client.GetIndexStatus(ctx, &sweepv1.GetIndexStatusRequest{
		Path: path,
	})
	if err != nil {
		return nil, fmt.Errorf("GetIndexStatus RPC failed: %w", err)
	}

	return &IndexStatus{
		Path:         status.GetPath(),
		State:        indexStateToString(status.GetState()),
		FilesIndexed: status.GetFilesIndexed(),
		DirsIndexed:  status.GetDirsIndexed(),
		TotalSize:    status.GetTotalSize(),
		LastUpdated:  time.Unix(status.GetLastUpdated(), 0),
		Progress:     status.GetProgress(),
	}, nil
}

// TriggerIndex starts indexing of the specified path.
// If force is true, re-indexes even if already indexed.
func (c *Client) TriggerIndex(ctx context.Context, path string, force bool) error {
	resp, err := c.client.TriggerIndex(ctx, &sweepv1.TriggerIndexRequest{
		Path:  path,
		Force: force,
	})
	if err != nil {
		return fmt.Errorf("TriggerIndex RPC failed: %w", err)
	}

	if !resp.GetStarted() {
		return fmt.Errorf("indexing not started: %s", resp.GetMessage())
	}

	return nil
}

// GetDaemonStatus returns the current status of the daemon.
func (c *Client) GetDaemonStatus(ctx context.Context) (*DaemonStatus, error) {
	status, err := c.client.GetDaemonStatus(ctx, &sweepv1.GetDaemonStatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetDaemonStatus RPC failed: %w", err)
	}

	return &DaemonStatus{
		Running:           status.GetRunning(),
		UptimeSeconds:     status.GetUptimeSeconds(),
		MemoryBytes:       status.GetMemoryBytes(),
		WatchedPaths:      status.GetWatchedPaths(),
		CacheSizeBytes:    status.GetCacheSizeBytes(),
		TotalFilesIndexed: status.GetTotalFilesIndexed(),
	}, nil
}

// Shutdown requests the daemon to shut down gracefully.
func (c *Client) Shutdown(ctx context.Context) error {
	resp, err := c.client.Shutdown(ctx, &sweepv1.ShutdownRequest{})
	if err != nil {
		return fmt.Errorf("Shutdown RPC failed: %w", err)
	}

	if !resp.GetSuccess() {
		return errors.New("shutdown request was not successful")
	}

	return nil
}

// ClearCache clears the cache for the specified path.
// Returns the number of entries cleared.
func (c *Client) ClearCache(ctx context.Context, path string) (int64, error) {
	resp, err := c.client.ClearCache(ctx, &sweepv1.ClearCacheRequest{
		Path: path,
	})
	if err != nil {
		return 0, fmt.Errorf("ClearCache RPC failed: %w", err)
	}

	if !resp.GetSuccess() {
		return 0, errors.New("cache clear was not successful")
	}

	return resp.GetEntriesCleared(), nil
}

// StartDaemon starts the sweepd daemon in the background.
// Returns nil if the daemon started successfully.
func StartDaemon() error {
	pidPath := DefaultPIDPath()

	// Check if already running
	if IsDaemonRunning(pidPath) {
		return fmt.Errorf("daemon is already running")
	}

	// Find the sweepd binary
	sweepdPath, err := findSweepdBinary()
	if err != nil {
		return fmt.Errorf("failed to find sweepd binary: %w", err)
	}

	// Start the daemon in the background using a context (required by linter)
	// We use Background context since daemon should outlive this call
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, sweepdPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Detach from parent process so daemon outlives caller
	if cmd.Process != nil {
		_ = cmd.Process.Release() // Intentionally ignored: we want detachment to proceed regardless
	}

	// Wait for socket to be ready
	socketPath := DefaultSocketPath()
	for range 50 {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(socketPath); err == nil {
			return nil
		}
	}

	return fmt.Errorf("daemon did not start within timeout")
}

// findSweepdBinary attempts to find the sweepd binary.
func findSweepdBinary() (string, error) {
	// First, try in the same directory as the current executable
	execPath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(execPath)
		sweepdPath := filepath.Join(dir, "sweepd")
		if _, err := os.Stat(sweepdPath); err == nil {
			return sweepdPath, nil
		}
	}

	// Try in PATH
	sweepdPath, err := exec.LookPath("sweepd")
	if err == nil {
		return sweepdPath, nil
	}

	return "", fmt.Errorf("sweepd not found in same directory as executable or in PATH")
}

// IsDaemonRunning checks if the daemon is running based on the PID file.
func IsDaemonRunning(pidPath string) bool {
	pid, err := readPIDFile(pidPath)
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// readPIDFile reads a PID from a file.
func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// protoToFileInfo converts a protobuf FileInfo to types.FileInfo.
func protoToFileInfo(p *sweepv1.FileInfo) types.FileInfo {
	return types.FileInfo{
		Path:       p.GetPath(),
		Size:       p.GetSize(),
		ModTime:    time.Unix(p.GetModTime(), 0),
		CreateTime: time.Unix(p.GetCreateTime(), 0),
		Mode:       os.FileMode(p.GetMode()),
		Owner:      p.GetOwner(),
		Group:      p.GetGroup(),
	}
}

// fileInfoToProto converts types.FileInfo to a protobuf FileInfo.
// This is the inverse of protoToFileInfo and is used for testing round-trip conversion.
func fileInfoToProto(f *types.FileInfo) *sweepv1.FileInfo {
	return &sweepv1.FileInfo{
		Path:       f.Path,
		Size:       f.Size,
		ModTime:    f.ModTime.Unix(),
		CreateTime: f.CreateTime.Unix(),
		Mode:       uint32(f.Mode),
		Owner:      f.Owner,
		Group:      f.Group,
	}
}

// indexStateToString converts an IndexState enum to a string.
func indexStateToString(state sweepv1.IndexState) string {
	switch state {
	case sweepv1.IndexState_INDEX_STATE_NOT_INDEXED:
		return "not_indexed"
	case sweepv1.IndexState_INDEX_STATE_INDEXING:
		return "indexing"
	case sweepv1.IndexState_INDEX_STATE_READY:
		return "ready"
	case sweepv1.IndexState_INDEX_STATE_STALE:
		return "stale"
	default:
		return "unknown"
	}
}
