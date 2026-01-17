// Package client provides a client for connecting to the sweepd daemon.
package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// mockSweepDaemonServer implements sweepv1.SweepDaemonServer for testing.
type mockSweepDaemonServer struct {
	sweepv1.UnimplementedSweepDaemonServer
	largeFiles    []*sweepv1.FileInfo
	indexStatus   *sweepv1.IndexStatus
	daemonStatus  *sweepv1.DaemonStatus
	triggerResp   *sweepv1.TriggerIndexResponse
	shutdownResp  *sweepv1.ShutdownResponse
	clearResp     *sweepv1.ClearCacheResponse
	shutdownCalls int
}

func (m *mockSweepDaemonServer) GetLargeFiles(_ *sweepv1.GetLargeFilesRequest, stream grpc.ServerStreamingServer[sweepv1.FileInfo]) error {
	for _, f := range m.largeFiles {
		if err := stream.Send(f); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockSweepDaemonServer) GetIndexStatus(_ context.Context, _ *sweepv1.GetIndexStatusRequest) (*sweepv1.IndexStatus, error) {
	if m.indexStatus != nil {
		return m.indexStatus, nil
	}
	return &sweepv1.IndexStatus{
		State: sweepv1.IndexState_INDEX_STATE_NOT_INDEXED,
	}, nil
}

func (m *mockSweepDaemonServer) TriggerIndex(_ context.Context, _ *sweepv1.TriggerIndexRequest) (*sweepv1.TriggerIndexResponse, error) {
	if m.triggerResp != nil {
		return m.triggerResp, nil
	}
	return &sweepv1.TriggerIndexResponse{
		Started: true,
		Message: "Indexing started",
	}, nil
}

func (m *mockSweepDaemonServer) GetDaemonStatus(_ context.Context, _ *sweepv1.GetDaemonStatusRequest) (*sweepv1.DaemonStatus, error) {
	if m.daemonStatus != nil {
		return m.daemonStatus, nil
	}
	return &sweepv1.DaemonStatus{
		Running:       true,
		UptimeSeconds: 100,
	}, nil
}

func (m *mockSweepDaemonServer) Shutdown(_ context.Context, _ *sweepv1.ShutdownRequest) (*sweepv1.ShutdownResponse, error) {
	m.shutdownCalls++
	if m.shutdownResp != nil {
		return m.shutdownResp, nil
	}
	return &sweepv1.ShutdownResponse{Success: true}, nil
}

func (m *mockSweepDaemonServer) ClearCache(_ context.Context, _ *sweepv1.ClearCacheRequest) (*sweepv1.ClearCacheResponse, error) {
	if m.clearResp != nil {
		return m.clearResp, nil
	}
	return &sweepv1.ClearCacheResponse{
		Success:        true,
		EntriesCleared: 10,
	}, nil
}

// setupTestServer creates a test gRPC server on a Unix socket.
func setupTestServer(t *testing.T, mock *mockSweepDaemonServer) (string, func()) {
	t.Helper()

	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("", "sweep-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Create gRPC server
	srv := grpc.NewServer()
	sweepv1.RegisterSweepDaemonServer(srv, mock)

	// Start serving in background
	go func() {
		_ = srv.Serve(listener)
	}()

	cleanup := func() {
		srv.GracefulStop()
		_ = os.RemoveAll(tmpDir)
	}

	return socketPath, cleanup
}

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if path == "" {
		t.Error("DefaultSocketPath() returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultSocketPath() should return absolute path, got %q", path)
	}
}

func TestDefaultPIDPath(t *testing.T) {
	path := DefaultPIDPath()
	if path == "" {
		t.Error("DefaultPIDPath() returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultPIDPath() should return absolute path, got %q", path)
	}
}

func TestConnect(t *testing.T) {
	mock := &mockSweepDaemonServer{}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	// Test successful connection
	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("Connect() returned client with nil conn")
	}
}

func TestConnectInvalidSocket(t *testing.T) {
	_, err := Connect("/nonexistent/path/to/socket.sock")
	if err == nil {
		t.Error("Connect() should fail for nonexistent socket")
	}
}

