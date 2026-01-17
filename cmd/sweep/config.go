package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jamesainslie/sweep/pkg/sweep/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long: `Manage sweep configuration settings.

Configuration is loaded from:
  1. $XDG_CONFIG_HOME/sweep/config.yaml (if set)
  2. ~/.config/sweep/config.yaml

Environment variables can override config file settings using the SWEEP_ prefix:
  SWEEP_MIN_SIZE=500M
  SWEEP_WORKERS_DIR=8
  SWEEP_EXCLUDE=/tmp,/var/cache`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration settings from all sources.`,
	RunE:  runConfigShow,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration file",
	Long: `Open the configuration file in your default editor.

The editor is determined by:
  1. $VISUAL environment variable
  2. $EDITOR environment variable
  3. Falls back to 'vi'

If the config file doesn't exist, a default one will be created first.`,
	RunE: runConfigEdit,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default configuration file",
	Long:  `Create a default configuration file if one doesn't exist.`,
	RunE:  runConfigInit,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	Long:  `Display the path to the configuration file.`,
	RunE:  runConfigPath,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}

// runConfigShow displays the current configuration.
func runConfigShow(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		printError("Failed to load configuration: %v", err)
		// Show defaults anyway
		cfg = &config.Config{
			MinSize:     config.DefaultMinSize,
			DefaultPath: config.DefaultPath,
			Exclude:     config.DefaultExclusions,
		}
		cfg.Workers.Dir = config.DefaultDirWorkers
		cfg.Workers.File = config.DefaultFileWorkers
		cfg.Manifest.Enabled = true
		cfg.Manifest.RetentionDays = config.DefaultRetentionDays
	}

	// Show config file being used
	if configFile := viper.ConfigFileUsed(); configFile != "" {
		fmt.Printf("Config file: %s\n\n", configFile)
	} else {
		fmt.Println("Config file: (using defaults, no file found)")
		fmt.Println()
	}

	// Display configuration
	fmt.Println("Current Configuration:")
	fmt.Println("----------------------")
	fmt.Printf("min_size:             %s\n", cfg.MinSize)
	fmt.Printf("default_path:         %s\n", cfg.DefaultPath)
	fmt.Printf("exclude:              %v\n", cfg.Exclude)
	fmt.Printf("workers.dir:          %d\n", cfg.Workers.Dir)
	fmt.Printf("workers.file:         %d\n", cfg.Workers.File)
	fmt.Printf("manifest.enabled:     %t\n", cfg.Manifest.Enabled)
	fmt.Printf("manifest.path:        %s\n", cfg.Manifest.Path)
	fmt.Printf("manifest.retention:   %d days\n", cfg.Manifest.RetentionDays)

	// Show any environment overrides
	fmt.Println("\nEnvironment Overrides:")
	fmt.Println("----------------------")
	envVars := []struct {
		name string
		key  string
	}{
		{"SWEEP_MIN_SIZE", "min_size"},
		{"SWEEP_DEFAULT_PATH", "default_path"},
		{"SWEEP_EXCLUDE", "exclude"},
		{"SWEEP_WORKERS_DIR", "workers.dir"},
		{"SWEEP_WORKERS_FILE", "workers.file"},
		{"SWEEP_MANIFEST_ENABLED", "manifest.enabled"},
		{"SWEEP_MANIFEST_PATH", "manifest.path"},
		{"SWEEP_MANIFEST_RETENTION_DAYS", "manifest.retention_days"},
	}

	anyOverrides := false
	for _, ev := range envVars {
		if val := os.Getenv(ev.name); val != "" {
			fmt.Printf("%s=%s\n", ev.name, val)
			anyOverrides = true
		}
	}
	if !anyOverrides {
		fmt.Println("(none)")
	}

	return nil
}

// runConfigEdit opens the config file in an editor.
func runConfigEdit(cmd *cobra.Command, args []string) error {
	// Ensure config file exists
	if err := config.WriteDefault(); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	// Get config file path
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	// Determine editor
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	printVerbose("Opening %s with %s", configPath, editor)

	// Open editor
	editorCmd := exec.Command(editor, configPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor command failed: %w", err)
	}

	return nil
}

// runConfigInit creates a default config file.
func runConfigInit(cmd *cobra.Command, args []string) error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		printInfo("Config file already exists: %s", configPath)
		printInfo("Use 'sweep config edit' to modify it.")
		return nil
	}

	// Create default config
	if err := config.WriteDefault(); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	printInfo("Created default config file: %s", configPath)
	return nil
}

// runConfigPath shows the config file path.
func runConfigPath(cmd *cobra.Command, args []string) error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	fmt.Println(configPath)

	// Show if file exists
	if _, err := os.Stat(configPath); err == nil {
		printVerbose("File exists")
	} else if os.IsNotExist(err) {
		printVerbose("File does not exist (will use defaults)")
	}

	return nil
}
