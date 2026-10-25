package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/crashinfo"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
)

// A crash report: everything known when a server crashed (or was ended, stuck), kept in <data>/crashes/<server>/.

// CrashReport is what is known of a crash.
type CrashReport struct {
	ID      string    `json:"id"` // 20261013-201502
	Server  string    `json:"server"`
	Name    string    `json:"name"`
	Game    string    `json:"game"`
	Time    time.Time `json:"time"`
	Started time.Time `json:"started"`
	Uptime  int64     `json:"uptime"` // seconds
	Cause   string    `json:"cause"`  // in a few words: what happened
	Why     string    `json:"why"`    // what the manager saw: "the process ended", "no answer to 3 queries"
	Killed  bool      `json:"killed"` // the manager ended it: it was stuck
	InARow  int       `json:"in_a_row"`
	Next    string    `json:"next"` // what the manager does now: "restart in 5s", "gave up"

	PID      int            `json:"pid"`
	Program  string         `json:"program,omitempty"`
	Exit     *proc.ExitInfo `json:"exit,omitempty"`
	ExitCode string         `json:"exit_code,omitempty"` // 0xC0000005, 139, SIGSEGV
	ExitText string         `json:"exit_text,omitempty"` // its meaning
	// Missing: the DLLs Windows did not find when it could not start (0xC0000135), and where each comes from
	Missing    []MissingDLL             `json:"missing_dlls,omitempty"`
	MissingSys string                   `json:"missing_sys,omitempty"` // where Windows' DLLs were looked for: System32, SysWOW64
	Windows    []string                 `json:"windows,omitempty"`     // error windows it showed (stuck)
	System     []crashinfo.SystemRecord `json:"system,omitempty"`
	Dumps      []crashinfo.DumpFile     `json:"dumps,omitempty"`
	Logs       []crashinfo.LogTail      `json:"logs,omitempty"`

	Hostname   string   `json:"hostname,omitempty"`
	Map        string   `json:"map,omitempty"`
	Players    int      `json:"players"`
	Bots       int      `json:"bots"`
	MaxPlayers int      `json:"max_players,omitempty"`
	PlayerList []string `json:"player_list,omitempty"`
	Version    string   `json:"version,omitempty"`
	CPU        float64  `json:"cpu"`
	Memory     uint64   `json:"memory"`
	// OS: the system it ran on ("Windows Server 2025 Datacenter 24H2 (build 26100.4061)"); Manager: the manager's version
	OS      string `json:"os,omitempty"`
	Manager string `json:"manager,omitempty"`

	Commands    []SentCommand `json:"commands,omitempty"` // sent through RCON in the 5 minutes before (this run)
	Console     []string      `json:"console,omitempty"`  // its last lines
	ConsoleNote string        `json:"console_note,omitempty"`
	Notes       []string      `json:"notes,omitempty"`

	Dump       *crashinfo.DumpAnalysis `json:"dump,omitempty"`        // the crash dump, read
	Changes    []Change                `json:"changes,omitempty"`     // what changed in the week before the run
	Signature  string                  `json:"signature,omitempty"`   // the same crash again has the same: server.dll+0x1A2B3C
	Suspect    string                  `json:"suspect,omitempty"`     // what most likely caused it
	SuspectWhy string                  `json:"suspect_why,omitempty"` // and why
	MapChange  bool                    `json:"map_change,omitempty"`  // it was changing map
	NextMap    string                  `json:"next_map,omitempty"`    // ... to this one
}

// CrashSummary is a line of the list of a server's crashes.
type CrashSummary struct {
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	Cause  string    `json:"cause"`
	Uptime int64     `json:"uptime"`
	Map    string    `json:"map,omitempty"`
	Killed bool      `json:"killed"`

	Signature string `json:"signature,omitempty"`
	Suspect   string `json:"suspect,omitempty"`
	MapChange bool   `json:"map_change,omitempty"`
	NextMap   string `json:"next_map,omitempty"`
	Players   int    `json:"players"` // people on (bots aside)
}

