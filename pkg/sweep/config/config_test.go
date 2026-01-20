package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Use a temp directory that doesn't have a config file
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MinSize != DefaultMinSize {
		t.Errorf("MinSize = %q, want %q", cfg.MinSize, DefaultMinSize)
	}

	if cfg.DefaultPath != DefaultPath {
		t.Errorf("DefaultPath = %q, want %q", cfg.DefaultPath, DefaultPath)
	}

	if cfg.Workers.Dir != DefaultDirWorkers {
		t.Errorf("Workers.Dir = %d, want %d", cfg.Workers.Dir, DefaultDirWorkers)
	}

	if cfg.Workers.File != DefaultFileWorkers {
		t.Errorf("Workers.File = %d, want %d", cfg.Workers.File, DefaultFileWorkers)
	}

	if !cfg.Manifest.Enabled {
		t.Error("Manifest.Enabled = false, want true")
	}

	if cfg.Manifest.RetentionDays != DefaultRetentionDays {
		t.Errorf("Manifest.RetentionDays = %d, want %d", cfg.Manifest.RetentionDays, DefaultRetentionDays)
	}

	expectedExclusions := len(DefaultExclusions)
	if len(cfg.Exclude) != expectedExclusions {
		t.Errorf("len(Exclude) = %d, want %d", len(cfg.Exclude), expectedExclusions)
	}
}

func TestLoad_FromFile(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config", "sweep")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
min_size: 50MB
default_path: /home/user
exclude:
  - /tmp
  - /var/cache
workers:
  dir: 2
  file: 4
manifest:
  enabled: false
  path: /custom/manifest
  retention_days: 7
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MinSize != "50MB" {
		t.Errorf("MinSize = %q, want %q", cfg.MinSize, "50MB")
	}

	if cfg.DefaultPath != "/home/user" {
		t.Errorf("DefaultPath = %q, want %q", cfg.DefaultPath, "/home/user")
	}

	if cfg.Workers.Dir != 2 {
		t.Errorf("Workers.Dir = %d, want %d", cfg.Workers.Dir, 2)
	}

	if cfg.Workers.File != 4 {
		t.Errorf("Workers.File = %d, want %d", cfg.Workers.File, 4)
	}

	if cfg.Manifest.Enabled {
		t.Error("Manifest.Enabled = true, want false")
	}

	if cfg.Manifest.Path != "/custom/manifest" {
		t.Errorf("Manifest.Path = %q, want %q", cfg.Manifest.Path, "/custom/manifest")
	}

	if cfg.Manifest.RetentionDays != 7 {
		t.Errorf("Manifest.RetentionDays = %d, want %d", cfg.Manifest.RetentionDays, 7)
	}

	if len(cfg.Exclude) != 2 {
		t.Errorf("len(Exclude) = %d, want %d", len(cfg.Exclude), 2)
	}
}

func TestLoad_XDGConfigHome(t *testing.T) {
	tempDir := t.TempDir()
	xdgConfigDir := filepath.Join(tempDir, "xdg-config", "sweep")
	if err := os.MkdirAll(xdgConfigDir, 0o755); err != nil {
		t.Fatalf("failed to create XDG config dir: %v", err)
	}

	configContent := `min_size: 200MB`
	configPath := filepath.Join(xdgConfigDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg-config"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MinSize != "200MB" {
		t.Errorf("MinSize = %q, want %q", cfg.MinSize, "200MB")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("SWEEP_MIN_SIZE", "500MB")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MinSize != "500MB" {
		t.Errorf("MinSize = %q, want %q", cfg.MinSize, "500MB")
	}
}

func TestConfigDir(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")

		dir, err := ConfigDir()
		if err != nil {
			t.Fatalf("ConfigDir() error = %v", err)
		}

		expected := "/custom/config/sweep"
		if dir != expected {
			t.Errorf("ConfigDir() = %q, want %q", dir, expected)
		}
	})

	t.Run("uses HOME/.config when XDG_CONFIG_HOME not set", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)
		t.Setenv("XDG_CONFIG_HOME", "")

		dir, err := ConfigDir()
		if err != nil {
			t.Fatalf("ConfigDir() error = %v", err)
		}

		expected := filepath.Join(tempDir, ".config", "sweep")
		if dir != expected {
			t.Errorf("ConfigDir() = %q, want %q", dir, expected)
		}
	})
}

