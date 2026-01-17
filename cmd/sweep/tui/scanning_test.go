package tui

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

func TestNewScanModel(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)

	if m.rootPath != "/test/path" {
		t.Errorf("expected root path '/test/path', got %s", m.rootPath)
	}
	if m.minSize != 100*types.MiB {
		t.Errorf("expected min size %d, got %d", 100*types.MiB, m.minSize)
	}
	if m.done {
		t.Error("expected done to be false initially")
	}
	if m.err != nil {
		t.Error("expected err to be nil initially")
	}
}

func TestScanModelSetProgress(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)

	progress := types.ScanProgress{
		DirsScanned:  100,
		FilesScanned: 1000,
		LargeFiles:   50,
		CurrentPath:  "/test/path/current",
		BytesScanned: 1024 * 1024 * 500,
	}

	m.SetProgress(progress)

	if m.progress.DirsScanned != 100 {
		t.Errorf("expected DirsScanned 100, got %d", m.progress.DirsScanned)
	}
	if m.progress.FilesScanned != 1000 {
		t.Errorf("expected FilesScanned 1000, got %d", m.progress.FilesScanned)
	}
	if m.progress.LargeFiles != 50 {
		t.Errorf("expected LargeFiles 50, got %d", m.progress.LargeFiles)
	}
	if m.currentPath != "/test/path/current" {
		t.Errorf("expected currentPath '/test/path/current', got %s", m.currentPath)
	}
}

func TestScanModelSetDone(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)

	// Test done without error
	m.SetDone(nil)
	if !m.done {
		t.Error("expected done to be true")
	}
	if m.err != nil {
		t.Error("expected err to be nil")
	}
}

func TestScanModelSetDoneWithError(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)

	err := &testError{"test error"}
	m.SetDone(err)
	if !m.done {
		t.Error("expected done to be true")
	}
	if m.err == nil {
		t.Error("expected err to be set")
	}
	if m.err.Error() != "test error" {
		t.Errorf("expected error message 'test error', got %s", m.err.Error())
	}
}

func TestScanModelIsDone(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)

	if m.IsDone() {
		t.Error("expected IsDone to be false initially")
	}

	m.SetDone(nil)

	if !m.IsDone() {
		t.Error("expected IsDone to be true after SetDone")
	}
}

func TestScanModelError(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)

	if m.Error() != nil {
		t.Error("expected Error to be nil initially")
	}

	err := &testError{"test error"}
	m.SetDone(err)

	if m.Error() == nil {
		t.Error("expected Error to be set after SetDone")
	}
}

func TestScanModelView(t *testing.T) {
	m := NewScanModel("/test/path", 100*types.MiB)
	m.width = 80
	m.height = 24

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  int
		expected string
	}{
		{0, "0:00"},
		{30, "0:30"},
		{60, "1:00"},
		{90, "1:30"},
		{120, "2:00"},
		{3600, "60:00"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			d := time.Duration(tt.seconds) * time.Second
			result := formatDuration(d)
			if result != tt.expected {
				t.Errorf("formatDuration(%d seconds) = %s, want %s", tt.seconds, result, tt.expected)
			}
		})
	}
}

// Helper type for testing errors
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
