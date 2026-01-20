package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/logging"
)

// TestInit tests the Init function with various configurations.
// Note: This test cannot run in parallel with other tests that use global state.
func TestInit(t *testing.T) {
	// Create temp dirs before subtests to avoid t.TempDir() in table
	validDir := t.TempDir()
	debugDir := t.TempDir()
	componentsDir := t.TempDir()
	invalidDir := t.TempDir()

	tests := []struct {
		name    string
		cfg     logging.Config
		wantErr bool
	}{
		{
			name: "valid config with defaults",
			cfg: logging.Config{
				Level: "info",
				Path:  filepath.Join(validDir, "test.log"),
			},
			wantErr: false,
		},
		{
			name: "valid config with debug level",
			cfg: logging.Config{
				Level: "debug",
				Path:  filepath.Join(debugDir, "debug.log"),
			},
			wantErr: false,
		},
		{
			name: "valid config with component overrides",
			cfg: logging.Config{
				Level: "info",
				Path:  filepath.Join(componentsDir, "components.log"),
				Components: map[string]string{
					"scanner": "debug",
					"daemon":  "warn",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			cfg: logging.Config{
				Level: "invalid",
				Path:  filepath.Join(invalidDir, "invalid.log"),
			},
			wantErr: true,
		},
		{
			name: "invalid path - directory without write permission",
			cfg: logging.Config{
				Level: "info",
				Path:  "/root/nonexistent/test.log",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: No t.Parallel() - these tests modify global state

			err := logging.Init(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if closeErr := logging.Close(); closeErr != nil {
					t.Errorf("Close() error = %v", closeErr)
				}
			}
		})
	}
}

func TestGet(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	cfg := logging.Config{
		Level: "info",
		Path:  filepath.Join(tempDir, "test.log"),
		Components: map[string]string{
			"scanner": "debug",
			"daemon":  "error",
		},
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		if err := logging.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	tests := []struct {
		name      string
		component string
	}{
		{"get scanner logger", "scanner"},
		{"get daemon logger", "daemon"},
		{"get tui logger", "tui"},
		{"get default logger", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logging.Get(tt.component)
			if logger == nil {
				t.Error("Get() returned nil")
			}
		})
	}
}

func TestLoggerWritesToFile(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "write.log")

	cfg := logging.Config{
		Level: "debug",
		Path:  logPath,
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	logger := logging.Get("test")
	logger.Info("test message", "key", "value")
	logger.Debug("debug message")

	if err := logging.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("log file does not contain expected message, got: %s", content)
	}
}

func TestLogLevels(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "levels.log")

	cfg := logging.Config{
		Level: "warn",
		Path:  logPath,
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	logger := logging.Get("test")
	logger.Debug("debug should not appear")
	logger.Info("info should not appear")
	logger.Warn("warn should appear")
	logger.Error("error should appear")

	if err := logging.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	logContent := string(content)

	if strings.Contains(logContent, "debug should not appear") {
		t.Error("debug message should not appear when level is warn")
	}
	if strings.Contains(logContent, "info should not appear") {
		t.Error("info message should not appear when level is warn")
	}
	if !strings.Contains(logContent, "warn should appear") {
		t.Error("warn message should appear when level is warn")
	}
	if !strings.Contains(logContent, "error should appear") {
		t.Error("error message should appear when level is warn")
	}
}

func TestComponentLevelOverride(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "component.log")

	cfg := logging.Config{
		Level: "error",
		Path:  logPath,
		Components: map[string]string{
			"verbose": "debug",
		},
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	normalLogger := logging.Get("normal")
	verboseLogger := logging.Get("verbose")

	normalLogger.Info("normal info should not appear")
	verboseLogger.Info("verbose info should appear")

	if err := logging.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	logContent := string(content)

	if strings.Contains(logContent, "normal info should not appear") {
		t.Error("normal info message should not appear when default level is error")
	}
	if !strings.Contains(logContent, "verbose info should appear") {
		t.Error("verbose info message should appear when component level is debug")
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "subscribe.log")

	cfg := logging.Config{
		Level: "info",
		Path:  logPath,
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		if err := logging.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	// Subscribe to log events
	ch := logging.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	// Write a log message
	logger := logging.Get("subtest")

	var receivedEntry logging.LogEntry
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case entry := <-ch:
			receivedEntry = entry
		case <-time.After(time.Second):
			t.Error("timeout waiting for log entry")
		}
	}()

	logger.Info("subscription test message")

	wg.Wait()

	if receivedEntry.Message == "" {
		t.Error("did not receive log entry")
	}
	if receivedEntry.Component != "subtest" {
		t.Errorf("expected component 'subtest', got '%s'", receivedEntry.Component)
	}
	if receivedEntry.Level != logging.LevelInfo {
		t.Errorf("expected level Info, got %v", receivedEntry.Level)
	}

	// Unsubscribe
	logging.Unsubscribe(ch)
}

