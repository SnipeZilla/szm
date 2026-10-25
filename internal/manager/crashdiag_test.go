package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/crashinfo"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
)

func TestCrashDiagnosis(t *testing.T) {
	dir := t.TempDir()
	inst := filepath.Join(dir, "css")
	now := time.Now()
	since := now.Add(-time.Hour)
	file := func(rel, content string, at time.Time) {
		p := filepath.Join(inst, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(content), 0o644)
		_ = os.Chtimes(p, at, at)
	}
	file("srcds.exe", "x", now.Add(-30*24*time.Hour))
	sm := func(t time.Time) string { return t.Format("01/02/2006 - 15:04:05") }
	// a log of an earlier run, then this run's lines: only this run's are kept
	file("cstrike/logs/L1014000.log", "L "+sm(since.Add(-time.Hour))+": earlier run\nL "+sm(now.Add(-10*time.Second))+": \"Bob<2>\" say \"hi\"\nL "+sm(now.Add(-5*time.Second))+": Loading map \"de_nuke\"", now)
	file("cstrike/addons/sourcemod/logs/errors_20261014.log", "L "+sm(now.Add(-3*time.Minute))+": [SM] Exception reported: Invalid client index 0\n[SM] Blaming: funcommands.smx", now.Add(-2*time.Minute))
	file("cstrike/addons/sourcemod/logs/L20261001.log", "old run", now.Add(-13*24*time.Hour))
	file("cstrike/addons/sourcemod/plugins/newplugin.smx", "smx", since.Add(-3*time.Hour))    // changed before the run
	file("cstrike/cfg/server.cfg", "hostname x", since.Add(-2*24*time.Hour))                  // changed before the run
	file("cstrike/addons/sourcemod/plugins/old.smx", "smx", now.Add(-40*24*time.Hour))        // long ago
	file("cstrike/addons/sourcemod/data/sqlite/clientprefs.sq3", "db", since.Add(-time.Hour)) // data: not a change
	file("cstrike/cfg/runtime.cfg", "x", now.Add(-time.Minute))                               // during the run

	cat, _ := games.Load("")
	m := &Manager{DataDir: dir, Log: events.New(filepath.Join(dir, "logs"))}
	m.SetCatalog(cat)
	ev, _ := json.Marshal(events.Event{Time: since.Add(-30 * time.Minute), Kind: events.Updated, Server: "c1", Name: "Test", Text: "Updated to build 9540945"})
	_ = os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "logs", since.Add(-30*time.Minute).Format("2006-01-02")+".log"), append(ev, '\n'), 0o644)
	_ = os.MkdirAll(m.DumpsDir(), 0o755)
	b, _ := os.ReadFile("testdata/srcds.exe.4242.dmp")
	_ = os.WriteFile(filepath.Join(m.DumpsDir(), "srcds.exe.4242.dmp"), b, 0o644)
	_ = os.WriteFile(filepath.Join(m.DumpsDir(), "srcds.exe.999.dmp"), b, 0o644) // another server's

	s := config.Server{ID: "c1", Name: "Test", Game: "css", InstallDir: inst}
	sn := crashSnap{s: s, g: cat["css"], at: now, since: since, pid: 4242, exit: proc.ExitInfo{Known: true, Code: 0xC0000005},
		console: []string{"── 20:00:00 started: srcds", "Changelevel to de_nuke", "Loading map \"de_nuke\""}}
	r := &CrashReport{Server: s.ID, Name: s.Name, Time: now, PID: 4242}
	r.Console = sn.console
	r.MapChange = afterMapChange(r.Console)
	r.Changes = m.crashChanges(sn)
	m.gatherSystem(r, sn)
	finishCrash(r)

	if len(r.Dumps) != 1 || !strings.HasSuffix(r.Dumps[0].Path, "srcds.exe.4242.dmp") {
		t.Fatalf("dumps: %+v", r.Dumps)
	}
	if r.Dump == nil || r.Dump.Module != "server.dll" || r.Suspect != "" || r.Dump.Suspect != "SourceMod extension sdkhooks" || r.Dump.Sure != "unsure" {
		t.Fatalf("dump: %+v suspect %q", r.Dump, r.Suspect)
	}
	if !strings.HasPrefix(r.Cause, "crashed in server.dll, access violation reading a null pointer (address 0x00000024)") || strings.Contains(r.Cause, "sdkhooks") {
		t.Fatal(r.Cause)
	}
	if r.Signature != "server.dll+0x123456" || !r.MapChange {
		t.Fatal(r.Signature, r.MapChange)
	}
	var logs []string
	for _, l := range r.Logs {
		logs = append(logs, filepath.Base(l.Path))
	}
	if strings.Join(logs, " ") != "L1014000.log errors_20261014.log" {
		t.Fatal("logs:", logs)
	}
	for _, l := range r.Logs[0].Lines {
		if strings.Contains(l, "earlier run") {
			t.Fatal("a line of an earlier run kept:", r.Logs[0].Lines)
		}
	}
	var ch []string
	for _, c := range r.Changes {
		ch = append(ch, c.Text)
	}
	got := strings.Join(ch, " | ")
	if !strings.Contains(got, "cstrike/addons/sourcemod/plugins/newplugin.smx") || !strings.Contains(got, "cstrike/cfg/server.cfg") ||
		!strings.Contains(got, "Updated to build 9540945") {
		t.Fatal("changes:", got)
	}
	for _, no := range []string{"old.smx", "clientprefs", "runtime.cfg"} {
		if strings.Contains(got, no) {
			t.Fatal("not a change:", no, got)
		}
	}
	if out := os.Getenv("SZM_REPORT_OUT"); out != "" {
		r.ID, r.Map, r.Players, r.MaxPlayers, r.Uptime, r.ExitCode, r.ExitText = "20261014-210000", "de_nuke", 7, 24, 3600, "0xC0000005", "access violation"
		jb, _ := json.MarshalIndent(r, "", "  ")
		_ = os.WriteFile(out, jb, 0o644)
	}
	txt := m.viewOf(r).Text()
	if f := os.Getenv("ZZ"); f != "" {
		_ = os.WriteFile(f, []byte(txt), 0o644)
	}
	if !strings.Contains(txt, "Cause (not sure)") || !strings.Contains(txt, "sdkhooks was found on the crashed thread's stack") || !strings.Contains(txt, "Changed in the week before this run") {
		t.Fatal(txt)
	}
	if progs := m.dumpPrograms(); !strings.Contains(strings.Join(progs, ","), "srcds.exe") || !strings.Contains(strings.Join(progs, ","), "cs2.exe") {
		t.Fatal(progs)
	}
}

