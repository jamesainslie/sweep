package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

// RotationConfig configures log file rotation.
type RotationConfig struct {
	MaxSize    string `mapstructure:"max_size"`
	MaxAge     int    `mapstructure:"max_age"`
	MaxBackups int    `mapstructure:"max_backups"`
	Daily      bool   `mapstructure:"daily"`
}

// LoggingConfig configures application logging.
type LoggingConfig struct {
	Level      string            `mapstructure:"level"`
	Path       string            `mapstructure:"path"`
	Rotation   RotationConfig    `mapstructure:"rotation"`
	Components map[string]string `mapstructure:"components"`
}

// DaemonConfig configures the background daemon.
type DaemonConfig struct {
	AutoStart  bool   `mapstructure:"auto_start"`
	BinaryPath string `mapstructure:"binary_path"` // Path to sweepd binary (auto-discovered if empty)
	SocketPath string `mapstructure:"socket_path"`
	PIDPath    string `mapstructure:"pid_path"`
}

// Config represents the application configuration.
type Config struct {
	MinSize     string   `mapstructure:"min_size"`
	DefaultPath string   `mapstructure:"default_path"`
	Exclude     []string `mapstructure:"exclude"`
	Workers     struct {
		Dir  int `mapstructure:"dir"`
		File int `mapstructure:"file"`
	} `mapstructure:"workers"`
	Manifest struct {
		Enabled       bool   `mapstructure:"enabled"`
		Path          string `mapstructure:"path"`
		RetentionDays int    `mapstructure:"retention_days"`
	} `mapstructure:"manifest"`
	Logging LoggingConfig `mapstructure:"logging"`
	Daemon  DaemonConfig  `mapstructure:"daemon"`
}

// Load loads configuration from file and environment variables.
// Config file locations (in order of precedence):
//   - $XDG_CONFIG_HOME/sweep/config.yaml
//   - $HOME/.config/sweep/config.yaml
//
// Environment variables are prefixed with SWEEP_ (e.g., SWEEP_MIN_SIZE).
func Load() (*Config, error) {
	v := viper.New()

	// Set config name and type
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Add config paths
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		v.AddConfigPath(filepath.Join(xdgConfigHome, "sweep"))
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	v.AddConfigPath(filepath.Join(homeDir, ".config", "sweep"))

	// Set environment variable prefix and enable auto env binding
	v.SetEnvPrefix("SWEEP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault("min_size", DefaultMinSize)
	v.SetDefault("default_path", DefaultPath)
	v.SetDefault("exclude", DefaultExclusions)
	v.SetDefault("workers.dir", DefaultDirWorkers)
	v.SetDefault("workers.file", DefaultFileWorkers)
	v.SetDefault("manifest.enabled", true)
	v.SetDefault("manifest.retention_days", DefaultRetentionDays)

	// Set default manifest path (needs home dir expansion)
	v.SetDefault("manifest.path", filepath.Join(homeDir, ".config", "sweep", ".manifest"))

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.path", "") // Empty means use DefaultLogPath
	v.SetDefault("logging.rotation.max_size", "10MB")
	v.SetDefault("logging.rotation.max_age", 30)
	v.SetDefault("logging.rotation.max_backups", 5)
	v.SetDefault("logging.rotation.daily", true)
	v.SetDefault("logging.components", map[string]string{
		"daemon":  "info",
		"watcher": "warn",
		"scanner": "info",
		"tui":     "info",
	})

	// Daemon defaults
	v.SetDefault("daemon.auto_start", true)
	v.SetDefault("daemon.socket_path", "") // Empty means use default XDG path
	v.SetDefault("daemon.pid_path", "")    // Empty means use default XDG path

	// Read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is acceptable; we use defaults
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand ~ in manifest path if present
	if strings.HasPrefix(cfg.Manifest.Path, "~") {
		cfg.Manifest.Path = filepath.Join(homeDir, cfg.Manifest.Path[1:])
	}

	return &cfg, nil
}

// ConfigDir returns the configuration directory path, expanding ~ to the user's home directory.
func ConfigDir() (string, error) {
	// Check XDG_CONFIG_HOME first
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "sweep"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "sweep"), nil
}

