package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
)

func TestPastKeep(t *testing.T) {
	got := pastKeep([]string{"20261001-050000-daily.zip", "20261003-050000-daily.zip", "20261002-050000-manual.zip"}, 2)
	if len(got) != 1 || got[0] != "20261001-050000-daily.zip" {
		t.Fatal(got)
	}
	if pastKeep([]string{"a"}, 3) != nil {
		t.Fatal("kept too few")
	}
}

// Each backup is copied to a folder destination, and the same number is kept there.
func TestOffsiteFolder(t *testing.T) {
	dir := t.TempDir()
	inst := filepath.Join(dir, "css")
	write(t, inst, "srcds_run", "#!/bin/sh")
	write(t, inst, "srcds_win64.exe", "x")
	write(t, inst, "cstrike/cfg/server.cfg", "hostname A")
	off := filepath.Join(dir, "drive", "SZM backups")
	cfg, _ := config.Load(filepath.Join(dir, "szm.json"))
	cfg.Servers = []*config.Server{{ID: "css1", Name: "Test", Game: "css", InstallDir: inst}}
	cfg.Backups.Keep = 2
	cfg.Backups.Offsite = []config.OffsiteCopy{{Name: "Drive", Kind: "folder", Target: off}}
	cat, _ := games.Load("")
	m := New(cfg, cat, dir)
	prev := ""
	for k := 0; k < 3; k++ {
		if err := m.BackupNow("css1", "test"); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool {
			local := m.Backups("css1")
			there, _ := os.ReadDir(filepath.Join(off, "css1"))
			m.offsite.mu.Lock()
			idle := !m.offsite.running
			m.offsite.mu.Unlock()
			return idle && len(local) == min(k+1, 2) && local[0].Name != prev && len(there) == len(local) && fileExists(filepath.Join(off, "css1", local[0].Name))
		})
		prev = m.Backups("css1")[0].Name
	}
	entries, _ := os.ReadDir(filepath.Join(off, "css1"))
	local := m.Backups("css1")
	if len(entries) != 2 || entries[0].Name() != local[1].Name || entries[1].Name() != local[0].Name {
		t.Fatalf("there: %v, here: %+v", entries, local)
	}
	if st := m.OffsiteStatuses()[0]; st.Error != "" {
		t.Fatal(st.Error)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for k := 0; k < 250; k++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}
