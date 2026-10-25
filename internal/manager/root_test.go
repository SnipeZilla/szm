package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
)

// The servers are E:\Counter-Strike 2 - Server 3 and 4; the setting points inside one of them by mistake.
func TestNewServersRoot(t *testing.T) {
	drive := t.TempDir()
	s3, s4 := filepath.Join(drive, "Counter-Strike 2 - Server 3"), filepath.Join(drive, "Counter-Strike 2 - Server 4")
	os.MkdirAll(filepath.Join(s3, "steamapps"), 0o755)
	cfg := config.Defaults()
	cfg.Servers = []*config.Server{{ID: "a", Name: "CS2 - Server 3", Game: "cs2", InstallDir: s3}, {ID: "b", Name: "CS2 - Server 4", Game: "cs2", InstallDir: s4}}
	cfg.ServersRoot = s3
	cat, _ := games.Load("")
	m := New(cfg, cat, t.TempDir())
	if got := m.NewServersRoot(); got != drive {
		t.Fatalf("root %q, want %q", got, drive)
	}
	cfg.ServersRoot = filepath.Join(drive, "Servers")
	if got := m.NewServersRoot(); got != cfg.ServersRoot {
		t.Fatalf("a good setting is kept: %q", got)
	}
	if !LooksInstalled(s3) || LooksInstalled(s4) {
		t.Fatal("LooksInstalled")
	}
	// A new server inside another one's folder is refused
	in := &config.Server{Name: "x", Game: "cs2", InstallDir: filepath.Join(s3, "CS2 - Server 5"), Port: 27099}
	if errs := cfg.Validate(in, cat); len(errs) == 0 {
		t.Fatal("nested folder accepted")
	}
	in.InstallDir = filepath.Join(drive, "Counter-Strike 2 - Server 5")
	if errs := cfg.Validate(in, cat); len(errs) != 0 {
		t.Fatal(errs)
	}
}