func TestManifestDir(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ManifestDir()
	if err != nil {
		t.Fatalf("ManifestDir() error = %v", err)
	}

	expected := filepath.Join(tempDir, ".config", "sweep", ".manifest")
	if dir != expected {
		t.Errorf("ManifestDir() = %q, want %q", dir, expected)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir() error = %v", err)
	}

	expectedDir := filepath.Join(tempDir, ".config", "sweep")
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", expectedDir, err)
	}

	if !info.IsDir() {
		t.Errorf("%q is not a directory", expectedDir)
	}
}

func TestEnsureManifestDir(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := EnsureManifestDir(); err != nil {
		t.Fatalf("EnsureManifestDir() error = %v", err)
	}

	expectedDir := filepath.Join(tempDir, ".config", "sweep", ".manifest")
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", expectedDir, err)
	}

	if !info.IsDir() {
		t.Errorf("%q is not a directory", expectedDir)
	}
}

func TestWriteDefault(t *testing.T) {
	t.Run("creates default config file", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)
		t.Setenv("XDG_CONFIG_HOME", "")

		if err := WriteDefault(); err != nil {
			t.Fatalf("WriteDefault() error = %v", err)
		}

		configPath := filepath.Join(tempDir, ".config", "sweep", "config.yaml")
		if _, err := os.Stat(configPath); err != nil {
			t.Fatalf("config file not created: %v", err)
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read config file: %v", err)
		}

		// Check that content contains expected values
		if len(content) == 0 {
			t.Error("config file is empty")
		}
	})

	t.Run("does not overwrite existing config", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)
		t.Setenv("XDG_CONFIG_HOME", "")

		configDir := filepath.Join(tempDir, ".config", "sweep")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}

		configPath := filepath.Join(configDir, "config.yaml")
		existingContent := "# existing config\nmin_size: 1GB"
		if err := os.WriteFile(configPath, []byte(existingContent), 0o644); err != nil {
			t.Fatalf("failed to write existing config: %v", err)
		}

		if err := WriteDefault(); err != nil {
			t.Fatalf("WriteDefault() error = %v", err)
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read config file: %v", err)
		}

		if string(content) != existingContent {
			t.Errorf("config file was overwritten: got %q, want %q", string(content), existingContent)
		}
	})
}

func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "expands tilde",
			input: "~/config/sweep",
			want:  filepath.Join(homeDir, "config/sweep"),
		},
		{
			name:  "leaves absolute path unchanged",
			input: "/etc/sweep",
			want:  "/etc/sweep",
		},
		{
			name:  "leaves relative path unchanged",
			input: "config/sweep",
			want:  "config/sweep",
		},
		{
			name:  "handles tilde only",
			input: "~",
			want:  homeDir,
		},
		{
			name:  "handles tilde with slash",
			input: "~/",
			want:  homeDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultExclusions(t *testing.T) {
	expected := []string{"/proc", "/sys", "/dev"}

	if len(DefaultExclusions) != len(expected) {
		t.Errorf("len(DefaultExclusions) = %d, want %d", len(DefaultExclusions), len(expected))
	}

	for i, v := range expected {
		if DefaultExclusions[i] != v {
			t.Errorf("DefaultExclusions[%d] = %q, want %q", i, DefaultExclusions[i], v)
		}
	}
}

func TestDefaultConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"DefaultMinSize", DefaultMinSize, "100MB"},
		{"DefaultPath", DefaultPath, "."},
		{"DefaultConfigDir", DefaultConfigDir, "~/.config/sweep"},
		{"DefaultManifestDir", DefaultManifestDir, "~/.config/sweep/.manifest"},
		{"DefaultRetentionDays", DefaultRetentionDays, 30},
		{"DefaultDirWorkers", DefaultDirWorkers, 4},
		{"DefaultFileWorkers", DefaultFileWorkers, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestLoad_LoggingDefaults(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "info")
	}

	if cfg.Logging.Path != "" {
		t.Errorf("Logging.Path = %q, want empty string", cfg.Logging.Path)
	}

	// Check rotation defaults
	if cfg.Logging.Rotation.MaxSize != "10MB" {
		t.Errorf("Logging.Rotation.MaxSize = %q, want %q", cfg.Logging.Rotation.MaxSize, "10MB")
	}

	if cfg.Logging.Rotation.MaxAge != 30 {
		t.Errorf("Logging.Rotation.MaxAge = %d, want %d", cfg.Logging.Rotation.MaxAge, 30)
	}

	if cfg.Logging.Rotation.MaxBackups != 5 {
		t.Errorf("Logging.Rotation.MaxBackups = %d, want %d", cfg.Logging.Rotation.MaxBackups, 5)
	}

	if !cfg.Logging.Rotation.Daily {
		t.Error("Logging.Rotation.Daily = false, want true")
	}

	// Check component defaults
	expectedComponents := map[string]string{
		"daemon":  "info",
		"watcher": "warn",
		"scanner": "info",
		"tui":     "info",
	}
	for component, level := range expectedComponents {
		if cfg.Logging.Components[component] != level {
			t.Errorf("Logging.Components[%q] = %q, want %q", component, cfg.Logging.Components[component], level)
		}
	}
}

