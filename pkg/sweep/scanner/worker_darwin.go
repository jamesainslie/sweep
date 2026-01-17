//go:build darwin

package scanner

import (
	"os"
	"syscall"
	"time"
)

// getCreateTime returns the creation time of a file.
// On macOS, this uses the birth time from the stat structure.
func getCreateTime(info os.FileInfo) time.Time {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime()
	}

	// On macOS, Birthtimespec contains the creation time.
	return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
}
