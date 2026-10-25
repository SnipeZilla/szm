package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFreeNameAndCopyTree(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "server.cfg"), []byte("a"), 0o644)
	if p := freeName(d, "server.cfg", " - copy"); filepath.Base(p) != "server - copy.cfg" {
		t.Fatal(p)
	}
	os.WriteFile(filepath.Join(d, "server - copy.cfg"), []byte("a"), 0o644)
	if p := freeName(d, "server.cfg", " - copy"); filepath.Base(p) != "server - copy (2).cfg" {
		t.Fatal(p)
	}
	if p := freeName(d, "server.cfg", ""); filepath.Base(p) != "server (2).cfg" {
		t.Fatal(p)
	}
	src := filepath.Join(d, "addons")
	os.MkdirAll(filepath.Join(src, "sourcemod", "configs"), 0o755)
	os.WriteFile(filepath.Join(src, "sourcemod", "configs", "admins.cfg"), []byte("12345"), 0o644)
	os.WriteFile(filepath.Join(src, "top.txt"), []byte("x"), 0o644)
	dst := filepath.Join(d, "other", "addons")
	n, b, err := copyTree(src, dst)
	if err != nil || n != 2 || b != 6 {
		t.Fatal(n, b, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "sourcemod", "configs", "admins.cfg")); string(got) != "12345" {
		t.Fatal(string(got))
	}
	if !inside(src, filepath.Join(src, "sourcemod")) || inside(src, dst) || !samePath(src, src+string(os.PathSeparator)) {
		t.Fatal("inside")
	}
	if _, err := os.Stat(filepath.Join(dst, "top.txt.szm-part")); err == nil {
		t.Fatal("part left")
	}
}