func TestTidyChanges(t *testing.T) {
	at := time.Date(2026, 10, 15, 6, 46, 0, 0, time.Local)
	push := time.Date(2026, 10, 14, 16, 46, 30, 0, time.Local)
	list := []Change{
		{Time: at, Kind: "event", Text: "SnipeZilla uploaded tickrate_x86-32.dll, tickrate_x86-32.vdf into /cstrike/addons"},
		{Time: at, Kind: "file", Text: "cstrike/addons/tickrate_x86-32.dll"},
		{Time: at, Kind: "file", Text: "cstrike/addons/tickrate_x86-32.vdf"},
		{Time: push, Kind: "event", Text: "Addons up to date (sourcemod): 1074 files copied (96.4 MB), 40 configs kept (11s)"},
		{Time: push.Add(-20 * time.Second), Kind: "file", Text: "cstrike/addons/sourcemod/gamedata/core.games/master.games.txt"},
		{Time: push.Add(-25 * time.Second), Kind: "file", Text: "cstrike/addons/sourcemod/gamedata/sdkhooks.games/game.tf2classic.txt"},
		{Time: at.Add(-3 * time.Hour), Kind: "file", Text: "cstrike/cfg/a.cfg"},
		{Time: at.Add(-3 * time.Hour), Kind: "file", Text: "cstrike/cfg/b.cfg"},
		{Time: at.Add(-3 * time.Hour), Kind: "file", Text: "cstrike/cfg/c.cfg"},
		{Time: at.Add(-3 * time.Hour), Kind: "file", Text: "cstrike/cfg/d.cfg"},
		{Time: at.Add(-5 * time.Hour), Kind: "file", Text: "cstrike/cfg/server.cfg"},
	}
	var got []string
	for _, c := range tidyChanges(list) {
		got = append(got, c.Text)
	}
	want := "SnipeZilla uploaded tickrate_x86-32.dll, tickrate_x86-32.vdf into /cstrike/addons | cstrike/cfg/: 4 files | cstrike/cfg/server.cfg | Addons up to date (sourcemod): 1074 files copied (96.4 MB), 40 configs kept (11s)"
	if strings.Join(got, " | ") != want {
		t.Fatal(strings.Join(got, " | "))
	}
	for _, s := range []string{"Logs older than 14 days deleted: 2 files (0 B)", "SnipeZilla deleted 23 files in cstrike/addons/sourcemod/logs"} {
		if !noChangeEventRe.MatchString(s) {
			t.Error("kept:", s)
		}
	}
	if noChangeEventRe.MatchString("SnipeZilla deleted cstrike/addons/metamod.vdf.00") {
		t.Error("a real change dropped")
	}
}