const (
	keepCrashes  = 30  // reports kept per server
	consoleLines = 200 // last lines of the console kept in a report
)

// crashSnap is what reapLocked notes of a crashed server (the lock held), for the report made after.
type crashSnap struct {
	s          config.Server
	g          *games.Game
	at         time.Time
	since      time.Time
	pid        int
	exit       proc.ExitInfo
	why        string
	killed     bool
	windows    []string
	console    []string
	consoleOff int64
	hostname   string
	mapName    string
	players    int
	bots       int
	maxPlayers int
	names      []string
	version    string
	cpu        float64
	mem        uint64
	inARow     int
	commands   []SentCommand
	// the DLLs Windows did not find (0xC0000135), when already looked for
	missing     []MissingDLL
	missingSys  string
	missingDone bool
}

func (m *Manager) crashDir(id string) string {
	return filepath.Join(m.DataDir, "crashes", safeName(id))
}

var unsafeNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func safeName(s string) string { return unsafeNameRe.ReplaceAllString(s, "_") }

// reportCrash makes the crash report, then writes the event (kind; text says it, given the cause) with it. What the
// system writes a little later (Windows' record, a crash dump) is added after.
func (m *Manager) reportCrash(sn crashSnap, kind string, text func(cause string) string, next string) {
	r := &CrashReport{
		ID: sn.at.Format("20060102-150405"), Server: sn.s.ID, Name: sn.s.Name, Game: sn.s.Game, Time: sn.at,
		Started: sn.since, Why: sn.why, Next: next, Killed: sn.killed, InARow: sn.inARow, PID: sn.pid, Windows: sn.windows,
		Hostname: sn.hostname, Map: sn.mapName, Players: sn.players, Bots: sn.bots, MaxPlayers: sn.maxPlayers,
		PlayerList: sn.names, Version: sn.version, CPU: sn.cpu, Memory: sn.mem, Commands: sn.commands,
		OS: proc.OSName(), Manager: Version,
	}
	if !sn.since.IsZero() {
		r.Uptime = int64(sn.at.Sub(sn.since).Seconds())
	}
	r.Program, _ = m.exePath(sn.s, sn.g)
	if sn.exit.Known {
		e := sn.exit
		r.Exit = &e
		win := runtime.GOOS == "windows"
		switch {
		case e.Signal != "":
			r.ExitCode = e.Signal
		case win && uint32(e.Code) >= 0x40000000:
			r.ExitCode = crashinfo.Hex(e.Code)
		default:
			r.ExitCode = fmt.Sprint(e.Code)
		}
		r.ExitText = crashinfo.Explain(e.Code, e.Signal, e.Core, win)
		if sn.killed {
			r.ExitText = "ended by the manager (it was stuck)"
		} else if win && uint32(e.Code) == 0xC0000135 && r.Program != "" {
			if sn.missingDone {
				r.Missing, r.MissingSys = sn.missing, sn.missingSys
			} else {
				r.Missing, r.MissingSys = missingDLLs(r.Program, sn.g, sn.s.InstallDir)
			}
			if len(r.Missing) > 0 {
				names := make([]string, len(r.Missing))
				for k, d := range r.Missing {
					names[k] = d.Name
				}
				r.ExitText = "a DLL it needs is missing (" + strings.Join(names, ", ") + ")"
			}
		}
	} else if !sn.killed {
		r.Notes = append(r.Notes, "How it ended is not known: the manager did not start it (it was running already, or as another Windows user). Restarted by the manager, it is known next time.")
	}
	r.Console, r.ConsoleNote = m.crashConsole(sn)
	r.MapChange = afterMapChange(r.Console)
	r.Changes = m.crashChanges(sn)
	m.gatherSystem(r, sn)
	finishCrash(r)
	_ = m.saveCrash(r)
	m.Log.AddReport(r.ID, kind, sn.s.ID, sn.s.Name, "%s", text(r.Cause))

	// Windows writes its record and the crash dump a moment after the process ended
	time.Sleep(20 * time.Second)
	before := len(r.System) + len(r.Dumps) + len(r.Logs)
	hadDump := r.Dump != nil
	m.gatherSystem(r, sn)
	if len(r.System)+len(r.Dumps)+len(r.Logs) != before || (r.Dump != nil) != hadDump {
		finishCrash(r)
		_ = m.saveCrash(r)
	}
	m.addonCrashed(r, kind) // an addon to blame: watch for its fix
}

