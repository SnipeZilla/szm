package manager

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/crashinfo"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/proc"
)

// What points at the cause of a crash: the crash dump read (the module, the plugins running), the logs written just
// before, what changed before the run, and a signature to see the same crash again.

// DumpsDir is where Windows writes the crash dumps of the servers.
func (m *Manager) DumpsDir() string { return filepath.Join(m.DataDir, "crashes", "dumps") }

// dumpPrograms are the servers' programs Windows writes a dump of: every game's, by name.
func (m *Manager) dumpPrograms() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range m.Catalog() {
		for _, e := range g.Exe["windows"] {
			n := strings.ToLower(filepath.Base(filepath.FromSlash(e)))
			if strings.HasSuffix(n, ".exe") && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// SetupDumps has Windows write a crash dump of each server crash, and show no "has stopped working" window. When
// Windows refuses (the manager is not an administrator), crashed servers still show no window, without dumps; the
// reason is kept for the web page (and said once in the Activity).
func (m *Manager) SetupDumps() {
	if runtime.GOOS != "windows" {
		return
	}
	dir := m.DumpsDir()
	_ = os.MkdirAll(dir, 0o755)
	err := crashinfo.EnableDumps(dir, m.dumpPrograms(), 30)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	m.dumpsMu.Lock()
	changed := msg != m.dumpsErr
	m.dumpsErr = msg
	m.dumpsMu.Unlock()
	if err != nil {
		if changed {
			m.Log.Add(events.Error, "", "", "Crash dumps: Windows could not be set to write them (%v). Run the manager as an administrator, or as its service.", err)
		}
		proc.NoCrashDialogs(false)
		return
	}
	proc.NoCrashDialogs(true)
}

// DumpsError says why Windows writes no crash dump ("" when it does, or off Windows).
func (m *Manager) DumpsError() string {
	m.dumpsMu.Lock()
	defer m.dumpsMu.Unlock()
	return m.dumpsErr
}

// crashDump reads the dump of this crash: the one of its PID, else the newest written then.
func (m *Manager) crashDump(r *CrashReport, sn crashSnap) {
	var pick *crashinfo.DumpFile
	for k := range r.Dumps {
		d := &r.Dumps[k]
		n := strings.ToLower(d.Path)
		if !strings.HasSuffix(n, ".dmp") && !strings.HasSuffix(n, ".mdmp") {
			continue
		}
		if p := crashinfo.DumpPID(d.Path); p != 0 && sn.pid != 0 {
			if p == sn.pid {
				pick = d
				break
			}
			continue // another server's
		}
		if pick == nil || d.Time.After(pick.Time) {
			pick = d
		}
	}
	if pick == nil || (r.Dump != nil && r.Dump.File == pick.Path) {
		return
	}
	a, err := crashinfo.AnalyzeDump(pick.Path, sn.s.InstallDir)
	if err != nil {
		r.Notes = append(r.Notes, "The crash dump could not be read: "+err.Error()+".")
		return
	}
	r.Dump = a
}

// skip: the dumps of other servers (named with their PID) in the dumps folder.
func dumpsOf(list []crashinfo.DumpFile, pid int, dumpsDir string) []crashinfo.DumpFile {
	var out []crashinfo.DumpFile
	for _, d := range list {
		if p := crashinfo.DumpPID(d.Path); p != 0 && pid != 0 && p != pid && strings.EqualFold(filepath.Dir(d.Path), dumpsDir) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// logsDirs are the folders named logs in a server's folder (not in its big folders).
func logsDirs(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() || p == root {
			return nil
		}
		name := strings.ToLower(d.Name())
		if skipDirs[name] {
			return fs.SkipDir
		}
		if name == "logs" {
			out = append(out, p)
			return fs.SkipDir
		}
		return nil
	})
	return out
}

// crashLogs gives the end of the logs written during the run: the newest two of each logs folder (and its folders),
// eight at most, the newest first.
func crashLogs(root string, from time.Time) []crashinfo.LogTail {
	type cand struct {
		path string
		t    time.Time
	}
	var all []cand
	for _, dir := range logsDirs(root) {
		var files []cand
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().After(from) && info.Size() > 0 {
				files = append(files, cand{p, info.ModTime()})
			}
			return nil
		})
		sort.Slice(files, func(a, b int) bool { return files[a].t.After(files[b].t) })
		if len(files) > 2 {
			files = files[:2]
		}
		all = append(all, files...)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].t.After(all[b].t) })
	if len(all) > 8 {
		all = all[:8]
	}
	var out []crashinfo.LogTail
	for _, c := range all {
		lines := thisRun(crashinfo.Tail(c.path, 200), from)
		if len(lines) > 40 {
			lines = lines[len(lines)-40:]
		}
		if len(lines) == 0 {
			continue
		}
		for k, l := range lines {
			lines[k] = config.Redact(l)
		}
		out = append(out, crashinfo.LogTail{Path: c.path, Time: c.t, Lines: lines})
	}
	return withoutCopies(out)
}

