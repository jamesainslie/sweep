package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/grpc"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/daemon/broadcaster"
	"github.com/jamesainslie/sweep/pkg/daemon/indexer"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
	"github.com/jamesainslie/sweep/pkg/daemon/watcher"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

// Config holds daemon configuration.
type Config struct {
	SocketPath       string
	DataDir          string
	MinLargeFileSize int64 // Threshold for large files index (0 = use default)
}

// MigrationStatus represents the current migration state.
type MigrationStatus struct {
	Running       bool
	Progress      store.MigrationProgress
	Error         error
	Completed     bool
	MigrationsRun int
}

// Server is the sweepd gRPC server.
type Server struct {
	cfg         Config
	grpc        *grpc.Server
	listener    net.Listener
	store       *store.Store
	service     *Service
	broadcaster *broadcaster.Broadcaster
	watcher     *watcher.Watcher
	watcherCtx  context.Context
	watcherStop context.CancelFunc

	// Migration state
	migrationMu     sync.RWMutex
	migrationStatus MigrationStatus
	migrationCancel context.CancelFunc

	// Shutdown signaling
	shutdownChan chan struct{}
}

// NewServer creates a new daemon server.
func NewServer(cfg Config) (*Server, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, err
	}

	// Remove stale socket if exists
	if err := os.RemoveAll(cfg.SocketPath); err != nil {
		return nil, err
	}

	// Ensure socket directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.SocketPath), 0755); err != nil {
		return nil, err
	}

	// Create Unix socket listener
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "unix", cfg.SocketPath)
	if err != nil {
		return nil, err
	}

	// Open the store
	dbPath := filepath.Join(cfg.DataDir, "index.db")
	st, err := store.Open(dbPath)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	// Determine large file threshold
	largeFileThreshold := cfg.MinLargeFileSize
	if largeFileThreshold == 0 {
		largeFileThreshold = indexer.DefaultMinLargeFileSize
	}

	// Create broadcaster for file events
	bc := broadcaster.New()

	// Create watcher for filesystem events
	w, err := watcher.New(st)
	if err != nil {
		_ = st.Close()
		_ = listener.Close()
		return nil, err
	}
	w.SetBroadcaster(bc)
	w.SetMinLargeFileSize(largeFileThreshold)

	// Create context for watcher goroutine
	watcherCtx, watcherStop := context.WithCancel(context.Background())

	// Create shutdown channel
	shutdownChan := make(chan struct{}, 1)

	// Create service with broadcaster and optional config
	svc := NewServiceWithBroadcaster(st, bc)
	svc.indexer.MinLargeFileSize = largeFileThreshold
	svc.SetWatcher(w)
	svc.SetShutdownChan(shutdownChan)

	srv := &Server{
		cfg:          cfg,
		grpc:         grpc.NewServer(),
		listener:     listener,
		store:        st,
		service:      svc,
		broadcaster:  bc,
		watcher:      w,
		watcherCtx:   watcherCtx,
		watcherStop:  watcherStop,
		shutdownChan: shutdownChan,
	}

	// Register gRPC service
	sweepv1.RegisterSweepDaemonServer(srv.grpc, svc)

	// Start watcher event loop in background
	go srv.watcher.Run(srv.watcherCtx, nil)

	// Check if migration is needed and start it in background
	if st.NeedsMigration() {
		srv.startMigration(largeFileThreshold)
	} else {
		// Mark schema as current if this is a fresh database
		if schema := st.GetSchema(); schema == nil {
			// Fresh database, set schema version
			_ = st.SetSchema(&store.Schema{Version: store.CurrentSchemaVersion})
		}
	}

	return srv, nil
}

// Serve starts the gRPC server. Blocks until stopped.
func (s *Server) Serve() error {
	return s.grpc.Serve(s.listener)
}

// ShutdownChan returns a channel that receives when shutdown is requested via RPC.
func (s *Server) ShutdownChan() <-chan struct{} {
	return s.shutdownChan
}

// Close stops the server and cleans up.
func (s *Server) Close() error {
	// Cancel any running migration
	if s.migrationCancel != nil {
		s.migrationCancel()
	}

	// Stop watcher
	if s.watcherStop != nil {
		s.watcherStop()
	}
	if s.watcher != nil {
		_ = s.watcher.Close()
	}

	s.grpc.GracefulStop()
	if s.broadcaster != nil {
		s.broadcaster.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
	return os.RemoveAll(s.cfg.SocketPath)
}

// GetMigrationStatus returns the current migration status.
func (s *Server) GetMigrationStatus() MigrationStatus {
	s.migrationMu.RLock()
	defer s.migrationMu.RUnlock()
	return s.migrationStatus
}

// IsMigrating returns true if a migration is currently running.
func (s *Server) IsMigrating() bool {
	s.migrationMu.RLock()
	defer s.migrationMu.RUnlock()
	return s.migrationStatus.Running
}

// startMigration starts the migration process in the background.
func (s *Server) startMigration(threshold int64) {
	ctx, cancel := context.WithCancel(context.Background())
	s.migrationCancel = cancel

	s.migrationMu.Lock()
	s.migrationStatus = MigrationStatus{Running: true}
	s.migrationMu.Unlock()

	go func() {
		log := logging.Get("daemon")
		log.Info("starting database migration")

		onProgress := func(p store.MigrationProgress) {
			s.migrationMu.Lock()
			s.migrationStatus.Progress = p
			s.migrationMu.Unlock()

			if p.EntriesTotal > 0 {
				pct := float64(p.EntriesDone) / float64(p.EntriesTotal) * 100
				log.Debug("migration progress",
					"percent", pct,
					"done", p.EntriesDone,
					"total", p.EntriesTotal)
			}
		}

		count, err := s.store.Migrate(ctx, threshold, onProgress)

		s.migrationMu.Lock()
		s.migrationStatus.Running = false
		s.migrationStatus.Completed = true
		s.migrationStatus.MigrationsRun = count
		s.migrationStatus.Error = err
		s.migrationMu.Unlock()

		if err != nil {
			log.Error("migration failed", "error", err)
		} else {
			log.Info("migration completed", "migrations_run", count)
		}
	}()
}
