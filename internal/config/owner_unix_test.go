//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Written by root (sudo szm listen, sudo szm password), a file stays its owner's: the manager runs as its own user.
func TestWriteKeepsOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("as root, on Linux")
	}
	dir := t.TempDir()
	os.Chown(dir, 1234, 1234)
	p := filepath.Join(dir, "szm.json")
	if err := WriteFileAtomic(p, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chown(p, 4321, 4321)
	if err := WriteFileAtomic(p, []byte("{ }"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if uid := st.Sys().(*syscall.Stat_t).Uid; uid != 4321 {
		t.Fatal(uid)
	}
}