func TestLoad_DaemonDefaults(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check daemon defaults
	if !cfg.Daemon.AutoStart {
		t.Error("Daemon.AutoStart = false, want true")
	}

	if cfg.Daemon.SocketPath != "" {
		t.Errorf("Daemon.SocketPath = %q, want empty string", cfg.Daemon.SocketPath)
	}

	if cfg.Daemon.PIDPath != "" {
		t.Errorf("Daemon.PIDPath = %q, want empty string", cfg.Daemon.PIDPath)
	}
}

func TestLoad_LoggingFromFile(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config", "sweep")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
logging:
  level: debug
  path: /var/log/sweep.log
  rotation:
    max_size: 50MB
    max_age: 7
    max_backups: 3
    daily: false
  components:
    daemon: debug
    watcher: info
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}

	if cfg.Logging.Path != "/var/log/sweep.log" {
		t.Errorf("Logging.Path = %q, want %q", cfg.Logging.Path, "/var/log/sweep.log")
	}

	if cfg.Logging.Rotation.MaxSize != "50MB" {
		t.Errorf("Logging.Rotation.MaxSize = %q, want %q", cfg.Logging.Rotation.MaxSize, "50MB")
	}

	if cfg.Logging.Rotation.MaxAge != 7 {
		t.Errorf("Logging.Rotation.MaxAge = %d, want %d", cfg.Logging.Rotation.MaxAge, 7)
	}

	if cfg.Logging.Rotation.MaxBackups != 3 {
		t.Errorf("Logging.Rotation.MaxBackups = %d, want %d", cfg.Logging.Rotation.MaxBackups, 3)
	}

	if cfg.Logging.Rotation.Daily {
		t.Error("Logging.Rotation.Daily = true, want false")
	}

	if cfg.Logging.Components["daemon"] != "debug" {
		t.Errorf("Logging.Components[daemon] = %q, want %q", cfg.Logging.Components["daemon"], "debug")
	}

	if cfg.Logging.Components["watcher"] != "info" {
		t.Errorf("Logging.Components[watcher] = %q, want %q", cfg.Logging.Components["watcher"], "info")
	}
}

func TestLoad_DaemonFromFile(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config", "sweep")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
daemon:
  auto_start: false
  socket_path: /tmp/sweep.sock
  pid_path: /tmp/sweep.pid
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Daemon.AutoStart {
		t.Error("Daemon.AutoStart = true, want false")
	}

	if cfg.Daemon.SocketPath != "/tmp/sweep.sock" {
		t.Errorf("Daemon.SocketPath = %q, want %q", cfg.Daemon.SocketPath, "/tmp/sweep.sock")
	}

	if cfg.Daemon.PIDPath != "/tmp/sweep.pid" {
		t.Errorf("Daemon.PIDPath = %q, want %q", cfg.Daemon.PIDPath, "/tmp/sweep.pid")
	}
}