// ManifestDir returns the manifest directory path, expanding ~ to the user's home directory.
func ManifestDir() (string, error) {
	configDir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, ".manifest"), nil
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	return nil
}

// EnsureManifestDir creates the manifest directory if it doesn't exist.
func EnsureManifestDir() error {
	dir, err := ManifestDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	return nil
}

// WriteDefault writes a default config file if none exists.
// Returns nil if a config file already exists.
func WriteDefault() error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	configDir, err := ConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.yaml")

	// Check if config file already exists
	if _, err := os.Stat(configPath); err == nil {
		// Config file exists, do nothing
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check config file: %w", err)
	}

	manifestDir, err := ManifestDir()
	if err != nil {
		return err
	}

	defaultConfig := fmt.Sprintf(`# Sweep Disk Analyzer Configuration

# Minimum file size to include in scans
min_size: %s

# Default path to scan when none is specified
default_path: %s

# Paths to exclude from scanning
exclude:
  - /proc
  - /sys
  - /dev

# Worker pool configuration
workers:
  dir: %d
  file: %d

# Manifest settings for tracking scan history
manifest:
  enabled: true
  path: %s
  retention_days: %d

# Logging configuration
logging:
  # Log level: debug, info, warn, error
  level: info
  # Log file path (empty means use default: $XDG_STATE_HOME/sweep/sweep.log)
  path: ""
  # Log rotation settings
  rotation:
    max_size: 10MB
    max_age: 30       # days
    max_backups: 5
    daily: true
  # Per-component log levels
  components:
    daemon: info
    watcher: warn
    scanner: info
    tui: info

# Daemon configuration
daemon:
  # Automatically start daemon when running sweep commands
  auto_start: true
  # Unix socket path (empty means use default: $XDG_DATA_HOME/sweep/sweep.sock)
  socket_path: ""
  # PID file path (empty means use default: $XDG_DATA_HOME/sweep/sweep.pid)
  pid_path: ""
`, DefaultMinSize, DefaultPath, DefaultDirWorkers, DefaultFileWorkers, manifestDir, DefaultRetentionDays)

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	return nil
}

// ExpandPath expands ~ in a path to the user's home directory.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, path[1:]), nil
}

// DataDir returns $XDG_DATA_HOME/sweep/ for database, socket, and pid files.
func DataDir() string {
	return filepath.Join(xdg.DataHome, "sweep")
}

// StateDir returns $XDG_STATE_HOME/sweep/ for log files.
func StateDir() string {
	return filepath.Join(xdg.StateHome, "sweep")
}

// CacheDir returns $XDG_CACHE_HOME/sweep/ (reserved for future use).
func CacheDir() string {
	return filepath.Join(xdg.CacheHome, "sweep")
}

// DefaultSocketPath returns the default Unix socket path.
func DefaultSocketPath() string {
	return filepath.Join(DataDir(), "sweep.sock")
}

// DefaultPIDPath returns the default PID file path.
func DefaultPIDPath() string {
	return filepath.Join(DataDir(), "sweep.pid")
}

// DefaultDBPath returns the default database path.
func DefaultDBPath() string {
	return filepath.Join(DataDir(), "sweep.db")
}

// DefaultLogPath returns the default log file path.
func DefaultLogPath() string {
	return filepath.Join(StateDir(), "sweep.log")
}

// EnsureDataDir creates the data directory if it doesn't exist.
func EnsureDataDir() error {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}
	return nil
}

// EnsureStateDir creates the state directory if it doesn't exist.
func EnsureStateDir() error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}
	return nil
}

// EnsureCacheDir creates the cache directory if it doesn't exist.
func EnsureCacheDir() error {
	if err := os.MkdirAll(CacheDir(), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	return nil
}
