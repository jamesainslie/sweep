package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

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
