package daemon_test

import (
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon"
)

func TestNewServer(t *testing.T) {
	cfg := daemon.Config{
		SocketPath: "/tmp/sweep-test.sock",
		DataDir:    t.TempDir(),
	}

	srv, err := daemon.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer srv.Close()

	if srv == nil {
		t.Fatal("Expected non-nil server")
	}
}
