// Package client provides a client for connecting to the sweepd daemon.
// It wraps the gRPC client with convenience methods and type conversions.
package client

import (
	"context"
	"encoding/json"
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
	"github.com/jamesainslie/sweep/pkg/sweep/config"
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

// FileEvent represents a file change event from the daemon.
type FileEvent struct {
	Type    string // "created", "modified", "deleted", "renamed"
	Path    string
	Size    int64
	ModTime int64
}

// TreeEvent represents a tree change event from the daemon.
// It includes ParentPath to enable efficient tree updates.
type TreeEvent struct {
	Type       string // "created", "modified", "deleted"
	Path       string
	Size       int64
	ModTime    int64
	ParentPath string
}

// TreeNode represents a node in the large file tree.
type TreeNode struct {
	Path           string
	Name           string
	IsDir          bool
	Size           int64
	ModTime        int64
	FileType       string
	LargeFileSize  int64
	LargeFileCount int
	Children       []*TreeNode
}

// DefaultSocketPath returns the default Unix socket path for sweepd.
func DefaultSocketPath() string {
	return filepath.Join(xdg.DataHome, "sweep", "sweep.sock")
}

// DefaultPIDPath returns the default PID file path for sweepd.
func DefaultPIDPath() string {
	return filepath.Join(xdg.DataHome, "sweep", "sweep.pid")
}

// DaemonPaths configures paths for daemon operations.
// Empty fields use defaults.
type DaemonPaths struct {
	Binary string // Path to sweepd binary (auto-discovered if empty)
	Socket string // Unix socket path
	PID    string // PID file path
}

// withDefaults returns a copy with empty fields filled with defaults.
func (p DaemonPaths) withDefaults() DaemonPaths {
	if p.Socket == "" {
		p.Socket = DefaultSocketPath()
	}
	if p.PID == "" {
		p.PID = DefaultPIDPath()
	}
	return p
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

// WatchLargeFiles subscribes to file events for large files under a path.
// Returns a channel that receives events until the context is cancelled.
func (c *Client) WatchLargeFiles(ctx context.Context, root string, minSize int64, exclude []string) (<-chan FileEvent, error) {
	req := &sweepv1.WatchRequest{
		Root:    root,
		MinSize: minSize,
		Exclude: exclude,
	}

	stream, err := c.client.WatchLargeFiles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("WatchLargeFiles RPC failed: %w", err)
	}

	events := make(chan FileEvent, 100)
	go func() {
		defer close(events)
		for {
			event, err := stream.Recv()
			if err != nil {
				return // Stream closed or error
			}

			typeStr := eventTypeToString(event.GetType())

			select {
			case events <- FileEvent{
				Type:    typeStr,
				Path:    event.GetPath(),
				Size:    event.GetSize(),
				ModTime: event.GetModTime(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

// eventTypeToString converts a FileEvent_EventType to string.
func eventTypeToString(t sweepv1.FileEvent_EventType) string {
	switch t {
	case sweepv1.FileEvent_CREATED:
		return "created"
	case sweepv1.FileEvent_MODIFIED:
		return "modified"
	case sweepv1.FileEvent_DELETED:
		return "deleted"
	case sweepv1.FileEvent_RENAMED:
		return "renamed"
	default:
		return "unknown"
	}
}

// WatchTree subscribes to tree events for files under a path.
// Returns a channel that receives TreeEvent updates until the context is cancelled.
// TreeEvent includes ParentPath to enable efficient tree updates.
func (c *Client) WatchTree(ctx context.Context, root string, minSize int64) (<-chan TreeEvent, error) {
	stream, err := c.client.WatchTree(ctx, &sweepv1.WatchTreeRequest{
		Root:    root,
		MinSize: minSize,
	})
	if err != nil {
		return nil, fmt.Errorf("WatchTree RPC failed: %w", err)
	}

	events := make(chan TreeEvent, 100)
	go func() {
		defer close(events)
		for {
			event, err := stream.Recv()
			if err != nil {
				return // Stream closed or error
			}

			var eventType string
			switch event.GetType() {
			case sweepv1.TreeEvent_CREATED:
				eventType = "created"
			case sweepv1.TreeEvent_MODIFIED:
				eventType = "modified"
			case sweepv1.TreeEvent_DELETED:
				eventType = "deleted"
			default:
				eventType = "unknown"
			}

			select {
			case events <- TreeEvent{
				Type:       eventType,
				Path:       event.GetPath(),
				Size:       event.GetSize(),
				ModTime:    event.GetModTime(),
				ParentPath: event.GetParentPath(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

// GetTree queries the daemon for a tree view of large files.
func (c *Client) GetTree(ctx context.Context, root string, minSize int64, exclude []string) (*TreeNode, error) {
	req := &sweepv1.GetTreeRequest{
		Root:    root,
		MinSize: minSize,
		Exclude: exclude,
	}

	resp, err := c.client.GetTree(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GetTree RPC failed: %w", err)
	}

	return protoToTreeNode(resp.GetRoot()), nil
}

// protoToTreeNode converts a proto TreeNode to a client TreeNode.
func protoToTreeNode(p *sweepv1.TreeNode) *TreeNode {
	if p == nil {
		return nil
	}

	node := &TreeNode{
		Path:           p.GetPath(),
		Name:           p.GetName(),
		IsDir:          p.GetIsDir(),
		Size:           p.GetSize(),
		ModTime:        p.GetModTime(),
		FileType:       p.GetFileType(),
		LargeFileSize:  p.GetLargeFileSize(),
		LargeFileCount: int(p.GetLargeFileCount()),
	}

	// Convert children recursively
	if len(p.GetChildren()) > 0 {
		node.Children = make([]*TreeNode, 0, len(p.GetChildren()))
		for _, child := range p.GetChildren() {
			node.Children = append(node.Children, protoToTreeNode(child))
		}
	}

	return node
}

// EnsureDaemon ensures the daemon is running, starting it if necessary.
// Idempotent: returns nil if daemon is already running.
func EnsureDaemon(paths DaemonPaths) error {
	return StartDaemon(paths)
}

// StartDaemon starts the sweepd daemon in the background.
// Idempotent: returns nil if daemon is already running.
func StartDaemon(paths DaemonPaths) error {
	paths = paths.withDefaults()

	if IsDaemonRunning(paths.PID) {
		return nil // Already running, nothing to do
	}

	binary, err := resolveBinary(paths.Binary)
	if err != nil {
		return fmt.Errorf("find sweepd: %w", err)
	}

	// Derive status path from socket path
	statusPath := strings.TrimSuffix(paths.Socket, ".sock") + ".status"

	// Clean up stale status file before starting
	_ = os.Remove(statusPath)

	// Use exec.Command (not CommandContext) intentionally: daemon must outlive caller
	cmd := exec.Command(binary) //nolint:gosec // binary path is validated
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Detach so daemon outlives caller
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	// Poll for socket OR status file
	for range 50 {
		time.Sleep(100 * time.Millisecond)

		// Check socket first (success fast path)
		if _, err := os.Stat(paths.Socket); err == nil {
			return nil
		}

		// Check status file for explicit ready or error
		if status, err := readStatusFile(statusPath); err == nil {
			switch status.Status {
			case "ready":
				return nil
			case "error":
				return fmt.Errorf("daemon failed to start: %s", status.Error)
			}
		}
	}

	return errors.New("daemon did not become ready within timeout")
}

// StopDaemon stops the daemon gracefully via RPC.
// Idempotent: returns nil if daemon is not running.
func StopDaemon(paths DaemonPaths) error {
	paths = paths.withDefaults()

	if !IsDaemonRunning(paths.PID) {
		return nil // Not running, nothing to do
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ConnectWithContext(ctx, paths.Socket)
	if err != nil {
		return fmt.Errorf("connect to daemon: %w", err)
	}
	defer client.Close()

	if err := client.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown daemon: %w", err)
	}

	// Wait for daemon to stop
	for range 20 {
		time.Sleep(250 * time.Millisecond)
		if !IsDaemonRunning(paths.PID) {
			return nil
		}
	}

	return errors.New("daemon did not stop within timeout")
}

// RestartDaemon stops and starts the daemon.
func RestartDaemon(paths DaemonPaths) error {
	if err := StopDaemon(paths); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	if err := StartDaemon(paths); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	return nil
}

// resolveBinary finds the sweepd binary path.
// Priority: configured path > same directory as executable > GOBIN/GOPATH > PATH.
func resolveBinary(configured string) (string, error) {
	// Use configured path if provided
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("configured binary not found: %s", configured)
		}
		return configured, nil
	}

	// Try same directory as current executable
	if execPath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "sweepd")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Try standard Go binary locations (GOBIN > GOPATH/bin > $HOME/go/bin)
	if goBinPath := config.DefaultBinaryPath(); goBinPath != "" {
		return goBinPath, nil
	}

	// Try PATH
	if path, err := exec.LookPath("sweepd"); err == nil {
		return path, nil
	}

	return "", errors.New("sweepd not found")
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

// statusFile represents the daemon startup status file.
type statusFile struct {
	Status string `json:"status"`
	PID    int    `json:"pid,omitempty"`
	Error  string `json:"error,omitempty"`
}

// readStatusFile reads and parses the daemon status file.
func readStatusFile(path string) (*statusFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var status statusFile
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
