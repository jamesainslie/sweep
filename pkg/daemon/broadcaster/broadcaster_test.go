package broadcaster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcaster_Subscribe(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("/tmp/test", 1024, nil)
	require.NotNil(t, sub)
	assert.NotEmpty(t, sub.ID)
	assert.Equal(t, "/tmp/test", sub.Root)
	assert.Equal(t, int64(1024), sub.MinSize)
}

func TestBroadcaster_Notify_MatchingFile(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("/tmp/test", 1024, nil)

	// Notify about a matching file
	b.Notify("/tmp/test/bigfile.zip", EventDeleted, 2048)

	select {
	case event := <-sub.Events:
		assert.Equal(t, EventDeleted, event.Type)
		assert.Equal(t, "/tmp/test/bigfile.zip", event.Path)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event not received")
	}
}

func TestBroadcaster_Notify_FiltersBySize(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("/tmp/test", 1024, nil)

	// Notify about a file below threshold
	b.Notify("/tmp/test/small.txt", EventCreated, 512)

	select {
	case <-sub.Events:
		t.Fatal("should not receive event for small file")
	case <-time.After(50 * time.Millisecond):
		// Expected - no event
	}
}

func TestBroadcaster_Notify_FiltersByPath(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("/tmp/test", 1024, nil)

	// Notify about a file outside the root
	b.Notify("/other/path/big.zip", EventCreated, 2048)

	select {
	case <-sub.Events:
		t.Fatal("should not receive event for file outside root")
	case <-time.After(50 * time.Millisecond):
		// Expected - no event
	}
}

func TestBroadcaster_Unsubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe("/tmp/test", 1024, nil)
	b.Unsubscribe(sub.ID)

	// Channel should be closed
	_, ok := <-sub.Events
	assert.False(t, ok, "channel should be closed after unsubscribe")
}
