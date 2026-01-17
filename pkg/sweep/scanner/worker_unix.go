//go:build unix

package scanner

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// getOwnership returns the owner and group names for a file.
// Falls back to UID/GID strings if names cannot be resolved.
func getOwnership(info os.FileInfo) (owner, group string) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "unknown", "unknown"
	}

	// Try to resolve UID to username.
	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	if u, err := user.LookupId(uid); err == nil {
		owner = u.Username
	} else {
		owner = uid
	}

	// Try to resolve GID to group name.
	gid := strconv.FormatUint(uint64(stat.Gid), 10)
	if g, err := user.LookupGroupId(gid); err == nil {
		group = g.Name
	} else {
		group = gid
	}

	return owner, group
}
