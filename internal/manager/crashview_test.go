package manager

import (
	"net"
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
	"github.com/SnipeZilla/szm/internal/query"
)

func viewManager(t *testing.T) *Manager {
	dir := t.TempDir()
	m := &Manager{DataDir: dir, Log: events.New(filepath.Join(dir, "logs")), inst: map[string]*Instance{}}
	cat, _ := games.Load("")
	m.SetCatalog(cat)
	return m
}

func cs2Crash(at time.Time, sure, suspect string) *CrashReport {
	d := &crashinfo.DumpAnalysis{File: "cs2.exe.1.dmp", Arch: "x64", Code: "0xC0000005", Exception: "access violation: it tried to use memory it has no access to",
		Access: "reading a null pointer (address 0x10)", Module: "server.dll", Offset: "0x5A1B2C", Class: "game", Threads: 80,
		Chain: []crashinfo.Frame{{Module: "server.dll", Offset: "0x5A1B2C", Class: "game"},
			{Module: "counterstrikesharp.dll", Offset: "0x1D2E3", Class: "addon", Label: "CounterStrikeSharp"},
			{Module: "engine2.dll", Offset: "0x10203", Class: "game"}},
		Loaded:  []crashinfo.Loaded{{Module: "counterstrikesharp.dll", Label: "CounterStrikeSharp", Version: "1.0.377", Path: `E:\cs2\game\csgo\addons\counterstrikesharp\bin\win64\counterstrikesharp.dll`}},
		Suspect: suspect, Sure: sure, Why: "it crashed in server.dll, the game's code. CounterStrikeSharp was in the call chain"}
	return &CrashReport{ID: at.Format("20060102-150405"), Server: "s1", Name: "CS2 #1", Game: "cs2", Time: at, Uptime: 3600, Map: "de_dust2", Players: 3,
		ExitCode: "0xC0000005", Dump: d,
		Commands: []SentCommand{
			{Time: at.Add(-50 * time.Second), By: "Bob", Command: "say hi"},
			{Time: at.Add(-20 * time.Second), Command: "status", Routine: true},
			{Time: at.Add(-2 * time.Second), By: "Bob", Command: "maps *"},
			{Time: at.Add(-1 * time.Second), Command: "status", Routine: true},
		}}
}

// What the report says must be what the evidence supports: an addon is named as the cause only when the call chain
// proves it (sure) or points at it (likely).
func TestCrashViewPlain(t *testing.T) {
	m := viewManager(t)
	at := time.Now().Add(-time.Hour)
	r := cs2Crash(at, "unsure", "")
	v := m.viewOf(r)
	p := v.Plain
	if p.Sure != "unsure" || strings.Contains(p.Cause, "The cause is") || strings.Contains(p.Cause, "Most likely") {
		t.Fatalf("unsure verdict named a cause: %+v", p)
	}
	if !strings.HasPrefix(p.Cause, "It crashed in server.dll") {
		t.Fatal(p.Cause)
	}
	if len(p.Before) == 0 || !strings.Contains(p.Before[0], `Bob sent the command "maps *"`) {
		t.Fatalf("trigger: %q", p.Before)
	}
	if len(p.Todo) == 0 || !strings.Contains(p.Todo[0], `Do not send "maps *"`) {
		t.Fatalf("todo: %q", p.Todo)
	}
	if p.ReportAt != "https://github.com/ValveSoftware/csgo-osx-linux/issues" || !strings.Contains(p.ReportTo, "if it also crashes without addons") {
		t.Fatalf("report to: %q %q", p.ReportTo, p.ReportAt)
	}
	if !strings.Contains(v.Summary, "Call chain: server.dll+0x5A1B2C < counterstrikesharp.dll+0x1D2E3 < engine2.dll+0x10203") ||
		!strings.Contains(v.Summary, "CounterStrikeSharp 1.0.377") || !strings.Contains(v.Summary, `RCON "maps *"`) {
		t.Fatal(v.Summary)
	}

	r.Logs = []crashinfo.LogTail{{Path: "log-all.txt", Lines: []string{"2026-10-21 19:47:55.010 -04:00 [INFO] (cssharp:PluginContext) Loading plugin HLstatsZ",
		"2026-10-21 19:47:55.110 -04:00 [INFO] (cssharp:PluginContext) Finished loading plugin HLstatsZ Classics"}}}
	r.Dump.Chain[0].Function = "?Split@CSplitString@@AEAAXPEBDHPEAPEBDH_N@Z+0x43"
	v = m.viewOf(r)
	if !strings.Contains(v.Summary, "Plugins:    HLstatsZ Classics") || !strings.Contains(v.Summary, "Function:   CSplitString::Split+0x43") {
		t.Fatal(v.Summary)
	}

	// likely: named, as likely
	r = cs2Crash(at, "likely", "CounterStrikeSharp")
	p = m.viewOf(r).Plain
	if !strings.HasPrefix(p.Cause, "Most likely CounterStrikeSharp") || p.ReportTo != "the author of CounterStrikeSharp" {
		t.Fatalf("%+v", p)
	}

	// no user command in the 30 s before: no trigger
	r = cs2Crash(at, "unsure", "")
	r.Commands = r.Commands[:2]
	if tr := trigger(r); tr != nil {
		t.Fatal("trigger:", tr)
	}
}