// MissingDLL: a DLL Windows did not find, and where it comes from ("" when not known)
type MissingDLL struct {
	Name string `json:"name"`
	Hint string `json:"hint,omitempty"`
	Part string `json:"part,omitempty"` // what it is part of, to install on Windows: "the DirectX End-User Runtime (June 2010)"
}

// cantStartFor: when the DLLs Windows did not find are all part of something to install on Windows, why the server
// can not start until then, and what to install ("" when one is a file of the game or an addon).
func cantStartFor(missing []MissingDLL) (cant, what string) {
	parts := installParts(missing)
	if parts == nil {
		return "", ""
	}
	names := make([]string, len(missing))
	for k, d := range missing {
		names[k] = d.Name
	}
	what = andList(parts)
	return "Windows does not have " + andList(names) + ", which it needs: install " + what + " on this machine, then start it.", what
}

// installParts: what to install on Windows for the DLLs it did not find, when each is part of something to install
// (nil when one is not: a file of the game or of an addon, which checking its files may bring back).
func installParts(missing []MissingDLL) []string {
	var parts []string
	for _, d := range missing {
		if d.Part == "" {
			return nil
		}
		if !slices.Contains(parts, d.Part) {
			parts = append(parts, d.Part)
		}
	}
	return parts
}

// missingDLLs: the DLLs a program that could not start imports and Windows does not find (where Windows looks: its
// folder, System32, Windows, the PATH it was started with).
func missingDLLs(exe string, g *games.Game, dir string) ([]MissingDLL, string) {
	path := os.Getenv("PATH")
	if g == nil {
		g = &games.Game{}
	}
	for _, kv := range gameEnv(g, dir) {
		if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, "PATH") {
			path = v
		}
	}
	windir := os.Getenv("SystemRoot")
	if windir == "" {
		windir = `C:\Windows`
	}
	x64, server := crashinfo.IsX64(exe), strings.Contains(proc.OSName(), "Server")
	var out []MissingDLL
	for _, name := range crashinfo.MissingDLLs(exe, windir, filepath.SplitList(path)) {
		out = append(out, MissingDLL{Name: name, Hint: crashinfo.DLLHint(name, x64, server), Part: crashinfo.DLLPart(name, x64)})
	}
	return out, filepath.Base(crashinfo.SystemDir(exe, windir))
}

// finishCrash says what happened and what most likely caused it, from all that is known.
func finishCrash(r *CrashReport) {
	if m := nextMap(r); m != "" {
		r.NextMap, r.MapChange = m, true
	}
	r.Suspect, r.SuspectWhy = suspectOf(r)
	r.Cause = crashCause(r)
	r.Signature = signature(r)
}

// gatherSystem adds what the system and the server's files tell: Windows' / the kernel's record, crash dumps, the
// error logs of SourceMod and CounterStrikeSharp.
func (m *Manager) gatherSystem(r *CrashReport, sn crashSnap) {
	names := programNames(sn.g, r.Program)
	if recs := crashinfo.SystemRecords(sn.pid, names, sn.since); len(recs) > 0 {
		r.System = recs
	}
	dirs := []string{sn.s.InstallDir, sn.g.Path(sn.s.InstallDir, sn.g.GameDir), sn.g.Path(sn.s.InstallDir, "bin"),
		sn.g.Path(sn.s.InstallDir, "bin/win64"), sn.g.Path(sn.s.InstallDir, "game/bin/win64"),
		sn.g.Path(sn.s.InstallDir, "game/bin/linuxsteamrt64")}
	if r.Program != "" {
		dirs = append(dirs, filepath.Dir(r.Program))
	}
	dirs = append(dirs, m.DumpsDir())
	dirs = append(dirs, crashinfo.CrashDumpDirs()...)
	from := sn.since
	if from.IsZero() {
		from = sn.at.Add(-time.Hour)
	}
	r.Dumps = dumpsOf(crashinfo.FindDumps(dirs, from, time.Now().Add(time.Minute)), sn.pid, m.DumpsDir())
	r.Logs = crashLogs(sn.s.InstallDir, from)
	m.crashDump(r, sn)
}

