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
	"b": Build,
	"t": Test,
	"l": Lint,
	"i": Install,
	"c": Clean,
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

// Install builds and installs both sweep and sweepd to the user's GOBIN or /usr/local/bin.
func Install() error {
	st.Deps(InstallCLI, InstallDaemon)
	return nil
}

// InstallCLI builds and installs sweep CLI to the user's GOBIN or /usr/local/bin.
func InstallCLI() error {
	st.Deps(BuildCLI)
	return installBinary(binaryName)
}

// InstallDaemon builds and installs sweepd daemon to the user's GOBIN or /usr/local/bin.
func InstallDaemon() error {
	st.Deps(BuildDaemon)
	return installBinary(daemonBinaryName)
}

// installBinary installs a binary to the user's GOBIN or /usr/local/bin.
func installBinary(name string) error {
	gocmd := st.GoCmd()
	bin, err := sh.Output(gocmd, "env", "GOBIN")
	if err != nil {
		return fmt.Errorf("determining GOBIN: %w", err)
	}
	if bin == "" {
		gopath, err := sh.Output(gocmd, "env", "GOPATH")
		if err != nil {
			return fmt.Errorf("determining GOPATH: %w", err)
		}
		if gopath != "" {
			bin = filepath.Join(gopath, "bin")
		} else {
			// Fallback to /usr/local/bin if GOPATH is not set.
			bin = "/usr/local/bin"
		}
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

// Uninstall removes both installed sweep and sweepd binaries.
func Uninstall() error {
	if err := uninstallBinary(binaryName); err != nil {
		return err
	}
	return uninstallBinary(daemonBinaryName)
}

// uninstallBinary removes an installed binary.
func uninstallBinary(name string) error {
	gocmd := st.GoCmd()
	bin, err := sh.Output(gocmd, "env", "GOBIN")
	if err != nil {
		return fmt.Errorf("determining GOBIN: %w", err)
	}
	if bin == "" {
		gopath, err := sh.Output(gocmd, "env", "GOPATH")
		if err != nil {
			return fmt.Errorf("determining GOPATH: %w", err)
		}
		if gopath != "" {
			bin = filepath.Join(gopath, "bin")
		} else {
			bin = "/usr/local/bin"
		}
	}

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
