// Package trash provides cross-platform file trash/delete functionality.
// It moves files to the system trash where available, falling back to
// permanent deletion when no trash support is detected.
package trash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// commandTimeout is the maximum time to wait for trash commands.
const commandTimeout = 30 * time.Second

// MoveToTrash moves a file or directory to the system trash.
// On macOS: uses AppleScript to move to Trash.
// On Linux: uses gio trash or trash-cli.
// Falls back to permanent delete if no trash available.
func MoveToTrash(path string) error {
	// Verify the path exists before attempting to trash it
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("cannot trash %q: %w", path, err)
	}

	// Convert to absolute path for reliable trash operations
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve absolute path for %q: %w", path, err)
	}

	switch runtime.GOOS {
	case "darwin":
		return moveToTrashMacOS(absPath)
	case "linux":
		return moveToTrashLinux(absPath)
	default:
		return fallbackDelete(absPath)
	}
}

// moveToTrashMacOS moves a file to Trash on macOS using AppleScript.
func moveToTrashMacOS(path string) error {
	// Use AppleScript to move to Trash - this is the standard way on macOS
	// and properly integrates with Finder's "Put Back" functionality
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	script := fmt.Sprintf(`tell application "Finder" to delete POSIX file %q`, path)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		// Fall back to permanent delete if AppleScript fails
		return fallbackDelete(path)
	}
	return nil
}

// moveToTrashLinux moves a file to trash on Linux using available tools.
func moveToTrashLinux(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	// Try gio first (GNOME/GTK desktop environments)
	if gioPath, err := exec.LookPath("gio"); err == nil {
		cmd := exec.CommandContext(ctx, gioPath, "trash", path)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try trash-cli (cross-desktop, XDG compliant)
	if trashPath, err := exec.LookPath("trash-put"); err == nil {
		cmd := exec.CommandContext(ctx, trashPath, path)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fall back to permanent delete if no trash tools available
	return fallbackDelete(path)
}

// fallbackDelete permanently removes a file or directory.
// This is used when no system trash is available.
func fallbackDelete(path string) error {
	// Use RemoveAll to handle both files and directories
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to delete %q: %w", path, err)
	}
	return nil
}
