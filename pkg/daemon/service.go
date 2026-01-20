package daemon

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/daemon/broadcaster"
	"github.com/jamesainslie/sweep/pkg/daemon/indexer"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
	"github.com/jamesainslie/sweep/pkg/daemon/watcher"
	"github.com/jamesainslie/sweep/pkg/sweep/filter"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
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
	watcher     *watcher.Watcher
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

// SetWatcher sets the filesystem watcher for the service.
func (s *Service) SetWatcher(w *watcher.Watcher) {
	s.watcher = w
}

// requestToFilter converts a GetLargeFilesRequest to a filter.Filter.
// This allows the daemon to apply server-side filtering using the filter package.
func requestToFilter(req *sweepv1.GetLargeFilesRequest) *filter.Filter {
	var opts []filter.Option

	// Size filter
	if req.GetMinSize() > 0 {
		opts = append(opts, filter.WithMinSize(req.GetMinSize()))
	}

	// Limit
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 1000 // Default limit
	}
	opts = append(opts, filter.WithLimit(limit))

	// Pattern filters
	if len(req.GetInclude()) > 0 {
		opts = append(opts, filter.WithInclude(req.GetInclude()...))
	}
	if len(req.GetExclude()) > 0 {
		opts = append(opts, filter.WithExclude(req.GetExclude()...))
	}

	// Extensions or type groups (type groups take precedence if both specified)
	if len(req.GetTypeGroups()) > 0 {
		opts = append(opts, filter.WithTypeGroups(req.GetTypeGroups()...))
	} else if len(req.GetExtensions()) > 0 {
		opts = append(opts, filter.WithExtensions(req.GetExtensions()...))
	}

	// Age filters
	if req.GetOlderThanSeconds() > 0 {
		opts = append(opts, filter.WithOlderThan(time.Duration(req.GetOlderThanSeconds())*time.Second))
	}
	if req.GetNewerThanSeconds() > 0 {
		opts = append(opts, filter.WithNewerThan(time.Duration(req.GetNewerThanSeconds())*time.Second))
	}

	// Depth control
	if req.GetMaxDepth() > 0 {
		opts = append(opts, filter.WithMaxDepth(int(req.GetMaxDepth())))
	}

	// Sorting
	sortField := protoSortToFilter(req.GetSortBy())
	opts = append(opts, filter.WithSortBy(sortField))
	opts = append(opts, filter.WithSortDescending(req.GetSortDescending()))

	return filter.New(opts...)
}

// protoSortToFilter converts a proto SortField to filter.SortField.
func protoSortToFilter(s sweepv1.SortField) filter.SortField {
	switch s {
	case sweepv1.SortField_SORT_SIZE:
		return filter.SortSize
	case sweepv1.SortField_SORT_MOD_TIME:
		return filter.SortAge
	case sweepv1.SortField_SORT_PATH:
		return filter.SortPath
	default:
		return filter.SortSize
	}
}

// storeEntryToFilterInfo converts a store.Entry to a filter.FileInfo.
func storeEntryToFilterInfo(e *store.Entry, root string) filter.FileInfo {
	// Calculate depth relative to root
	relPath := strings.TrimPrefix(e.Path, root)
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
	depth := 0
	if relPath != "" {
		depth = strings.Count(relPath, string(filepath.Separator)) + 1
	}

	return filter.FileInfo{
		Path:    e.Path,
		Name:    filepath.Base(e.Path),
		Dir:     filepath.Dir(e.Path),
		Ext:     strings.ToLower(filepath.Ext(e.Path)),
		Size:    e.Size,
		ModTime: time.Unix(e.ModTime, 0),
		Depth:   depth,
	}
}