func TestCrashHistory(t *testing.T) {
	m := viewManager(t)
	now := time.Now()
	for k, s := range []string{"s1", "s2", "s1"} {
		r := cs2Crash(now.Add(time.Duration(k-3)*24*time.Hour), "unsure", "")
		r.Server, r.Name = s, "CS2 "+s
		if k == 0 {
			r.Changes = []Change{{Time: r.Time.Add(-5 * time.Hour), Kind: "event", Text: "Up to date: CS2 v1.41.9.1 (9m 46s)"}} // a report of before
		}
		r.Signature = signature(r)
		if err := m.saveCrash(r); err != nil {
			t.Fatal(err)
		}
	}
	other := cs2Crash(now.Add(-time.Hour), "unsure", "")
	other.Dump.Offset = "0x1"
	other.Signature = signature(other)
	_ = m.saveCrash(other)

	r := cs2Crash(now, "unsure", "")
	v := m.viewOf(r)
	h := v.History
	if h == nil || h.Count != 3 || strings.Join(h.Servers, ",") != "CS2 s1,CS2 s2" || !strings.HasPrefix(h.After, "Game updated: CS2 v1.41.9.1 (9m 46s) (") {
		t.Fatalf("%+v", h)
	}
	found := false
	for _, b := range v.Plain.Before {
		found = found || strings.Contains(b, "The same crash 3 times") && strings.Contains(b, "The first one kept came after: Game updated")
	}
	if !found {
		t.Fatalf("%q", v.Plain.Before)
	}
	if out := os.Getenv("ZZ"); out != "" {
		_ = os.WriteFile(out, []byte(v.Text()), 0o644)
	}
}

func TestNoSuchMaps(t *testing.T) {
	got := noSuchMaps([]string{"Setting mapgroup to 'mg_bomb'", "map de_dust3:  invalid map name", "map load failed: cs_x not found or invalid",
		"map change failed: 'de_y' not found on server.", "map de_dust3:  invalid map name", "changelevel de_z:  invalid map name"})
	if strings.Join(got, " ") != "de_dust3 cs_x de_y de_z" {
		t.Fatal(got)
	}
}

func TestFoldRepeats(t *testing.T) {
	in := []string{"a", "Long frame: 22.43ms", "Long frame: 31.2ms", "Long frame: 19ms", "Long frame: 40.1ms", "b", "b", "c"}
	got := strings.Join(foldRepeats(in), "|")
	if got != "a|Long frame: 22.43ms|   … 3 more lines like it|b|b|c" {
		t.Fatal(got)
	}
}

