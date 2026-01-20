package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesainslie/sweep/pkg/client"
	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/jamesainslie/sweep/pkg/sweep/logging"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "sweep [path]",
		Short: "Find large files consuming disk space",
		Long: `Sweep scans directories for large files and helps you reclaim disk space.

By default, sweep launches an interactive TUI to browse and manage large files.
Use --no-interactive or --output for non-interactive output.

Examples:
  sweep                      # Scan current directory with TUI
  sweep ~/Downloads          # Scan specific directory
  sweep -s 500M .            # Find files larger than 500MB
  sweep -n -o json .         # Non-interactive JSON output
  sweep -n -o pretty .       # Non-interactive pretty table output
  sweep --type video .       # Find video files
  sweep --older-than 30d .   # Find files older than 30 days
  sweep config show          # Show configuration
  sweep history              # View operation history`,
		Args:              cobra.MaximumNArgs(1),
		SilenceUsage:      true, // Don't show usage on runtime errors
		PersistentPreRunE: initializeLogging,
		RunE:              runScan,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	// Persistent flags (available to all commands)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/sweep/config.yaml)")
	rootCmd.PersistentFlags().StringP("min-size", "s", "", "minimum file size (e.g., 100M, 1G)")
	rootCmd.PersistentFlags().IntP("workers", "w", 0, "override worker count (0=auto)")
	rootCmd.PersistentFlags().StringSliceP("exclude", "e", nil, "exclude patterns (can be specified multiple times)")
	rootCmd.PersistentFlags().BoolP("no-interactive", "n", false, "disable TUI, use text output")
	rootCmd.PersistentFlags().BoolP("dry-run", "d", false, "don't delete files (preview only)")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "minimal output")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "debug output")
	rootCmd.PersistentFlags().Bool("no-cache", false, "bypass cache, perform full scan")
	rootCmd.PersistentFlags().Bool("no-daemon", false, "bypass daemon, perform direct scan")

	// Output format flags
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "pretty", "output format (pretty, plain, json, jsonl, csv, tsv, yaml, paths, markdown, template)")
	rootCmd.PersistentFlags().StringVar(&templateStr, "template", "", "Go template for template format")
	rootCmd.PersistentFlags().StringVarP(&columns, "columns", "c", "size,path", "columns to display (comma-separated)")

	// Filter flags
	rootCmd.PersistentFlags().IntVarP(&limit, "limit", "l", 50, "max files to return (0 for unlimited)")
	rootCmd.PersistentFlags().StringVar(&olderThan, "older-than", "", "files older than duration (e.g., 30d, 2w, 1mo)")
	rootCmd.PersistentFlags().StringVar(&newerThan, "newer-than", "", "files newer than duration (e.g., 7d, 1w)")
	rootCmd.PersistentFlags().StringVar(&fileTypes, "type", "", "file type groups (video, audio, image, archive, document, code, log)")
	rootCmd.PersistentFlags().StringVar(&extensions, "ext", "", "file extensions (comma-separated, e.g., .mp4,.mkv)")
	rootCmd.PersistentFlags().StringVar(&include, "include", "", "include glob patterns (comma-separated)")
	rootCmd.PersistentFlags().IntVar(&maxDepth, "max-depth", 0, "max directory depth (0 for unlimited)")
	rootCmd.PersistentFlags().StringVar(&sortBy, "sort", "size", "sort by: size, age, path")
	rootCmd.PersistentFlags().BoolVar(&reverse, "reverse", false, "reverse sort order")

	// Daemon/cache control flags
	rootCmd.PersistentFlags().StringVar(&maxAge, "max-age", "", "max index age before rescan (e.g., 1h, 30m)")
	rootCmd.PersistentFlags().BoolVar(&forceDaemon, "force-daemon", false, "fail if daemon unavailable")
	rootCmd.PersistentFlags().BoolVar(&forceScan, "force-scan", false, "always perform direct scan, ignore daemon")

	// Bind flags to viper
	_ = viper.BindPFlag("min_size", rootCmd.PersistentFlags().Lookup("min-size"))
	_ = viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))
	_ = viper.BindPFlag("exclude", rootCmd.PersistentFlags().Lookup("exclude"))
	_ = viper.BindPFlag("no_interactive", rootCmd.PersistentFlags().Lookup("no-interactive"))
	_ = viper.BindPFlag("dry_run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("no_cache", rootCmd.PersistentFlags().Lookup("no-cache"))
	_ = viper.BindPFlag("no_daemon", rootCmd.PersistentFlags().Lookup("no-daemon"))

	// Bind new flags to viper
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("template", rootCmd.PersistentFlags().Lookup("template"))
	_ = viper.BindPFlag("columns", rootCmd.PersistentFlags().Lookup("columns"))
	_ = viper.BindPFlag("limit", rootCmd.PersistentFlags().Lookup("limit"))
	_ = viper.BindPFlag("older_than", rootCmd.PersistentFlags().Lookup("older-than"))
	_ = viper.BindPFlag("newer_than", rootCmd.PersistentFlags().Lookup("newer-than"))
	_ = viper.BindPFlag("type", rootCmd.PersistentFlags().Lookup("type"))
	_ = viper.BindPFlag("ext", rootCmd.PersistentFlags().Lookup("ext"))
	_ = viper.BindPFlag("include", rootCmd.PersistentFlags().Lookup("include"))
	_ = viper.BindPFlag("max_depth", rootCmd.PersistentFlags().Lookup("max-depth"))
	_ = viper.BindPFlag("sort", rootCmd.PersistentFlags().Lookup("sort"))
	_ = viper.BindPFlag("reverse", rootCmd.PersistentFlags().Lookup("reverse"))
	_ = viper.BindPFlag("max_age", rootCmd.PersistentFlags().Lookup("max-age"))
	_ = viper.BindPFlag("force_daemon", rootCmd.PersistentFlags().Lookup("force-daemon"))
	_ = viper.BindPFlag("force_scan", rootCmd.PersistentFlags().Lookup("force-scan"))
}