func TestConnectWithTimeout(t *testing.T) {
	mock := &mockSweepDaemonServer{}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := ConnectWithContext(ctx, socketPath)
	if err != nil {
		t.Fatalf("ConnectWithContext() failed: %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("ConnectWithContext() returned client with nil conn")
	}
}

func TestGetLargeFiles(t *testing.T) {
	mock := &mockSweepDaemonServer{
		largeFiles: []*sweepv1.FileInfo{
			{Path: "/tmp/file1.bin", Size: 1024 * 1024 * 100, ModTime: time.Now().Unix()},
			{Path: "/tmp/file2.bin", Size: 1024 * 1024 * 50, ModTime: time.Now().Unix()},
		},
	}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	files, err := client.GetLargeFiles(ctx, "/tmp", 1024*1024, nil, 0)
	if err != nil {
		t.Fatalf("GetLargeFiles() failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("GetLargeFiles() returned %d files, expected 2", len(files))
	}

	if files[0].Path != "/tmp/file1.bin" {
		t.Errorf("GetLargeFiles() first file path = %q, expected /tmp/file1.bin", files[0].Path)
	}
}

func TestGetLargeFilesEmpty(t *testing.T) {
	mock := &mockSweepDaemonServer{
		largeFiles: []*sweepv1.FileInfo{},
	}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	files, err := client.GetLargeFiles(ctx, "/tmp", 1024*1024, nil, 0)
	if err != nil {
		t.Fatalf("GetLargeFiles() failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("GetLargeFiles() returned %d files, expected 0", len(files))
	}
}

func TestIsIndexReady(t *testing.T) {
	tests := []struct {
		name     string
		status   *sweepv1.IndexStatus
		expected bool
	}{
		{
			name: "ready",
			status: &sweepv1.IndexStatus{
				State: sweepv1.IndexState_INDEX_STATE_READY,
			},
			expected: true,
		},
		{
			name: "indexing",
			status: &sweepv1.IndexStatus{
				State: sweepv1.IndexState_INDEX_STATE_INDEXING,
			},
			expected: false,
		},
		{
			name: "not indexed",
			status: &sweepv1.IndexStatus{
				State: sweepv1.IndexState_INDEX_STATE_NOT_INDEXED,
			},
			expected: false,
		},
		{
			name: "stale",
			status: &sweepv1.IndexStatus{
				State: sweepv1.IndexState_INDEX_STATE_STALE,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSweepDaemonServer{
				indexStatus: tt.status,
			}
			socketPath, cleanup := setupTestServer(t, mock)
			defer cleanup()

			client, err := Connect(socketPath)
			if err != nil {
				t.Fatalf("Connect() failed: %v", err)
			}
			defer client.Close()

			ctx := context.Background()
			ready, err := client.IsIndexReady(ctx, "/tmp")
			if err != nil {
				t.Fatalf("IsIndexReady() failed: %v", err)
			}

			if ready != tt.expected {
				t.Errorf("IsIndexReady() = %v, expected %v", ready, tt.expected)
			}
		})
	}
}

func TestGetIndexStatus(t *testing.T) {
	mock := &mockSweepDaemonServer{
		indexStatus: &sweepv1.IndexStatus{
			Path:         "/tmp",
			State:        sweepv1.IndexState_INDEX_STATE_READY,
			FilesIndexed: 1000,
			DirsIndexed:  50,
			TotalSize:    1024 * 1024 * 500,
			LastUpdated:  time.Now().Unix(),
			Progress:     1.0,
		},
	}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	status, err := client.GetIndexStatus(ctx, "/tmp")
	if err != nil {
		t.Fatalf("GetIndexStatus() failed: %v", err)
	}

	if status.Path != "/tmp" {
		t.Errorf("GetIndexStatus().Path = %q, expected /tmp", status.Path)
	}
	if status.FilesIndexed != 1000 {
		t.Errorf("GetIndexStatus().FilesIndexed = %d, expected 1000", status.FilesIndexed)
	}
}

func TestTriggerIndex(t *testing.T) {
	mock := &mockSweepDaemonServer{
		triggerResp: &sweepv1.TriggerIndexResponse{
			Started: true,
			Message: "Indexing started",
		},
	}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	err = client.TriggerIndex(ctx, "/tmp", false)
	if err != nil {
		t.Fatalf("TriggerIndex() failed: %v", err)
	}
}

func TestTriggerIndexForce(t *testing.T) {
	mock := &mockSweepDaemonServer{}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	err = client.TriggerIndex(ctx, "/tmp", true)
	if err != nil {
		t.Fatalf("TriggerIndex(force=true) failed: %v", err)
	}
}

func TestGetDaemonStatus(t *testing.T) {
	mock := &mockSweepDaemonServer{
		daemonStatus: &sweepv1.DaemonStatus{
			Running:           true,
			UptimeSeconds:     3600,
			MemoryBytes:       1024 * 1024 * 100,
			WatchedPaths:      []string{"/home", "/var"},
			CacheSizeBytes:    1024 * 1024 * 50,
			TotalFilesIndexed: 5000,
		},
	}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	status, err := client.GetDaemonStatus(ctx)
	if err != nil {
		t.Fatalf("GetDaemonStatus() failed: %v", err)
	}

	if !status.Running {
		t.Error("GetDaemonStatus().Running = false, expected true")
	}
	if status.UptimeSeconds != 3600 {
		t.Errorf("GetDaemonStatus().UptimeSeconds = %d, expected 3600", status.UptimeSeconds)
	}
	if len(status.WatchedPaths) != 2 {
		t.Errorf("GetDaemonStatus().WatchedPaths length = %d, expected 2", len(status.WatchedPaths))
	}
}

func TestShutdown(t *testing.T) {
	mock := &mockSweepDaemonServer{}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	err = client.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown() failed: %v", err)
	}

	if mock.shutdownCalls != 1 {
		t.Errorf("Shutdown was called %d times, expected 1", mock.shutdownCalls)
	}
}

func TestClearCache(t *testing.T) {
	mock := &mockSweepDaemonServer{
		clearResp: &sweepv1.ClearCacheResponse{
			Success:        true,
			EntriesCleared: 42,
		},
	}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	cleared, err := client.ClearCache(ctx, "/tmp")
	if err != nil {
		t.Fatalf("ClearCache() failed: %v", err)
	}

	if cleared != 42 {
		t.Errorf("ClearCache() returned %d entries cleared, expected 42", cleared)
	}
}

func TestIsDaemonRunning(t *testing.T) {
	// Create temp dir for PID file
	tmpDir, err := os.MkdirTemp("", "sweep-pid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pidPath := filepath.Join(tmpDir, "sweep.pid")

	// No PID file - should return false
	if IsDaemonRunning(pidPath) {
		t.Error("IsDaemonRunning() returned true with no PID file")
	}

	// Write current process PID (which is definitely running)
	currentPID := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", currentPID)), 0600); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Current process PID should be running
	if !IsDaemonRunning(pidPath) {
		t.Error("IsDaemonRunning() returned false for running process")
	}

	// Write invalid PID (very high number that shouldn't exist)
	if err := os.WriteFile(pidPath, []byte("999999999"), 0600); err != nil {
		t.Fatalf("Failed to write PID file: %v", err)
	}

	// Invalid PID should return false
	if IsDaemonRunning(pidPath) {
		t.Error("IsDaemonRunning() returned true for non-running PID")
	}
}

func TestFileInfoConversion(t *testing.T) {
	now := time.Now()
	protoInfo := &sweepv1.FileInfo{
		Path:       "/tmp/test.bin",
		Size:       1024 * 1024 * 100,
		ModTime:    now.Unix(),
		CreateTime: now.Add(-time.Hour).Unix(),
		Owner:      "user",
		Group:      "staff",
		Mode:       0644,
	}

	fileInfo := protoToFileInfo(protoInfo)

	if fileInfo.Path != protoInfo.GetPath() {
		t.Errorf("Path = %q, expected %q", fileInfo.Path, protoInfo.GetPath())
	}
	if fileInfo.Size != protoInfo.GetSize() {
		t.Errorf("Size = %d, expected %d", fileInfo.Size, protoInfo.GetSize())
	}
	if fileInfo.ModTime.Unix() != protoInfo.GetModTime() {
		t.Errorf("ModTime = %v, expected %v", fileInfo.ModTime.Unix(), protoInfo.GetModTime())
	}
	if fileInfo.Owner != protoInfo.GetOwner() {
		t.Errorf("Owner = %q, expected %q", fileInfo.Owner, protoInfo.GetOwner())
	}
	if fileInfo.Group != protoInfo.GetGroup() {
		t.Errorf("Group = %q, expected %q", fileInfo.Group, protoInfo.GetGroup())
	}
}

func TestClientClose(t *testing.T) {
	mock := &mockSweepDaemonServer{}
	socketPath, cleanup := setupTestServer(t, mock)
	defer cleanup()

	client, err := Connect(socketPath)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Double close should not panic (may error, but shouldn't panic)
	_ = client.Close()
}

// TestTypesConversionRoundTrip verifies that types conversion preserves all fields.
func TestTypesConversionRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := types.FileInfo{
		Path:       "/test/file.dat",
		Size:       12345678,
		ModTime:    now,
		CreateTime: now.Add(-24 * time.Hour),
		Mode:       0755,
		Owner:      "testuser",
		Group:      "testgroup",
	}

	// Convert to proto
	proto := fileInfoToProto(&original)

	// Convert back
	converted := protoToFileInfo(proto)

	if converted.Path != original.Path {
		t.Errorf("Path mismatch: got %q, want %q", converted.Path, original.Path)
	}
	if converted.Size != original.Size {
		t.Errorf("Size mismatch: got %d, want %d", converted.Size, original.Size)
	}
	if converted.ModTime.Unix() != original.ModTime.Unix() {
		t.Errorf("ModTime mismatch: got %v, want %v", converted.ModTime, original.ModTime)
	}
	if converted.Owner != original.Owner {
		t.Errorf("Owner mismatch: got %q, want %q", converted.Owner, original.Owner)
	}
	if converted.Group != original.Group {
		t.Errorf("Group mismatch: got %q, want %q", converted.Group, original.Group)
	}
}

// Compile-time interface check.
var _ io.Closer = (*Client)(nil)