// logStampRes: the times log lines start with: CounterStrikeSharp's, SourceMod's (and the game's), ISO
var logStampRes = []struct {
	re     *regexp.Regexp
	layout string
}{
	{regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)`), "2006-01-02 15:04:05"},
	{regexp.MustCompile(`^L (\d\d/\d\d/\d{4} - \d\d:\d\d:\d\d)`), "01/02/2006 - 15:04:05"},
	{regexp.MustCompile(`^\[(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)`), "2006-01-02 15:04:05"},
}

func lineTime(l string) (time.Time, bool) {
	for _, s := range logStampRes {
		if m := s.re.FindStringSubmatch(l); m != nil {
			if t, err := time.ParseInLocation(s.layout, m[1], time.Local); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// thisRun keeps the lines written since the run started (a line without a time goes with the one before it).
func thisRun(lines []string, from time.Time) []string {
	if from.IsZero() {
		return lines
	}
	from = from.Add(-2 * time.Second) // the log's clock and the start's are a moment apart
	keep := true
	var out []string
	for _, l := range lines {
		if t, ok := lineTime(l); ok {
			keep = !t.Before(from)
		}
		if keep {
			out = append(out, l)
		}
	}
	return out
}

// withoutCopies leaves out a log whose lines are all in another one (CounterStrikeSharp writes the same lines in
// log-all and log-cssharp).
func withoutCopies(logs []crashinfo.LogTail) []crashinfo.LogTail {
	set := func(l crashinfo.LogTail) map[string]bool {
		m := map[string]bool{}
		for _, x := range l.Lines {
			m[x] = true
		}
		return m
	}
	var out []crashinfo.LogTail
	for k, l := range logs {
		copyOf := false
		for j, o := range logs {
			if j == k || len(o.Lines) < len(l.Lines) || len(o.Lines) == len(l.Lines) && j > k {
				continue
			}
			in := set(o)
			all := true
			for _, x := range l.Lines {
				if !in[x] {
					all = false
					break
				}
			}
			if all {
				copyOf = true
				break
			}
		}
		if !copyOf {
			out = append(out, l)
		}
	}
	return out
}

// Change is something that changed in a server before the run that crashed: a file, an update, a push.
type Change struct {
	Time time.Time `json:"time"`
	Kind string    `json:"kind"` // file, event
	Text string    `json:"text"` // the file's path in the server's folder, or what the manager did
	Size int64     `json:"size,omitempty"`
}

// noChangeExt: files that change by themselves (data, logs, demos, maps, game files).
var noChangeExt = map[string]bool{".log": true, ".db": true, ".sq3": true, ".sqlite": true, ".dem": true, ".bsp": true,
	".vpk": true, ".nav": true, ".ain": true, ".cache": true, ".tmp": true, ".bak": true, ".mdmp": true, ".dmp": true}

var changeEventRe = regexp.MustCompile(`(?i)\b(saved|uploaded|deleted|renamed|workshop|settings of|restored|installed|addons|updated to|update)\b`)

// crashChanges lists what changed in the week before the run started: the files of the game folder modified then
// (plugins, configs), and what the manager did (updates, addons pushed, files saved). The newest first.
func (m *Manager) crashChanges(sn crashSnap) []Change {
	end := sn.since
	if end.IsZero() {
		end = sn.at
	}
	start := end.Add(-7 * 24 * time.Hour)
	root := sn.s.InstallDir
	gameDir := sn.g.Path(root, sn.g.GameDir)
	var out []Change
	_ = filepath.WalkDir(gameDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if d.IsDir() {
			if p != gameDir && (skipDirs[name] || name == "logs" || name == "download" || name == "downloads" || name == "cache" || name == "screenshots" || name == "data") {
				return fs.SkipDir
			}
			return nil
		}
		if noChangeExt[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if t := info.ModTime(); t.After(start) && t.Before(end) {
			rel, _ := filepath.Rel(root, p)
			out = append(out, Change{Time: t, Kind: "file", Text: filepath.ToSlash(rel), Size: info.Size()})
		}
		return nil
	})
	sort.Slice(out, func(a, b int) bool { return out[a].Time.After(out[b].Time) })
	if len(out) > 25 {
		out = out[:25]
	}
	// what the manager did then
	for d := 0; d <= 7; d++ {
		day := end.AddDate(0, 0, -d).Format("2006-01-02")
		evs, err := m.Log.Day(day)
		if err != nil {
			continue
		}
		for _, e := range evs {
			if e.Server != sn.s.ID || e.Time.Before(start) || !e.Time.Before(end) {
				continue
			}
			if noChangeEventRe.MatchString(e.Text) {
				continue // the manager's own housekeeping
			}
			if e.Kind == events.Updated || (e.Kind == events.Info && changeEventRe.MatchString(e.Text)) {
				out = append(out, Change{Time: e.Time, Kind: "event", Text: e.Text})
			}
		}
	}
	out = tidyChanges(out)
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// noChangeEventRe: what the manager does by itself, that changes nothing of the server's.
var noChangeEventRe = regexp.MustCompile(`(?i)^(logs older than|backed up|backup .* removed|crash reports removed|deleted \d+ files? in .*/logs$)|deleted \d+ files? in [^ ]*logs$`)

// tidyChanges keeps the list short: a file the manager's own line already names (saved, uploaded) is not listed
// again; the files of an addons push or an update are its line ("1074 files copied"); many files of one folder changed
// in the same minute are one line. The newest first.
func tidyChanges(list []Change) []Change {
	sort.SliceStable(list, func(a, b int) bool { return list[a].Time.After(list[b].Time) })
	near := func(a, b time.Time, d time.Duration) bool { x := a.Sub(b); return x < d && x > -d }
	var events []Change
	for _, c := range list {
		if c.Kind == "event" {
			events = append(events, c)
		}
	}
	// the files of an update (the game's or the addons'), counted on its line
	bulk := map[int]int{}
	covered := func(c Change) bool {
		base := strings.ToLower(filepath.Base(c.Text))
		for k, e := range events {
			t := strings.ToLower(e.Text)
			if near(c.Time, e.Time, 2*time.Minute) && (strings.Contains(t, strings.ToLower(c.Text)) || strings.Contains(t, base)) {
				return true
			}
			// an addons push, an update, an install or a restore: its files
			if strings.HasPrefix(t, "addons up to date") || strings.HasPrefix(t, "updated") || strings.HasPrefix(t, "up to date") || strings.HasPrefix(t, "installed") || strings.HasPrefix(t, "restored") {
				if c.Time.After(e.Time.Add(-15*time.Minute)) && !c.Time.After(e.Time.Add(time.Minute)) {
					if strings.HasPrefix(t, "up to date") || strings.HasPrefix(t, "updated") {
						bulk[k]++
					}
					return true
				}
			}
		}
		return false
	}
	var out []Change
	for k := 0; k < len(list); k++ {
		c := list[k]
		if c.Kind != "file" {
			out = append(out, c)
			continue
		}
		if covered(c) {
			continue
		}
		// the files of the same folder changed in the same minute: one line when they are many
		dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(c.Text)))
		j := k + 1
		for j < len(list) && list[j].Kind == "file" && filepath.ToSlash(filepath.Dir(filepath.FromSlash(list[j].Text))) == dir && near(list[j].Time, c.Time, time.Minute) {
			j++
		}
		if j-k > 3 {
			var size int64
			for _, x := range list[k:j] {
				size += x.Size
			}
			out = append(out, Change{Time: c.Time, Kind: "file", Text: fmt.Sprintf("%s/: %d files", dir, j-k), Size: size})
			k = j - 1
			continue
		}
		out = append(out, c)
	}
	// "Up to date: CS2 v1.41.9.1 (9m 46s)" said as what it is, with the files it changed
	for k := range out {
		for j, e := range events {
			if out[k].Kind == "event" && out[k].Time.Equal(e.Time) && out[k].Text == e.Text {
				t := strings.TrimPrefix(e.Text, "Up to date: ")
				if t != e.Text {
					out[k].Text = "Game updated: " + t
				}
				if n := bulk[j]; n > 0 {
					out[k].Text += fmt.Sprintf(", %d file%s changed", n, map[bool]string{true: "", false: "s"}[n == 1])
				}
			}
		}
	}
	return out
}

var mapChangeRe = regexp.MustCompile(`(?i)(changelevel|changing level|change ?level|map ?change|mapchange|loading map|spawnserver|spawn server|host_workshop_map|\bmap "?[a-z0-9_]+"? ?$)`)

// afterMapChange says whether the server was changing map when it crashed (in its last console lines).
func afterMapChange(lines []string) bool {
	from := len(lines) - 25
	if from < 0 {
		from = 0
	}
	for _, l := range lines[from:] {
		if strings.HasPrefix(l, "── ") {
			continue
		}
		if mapChangeRe.MatchString(l) {
			return true
		}
	}
	return false
}

// signature names the crash, to see the same one again: where it crashed, else how it ended.
func signature(r *CrashReport) string {
	if r.Killed {
		return "stuck"
	}
	if d := r.Dump; d != nil && d.Code != "" {
		if d.Module != "" {
			return d.Module + "+" + d.Offset
		}
		return "outside any module, " + d.Code
	}
	for _, s := range r.System {
		if s.Module != "" {
			if s.Offset != "" {
				return s.Module + "+" + s.Offset
			}
			return s.Module
		}
	}
	if r.ExitCode != "" {
		return "exit " + r.ExitCode
	}
	return "unknown"
}

// suspectOf names what most likely caused it, from the dump, else from the module Windows named: only what the
// evidence supports (an addon merely near the crash is not named here; the report says it, with how sure).
func suspectOf(r *CrashReport) (string, string) {
	if d := r.Dump; d != nil && d.Code != "" {
		if d.Suspect != "" && (d.Sure == "sure" || d.Sure == "likely") {
			return d.Suspect, d.Why
		}
		return "", ""
	}
	for _, s := range r.System {
		if s.ModPath != "" && strings.Contains(strings.ToLower(strings.ReplaceAll(s.ModPath, `\`, "/")), "/addons/") {
			return s.Module, "it crashed inside it (Windows' record)"
		}
	}
	return "", ""
}

// SourceMod's log: L 10/15/2026 - 19:51:16: [SM] Changed map to "css_aim_ag_texture2" (it puts the voted map in
// place of the map cycle's), and the console's CHANGE LEVEL: <map>.
var (
	smChangedRe   = regexp.MustCompile(`^L (\d\d/\d\d/\d{4} - \d\d:\d\d:\d\d): \[SM\] Changed map to "([^"]+)"`)
	consoleNextRe = regexp.MustCompile(`^CHANGE LEVEL: (\S+)`)
)

// nextMap says which map the server was changing to when it crashed: the one SourceMod logged in the two minutes
// before (the map really loaded, after a vote), else the console's last CHANGE LEVEL.
func nextMap(r *CrashReport) string {
	if m := smNextMap(r); m != "" {
		return m
	}
	from := len(r.Console) - 25
	if from < 0 {
		from = 0
	}
	for k := len(r.Console) - 1; k >= from; k-- {
		if strings.HasPrefix(r.Console[k], "── ") {
			break
		}
		if m := consoleNextRe.FindStringSubmatch(r.Console[k]); m != nil {
			return m[1]
		}
	}
	return ""
}

// smNextMap: the map SourceMod logged it was changing to, in the two minutes before the crash.
func smNextMap(r *CrashReport) string {
	for _, l := range r.Logs {
		for k := len(l.Lines) - 1; k >= 0; k-- {
			m := smChangedRe.FindStringSubmatch(l.Lines[k])
			if m == nil {
				continue
			}
			t, err := time.ParseInLocation("01/02/2006 - 15:04:05", m[1], time.Local)
			if err == nil && !t.After(r.Time.Add(time.Minute)) && r.Time.Sub(t) < 2*time.Minute {
				return m[2]
			}
			break
		}
	}
	return ""
}
