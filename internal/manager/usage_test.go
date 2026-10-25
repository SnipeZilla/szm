package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuietRcon(t *testing.T) {
	for _, l := range []string{
		`rcon from "127.0.0.1:50123": command "status"`,
		`L 10/13/2026 - 13:00:00: rcon from "127.0.0.1:50123": command "status"`,
		`Rcon: "rcon 1234567 "secret" status" from "127.0.0.1:27005"`,
	} {
		if !quietRconRe.MatchString(l) {
			t.Errorf("not hidden: %s", l)
		}
	}
	for _, l := range []string{`rcon from "1.2.3.4:5": command "changelevel de_dust2"`, `Player said "status"`} {
		if quietRconRe.MatchString(l) {
			t.Errorf("hidden: %s", l)
		}
	}
}

func TestFolderSize(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "x"), make([]byte, 100), 0o644)
	os.WriteFile(filepath.Join(dir, "a", "b", "y"), make([]byte, 50), 0o644)
	if n := FolderSize(dir); n != 150 {
		t.Fatalf("size %d", n)
	}
	if n := FolderSize(filepath.Join(dir, "none")); n != 0 {
		t.Fatalf("missing folder %d", n)
	}
}
