package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time variables set by goreleaser or go build -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version, commit hash, and build date of sweep.`,
	Run:   runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// runVersion prints version information.
func runVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("sweep %s\n", version)
	fmt.Printf("  commit:  %s\n", commit)
	fmt.Printf("  built:   %s\n", date)
	fmt.Printf("  go:      %s\n", runtime.Version())
	fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