func TestDataDir(t *testing.T) {
	// DataDir should return a path ending in /sweep under the xdg data home
	// Note: adrg/xdg caches values at init time, so we test the structure
	dir := DataDir()
	if !filepath.IsAbs(dir) {
		t.Errorf("DataDir() = %q, want absolute path", dir)
	}
	if filepath.Base(dir) != "sweep" {
		t.Errorf("DataDir() = %q, want path ending in 'sweep'", dir)
	}
}

func TestStateDir(t *testing.T) {
	// StateDir should return a path ending in /sweep under the xdg state home
	dir := StateDir()
	if !filepath.IsAbs(dir) {
		t.Errorf("StateDir() = %q, want absolute path", dir)
	}
	if filepath.Base(dir) != "sweep" {
		t.Errorf("StateDir() = %q, want path ending in 'sweep'", dir)
	}
}

func TestCacheDir(t *testing.T) {
	// CacheDir should return a path ending in /sweep under the xdg cache home
	dir := CacheDir()
	if !filepath.IsAbs(dir) {
		t.Errorf("CacheDir() = %q, want absolute path", dir)
	}
	if filepath.Base(dir) != "sweep" {
		t.Errorf("CacheDir() = %q, want path ending in 'sweep'", dir)
	}
}

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultSocketPath() = %q, want absolute path", path)
	}
	if filepath.Base(path) != "sweep.sock" {
		t.Errorf("DefaultSocketPath() = %q, want path ending in 'sweep.sock'", path)
	}
	// Should be under DataDir
	if filepath.Dir(path) != DataDir() {
		t.Errorf("DefaultSocketPath() dir = %q, want %q", filepath.Dir(path), DataDir())
	}
}

func TestDefaultPIDPath(t *testing.T) {
	path := DefaultPIDPath()
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultPIDPath() = %q, want absolute path", path)
	}
	if filepath.Base(path) != "sweep.pid" {
		t.Errorf("DefaultPIDPath() = %q, want path ending in 'sweep.pid'", path)
	}
	// Should be under DataDir
	if filepath.Dir(path) != DataDir() {
		t.Errorf("DefaultPIDPath() dir = %q, want %q", filepath.Dir(path), DataDir())
	}
}

func TestDefaultDBPath(t *testing.T) {
	path := DefaultDBPath()
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultDBPath() = %q, want absolute path", path)
	}
	if filepath.Base(path) != "sweep.db" {
		t.Errorf("DefaultDBPath() = %q, want path ending in 'sweep.db'", path)
	}
	// Should be under DataDir
	if filepath.Dir(path) != DataDir() {
		t.Errorf("DefaultDBPath() dir = %q, want %q", filepath.Dir(path), DataDir())
	}
}

func TestDefaultLogPath(t *testing.T) {
	path := DefaultLogPath()
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultLogPath() = %q, want absolute path", path)
	}
	if filepath.Base(path) != "sweep.log" {
		t.Errorf("DefaultLogPath() = %q, want path ending in 'sweep.log'", path)
	}
	// Should be under StateDir
	if filepath.Dir(path) != StateDir() {
		t.Errorf("DefaultLogPath() dir = %q, want %q", filepath.Dir(path), StateDir())
	}
}

func TestEnsureDataDir(t *testing.T) {
	// EnsureDataDir should create the data directory without error
	if err := EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir() error = %v", err)
	}

	expectedDir := DataDir()
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", expectedDir, err)
	}

	if !info.IsDir() {
		t.Errorf("%q is not a directory", expectedDir)
	}
}

func TestEnsureStateDir(t *testing.T) {
	// EnsureStateDir should create the state directory without error
	if err := EnsureStateDir(); err != nil {
		t.Fatalf("EnsureStateDir() error = %v", err)
	}

	expectedDir := StateDir()
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", expectedDir, err)
	}

	if !info.IsDir() {
		t.Errorf("%q is not a directory", expectedDir)
	}
}

func TestEnsureCacheDir(t *testing.T) {
	// EnsureCacheDir should create the cache directory without error
	if err := EnsureCacheDir(); err != nil {
		t.Fatalf("EnsureCacheDir() error = %v", err)
	}

	expectedDir := CacheDir()
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", expectedDir, err)
	}

	if !info.IsDir() {
		t.Errorf("%q is not a directory", expectedDir)
	}
}
