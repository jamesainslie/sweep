package daemon

import (
	"context"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/daemon/broadcaster"
	"github.com/jamesainslie/sweep/pkg/daemon/indexer"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

// indexState tracks the state of an index operation.
type indexState struct {
	state    sweepv1.IndexState
	progress float32
	files    int64
	dirs     int64
	current  string
}

// Service implements the SweepDaemon gRPC service.
type Service struct {
	sweepv1.UnimplementedSweepDaemonServer

	store       *store.Store
	indexer     *indexer.Indexer
	broadcaster *broadcaster.Broadcaster
	startTime   time.Time

	// Track indexing state per path
	indexMu     sync.RWMutex
	indexStates map[string]*indexState
}

// NewService creates a new gRPC service.
func NewService(s *store.Store) *Service {
	return &Service{
		store:       s,
		indexer:     indexer.New(s),
		startTime:   time.Now(),
		indexStates: make(map[string]*indexState),
	}
}

// NewServiceWithBroadcaster creates a new gRPC service with a broadcaster.
func NewServiceWithBroadcaster(s *store.Store, b *broadcaster.Broadcaster) *Service {
	return &Service{
		store:       s,
		indexer:     indexer.New(s),
		broadcaster: b,
		startTime:   time.Now(),
		indexStates: make(map[string]*indexState),
	}
}

// GetLargeFiles streams large files matching the criteria.
func (s *Service) GetLargeFiles(req *sweepv1.GetLargeFilesRequest, stream grpc.ServerStreamingServer[sweepv1.FileInfo]) error {
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 1000 // Default limit
	}

	root := req.GetPath()
	minSize := req.GetMinSize()

	// Check if large files index exists, rebuild if needed (migration)
	if !s.store.HasLargeFilesIndex(root) && s.store.HasIndex(root) {
		// Migrate: rebuild large files index from existing data
		_, _ = s.store.RebuildLargeFilesIndex(root, s.indexer.MinLargeFileSize)
	}

	files, err := s.store.GetLargeFiles(root, minSize, limit)
	if err != nil {
		return err
	}

	for _, f := range files {
		info := &sweepv1.FileInfo{
			Path:    f.Path,
			Size:    f.Size,
			ModTime: f.ModTime,
		}
		if err := stream.Send(info); err != nil {
			return err
		}
	}

	return nil
}

// GetIndexStatus returns the index status for a path.
func (s *Service) GetIndexStatus(_ context.Context, req *sweepv1.GetIndexStatusRequest) (*sweepv1.IndexStatus, error) {
	reqPath := req.GetPath()

	s.indexMu.RLock()
	state, exists := s.indexStates[reqPath]
	s.indexMu.RUnlock()

	idxStatus := &sweepv1.IndexStatus{
		Path: reqPath,
	}

	switch {
	case exists:
		idxStatus.State = state.state
		idxStatus.Progress = state.progress
		idxStatus.FilesIndexed = state.files
		idxStatus.DirsIndexed = state.dirs
	case s.store.HasIndex(reqPath):
		idxStatus.State = sweepv1.IndexState_INDEX_STATE_READY
		files, dirs, _ := s.store.CountEntries(reqPath)
		idxStatus.FilesIndexed = files
		idxStatus.DirsIndexed = dirs
	default:
		idxStatus.State = sweepv1.IndexState_INDEX_STATE_NOT_INDEXED
	}

	return idxStatus, nil
}

// TriggerIndex starts indexing a path.
func (s *Service) TriggerIndex(_ context.Context, req *sweepv1.TriggerIndexRequest) (*sweepv1.TriggerIndexResponse, error) {
	reqPath := req.GetPath()

	s.indexMu.Lock()
	if state, exists := s.indexStates[reqPath]; exists && state.state == sweepv1.IndexState_INDEX_STATE_INDEXING {
		s.indexMu.Unlock()
		return &sweepv1.TriggerIndexResponse{
			Started: false,
			Message: "already indexing",
		}, nil
	}

	// Clear existing if force
	if req.GetForce() {
		_ = s.store.DeletePrefix(reqPath)
	}

	s.indexStates[reqPath] = &indexState{
		state: sweepv1.IndexState_INDEX_STATE_INDEXING,
	}
	s.indexMu.Unlock()

	// Start indexing in background
	// We intentionally use a fresh context because indexing should continue
	// even if the client disconnects from the TriggerIndex RPC call
	go s.runIndexing(context.Background(), reqPath) //nolint:contextcheck // intentionally new context for long-running background task

	return &sweepv1.TriggerIndexResponse{
		Started: true,
		Message: "indexing started",
	}, nil
}