// The console says a map is not there: the Maps tab knows it, its activity says it once, a map it played is cleared.
func TestNoMapFromConsole(t *testing.T) {
	m := viewManager(t)
	i := m.instance("s1")
	i.say("Setting mapgroup to 'mg_bomb'", "map de_dust3:  invalid map name", "[rcon] changelevel de_dust3:  invalid map name", "L 10/21/2026 - 20:33:01: map load failed: cs_x not found or invalid")
	got := m.NoSuchMaps("s1")
	if len(got) != 2 || got["de_dust3"].IsZero() || got["cs_x"].IsZero() {
		t.Fatal(got)
	}
	m.sayNoMaps(i, "s1", "S1")
	m.sayNoMaps(i, "s1", "S1")
	if ev := m.Log.Recent("s1", 10); len(ev) != 2 || !strings.HasPrefix(ev[0].Text, "Map ") {
		t.Fatalf("%+v", ev)
	}
	i.mu.Lock()
	i.info = &query.Info{Map: "CS_X"}
	i.mu.Unlock()
	if got := m.NoSuchMaps("s1"); len(got) != 1 {
		t.Fatal(got)
	}
}

// Started on a map it does not have: it never came up, and the report says why (a real CS2 report).
func TestNoStartMapReport(t *testing.T) {
	m := viewManager(t)
	at := time.Now()
	r := &CrashReport{ID: at.Format("20060102-150405"), Server: "s1", Name: "CS2 - Server 1", Game: "cs2", Time: at, Uptime: 362, Killed: true,
		Why: "no answer in the 6m 00s after it started", MapChange: true,
		Console: []string{"Source2Init OK", "Host activate: Idle (console)", "Setting mapgroup to 'mg_bomb'", "map de_dust3:  invalid map name"}}
	p := m.viewOf(r).Plain
	if !strings.Contains(p.Headline, "never came up") || p.Sure != "sure" || !strings.Contains(p.Cause, "the map de_dust3 does not exist") ||
		len(p.Todo) != 1 || !strings.HasPrefix(p.Todo[0], "Change its start map") {
		t.Fatalf("%+v", p)
	}
	for _, b := range p.Before {
		if strings.Contains(b, "changing map") || strings.Contains(b, "de_dust3") {
			t.Fatal("said twice, or wrong:", b)
		}
	}
}

// The manager ends a server whose start map its console says is not there, without waiting, and does not restart it.
func TestStartMapGone(t *testing.T) {
	i := newInstance("s1")
	s := config.Server{ID: "s1", Map: "de_dust3"}
	i.mu.Lock()
	i.status, i.since = StatusStarting, time.Now().Add(-time.Second)
	i.mu.Unlock()
	i.say("map de_dust2:  invalid map name")
	i.mu.Lock()
	if got := i.startMapGoneLocked(s); got != "" {
		t.Fatal(got)
	}
	i.mu.Unlock()
	i.say("map de_dust3:  invalid map name")
	i.mu.Lock()
	defer i.mu.Unlock()
	if got := i.startMapGoneLocked(s); got != "de_dust3" {
		t.Fatal(got)
	}
	i.status = StatusRunning
	if got := i.startMapGoneLocked(s); got != "" {
		t.Fatal("running:", got)
	}
}

func TestReportSystem(t *testing.T) {
	m := viewManager(t)
	r := cs2Crash(time.Now(), "unsure", "")
	r.OS, r.Manager = "Windows Server 2025 Datacenter 24H2 (build 26100.4061)", "2.0.0"
	v := m.viewOf(r)
	if !strings.Contains(v.Summary, "System:     Windows Server 2025 Datacenter 24H2 (build 26100.4061)") || !strings.Contains(v.Summary, "Manager:    SnipeZilla Server Manager 2.0.0") ||
		!strings.Contains(v.Text(), "System:    Windows Server 2025") {
		t.Fatal(v.Summary)
	}
	if !strings.Contains(proc.OSName(), "(") {
		t.Fatal("os name:", proc.OSName())
	}
}

func TestPublicIP(t *testing.T) {
	for ip, want := range map[string]bool{"69.169.101.210": true, "10.0.0.5": false, "192.168.1.20": false, "172.20.0.3": false,
		"172.32.0.1": true, "100.64.1.1": false, "100.128.0.1": true, "127.0.0.1": false, "169.254.3.4": false, "0.0.0.0": false} {
		if publicIP(net.ParseIP(ip)) != want {
			t.Error(ip, !want)
		}
	}
}