// initConfig reads in config file and environment variables.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Set config name and type
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		// Add config paths in order of precedence
		if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
			viper.AddConfigPath(filepath.Join(xdgConfigHome, "sweep"))
		}

		homeDir, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(homeDir, ".config", "sweep"))
		}
	}

	// Set environment variable prefix and enable auto env binding
	viper.SetEnvPrefix("SWEEP")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Set defaults from config package
	viper.SetDefault("min_size", config.DefaultMinSize)
	viper.SetDefault("default_path", config.DefaultPath)
	viper.SetDefault("exclude", config.DefaultExclusions)
	viper.SetDefault("workers.dir", config.DefaultDirWorkers)
	viper.SetDefault("workers.file", config.DefaultFileWorkers)
	viper.SetDefault("manifest.enabled", true)
	viper.SetDefault("manifest.retention_days", config.DefaultRetentionDays)

	// Read config file (ignore if not found)
	_ = viper.ReadInConfig()
}

// Execute runs the root command.
// Logging is initialized in PersistentPreRunE after flag parsing.
func Execute() error {
	defer func() {
		_ = logging.Close()
	}()
	return rootCmd.Execute()
}

// initializeLogging is called by PersistentPreRunE after flags are parsed.
// This ensures verbose flag is available when configuring console output.
func initializeLogging(_ *cobra.Command, _ []string) error {
	// Ensure XDG directories
	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := config.EnsureDataDir(); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	if err := config.EnsureStateDir(); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	// Build logging configuration
	logCfg, cfg, err := buildLoggingConfig()
	if err != nil {
		return err
	}

	// Configure console output for verbose mode (non-TUI)
	// TUI mode will re-initialize with TUIMode: true
	if viper.GetBool("verbose") {
		logCfg.ConsoleLevel = "debug"
	}

	if err := logging.Init(logCfg); err != nil {
		return fmt.Errorf("initializing logging: %w", err)
	}

	log := logging.Get("client")
	log.Debug("sweep starting", "version", "0.1.0")

	// Auto-start daemon if configured and not bypassed
	if cfg.Daemon.AutoStart && !viper.GetBool("no_daemon") {
		paths := client.DaemonPaths{
			Binary: cfg.Daemon.BinaryPath,
			Socket: cfg.Daemon.SocketPath,
			PID:    cfg.Daemon.PIDPath,
		}
		if err := client.EnsureDaemon(paths); err != nil {
			log.Warn("failed to auto-start daemon", "error", err)
			// Continue anyway - not fatal
		}
	}

	return nil
}

// parseRotationConfig converts config.RotationConfig to logging.RotationConfig.
func parseRotationConfig(cfg config.RotationConfig) logging.RotationConfig {
	// Parse max_size string to bytes
	maxSize, err := types.ParseSize(cfg.MaxSize)
	if err != nil || maxSize == 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}
	return logging.RotationConfig{
		MaxSize:    maxSize,
		MaxAge:     cfg.MaxAge,
		MaxBackups: cfg.MaxBackups,
		Daily:      cfg.Daily,
	}
}

// buildLoggingConfig builds a logging config from the application config.
// It handles verbose flag override and default path.
// Returns both the logging config and the full application config.
func buildLoggingConfig() (logging.Config, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return logging.Config{}, nil, fmt.Errorf("loading config: %w", err)
	}

	logPath := cfg.Logging.Path
	if logPath == "" {
		logPath = config.DefaultLogPath()
	}

	logLevel := cfg.Logging.Level
	if viper.GetBool("verbose") {
		logLevel = "debug"
	}

	logCfg := logging.Config{
		Level:      logLevel,
		Path:       logPath,
		Rotation:   parseRotationConfig(cfg.Logging.Rotation),
		Components: cfg.Logging.Components,
	}

	return logCfg, cfg, nil
}

// initTUILogging re-initializes logging for TUI mode.
// This enables the log buffer for the TUI log panel and disables console output.
func initTUILogging() error {
	logCfg, _, err := buildLoggingConfig()
	if err != nil {
		return err
	}
	logCfg.TUIMode = true // Enable TUI mode (log buffer, no console)
	return logging.Init(logCfg)
}

// getQuiet returns true if quiet mode is enabled.
func getQuiet() bool {
	return viper.GetBool("quiet")
}

// printVerbose logs a debug message. Console output is handled by the logger
// when ConsoleLevel is set (via -v flag).
// Deprecated: prefer using logging.Get("client").Debug() with structured key-value pairs.
func printVerbose(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logging.Get("client").Debug(msg)
}

// printInfo prints a user-facing info message and logs it.
// This is for UI output that should always be shown to the user (unless quiet).
func printInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logging.Get("client").Info(msg)
	if !getQuiet() {
		fmt.Println(msg)
	}
}

// printError prints a user-facing error message and logs it.
// Errors are always shown regardless of quiet mode.
func printError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logging.Get("client").Error(msg)
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
}
