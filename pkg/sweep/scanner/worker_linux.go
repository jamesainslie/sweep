//go:build linux

package scanner

import (
	"os"
	"time"
)

// getCreateTime returns the creation time of a file.
// On Linux, birth time is not always available, so we return modification time.
// Note: On Linux 4.11+ with ext4/xfs/btrfs, statx() can be used to get birth time,
// but this requires Go 1.20+ and more complex handling.
func getCreateTime(info os.FileInfo) time.Time {
	// Linux doesn't reliably expose birth time through syscall.Stat_t.
	// Fall back to modification time.
	return info.ModTime()
}