// runIndexing performs the indexing operation in the background.
func (s *Service) runIndexing(ctx context.Context, path string) {
	progress := func(p indexer.Progress) {
		s.indexMu.Lock()
		if state, exists := s.indexStates[path]; exists {
			state.files = p.FilesScanned
			state.dirs = p.DirsScanned
			state.current = p.CurrentPath
		}
		s.indexMu.Unlock()
	}

	result, err := s.indexer.Index(ctx, path, progress)

	s.indexMu.Lock()
	if err != nil {
		s.indexStates[path] = &indexState{
			state: sweepv1.IndexState_INDEX_STATE_STALE,
		}
	} else {
		s.indexStates[path] = &indexState{
			state:    sweepv1.IndexState_INDEX_STATE_READY,
			progress: 1.0,
			files:    result.FilesIndexed,
			dirs:     result.DirsIndexed,
		}
	}
	s.indexMu.Unlock()
}

// WatchIndexProgress streams indexing progress.
func (s *Service) WatchIndexProgress(req *sweepv1.WatchIndexProgressRequest, stream grpc.ServerStreamingServer[sweepv1.IndexProgress]) error {
	reqPath := req.GetPath()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			s.indexMu.RLock()
			state, exists := s.indexStates[reqPath]
			s.indexMu.RUnlock()

			progress := &sweepv1.IndexProgress{
				Path: reqPath,
			}

			if exists {
				progress.State = state.state
				progress.Progress = state.progress
				progress.FilesScanned = state.files
				progress.DirsScanned = state.dirs
				progress.CurrentPath = state.current
			} else {
				progress.State = sweepv1.IndexState_INDEX_STATE_NOT_INDEXED
			}

			if err := stream.Send(progress); err != nil {
				return err
			}

			// Stop streaming if indexing is done
			if progress.GetState() == sweepv1.IndexState_INDEX_STATE_READY ||
				progress.GetState() == sweepv1.IndexState_INDEX_STATE_STALE {
				return nil
			}
		}
	}
}

// GetDaemonStatus returns daemon health information.
func (s *Service) GetDaemonStatus(_ context.Context, _ *sweepv1.GetDaemonStatusRequest) (*sweepv1.DaemonStatus, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	s.indexMu.RLock()
	var watchedPaths []string
	var totalFiles int64
	for path, state := range s.indexStates {
		if state.state == sweepv1.IndexState_INDEX_STATE_READY {
			watchedPaths = append(watchedPaths, path)
			totalFiles += state.files
		}
	}
	s.indexMu.RUnlock()

	return &sweepv1.DaemonStatus{
		Running:           true,
		UptimeSeconds:     int64(time.Since(s.startTime).Seconds()),
		MemoryBytes:       int64(mem.Alloc),
		WatchedPaths:      watchedPaths,
		TotalFilesIndexed: totalFiles,
	}, nil
}

// Shutdown gracefully shuts down the daemon.
func (s *Service) Shutdown(_ context.Context, _ *sweepv1.ShutdownRequest) (*sweepv1.ShutdownResponse, error) {
	return &sweepv1.ShutdownResponse{Success: true}, nil
}

// ClearCache clears the cache for a path.
func (s *Service) ClearCache(_ context.Context, req *sweepv1.ClearCacheRequest) (*sweepv1.ClearCacheResponse, error) {
	reqPath := req.GetPath()
	var count int64

	if reqPath == "" {
		files, dirs, _ := s.store.CountEntries("")
		count = files + dirs
		_ = s.store.DeletePrefix("")
	} else {
		files, dirs, _ := s.store.CountEntries(reqPath)
		count = files + dirs
		_ = s.store.DeletePrefix(reqPath)
	}

	s.indexMu.Lock()
	delete(s.indexStates, reqPath)
	s.indexMu.Unlock()

	return &sweepv1.ClearCacheResponse{
		Success:        true,
		EntriesCleared: count,
	}, nil
}

// WatchLargeFiles streams file system events for large files in real-time.
func (s *Service) WatchLargeFiles(req *sweepv1.WatchRequest, stream grpc.ServerStreamingServer[sweepv1.FileEvent]) error {
	if s.broadcaster == nil {
		return status.Error(codes.Unavailable, "file watching not available")
	}

	sub := s.broadcaster.Subscribe(req.GetRoot(), req.GetMinSize(), req.GetExclude())
	if sub == nil {
		return status.Error(codes.Unavailable, "failed to subscribe")
	}
	defer s.broadcaster.Unsubscribe(sub.ID)

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-sub.Events:
			if !ok {
				return nil
			}
			protoEvent := &sweepv1.FileEvent{
				Type:    sweepv1.FileEvent_EventType(event.Type),
				Path:    event.Path,
				Size:    event.Size,
				ModTime: event.ModTime,
			}
			if err := stream.Send(protoEvent); err != nil {
				return err
			}
		}
	}
}
