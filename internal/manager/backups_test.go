package manager

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	inst := filepath.Join(dir, "css")
	game := filepath.Join(inst, "cstrike")
	for rel, body := range map[string]string{
		"cfg/server.cfg":                           "hostname A",
		"mapcycle.txt":                             "de_dust2",
		"addons/sourcemod/plugins/x.smx":           "plugin",
		"addons/sourcemod/logs/errors.log":         "log",
		"addons/counterstrikesharp/dotnet/big.dll": "runtime",
		"maps/de_dust2.bsp":                        "map",
		"demo.dem":                                 "demo",
	} {
		write(t, game, rel, body)
	}
	cfg := config.Defaults()
	cfg.Backups.Keep = 2
	m := &Manager{DataDir: dir, Cfg: cfg, Log: events.New(filepath.Join(dir, "logs"))}
	cat, _ := games.Load("")
	g := cat["css"]
	s := config.Server{ID: "css1", Name: "Test", Game: "css", InstallDir: inst}
	i := newInstance("css1")

	b, err := m.backup(i, s, g, "manual")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := m.BackupFiles("css1", b.Name)
	sort.Strings(files)
	want := "addons/sourcemod/plugins/x.smx|cfg/server.cfg|mapcycle.txt"
	if strings.Join(files, "|") != want {
		t.Fatalf("backed up %v", files)
	}

	// a bad change, then the restore puts the files back (and keeps what the backup does not have)
	write(t, game, "cfg/server.cfg", "hostname BROKEN")
	write(t, game, "addons/sourcemod/plugins/new.smx", "new")
	if _, err := restoreZip(context.Background(), filepath.Join(m.BackupsDir("css1"), b.Name), game); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(game, "cfg", "server.cfg")); string(got) != "hostname A" {
		t.Fatalf("restored %q", got)
	}
	if !fileExists(filepath.Join(game, "addons/sourcemod/plugins/new.smx")) {
		t.Fatal("a file the backup does not have was removed")
	}

	// the newest are kept
	for k := 0; k < 3; k++ {
		if _, err := m.backup(i, s, g, "daily"); err != nil {
			t.Fatal(err)
		}
	}
	if l := m.Backups("css1"); len(l) != 2 || l[0].Reason != "daily" {
		t.Fatalf("kept %+v", l)
	}
	if _, err := m.BackupPath("css1", "../szm.json"); err == nil {
		t.Fatal("a path outside the backups")
	}
}

func TestRestoreRefusesEscapes(t *testing.T) {
	// a zip whose entry climbs out of the folder stays inside it
	dir := t.TempDir()
	zp := filepath.Join(dir, "evil.zip")
	f, _ := os.Create(zp)
	zw := newZip(f)
	w, _ := zw.Create("../outside.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	root := filepath.Join(dir, "game")
	_ = os.MkdirAll(root, 0o755)
	_, _ = restoreZip(context.Background(), zp, root)
	if fileExists(filepath.Join(dir, "outside.txt")) {
		t.Fatal("file written outside")
	}
}

func newZip(f *os.File) *zip.Writer { return zip.NewWriter(f) }

func TestDailyBackup(t *testing.T) {
	dir := t.TempDir()
	inst := filepath.Join(dir, "css")
	write(t, inst, "srcds_run", "#!/bin/sh")
	write(t, inst, "srcds_win64.exe", "x")
	write(t, inst, "cstrike/cfg/server.cfg", "hostname A")
	cfg, _ := config.Load(filepath.Join(dir, "szm.json"))
	cfg.Servers = []*config.Server{{ID: "css1", Name: "Test", Game: "css", InstallDir: inst}}
	cfg.Backups.Schedule = "daily"
	cat, _ := games.Load("")
	m := New(cfg, cat, dir)
	day := func(h, mi int) time.Time {
		n := time.Now()
		return time.Date(n.Year(), n.Month(), n.Day(), h, mi, 0, 0, time.Local)
	}
	wait := func() {
		for k := 0; k < 100; k++ {
			m.backupMu.Lock()
			r := m.backupRunning
			m.backupMu.Unlock()
			if !r {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	m.scheduledBackups(day(5, 29)) // before the time
	wait()
	if len(m.Backups("css1")) != 0 {
		t.Fatal("too early")
	}
	m.scheduledBackups(day(5, 31))
	wait()
	m.scheduledBackups(day(5, 45)) // once a day
	wait()
	if l := m.Backups("css1"); len(l) != 1 || l[0].Reason != "daily" {
		t.Fatalf("%+v", l)
	}
	// the manager restarted the same day: not again
	m2 := New(cfg, cat, dir)
	m2.scheduledBackups(day(6, 0))
	for k := 0; k < 50; k++ {
		time.Sleep(20 * time.Millisecond)
	}
	if len(m2.Backups("css1")) != 1 {
		t.Fatal("backed up twice the same day")
	}
}