func TestConcurrentWrites(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "concurrent.log")

	cfg := logging.Config{
		Level: "debug",
		Path:  logPath,
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	const numGoroutines = 10
	const numMessages = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			logger := logging.Get("concurrent")
			for j := 0; j < numMessages; j++ {
				logger.Info("message", "goroutine", id, "index", j)
			}
		}(i)
	}

	wg.Wait()

	if err := logging.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Count lines (each message should be on its own line)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	expectedMessages := numGoroutines * numMessages
	if len(lines) != expectedMessages {
		t.Errorf("expected %d log lines, got %d", expectedMessages, len(lines))
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	path := logging.DefaultLogPath()
	if path == "" {
		t.Error("DefaultLogPath() returned empty string")
	}
	if !strings.Contains(path, "sweep") {
		t.Errorf("DefaultLogPath() should contain 'sweep', got: %s", path)
	}
	if !strings.HasSuffix(path, "sweep.log") {
		t.Errorf("DefaultLogPath() should end with 'sweep.log', got: %s", path)
	}
}

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   string
		want    logging.Level
		wantErr bool
	}{
		{"debug level", "debug", logging.LevelDebug, false},
		{"info level", "info", logging.LevelInfo, false},
		{"warn level", "warn", logging.LevelWarn, false},
		{"error level", "error", logging.LevelError, false},
		{"DEBUG uppercase", "DEBUG", logging.LevelDebug, false},
		{"Info mixed case", "Info", logging.LevelInfo, false},
		{"invalid level", "invalid", logging.LevelInfo, true},
		{"empty level", "", logging.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := logging.ParseLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogEntryFields(t *testing.T) {
	t.Parallel()

	entry := logging.LogEntry{
		Time:      time.Now(),
		Level:     logging.LevelInfo,
		Component: "test",
		Message:   "test message",
	}

	if entry.Time.IsZero() {
		t.Error("LogEntry.Time should not be zero")
	}
	if entry.Level != logging.LevelInfo {
		t.Error("LogEntry.Level should be Info")
	}
	if entry.Component != "test" {
		t.Error("LogEntry.Component should be 'test'")
	}
	if entry.Message != "test message" {
		t.Error("LogEntry.Message should be 'test message'")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	// No t.Parallel() - uses global state

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "multi_sub.log")

	cfg := logging.Config{
		Level: "info",
		Path:  logPath,
	}

	if err := logging.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		if err := logging.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	ch1 := logging.Subscribe()
	ch2 := logging.Subscribe()

	var wg sync.WaitGroup
	wg.Add(2)

	received1 := false
	received2 := false

	go func() {
		defer wg.Done()
		select {
		case <-ch1:
			received1 = true
		case <-time.After(time.Second):
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case <-ch2:
			received2 = true
		case <-time.After(time.Second):
		}
	}()

	logger := logging.Get("multitest")
	logger.Info("broadcast message")

	wg.Wait()

	if !received1 {
		t.Error("subscriber 1 did not receive message")
	}
	if !received2 {
		t.Error("subscriber 2 did not receive message")
	}

	logging.Unsubscribe(ch1)
	logging.Unsubscribe(ch2)
}
