//go:build !darwin && !linux

package scanner

import (
	"os"
	"time"
)

// getCreateTime returns the creation time of a file.
// On unsupported platforms, falls back to modification time.
func getCreateTime(info os.FileInfo) time.Time {
	return info.ModTime()
}
