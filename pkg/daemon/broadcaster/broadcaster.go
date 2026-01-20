// Package broadcaster manages subscribers and distributes file events.
package broadcaster

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// EventType represents the type of file event.
type EventType int

const (
	EventCreated EventType = iota
	EventModified
	EventDeleted
	EventRenamed
)

// FileEvent represents a file system event.
type FileEvent struct {
	Type    EventType
	Path    string
	Size    int64
	ModTime int64
}

// Subscriber represents a client subscribed to file events.
type Subscriber struct {
	ID      string
	Root    string
	MinSize int64
	Exclude []string
	Events  chan *FileEvent
}

// Broadcaster manages subscribers and distributes file events.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber
	closed      bool
}

// New creates a new Broadcaster.
func New() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]*Subscriber),
	}
}

// Subscribe creates a new subscription for file events.
func (b *Broadcaster) Subscribe(root string, minSize int64, exclude []string) *Subscriber {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	sub := &Subscriber{
		ID:      uuid.New().String(),
		Root:    root,
		MinSize: minSize,
		Exclude: exclude,
		Events:  make(chan *FileEvent, 100),
	}

	b.subscribers[sub.ID] = sub
	return sub
}

// Unsubscribe removes a subscription.
func (b *Broadcaster) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sub, ok := b.subscribers[id]; ok {
		close(sub.Events)
		delete(b.subscribers, id)
	}
}

// Notify sends an event to all matching subscribers.
func (b *Broadcaster) Notify(path string, eventType EventType, size int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, sub := range b.subscribers {
		if b.matches(sub, path, size) {
			event := &FileEvent{
				Type: eventType,
				Path: path,
				Size: size,
			}
			select {
			case sub.Events <- event:
			default:
				// Channel full, event dropped
			}
		}
	}
}

// matches checks if an event matches a subscriber's filters.
func (b *Broadcaster) matches(sub *Subscriber, path string, size int64) bool {
	// Check path is under root
	if !strings.HasPrefix(path, sub.Root) {
		return false
	}
	// Ensure it's actually under the root (not just a prefix match)
	if len(path) > len(sub.Root) && path[len(sub.Root)] != filepath.Separator {
		return false
	}

	// Check size threshold (skip for deletions - size might be 0)
	if size > 0 && size < sub.MinSize {
		return false
	}

	// Check exclusions
	for _, pattern := range sub.Exclude {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return false
		}
	}

	return true
}

// Close closes the broadcaster and all subscriptions.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for _, sub := range b.subscribers {
		close(sub.Events)
	}
	b.subscribers = make(map[string]*Subscriber)
}

// SubscriberCount returns the number of active subscribers.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