// programNames are the program names of the game (the ones a crash record can name).
func programNames(g *games.Game, exe string) []string {
	var out []string
	if exe != "" {
		out = append(out, filepath.Base(exe))
	}
	for _, list := range g.Exe {
		for _, e := range list {
			out = append(out, filepath.Base(e))
		}
	}
	if runtime.GOOS != "windows" {
		out = append(out, "srcds_linux", "srcds_linux64", "hlds_linux", "cs2")
	}
	return out
}

// crashConsole gives the last lines of the console, with what the console log had not yet shown (the very last
// lines, written as it crashed).
func (m *Manager) crashConsole(sn crashSnap) ([]string, string) {
	lines := append([]string(nil), sn.console...)
	// this run only: from its "── 17:03:13 started: …" line
	for k := len(lines) - 1; k >= 0; k-- {
		if strings.HasPrefix(lines[k], "── ") && strings.Contains(lines[k], " started: ") {
			lines = lines[k:]
			break
		}
	}
	note := ""
	if sn.s.ConsoleLog {
		if rest := readRest(consolePath(sn.s, sn.g), sn.consoleOff); len(rest) > 0 {
			lines = append(lines, rest...)
		}
	} else {
		note = "Its console log is off (Settings of the server, \"Console log\"): only the manager's own lines are kept. Turn it on to see what the server wrote before it crashed."
	}
	if len(lines) > consoleLines {
		lines = lines[len(lines)-consoleLines:]
	}
	return lines, note
}

// readRest reads a log from an offset to its end, the last line too even without its line end.
func readRest(path string, off int64) []string {
	skipFirst := off < 0
	if skipFirst {
		off = -off
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() <= off {
		return nil
	}
	if st.Size()-off > 1<<20 {
		off, skipFirst = st.Size()-1<<20, true
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}
	b, _ := io.ReadAll(f)
	var out []string
	for k, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
		if k == 0 && skipFirst {
			continue
		}
		if strings.TrimSpace(l) == "" || quietRconRe.MatchString(strings.TrimSpace(l)) {
			continue
		}
		out = append(out, config.Redact(strings.ToValidUTF8(l, "?")))
	}
	return out
}

// crashCause says in a few words what happened, from the best that is known.
func crashCause(r *CrashReport) string {
	if r.Killed {
		c := "stuck: " + r.Why
		if len(r.Windows) > 0 {
			w := r.Windows[0]
			if len(w) > 160 {
				w = w[:160] + "…"
			}
			c += ", showing \"" + w + "\""
		}
		return c
	}
	if d := r.Dump; d != nil && d.Code != "" {
		c := "crashed in code outside any module"
		if d.Module != "" {
			c = "crashed in " + d.Module
		}
		if d.Exception != "" {
			c += ", " + firstPart(d.Exception)
		}
		if d.Access != "" {
			c += " " + d.Access
		}
		if r.Suspect != "" && d.Class != "addon" && !d.Unloaded {
			c += "; called by " + r.Suspect
		}
		return c
	}
	for _, s := range r.System {
		if s.Module != "" {
			c := "crashed in " + s.Module
			if s.Exception != "" {
				c += ", " + s.Exception
			}
			if s.Meaning != "" {
				c += ": " + firstPart(s.Meaning)
			}
			return c
		}
	}
	if r.ExitText != "" {
		code := r.ExitCode
		if code != "" && !strings.Contains(r.ExitText, code) && !strings.HasPrefix(r.ExitText, "code ") {
			return firstPart(r.ExitText) + " (" + code + ")"
		}
		return firstPart(r.ExitText)
	}
	for _, s := range r.System {
		if s.Text != "" {
			return s.Text
		}
	}
	return r.Why
}

