// Package logging provides a unified logging system with rotation support
// for the sweep disk analyzer. Both CLI/TUI and daemon share this package.
//
// Basic usage:
//
//	cfg := logging.Config{
//	    Level: "info",
//	    Path:  logging.DefaultLogPath(),
//	}
//	if err := logging.Init(cfg); err != nil {
//	    log.Fatal(err)
//	}
//	defer logging.Close()
//
//	logger := logging.Get("scanner")
//	logger.Info("scan started", "path", "/home/user")
package logging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/charmbracelet/log"
)

// Level represents a logging level.
type Level int

// Log levels from least to most severe.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// toCharmLevel converts our Level to charmbracelet/log level.
func (l Level) toCharmLevel() log.Level {
	switch l {
	case LevelDebug:
		return log.DebugLevel
	case LevelInfo:
		return log.InfoLevel
	case LevelWarn:
		return log.WarnLevel
	case LevelError:
		return log.ErrorLevel
	default:
		return log.InfoLevel
	}
}

// ErrInvalidLevel is returned when an invalid log level string is provided.
var ErrInvalidLevel = errors.New("invalid log level")

// ParseLevel parses a string into a Level.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("%w: %s", ErrInvalidLevel, s)
	}
}

// Config configures the logging system.
type Config struct {
	// Level is the default log level (debug, info, warn, error).
	Level string

	// Path is the log file path. Empty uses DefaultLogPath().
	Path string

	// Rotation configures log file rotation.
	Rotation RotationConfig

	// Components maps component names to their log levels.
	// This allows per-component log level overrides.
	Components map[string]string
}

// LogEntry represents a single log entry for TUI subscription.
type LogEntry struct {
	// Time is when the log entry was created.
	Time time.Time

	// Level is the severity level.
	Level Level

	// Component is the logger component name.
	Component string

	// Message is the log message.
	Message string
}

// Logger wraps charmbracelet/log with component identification.
type Logger struct {
	inner     *log.Logger
	component string
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

// log is the internal logging method that broadcasts to subscribers.
func (l *Logger) log(level Level, msg string, args ...interface{}) {
	// Log to the underlying logger
	switch level {
	case LevelDebug:
		l.inner.Debug(msg, args...)
	case LevelInfo:
		l.inner.Info(msg, args...)
	case LevelWarn:
		l.inner.Warn(msg, args...)
	case LevelError:
		l.inner.Error(msg, args...)
	}

	// Broadcast to subscribers
	globalState.broadcast(LogEntry{
		Time:      time.Now(),
		Level:     level,
		Component: l.component,
		Message:   msg,
	})
}

// With returns a new logger with additional context.
func (l *Logger) With(args ...interface{}) *Logger {
	return &Logger{
		inner:     l.inner.With(args...),
		component: l.component,
	}
}

// state holds the global logging state.
type state struct {
	mu          sync.RWMutex
	initialized bool
	writer      *RotatingWriter
	level       Level
	components  map[string]Level
	loggers     map[string]*Logger
	subscribers map[chan LogEntry]struct{}
}

var globalState = &state{
	loggers:     make(map[string]*Logger),
	components:  make(map[string]Level),
	subscribers: make(map[chan LogEntry]struct{}),
}

// Init initializes the logging system with the given configuration.
// It must be called before any logging operations.
func Init(cfg Config) error {
	globalState.mu.Lock()
	defer globalState.mu.Unlock()

	// Close any existing state
	if globalState.initialized {
		if globalState.writer != nil {
			if err := globalState.writer.Close(); err != nil {
				return fmt.Errorf("closing existing writer: %w", err)
			}
		}
		globalState.loggers = make(map[string]*Logger)
		globalState.components = make(map[string]Level)
	}

	// Parse default level
	level, err := ParseLevel(cfg.Level)
	if err != nil {
		return fmt.Errorf("parsing log level: %w", err)
	}
	globalState.level = level

	// Parse component levels
	for comp, lvl := range cfg.Components {
		parsedLevel, err := ParseLevel(lvl)
		if err != nil {
			return fmt.Errorf("parsing level for component %s: %w", comp, err)
		}
		globalState.components[comp] = parsedLevel
	}

	// Determine log path
	path := cfg.Path
	if path == "" {
		path = DefaultLogPath()
	}

	// Create rotating writer
	writer, err := NewRotatingWriter(path, cfg.Rotation)
	if err != nil {
		return fmt.Errorf("creating log writer: %w", err)
	}
	globalState.writer = writer

	globalState.initialized = true
	return nil
}

// Get returns a logger for the given component.
// If the component has a level override in the config, it uses that level.
// Otherwise, it uses the default level.
func Get(component string) *Logger {
	globalState.mu.RLock()
	if logger, ok := globalState.loggers[component]; ok {
		globalState.mu.RUnlock()
		return logger
	}
	globalState.mu.RUnlock()

	globalState.mu.Lock()
	defer globalState.mu.Unlock()

	// Double-check after acquiring write lock
	if logger, ok := globalState.loggers[component]; ok {
		return logger
	}

	// Determine log level for this component
	level := globalState.level
	if compLevel, ok := globalState.components[component]; ok {
		level = compLevel
	}

	// Create the underlying charmbracelet logger
	var writer io.Writer = os.Stderr
	if globalState.writer != nil {
		writer = globalState.writer
	}

	inner := log.NewWithOptions(writer, log.Options{
		Level:           level.toCharmLevel(),
		ReportCaller:    false,
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
		Prefix:          component,
	})

	logger := &Logger{
		inner:     inner,
		component: component,
	}

	globalState.loggers[component] = logger
	return logger
}

// Close flushes and closes the log file.
// It should be called when the application exits.
func Close() error {
	globalState.mu.Lock()
	defer globalState.mu.Unlock()

	if !globalState.initialized {
		return nil
	}

	// Close all subscriber channels
	for ch := range globalState.subscribers {
		close(ch)
		delete(globalState.subscribers, ch)
	}

	if globalState.writer != nil {
		if err := globalState.writer.Close(); err != nil {
			return fmt.Errorf("closing log writer: %w", err)
		}
		globalState.writer = nil
	}

	globalState.initialized = false
	globalState.loggers = make(map[string]*Logger)
	globalState.components = make(map[string]Level)

	return nil
}

// Subscribe returns a channel that receives log entries.
// The TUI uses this to display real-time log updates.
// The channel is buffered to prevent blocking the logging goroutine.
func Subscribe() <-chan LogEntry {
	globalState.mu.Lock()
	defer globalState.mu.Unlock()

	ch := make(chan LogEntry, 100)
	globalState.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscription channel.
// It should be called when the TUI no longer needs log updates.
func Unsubscribe(ch <-chan LogEntry) {
	globalState.mu.Lock()
	defer globalState.mu.Unlock()

	// Find the matching channel (we need to cast to the bidirectional type)
	for subCh := range globalState.subscribers {
		if subCh == ch {
			delete(globalState.subscribers, subCh)
			// Don't close the channel here - the caller should drain it
			return
		}
	}
}

// broadcast sends a log entry to all subscribers.
func (s *state) broadcast(entry LogEntry) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- entry:
		default:
			// Drop message if channel is full to prevent blocking
		}
	}
}

// DefaultLogPath returns the default log file path.
// It uses $XDG_STATE_HOME/sweep/sweep.log.
func DefaultLogPath() string {
	return filepath.Join(xdg.StateHome, "sweep", "sweep.log")
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Level:    "info",
		Path:     DefaultLogPath(),
		Rotation: DefaultRotationConfig(),
	}
}