// A server that could not start (0xC0000135): the DLL it did not find is said, with what it is part of.
func TestMissingDLLView(t *testing.T) {
	r := &CrashReport{Name: "DayZ - Server 1", Game: "dayz", Uptime: 0, ExitCode: "0xC0000135", Exit: &proc.ExitInfo{Known: true, Code: 0xC0000135},
		ExitText: "a DLL it needs is missing (MFReadWrite.dll)", MissingSys: "System32",
		Missing: []MissingDLL{{Name: "MFReadWrite.dll", Part: "Media Foundation", Hint: "it is part of Media Foundation, which Windows Server does not have by default: add it with Install-WindowsFeature Server-Media-Foundation (PowerShell, as administrator), then restart Windows"}}}
	p := plainOf(r, nil, nil)
	t.Log(p.Headline, "|", p.Cause, "|", p.Todo)
	if p.Sure != "sure" || !strings.Contains(p.Headline, "could not start") || !strings.Contains(p.Cause, "Its program needs MFReadWrite.dll, which is not in its folder, in System32 or on the PATH, so it ended before it ran.") ||
		!strings.Contains(p.Cause, "MFReadWrite.dll is part of Media Foundation, which Windows Server") || len(p.Todo) != 1 || !strings.Contains(p.Todo[0], "Install Media Foundation on this machine (above), then click Start: until then the manager does not start it again") {
		t.Fatal(p)
	}
	s := summaryOf(r, nil, "DayZ")
	if !strings.Contains(s, "Crash:      exit 0xC0000135, a DLL it needs is missing (MFReadWrite.dll) (no crash dump)") {
		t.Fatal(s)
	}
	v := &CrashView{CrashReport: r, Plain: p, Summary: s}
	if txt := v.Text(); !strings.Contains(txt, "Missing:   MFReadWrite.dll (it is part of Media Foundation") {
		t.Fatal(txt)
	}
	// a file of the game: put back
	r.Missing = []MissingDLL{{Name: "steam_api64.dll"}}
	if p := plainOf(r, nil, nil); len(p.Todo) != 1 || !strings.Contains(p.Todo[0], "Put back steam_api64.dll: Verify files") {
		t.Fatal(p.Todo)
	}
	// not known which: what to do to know
	r.Missing, r.ExitText = nil, "a DLL it needs is missing (an addon, or the Visual C++ runtime)"
	p = plainOf(r, nil, nil)
	if !strings.Contains(p.Cause, "Which one is not known") || len(p.Todo) != 1 || !strings.Contains(p.Todo[0], "double-click") {
		t.Fatal(p)
	}
}

// A DLL Windows lacks (DirectX, Visual C++…): the server waits for it to be installed; one of the game's or an
// addon's: no.
func TestCantStartFor(t *testing.T) {
	dx := MissingDLL{Name: "XAPOFX1_5.dll", Part: "the DirectX End-User Runtime (June 2010)"}
	vc := MissingDLL{Name: "VCRUNTIME140.dll", Part: "the Visual C++ 2015-2022 runtime (x64)"}
	x3d := MissingDLL{Name: "X3DAudio1_7.dll", Part: dx.Part}
	cant, what := cantStartFor([]MissingDLL{dx})
	if cant != "Windows does not have XAPOFX1_5.dll, which it needs: install the DirectX End-User Runtime (June 2010) on this machine, then start it." || what != dx.Part {
		t.Fatal(cant, what)
	}
	if cant, _ := cantStartFor([]MissingDLL{dx, x3d, vc}); cant != "Windows does not have XAPOFX1_5.dll, X3DAudio1_7.dll and VCRUNTIME140.dll, which it needs: install the DirectX End-User Runtime (June 2010) and the Visual C++ 2015-2022 runtime (x64) on this machine, then start it." {
		t.Fatal(cant)
	}
	if cant, _ := cantStartFor([]MissingDLL{dx, {Name: "steam_api64.dll"}}); cant != "" {
		t.Fatal(cant)
	}
	if cant, _ := cantStartFor(nil); cant != "" {
		t.Fatal(cant)
	}
}
