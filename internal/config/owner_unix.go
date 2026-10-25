//go:build !windows

package config

import (
	"os"
	"syscall"
)

// keepOwner gives a file written by root (a command run with sudo) back to the owner it had, or to its folder's:
// the manager, which runs as its own user, must still read it.
func keepOwner(path string, was os.FileInfo) {
	if was == nil || os.Geteuid() != 0 {
		return
	}
	if st, ok := was.Sys().(*syscall.Stat_t); ok && st.Uid != 0 {
		_ = os.Chown(path, int(st.Uid), int(st.Gid))
	}
}