// firstPart is a meaning up to its colon: "access violation".
func firstPart(s string) string {
	if k := strings.Index(s, ":"); k > 0 {
		return s[:k]
	}
	return s
}

func (m *Manager) saveCrash(r *CrashReport) error {
	dir := m.crashDir(r.Server)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, r.ID+".json"), b, 0o644); err != nil {
		return err
	}
	// keep the last ones
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keepCrashes {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return nil
}

// Crashes lists a server's crash reports, the newest first.
func (m *Manager) Crashes(id string) []CrashSummary {
	entries, _ := os.ReadDir(m.crashDir(id))
	var out []CrashSummary
	for k := len(entries) - 1; k >= 0; k-- {
		n := entries[k].Name()
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		r, err := m.Crash(id, strings.TrimSuffix(n, ".json"))
		if err != nil {
			continue
		}
		if r.Signature == "" {
			r.Signature = signature(r) // a report of before
		}
		if r.NextMap == "" {
			r.NextMap = smNextMap(r) // a report of before: only what SourceMod's log says (the console's map can be replaced)
		}
		if !r.Killed {
			r.Suspect, _ = suspectOf(r) // a report of before named an addon merely found near the crash
			r.Cause = crashCause(r)
		}
		out = append(out, CrashSummary{ID: r.ID, Time: r.Time, Cause: r.Cause, Uptime: r.Uptime, Map: r.Map, Killed: r.Killed,
			Signature: r.Signature, Suspect: r.Suspect, MapChange: r.MapChange || afterMapChange(r.Console), Players: r.Players - r.Bots,
			NextMap: r.NextMap})
	}
	return out
}

var crashIDRe = regexp.MustCompile(`^\d{8}-\d{6}$`)

// Crash reads a crash report.
func (m *Manager) Crash(id, report string) (*CrashReport, error) {
	if !crashIDRe.MatchString(report) {
		return nil, errors.New("no such report")
	}
	b, err := os.ReadFile(filepath.Join(m.crashDir(id), report+".json"))
	if err != nil {
		return nil, errors.New("no such report")
	}
	var r CrashReport
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteCrashes removes a server's crash reports.
func (m *Manager) DeleteCrashes(id string) error { return os.RemoveAll(m.crashDir(id)) }

// noteWindows reads the windows a stuck server shows (an error box), before it is ended. The lock must not be held.
func (i *Instance) noteWindows(pid int) {
	w := proc.WindowTexts(pid)
	for k := range w {
		w[k] = config.Redact(w[k])
	}
	i.mu.Lock()
	i.crashWindows = w
	i.mu.Unlock()
}

// crashSnapLocked notes what is known of a server that just ended, for its crash report. The lock is held.
func (i *Instance) crashSnapLocked(s config.Server, g *games.Game) crashSnap {
	sn := crashSnap{s: s, g: g, at: time.Now(), since: i.since, pid: i.pid, windows: i.crashWindows,
		consoleOff: i.consoleOff, hostname: i.hostname, version: i.version, cpu: i.cpu, mem: i.mem}
	if i.goodMem > 0 && i.mem < i.goodMem/10 {
		sn.cpu, sn.mem = i.goodCPU, i.goodMem // it was ending when last measured
	}
	if i.exited != nil {
		sn.exit = i.exited.Info()
	}
	if i.console != nil {
		sn.console = i.console.all()
	}
	from := sn.at.Add(-5 * time.Minute)
	if i.since.After(from) {
		from = i.since
	}
	sn.commands = i.sentSince(from)
	if in := i.lastInfo; in != nil {
		sn.mapName, sn.players, sn.bots, sn.maxPlayers = in.Map, in.Players, in.Bots, in.MaxPlayers
	}
	for _, p := range i.lastPlayers {
		if p.Name != "" && !p.Bot {
			sn.names = append(sn.names, p.Name)
		}
	}
	return sn
}
