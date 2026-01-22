//go:build stave

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yaklabco/stave/pkg/sh"
	"github.com/yaklabco/stave/pkg/st"
)

// Default target when running `stave` with no arguments.
var Default = Build

// Aliases for common targets.
var Aliases = map[string]interface{}{
	"b":     Build,
	"t":     Test,
	"l":     Lint,
	"i":     Install,
	"c":     Clean,
	"check": Check,
}

const (
	binaryName       = "sweep"
	daemonBinaryName = "sweepd"
	mainPkg          = "./cmd/sweep"
	daemonPkg        = "./cmd/sweepd"
	binDir           = "bin"
)

// All runs the complete build pipeline.
func All() error {
	st.Deps(Lint, Test)
	st.Deps(Build)
	return nil
}

// Check runs the CI pipeline locally: lint, test, and build.
// This mirrors the GitHub Actions checks workflow for local verification.
func Check() error {
	fmt.Println("Running CI checks...")

	fmt.Println("\n=== Lint ===")
	if err := Lint(); err != nil {
		return fmt.Errorf("lint failed: %w", err)
	}

	fmt.Println("\n=== Test ===")
	if err := TestCoverage(); err != nil {
		return fmt.Errorf("test failed: %w", err)
	}

	fmt.Println("\n=== Build ===")
	if err := Build(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println("\n✓ All checks passed")
	return nil
}

// TestCoverage runs tests with race detection and generates coverage output.
func TestCoverage() error {
	return sh.RunV("go", "tool", "gotestsum", "-f", "pkgname-and-test-fails", "--", "./...", "-race", "-coverprofile=coverage.out", "-covermode=atomic")
}

// Build compiles both sweep and sweepd binaries.
func Build() error {
	st.Deps(BuildCLI, BuildDaemon)
	return nil
}

// BuildCLI compiles the sweep CLI binary.
func BuildCLI() error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	ldflags := buildLdflags()
	output := filepath.Join(binDir, binaryName)
	if runtime.GOOS == "windows" {
		output += ".exe"
	}

	return sh.RunV("go", "build", "-ldflags", ldflags, "-o", output, mainPkg)
}

// BuildDaemon compiles the sweepd daemon binary.
func BuildDaemon() error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	ldflags := buildLdflags()
	output := filepath.Join(binDir, daemonBinaryName)
	if runtime.GOOS == "windows" {
		output += ".exe"
	}

	return sh.RunV("go", "build", "-ldflags", ldflags, "-o", output, daemonPkg)
}

// Install builds and installs both sweep and sweepd to ~/.local/bin (or GOBIN if set).
func Install() error {
	st.Deps(InstallCLI, InstallDaemon)
	return nil
}

// InstallCLI builds and installs sweep CLI to ~/.local/bin (or GOBIN if set).
func InstallCLI() error {
	st.Deps(BuildCLI)
	return installBinary(binaryName)
}

// InstallDaemon builds and installs sweepd daemon to ~/.local/bin (or GOBIN if set).
func InstallDaemon() error {
	st.Deps(BuildDaemon)
	return installBinary(daemonBinaryName)
}

// installBinary installs a binary to ~/.local/bin (XDG user-local standard).
// Respects GOBIN if explicitly set.
func installBinary(name string) error {
	bin := getInstallDir()

	// Create install directory if it doesn't exist.
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return fmt.Errorf("creating install directory %s: %w", bin, err)
	}

	src := filepath.Join(binDir, name)
	if runtime.GOOS == "windows" {
		src += ".exe"
	}

	dst := filepath.Join(bin, name)
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}

	if st.Verbose() {
		fmt.Printf("Installing %s to %s\n", src, dst)
	}

	return sh.Copy(dst, src)
}

// getInstallDir returns the installation directory.
// Priority: GOBIN (if set) > ~/.local/bin (XDG standard).
func getInstallDir() string {
	gocmd := st.GoCmd()
	if bin, err := sh.Output(gocmd, "env", "GOBIN"); err == nil && bin != "" {
		return bin
	}

	// XDG user-local standard: ~/.local/bin
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to /usr/local/bin if home directory cannot be determined.
		return "/usr/local/bin"
	}
	return filepath.Join(home, ".local", "bin")
}

// Uninstall removes both installed sweep and sweepd binaries.
func Uninstall() error {
	if err := uninstallBinary(binaryName); err != nil {
		return err
	}
	return uninstallBinary(daemonBinaryName)
}

// uninstallBinary removes an installed binary.
func uninstallBinary(name string) error {
	bin := getInstallDir()

	target := filepath.Join(bin, name)
	if runtime.GOOS == "windows" {
		target += ".exe"
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		if st.Verbose() {
			fmt.Printf("Binary not found at %s, nothing to uninstall\n", target)
		}
		return nil
	}

	if st.Verbose() {
		fmt.Printf("Removing %s\n", target)
	}

	return os.Remove(target)
}

// Test runs all tests with race detection and coverage.
func Test() error {
	return sh.RunV("go", "test", "-race", "-cover", "./...")
}

// Lint runs golangci-lint.
func Lint() error {
	return sh.RunV("golangci-lint", "run", "./...")
}

// Clean removes build artifacts.
func Clean() error {
	if st.Verbose() {
		fmt.Printf("Removing %s/\n", binDir)
	}
	return sh.Rm(binDir + "/")
}

// Fmt formats all Go code.
func Fmt() error {
	if err := sh.Run("gofmt", "-w", "."); err != nil {
		return fmt.Errorf("running gofmt: %w", err)
	}
	return sh.Run("goimports", "-w", ".")
}

// Tidy runs go mod tidy.
func Tidy() error {
	return sh.RunV("go", "mod", "tidy")
}

// buildLdflags returns ldflags for version injection.
func buildLdflags() string {
	version := "dev"
	commit := "unknown"
	date := time.Now().Format(time.RFC3339)

	if v, err := sh.Output("git", "describe", "--tags", "--always"); err == nil && v != "" {
		version = strings.TrimSpace(v)
	}

	if c, err := sh.Output("git", "rev-parse", "--short", "HEAD"); err == nil && c != "" {
		commit = strings.TrimSpace(c)
	}

	pkg := "github.com/jamesainslie/sweep/cmd/sweep"
	return fmt.Sprintf(
		"-X %s.version=%s -X %s.commit=%s -X %s.date=%s",
		pkg, version, pkg, commit, pkg, date,
	)
}