func TestNextMap(t *testing.T) {
	at := time.Date(2026, 10, 15, 19, 51, 18, 0, time.Local)
	r := &CrashReport{Time: at, Logs: []crashinfo.LogTail{{Lines: []string{
		`L 10/15/2026 - 19:19:27: -------- Mapchange to awp_roofcastles --------`,
		`L 10/15/2026 - 19:46:31: [mapchooser.smx] Voting for next map has started.`,
		`L 10/15/2026 - 19:51:16: [SM] Changed map to "css_aim_ag_texture2"`,
	}}}, Console: []string{"Going to intermission...", "CHANGE LEVEL: cs_xfx_casino", "---- Host_Changelevel ----"}}
	if m := nextMap(r); m != "css_aim_ag_texture2" {
		t.Fatal(m)
	}
	r.Time = at.Add(time.Hour) // SourceMod's line is old: the console's
	if m := nextMap(r); m != "cs_xfx_casino" {
		t.Fatal(m)
	}
	finishCrash(r)
	if !r.MapChange || r.NextMap != "cs_xfx_casino" {
		t.Fatal(r.MapChange, r.NextMap)
	}
}

func TestLogsWithoutCopies(t *testing.T) {
	all := crashinfo.LogTail{Path: "log-all.txt", Lines: []string{"a", "plugin: x", "b"}}
	cs := crashinfo.LogTail{Path: "log-cssharp.txt", Lines: []string{"a", "b"}}
	other := crashinfo.LogTail{Path: "errors.log", Lines: []string{"c"}}
	got := withoutCopies([]crashinfo.LogTail{cs, all, other})
	if len(got) != 2 || got[0].Path != "log-all.txt" || got[1].Path != "errors.log" {
		t.Fatalf("%+v", got)
	}
	// two identical: one kept
	if got := withoutCopies([]crashinfo.LogTail{cs, cs}); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestGameUpdateOneLine(t *testing.T) {
	at := time.Date(2026, 10, 19, 18, 54, 0, 0, time.Local)
	list := []Change{{Time: at, Kind: "event", Text: "Up to date: CS2 v1.41.9.1 (9m 46s)"}}
	for k := 0; k < 12; k++ {
		list = append(list, Change{Time: at.Add(-time.Duration(k) * time.Minute), Kind: "file", Text: fmt.Sprintf("game/csgo/panorama/videos/v%d.webm", k)})
	}
	list = append(list, Change{Time: at.Add(2 * time.Minute), Kind: "event", Text: "SnipeZilla (snipezilla) deleted game/csgo/addons/counterstrikesharp/plugins/HLstatsZ"})
	got := tidyChanges(list)
	if len(got) != 2 || got[1].Text != "Game updated: CS2 v1.41.9.1 (9m 46s), 12 files changed" {
		t.Fatalf("%+v", got)
	}
}
