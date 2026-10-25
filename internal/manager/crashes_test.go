package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
)

func TestCrashReport(t *testing.T) {
	dir := t.TempDir()
	inst := filepath.Join(dir, "css")
	_ = os.MkdirAll(filepath.Join(inst, "cstrike"), 0o755)
	log := filepath.Join(inst, "cstrike", "console.log")
	_ = os.WriteFile(log, []byte("old line\nchangelevel cs_office\nHost_Error: something bad"), 0o644)
	cat, _ := games.Load("")
	g := cat["css"]
	m := &Manager{DataDir: dir, Log: events.New(filepath.Join(dir, "logs"))}
	s := config.Server{ID: "a/b", Name: "Test", Game: "css", InstallDir: inst, ConsoleLog: true}
	sn := crashSnap{s: s, g: g, at: time.Now(), since: time.Now().Add(-90 * time.Second), pid: 4242,
		exit: proc.ExitInfo{Known: true, Code: 0xC0000005}, why: "the process ended", inARow: 1, mapName: "de_dust2",
		console:    []string{"── 10:00:00 started: srcds -game css", "before", "── 10:01:00 started: srcds -game css", "Loading map"},
		consoleOff: int64(len("old line\n"))}
	go m.reportCrash(sn, events.Crash, func(c string) string { return "Crashed (" + c + ")." }, "restart in 5s")
	var list []CrashSummary
	for k := 0; k < 50 && len(list) == 0; k++ {
		time.Sleep(20 * time.Millisecond)
		list = m.Crashes("a/b")
	}
	if len(list) != 1 {
		t.Fatal("no report")
	}
	r, err := m.Crash("a/b", list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"── 10:01:00 started: srcds -game css", "Loading map", "changelevel cs_office", "Host_Error: something bad"}
	if strings.Join(r.Console, "|") != strings.Join(want, "|") {
		t.Fatalf("console: %q", r.Console)
	}
	if r.Map != "de_dust2" || r.Uptime < 89 || r.Next != "restart in 5s" {
		t.Fatalf("%+v", r)
	}
	txt := m.viewOf(r).Text()
	if !strings.Contains(txt, "Host_Error: something bad") || !strings.Contains(txt, "Cause") || !strings.Contains(txt, "FOR THE DEVELOPERS") {
		t.Fatal(txt)
	}
	ev := m.Log.Recent("a/b", 1)
	if len(ev) != 1 || ev[0].Report != r.ID || !strings.HasPrefix(ev[0].Text, "Crashed (") {
		t.Fatalf("%+v", ev)
	}
	if _, err := m.Crash("a/b", "../../x"); err == nil {
		t.Fatal("bad id accepted")
	}
}

func TestCrashCause(t *testing.T) {
	r := &CrashReport{Killed: true, Why: "no answer to 3 queries", Windows: []string{"Engine Error: Map not found"}}
	if c := crashCause(r); c != `stuck: no answer to 3 queries, showing "Engine Error: Map not found"` {
		t.Fatal(c)
	}
	r = &CrashReport{ExitCode: "0xC0000005", ExitText: "access violation: it read…"}
	if c := crashCause(r); c != "access violation (0xC0000005)" {
		t.Fatal(c)
	}
}
