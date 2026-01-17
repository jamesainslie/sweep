package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesainslie/sweep/pkg/sweep/config"
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
Use --no-interactive or --json for non-interactive output.

Examples:
  sweep                      # Scan current directory with TUI
  sweep ~/Downloads          # Scan specific directory
  sweep -s 500M .            # Find files larger than 500MB
  sweep -n -j .              # Non-interactive JSON output
  sweep config show          # Show configuration
  sweep history              # View operation history`,
		Args: cobra.MaximumNArgs(1),
		RunE: runScan,
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
	rootCmd.PersistentFlags().BoolP("json", "j", false, "output JSON format")
	rootCmd.PersistentFlags().BoolP("dry-run", "d", false, "don't delete files (preview only)")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "minimal output")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "debug output")
	rootCmd.PersistentFlags().Bool("no-cache", false, "bypass cache, perform full scan")
	rootCmd.PersistentFlags().Bool("no-daemon", false, "bypass daemon, perform direct scan")

	// Bind flags to viper
	_ = viper.BindPFlag("min_size", rootCmd.PersistentFlags().Lookup("min-size"))
	_ = viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))
	_ = viper.BindPFlag("exclude", rootCmd.PersistentFlags().Lookup("exclude"))
	_ = viper.BindPFlag("no_interactive", rootCmd.PersistentFlags().Lookup("no-interactive"))
	_ = viper.BindPFlag("json", rootCmd.PersistentFlags().Lookup("json"))
	_ = viper.BindPFlag("dry_run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("no_cache", rootCmd.PersistentFlags().Lookup("no-cache"))
	_ = viper.BindPFlag("no_daemon", rootCmd.PersistentFlags().Lookup("no-daemon"))
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
func Execute() error {
	return rootCmd.Execute()
}

// getVerbose returns true if verbose mode is enabled.
func getVerbose() bool {
	return viper.GetBool("verbose")
}

// getQuiet returns true if quiet mode is enabled.
func getQuiet() bool {
	return viper.GetBool("quiet")
}

// printVerbose prints a message if verbose mode is enabled.
func printVerbose(format string, args ...interface{}) {
	if getVerbose() && !getQuiet() {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// printInfo prints a message if quiet mode is not enabled.
func printInfo(format string, args ...interface{}) {
	if !getQuiet() {
		fmt.Printf(format+"\n", args...)
	}
}

// printError prints an error message to stderr.
func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}
