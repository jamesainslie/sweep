package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/daemon/broadcaster"
)

// mockWatchStream implements grpc.ServerStreamingServer[sweepv1.FileEvent] for testing.
type mockWatchStream struct {
	grpc.ServerStream
	events []*sweepv1.FileEvent
	ctx    context.Context
}

func (m *mockWatchStream) Send(event *sweepv1.FileEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockWatchStream) Context() context.Context {
	return m.ctx
}

func TestService_WatchLargeFiles(t *testing.T) {
	// Create service with broadcaster
	b := broadcaster.New()
	defer b.Close()

	// Create a service with the broadcaster
	svc := &Service{
		broadcaster: b,
		indexStates: make(map[string]*indexState),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stream := &mockWatchStream{ctx: ctx}
	req := &sweepv1.WatchRequest{
		Root:    "/tmp/test",
		MinSize: 1024,
	}

	// Start watching in goroutine
	done := make(chan error)
	go func() {
		done <- svc.WatchLargeFiles(req, stream)
	}()

	// Give it time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Send an event through the broadcaster
	b.Notify("/tmp/test/big.zip", broadcaster.EventCreated, 2048)

	// Wait for stream to end (context timeout)
	err := <-done
	require.NoError(t, err)

	// Verify event was sent
	require.Len(t, stream.events, 1)
	assert.Equal(t, sweepv1.FileEvent_CREATED, stream.events[0].GetType())
	assert.Equal(t, "/tmp/test/big.zip", stream.events[0].GetPath())
	assert.Equal(t, int64(2048), stream.events[0].GetSize())
}

func TestService_WatchLargeFiles_NoBroadcaster(t *testing.T) {
	// Create service without broadcaster
	svc := &Service{
		broadcaster: nil,
		indexStates: make(map[string]*indexState),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stream := &mockWatchStream{ctx: ctx}
	req := &sweepv1.WatchRequest{
		Root:    "/tmp/test",
		MinSize: 1024,
	}

	err := svc.WatchLargeFiles(req, stream)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file watching not available")
}

func TestService_WatchLargeFiles_MultipleEvents(t *testing.T) {
	b := broadcaster.New()
	defer b.Close()

	svc := &Service{
		broadcaster: b,
		indexStates: make(map[string]*indexState),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	stream := &mockWatchStream{ctx: ctx}
	req := &sweepv1.WatchRequest{
		Root:    "/tmp/test",
		MinSize: 1024,
	}

	done := make(chan error)
	go func() {
		done <- svc.WatchLargeFiles(req, stream)
	}()

	// Give it time to subscribe
	time.Sleep(10 * time.Millisecond)

	// Send multiple events
	b.Notify("/tmp/test/file1.zip", broadcaster.EventCreated, 2048)
	b.Notify("/tmp/test/file2.zip", broadcaster.EventModified, 4096)
	b.Notify("/tmp/test/file3.zip", broadcaster.EventDeleted, 0)

	// Wait for stream to end
	err := <-done
	require.NoError(t, err)

	// Should have received 2 events (created, modified) but not deleted (size 0 < minSize)
	// Actually deleted events don't check size threshold, so should get 3
	require.Len(t, stream.events, 3)
	assert.Equal(t, sweepv1.FileEvent_CREATED, stream.events[0].GetType())
	assert.Equal(t, sweepv1.FileEvent_MODIFIED, stream.events[1].GetType())
	assert.Equal(t, sweepv1.FileEvent_DELETED, stream.events[2].GetType())
}

func TestService_WatchLargeFiles_FiltersBySize(t *testing.T) {
	b := broadcaster.New()
	defer b.Close()

	svc := &Service{
		broadcaster: b,
		indexStates: make(map[string]*indexState),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	stream := &mockWatchStream{ctx: ctx}
	req := &sweepv1.WatchRequest{
		Root:    "/tmp/test",
		MinSize: 5000, // Only files >= 5000 bytes
	}

	done := make(chan error)
	go func() {
		done <- svc.WatchLargeFiles(req, stream)
	}()

	time.Sleep(10 * time.Millisecond)

	// Send events with different sizes
	b.Notify("/tmp/test/small.txt", broadcaster.EventCreated, 100)   // Should be filtered out
	b.Notify("/tmp/test/large.zip", broadcaster.EventCreated, 10000) // Should pass
	b.Notify("/tmp/test/medium.doc", broadcaster.EventCreated, 4000) // Should be filtered out

	err := <-done
	require.NoError(t, err)

	// Only large.zip should have been received
	require.Len(t, stream.events, 1)
	assert.Equal(t, "/tmp/test/large.zip", stream.events[0].GetPath())
}

func TestService_WatchLargeFiles_FiltersByPath(t *testing.T) {
	b := broadcaster.New()
	defer b.Close()

	svc := &Service{
		broadcaster: b,
		indexStates: make(map[string]*indexState),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	stream := &mockWatchStream{ctx: ctx}
	req := &sweepv1.WatchRequest{
		Root:    "/tmp/test/subdir",
		MinSize: 1024,
	}

	done := make(chan error)
	go func() {
		done <- svc.WatchLargeFiles(req, stream)
	}()

	time.Sleep(10 * time.Millisecond)

	// Send events from different paths
	b.Notify("/tmp/test/other/file.zip", broadcaster.EventCreated, 2048)       // Should be filtered (different path)
	b.Notify("/tmp/test/subdir/file.zip", broadcaster.EventCreated, 2048)      // Should pass
	b.Notify("/tmp/test/subdirother/file.zip", broadcaster.EventCreated, 2048) // Should be filtered (not under root)

	err := <-done
	require.NoError(t, err)

	// Only subdir file should have been received
	require.Len(t, stream.events, 1)
	assert.Equal(t, "/tmp/test/subdir/file.zip", stream.events[0].GetPath())
}
