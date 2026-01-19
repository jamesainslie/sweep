package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
	"github.com/jamesainslie/sweep/pkg/daemon/broadcaster"
	"github.com/jamesainslie/sweep/pkg/daemon/store"
)

// Config holds daemon configuration.
type Config struct {
	SocketPath       string
	DataDir          string
	MinLargeFileSize int64 // Threshold for large files index (0 = use default)
}

// Server is the sweepd gRPC server.
type Server struct {
	cfg         Config
	grpc        *grpc.Server
	listener    net.Listener
	store       *store.Store
	service     *Service
	broadcaster *broadcaster.Broadcaster
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

	// Create broadcaster for file events
	bc := broadcaster.New()

	// Create service with broadcaster and optional config
	svc := NewServiceWithBroadcaster(st, bc)
	if cfg.MinLargeFileSize > 0 {
		svc.indexer.MinLargeFileSize = cfg.MinLargeFileSize
	}

	srv := &Server{
		cfg:         cfg,
		grpc:        grpc.NewServer(),
		listener:    listener,
		store:       st,
		service:     svc,
		broadcaster: bc,
	}

	// Register gRPC service
	sweepv1.RegisterSweepDaemonServer(srv.grpc, svc)

	return srv, nil
}

// Serve starts the gRPC server. Blocks until stopped.
func (s *Server) Serve() error {
	return s.grpc.Serve(s.listener)
}

// Close stops the server and cleans up.
func (s *Server) Close() error {
	s.grpc.GracefulStop()
	if s.broadcaster != nil {
		s.broadcaster.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
	return os.RemoveAll(s.cfg.SocketPath)
}
