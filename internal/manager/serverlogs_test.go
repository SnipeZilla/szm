package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupDue(t *testing.T) {
	wed := time.Date(2026, 10, 14, 6, 0, 0, 0, time.Local) // a Wednesday
	feb := time.Date(2026, 2, 28, 6, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		schedule     string
		weekday, day int
		t            time.Time
		want         string
	}{
		{"manual", 0, 1, wed, ""},
		{"daily", 0, 1, wed, "daily"},
		{"weekly", 3, 1, wed, "weekly"},
		{"weekly", 1, 1, wed, ""},
		{"monthly", 0, 14, wed, "monthly"},
		{"monthly", 0, 15, wed, ""},
		{"monthly", 0, 31, feb, "monthly"}, // the last day of a shorter month
		{"monthly", 0, 30, feb, "monthly"},
		{"monthly", 0, 31, wed, ""},
	} {
		if got := BackupDue(tc.schedule, tc.weekday, tc.day, tc.t); got != tc.want {
			t.Errorf("%s %d %d %s: %q, want %q", tc.schedule, tc.weekday, tc.day, tc.t.Format("Jan 2"), got, tc.want)
		}
	}
}

func TestCleanLogs(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old, recent := now.Add(-40*24*time.Hour), now.Add(-2*24*time.Hour)
	mk := func(rel string, at time.Time) string {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
		os.Chtimes(p, at, at)
		return p
	}
	a := mk("csgo/logs/L001.log", old)
	b := mk("csgo/logs/L002.log", recent)
	c := mk("csgo/addons/sourcemod/logs/errors_1.log", old)
	d := mk("csgo/addons/sourcemod/Logs/sub/x.txt", old) // case, and a folder in it
	e := mk("csgo/cfg/old.cfg", old)                     // not a logs folder
	f := mk("csgo/maps/logs/a.log", old)                 // in a folder never walked
	n, size := CleanLogs(root, 30, now)
	if n != 3 || size != 3 {
		t.Fatal(n, size)
	}
	for _, p := range []string{a, c, d} {
		if fileThere(p) {
			t.Error("still there:", p)
		}
	}
	for _, p := range []string{b, e, f} {
		if !fileThere(p) {
			t.Error("deleted:", p)
		}
	}
	if fileThere(filepath.Dir(d)) || !fileThere(filepath.Join(root, "csgo/addons/sourcemod/Logs")) {
		t.Error("the empty folder in logs goes, the logs folder stays")
	}
	if n, _ := CleanLogs(root, 0, now); n != 0 {
		t.Fatal("0 keeps all")
	}
}

func fileThere(p string) bool { _, err := os.Stat(p); return err == nil }
