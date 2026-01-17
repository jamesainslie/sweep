package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	sweepv1 "github.com/jamesainslie/sweep/pkg/api/sweep/v1"
)

// Config holds daemon configuration.
type Config struct {
	SocketPath string
	DataDir    string
}

// Server is the sweepd gRPC server.
type Server struct {
	sweepv1.UnimplementedSweepDaemonServer

	cfg      Config
	grpc     *grpc.Server
	listener net.Listener
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

	srv := &Server{
		cfg:      cfg,
		grpc:     grpc.NewServer(),
		listener: listener,
	}

	// Register gRPC service
	sweepv1.RegisterSweepDaemonServer(srv.grpc, srv)

	return srv, nil
}

// Serve starts the gRPC server. Blocks until stopped.
func (s *Server) Serve() error {
	return s.grpc.Serve(s.listener)
}

// Close stops the server and cleans up.
func (s *Server) Close() error {
	s.grpc.GracefulStop()
	return os.RemoveAll(s.cfg.SocketPath)
}
