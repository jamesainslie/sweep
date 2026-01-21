package daemon_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/daemon"
)

func createTestFiles(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "small.txt"), make([]byte, 100), 0644); err != nil {
		t.Fatalf("failed to create small.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), make([]byte, 10000), 0644); err != nil {
		t.Fatalf("failed to create large.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "huge.dat"), make([]byte, 100000), 0644); err != nil {
		t.Fatalf("failed to create huge.dat: %v", err)
	}

	return root
}

func TestServiceGetLargeFiles(t *testing.T) {
	testDir := createTestFiles(t)
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cfg := daemon.Config{
		SocketPath:       socketPath,
		DataDir:          filepath.Join(tmpDir, "data"),
		MinLargeFileSize: 5000, // Lower threshold for testing
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// Start server in background
	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	// Wait for socket
	time.Sleep(100 * time.Millisecond)

	// Connect client
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	// First trigger indexing
	_, err = client.TriggerIndex(context.Background(), &sweepv1.TriggerIndexRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("TriggerIndex failed: %v", err)
	}

	// Wait for indexing
	time.Sleep(500 * time.Millisecond)

	// Query large files
	stream, err := client.GetLargeFiles(context.Background(), &sweepv1.GetLargeFilesRequest{
		Path:    testDir,
		MinSize: 5000,
	})
	if err != nil {
		t.Fatalf("GetLargeFiles failed: %v", err)
	}

	var files []*sweepv1.FileInfo
	for {
		file, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}
		files = append(files, file)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 large files, got %d", len(files))
	}
}

func TestServiceGetIndexStatus(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	testDir := createTestFiles(t)

	cfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    filepath.Join(tmpDir, "data"),
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	// Initially should be not indexed
	status, err := client.GetIndexStatus(context.Background(), &sweepv1.GetIndexStatusRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("GetIndexStatus failed: %v", err)
	}

	if status.GetState() != sweepv1.IndexState_INDEX_STATE_NOT_INDEXED {
		t.Errorf("Expected NOT_INDEXED state, got %v", status.GetState())
	}

	// Trigger indexing
	_, err = client.TriggerIndex(context.Background(), &sweepv1.TriggerIndexRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("TriggerIndex failed: %v", err)
	}

	// Wait for indexing to complete
	time.Sleep(500 * time.Millisecond)

	// Check status again
	status, err = client.GetIndexStatus(context.Background(), &sweepv1.GetIndexStatusRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("GetIndexStatus failed: %v", err)
	}

	if status.GetState() != sweepv1.IndexState_INDEX_STATE_READY {
		t.Errorf("Expected READY state, got %v", status.GetState())
	}

	if status.GetFilesIndexed() < 3 {
		t.Errorf("Expected at least 3 files indexed, got %d", status.GetFilesIndexed())
	}
}

func TestServiceGetDaemonStatus(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    filepath.Join(tmpDir, "data"),
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	status, err := client.GetDaemonStatus(context.Background(), &sweepv1.GetDaemonStatusRequest{})
	if err != nil {
		t.Fatalf("GetDaemonStatus failed: %v", err)
	}

	if !status.GetRunning() {
		t.Error("Expected daemon to be running")
	}

	if status.GetUptimeSeconds() < 0 {
		t.Error("Expected positive uptime")
	}

	if status.GetMemoryBytes() <= 0 {
		t.Error("Expected positive memory usage")
	}
}

func TestServiceClearCache(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	testDir := createTestFiles(t)

	cfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    filepath.Join(tmpDir, "data"),
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	// Index first
	_, err = client.TriggerIndex(context.Background(), &sweepv1.TriggerIndexRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("TriggerIndex failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify indexed
	status, err := client.GetIndexStatus(context.Background(), &sweepv1.GetIndexStatusRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("GetIndexStatus failed: %v", err)
	}
	if status.GetState() != sweepv1.IndexState_INDEX_STATE_READY {
		t.Fatalf("Expected READY state before clear, got %v", status.GetState())
	}

	// Clear cache
	clearResp, err := client.ClearCache(context.Background(), &sweepv1.ClearCacheRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}

	if !clearResp.GetSuccess() {
		t.Error("Expected ClearCache success")
	}

	if clearResp.GetEntriesCleared() == 0 {
		t.Error("Expected some entries to be cleared")
	}

	// Verify cleared
	status, err = client.GetIndexStatus(context.Background(), &sweepv1.GetIndexStatusRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("GetIndexStatus after clear failed: %v", err)
	}

	if status.GetState() != sweepv1.IndexState_INDEX_STATE_NOT_INDEXED {
		t.Errorf("Expected NOT_INDEXED state after clear, got %v", status.GetState())
	}
}

func TestServiceTriggerIndexAlreadyIndexing(t *testing.T) {
	tmpDir := t.TempDir()
	// Use /tmp for socket to avoid path length limits on macOS
	socketPath := filepath.Join("/tmp", "sweep-test-already-indexing.sock")
	t.Cleanup(func() {
		_ = os.Remove(socketPath)
	})
	testDir := createTestFiles(t)

	cfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    filepath.Join(tmpDir, "data"),
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	// First trigger should start
	resp1, err := client.TriggerIndex(context.Background(), &sweepv1.TriggerIndexRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("First TriggerIndex failed: %v", err)
	}

	if !resp1.GetStarted() {
		t.Error("Expected first trigger to start")
	}

	// Second trigger immediately after should not start (already indexing)
	resp2, err := client.TriggerIndex(context.Background(), &sweepv1.TriggerIndexRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("Second TriggerIndex failed: %v", err)
	}

	if resp2.GetStarted() {
		t.Error("Expected second trigger to not start (already indexing)")
	}
}

func TestServiceShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cfg := daemon.Config{
		SocketPath: socketPath,
		DataDir:    filepath.Join(tmpDir, "data"),
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	resp, err := client.Shutdown(context.Background(), &sweepv1.ShutdownRequest{})
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if !resp.GetSuccess() {
		t.Error("Expected shutdown success")
	}
}

func TestServiceGetTree(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create test directory structure with subdirectories
	testDir := filepath.Join(tmpDir, "testdata")
	if err := os.MkdirAll(filepath.Join(testDir, "subdir"), 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Create files in root and subdir
	if err := os.WriteFile(filepath.Join(testDir, "large.txt"), make([]byte, 10000), 0644); err != nil {
		t.Fatalf("failed to create large.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "subdir", "huge.dat"), make([]byte, 100000), 0644); err != nil {
		t.Fatalf("failed to create subdir/huge.dat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "small.txt"), make([]byte, 100), 0644); err != nil {
		t.Fatalf("failed to create small.txt: %v", err)
	}

	cfg := daemon.Config{
		SocketPath:       socketPath,
		DataDir:          filepath.Join(tmpDir, "data"),
		MinLargeFileSize: 5000, // Lower threshold for testing
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	defer func() {
		_ = srv.Close()
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	client := sweepv1.NewSweepDaemonClient(conn)

	// Trigger indexing first
	_, err = client.TriggerIndex(context.Background(), &sweepv1.TriggerIndexRequest{
		Path: testDir,
	})
	if err != nil {
		t.Fatalf("TriggerIndex failed: %v", err)
	}

	// Wait for indexing
	time.Sleep(500 * time.Millisecond)

	// Get tree
	resp, err := client.GetTree(context.Background(), &sweepv1.GetTreeRequest{
		Root:    testDir,
		MinSize: 5000,
	})
	if err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}

	// Verify response
	if resp.GetRoot() == nil {
		t.Fatal("Expected root node in response")
	}

	root := resp.GetRoot()
	if root.GetPath() != testDir {
		t.Errorf("Expected root path %s, got %s", testDir, root.GetPath())
	}

	if !root.GetIsDir() {
		t.Error("Expected root to be a directory")
	}

	// Should have 2 large files total (large.txt and subdir/huge.dat)
	if resp.GetTotalIndexed() != 2 {
		t.Errorf("Expected 2 total indexed files, got %d", resp.GetTotalIndexed())
	}

	// Root should have children (subdir and large.txt)
	if len(root.GetChildren()) == 0 {
		t.Error("Expected root to have children")
	}

	// Verify aggregate stats on root
	if root.GetLargeFileCount() != 2 {
		t.Errorf("Expected root LargeFileCount=2, got %d", root.GetLargeFileCount())
	}

	expectedSize := int64(10000 + 100000) // large.txt + huge.dat
	if root.GetLargeFileSize() != expectedSize {
		t.Errorf("Expected root LargeFileSize=%d, got %d", expectedSize, root.GetLargeFileSize())
	}
}