// GetLargeFiles streams large files matching the criteria.
func (s *Service) GetLargeFiles(req *sweepv1.GetLargeFilesRequest, stream grpc.ServerStreamingServer[sweepv1.FileInfo]) error {
	root := req.GetPath()
	minSize := req.GetMinSize()

	// Build the filter from request
	f := requestToFilter(req)

	// Query a larger set from the store to allow for filtering
	// We fetch more than the limit to ensure we have enough after filtering
	fetchLimit := f.Limit * 10
	if fetchLimit < 10000 {
		fetchLimit = 10000 // Minimum fetch to ensure good filtering coverage
	}

	// Query the large files index (populated during indexing or migration)
	entries, err := s.store.GetLargeFiles(root, minSize, fetchLimit)
	if err != nil {
		return err
	}

	// Convert store entries to filter.FileInfo
	fileInfos := make([]filter.FileInfo, 0, len(entries))
	for _, e := range entries {
		fileInfos = append(fileInfos, storeEntryToFilterInfo(e, root))
	}

	// Apply the filter (match, sort, limit)
	filtered := f.Apply(fileInfos)

	// Stream the results
	for _, fi := range filtered {
		info := &sweepv1.FileInfo{
			Path:    fi.Path,
			Size:    fi.Size,
			ModTime: fi.ModTime.Unix(),
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
		// Use cached metadata for fast lookups
		if meta := s.store.GetIndexMeta(reqPath); meta != nil {
			idxStatus.FilesIndexed = meta.Files
			idxStatus.DirsIndexed = meta.Dirs
		}
		// If no metadata, counts will be 0 (old index without metadata)
	default:
		idxStatus.State = sweepv1.IndexState_INDEX_STATE_NOT_INDEXED
	}

	return idxStatus, nil
}

// TriggerIndex starts indexing a path.
func (s *Service) TriggerIndex(_ context.Context, req *sweepv1.TriggerIndexRequest) (*sweepv1.TriggerIndexResponse, error) {
	reqPath := req.GetPath()
	log := logging.Get("daemon")

	s.indexMu.Lock()
	if state, exists := s.indexStates[reqPath]; exists && state.state == sweepv1.IndexState_INDEX_STATE_INDEXING {
		s.indexMu.Unlock()
		log.Debug("index already in progress", "path", reqPath)
		return &sweepv1.TriggerIndexResponse{
			Started: false,
			Message: "already indexing",
		}, nil
	}

	// Clear existing if force
	if req.GetForce() {
		log.Info("force re-index requested, clearing existing data", "path", reqPath)
		_ = s.store.DeletePrefix(reqPath)
	}

	s.indexStates[reqPath] = &indexState{
		state: sweepv1.IndexState_INDEX_STATE_INDEXING,
	}
	s.indexMu.Unlock()

	log.Info("starting index", "path", reqPath)

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
	log := logging.Get("indexer")

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
		log.Error("indexing failed", "path", path, "error", err)
		s.indexStates[path] = &indexState{
			state: sweepv1.IndexState_INDEX_STATE_STALE,
		}
	} else {
		log.Info("indexing complete", "path", path, "files", result.FilesIndexed, "dirs", result.DirsIndexed)
		s.indexStates[path] = &indexState{
			state:    sweepv1.IndexState_INDEX_STATE_READY,
			progress: 1.0,
			files:    result.FilesIndexed,
			dirs:     result.DirsIndexed,
		}
		// Start watching the indexed path for changes
		if s.watcher != nil {
			if watchErr := s.watcher.Watch(path); watchErr != nil {
				log.Warn("failed to start watching indexed path", "path", path, "error", watchErr)
			}
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
	log := logging.Get("daemon")
	var count int64

	if reqPath == "" {
		files, dirs, _ := s.store.CountEntries("")
		count = files + dirs
		_ = s.store.DeletePrefix("")
		log.Info("cleared all cache", "entries", count)
	} else {
		files, dirs, _ := s.store.CountEntries(reqPath)
		count = files + dirs
		_ = s.store.DeletePrefix(reqPath)
		log.Info("cleared cache", "path", reqPath, "entries", count)
	}

	// Stop watching the cleared path
	if s.watcher != nil && reqPath != "" {
		s.watcher.Unwatch(reqPath)
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
