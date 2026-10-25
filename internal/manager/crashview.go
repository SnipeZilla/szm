package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/crashinfo"
	"github.com/SnipeZilla/szm/internal/games"
)

// A crash report read by people: first what happened and what to do, in plain words (only what the evidence
// supports, and how sure it is), then the details for the developers (the plugin's author, Valve).

// CrashPlain is the plain-words part.
type CrashPlain struct {
	Headline string   `json:"headline"`        // what happened, in one or two sentences
	Sure     string   `json:"sure,omitempty"`  // sure, likely, unsure: how much the evidence proves the cause
	Cause    string   `json:"cause,omitempty"` // what is at fault, and why that is said
	Error    string   `json:"error,omitempty"` // the error itself, in words
	Before   []string `json:"before,omitempty"`
	Todo     []string `json:"todo,omitempty"`
	// ReportTo: who can fix it, and where (a plugin's author, the game's developers); empty when not known
	ReportTo string `json:"report_to,omitempty"`
	ReportAt string `json:"report_at,omitempty"` // a link, when known
}

// where Valve takes the bug reports of its games' servers
var valveIssues = map[string]string{
	"cs2": "https://github.com/ValveSoftware/csgo-osx-linux/issues",
	"css": "https://github.com/ValveSoftware/Source-1-Games/issues", "dods": "https://github.com/ValveSoftware/Source-1-Games/issues",
	"hl2dm": "https://github.com/ValveSoftware/Source-1-Games/issues", "hldms": "https://github.com/ValveSoftware/Source-1-Games/issues",
	"tf2":  "https://github.com/ValveSoftware/Source-1-Games/issues",
	"cs16": "https://github.com/ValveSoftware/halflife/issues", "czero": "https://github.com/ValveSoftware/halflife/issues",
	"tfc": "https://github.com/ValveSoftware/halflife/issues", "dod": "https://github.com/ValveSoftware/halflife/issues",
	"dmc": "https://github.com/ValveSoftware/halflife/issues", "ricochet": "https://github.com/ValveSoftware/halflife/issues",
	"hldm": "https://github.com/ValveSoftware/halflife/issues", "gearbox": "https://github.com/ValveSoftware/halflife/issues",
}

func orOutside(mod string) string {
	if mod == "" {
		return "code outside any module"
	}
	return mod
}

// CrashHistory: the same crash (the same signature) on this machine's servers.
type CrashHistory struct {
	Count   int       `json:"count"` // this one included
	Servers []string  `json:"servers"`
	First   time.Time `json:"first"`
	After   string    `json:"after,omitempty"` // what changed just before the first one (a game update)
}

// CrashView is a report with what is said of it.
type CrashView struct {
	*CrashReport
	Plain   CrashPlain    `json:"plain"`
	History *CrashHistory `json:"history,omitempty"`
	Summary string        `json:"summary"`       // for a bug report, to paste
	Short   []string      `json:"console_short"` // the console, repeated lines folded
	// AddonsLoaded: the addons the dump shows loaded, each once with its version; Plugins: the CounterStrikeSharp
	// plugins its log says it loaded this run (a .NET plugin is loaded from memory: the dump does not show it)
	AddonsLoaded []crashinfo.AddonFamily `json:"addons_loaded,omitempty"`
	Plugins      []string                `json:"plugins_logged,omitempty"`
}

// CrashView reads a report and says what it means.
func (m *Manager) CrashView(id, report string) (*CrashView, error) {
	r, err := m.Crash(id, report)
	if err != nil {
		return nil, err
	}
	return m.viewOf(r), nil
}

func (m *Manager) viewOf(r *CrashReport) *CrashView {
	// a report of before the call chain: its dump read again, when it is still there
	if d := r.Dump; d != nil && d.Code != "" && d.Sure == "" && d.File != "" {
		if s, ok := m.server(r.Server); ok {
			if a, err := crashinfo.AnalyzeDump(d.File, s.InstallDir); err == nil && a.Code != "" {
				r.Dump = a
				finishCrash(r)
			}
		}
	}
	if r.Signature == "" {
		r.Signature = signature(r)
	}
	v := &CrashView{CrashReport: r, Short: foldRepeats(r.Console), Plugins: loggedPlugins(r)}
	if d := r.Dump; d != nil {
		v.AddonsLoaded = crashinfo.Families(d.Loaded)
		for _, fs := range [][]crashinfo.Frame{d.Chain, d.Frames} {
			for k := range fs {
				fs[k].Function = crashinfo.ShortFunc(fs[k].Function)
			}
		}
	}
	v.History = m.history(r)
	v.Plain = plainOf(r, v.History, m.Catalog()[r.Game])
	v.Summary = summaryOf(r, v.History, m.gameName(r.Game))
	return v
}

func (m *Manager) gameName(key string) string {
	if g := m.Catalog()[key]; g != nil {
		return g.Name
	}
	return key
}

// history finds the same crash in the reports of this machine's servers.
func (m *Manager) history(r *CrashReport) *CrashHistory {
	if r.Signature == "" || r.Signature == "unknown" || r.Signature == "stuck" {
		return nil
	}
	h := &CrashHistory{First: r.Time}
	servers := map[string]bool{}
	var first *CrashReport
	root := filepath.Join(m.DataDir, "crashes")
	dirs, _ := os.ReadDir(root)
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == "dumps" {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(root, d.Name()))
		for _, f := range files {
			if !crashIDRe.MatchString(strings.TrimSuffix(f.Name(), ".json")) || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, d.Name(), f.Name()))
			if err != nil {
				continue
			}
			o := &CrashReport{}
			if json.Unmarshal(b, o) != nil {
				continue
			}
			if o.Signature == "" {
				o.Signature = signature(o)
			}
			if o.Signature != r.Signature {
				continue
			}
			h.Count++
			servers[o.Name] = true
			if first == nil || o.Time.Before(h.First) || o.Time.Equal(h.First) {
				h.First, first = o.Time, o
			}
		}
	}
	if h.Count == 0 {
		h.Count, servers[r.Name] = 1, true
	}
	for s := range servers {
		h.Servers = append(h.Servers, s)
	}
	sort.Strings(h.Servers)
	// a game update in the 2 days before the first one
	if first != nil {
		for _, c := range first.Changes {
			t := c.Text
			if strings.HasPrefix(t, "Up to date: ") { // a report of before: the event's own words
				t = "Game updated: " + strings.TrimPrefix(t, "Up to date: ")
			}
			if c.Kind == "event" && strings.HasPrefix(t, "Game updated") && first.Time.Sub(c.Time) < 48*time.Hour {
				h.After = t + " (" + c.Time.Local().Format("Jan 2 15:04") + ")"
				break
			}
		}
	}
	return h
}

// trigger: the last command a user sent in the 30 seconds before the crash (the manager's own checks aside)
func trigger(r *CrashReport) *SentCommand {
	for k := len(r.Commands) - 1; k >= 0; k-- {
		c := r.Commands[k]
		if c.Routine || c.By == "" {
			continue
		}
		if d := r.Time.Sub(c.Time); d >= 0 && d <= 30*time.Second {
			return &r.Commands[k]
		}
		break
	}
	return nil
}

func ago(d time.Duration) string {
	if d < time.Second {
		return "less than a second"
	}
	return fmtDur(d.Round(time.Second))
}

// plainOf says what happened and what to do, from what is known: nothing is said the evidence does not support.
func plainOf(r *CrashReport, h *CrashHistory, g *games.Game) CrashPlain {
	var p CrashPlain
	where := ""
	if r.Map != "" {
		where = " on " + r.Map
		switch n := r.Players - r.Bots; {
		case n <= 0:
			where += ", with nobody on"
		case n == 1:
			where += ", with 1 player on"
		default:
			where += fmt.Sprintf(", with %d players on", n)
		}
	}
	ran := ""
	if r.Uptime > 0 {
		ran = " after " + fmtDur(time.Duration(r.Uptime)*time.Second)
	}
	// it never came up (no map, no answer since it started), and the console says why: the map it was to load
	neverUp := r.Killed && r.Map == "" && (strings.Contains(r.Why, "after it started") || strings.Contains(r.Why, "start map"))
	noStartMap, noStartLine := "", ""
	if neverUp {
		for _, l := range r.Console {
			if n := noSuchMap(l); n != "" {
				noStartMap, noStartLine = n, strings.TrimSpace(l)
				break
			}
		}
	}
	noDLL := r.ExitCode == "0xC0000135" && !r.Killed // STATUS_DLL_NOT_FOUND: it never ran
	switch {
	case neverUp:
		p.Headline = fmt.Sprintf("%s never came up: it ran %s without answering, and the manager ended it.", r.Name, fmtDur(time.Duration(r.Uptime)*time.Second))
	case r.Killed:
		p.Headline = fmt.Sprintf("%s stopped answering%s%s, and the manager ended it.", r.Name, ran, where)
	case noDLL:
		p.Headline = fmt.Sprintf("%s could not start: Windows did not find a DLL it needs.", r.Name)
	default:
		p.Headline = fmt.Sprintf("%s crashed%s%s.", r.Name, ran, where)
	}
	tr := trigger(r)
	d := r.Dump

	// what is at fault
	switch {
	case noStartMap != "":
		p.Sure = "sure"
		p.Cause = "It never got a map: its console said the map " + noStartMap + " does not exist on this server (\"" + noStartLine + "\"), so it waited with no map, answering nothing."
	case r.Killed:
		p.Sure = "sure"
		p.Cause = "It was stuck, not crashed: " + r.Why + "."
		if len(r.Windows) > 0 {
			p.Cause += " It was showing an error window (below)."
		}
	case d != nil && d.Code != "":
		p.Sure = d.Sure
		if p.Sure == "" {
			p.Sure = "unsure" // a report of before the call chain: a stack scan only
		}
		switch {
		case d.Suspect != "" && p.Sure == "sure":
			p.Cause = "The cause is " + d.Suspect + ": " + d.Why + "."
		case d.Suspect != "" && p.Sure == "likely":
			p.Cause = "Most likely " + d.Suspect + ": " + d.Why + "."
		case d.Why != "":
			p.Cause = d.Why + "."
		default:
			p.Cause = "It crashed in " + orOutside(d.Module) + "."
		}
		p.Error = d.Exception
		if d.Access != "" {
			p.Error += ", " + d.Access
		}
		if d.Module != "" {
			p.Error += ", in " + d.Module
		}
		p.Error = capFirst(p.Error) + "."
	case noDLL && len(r.Missing) > 0:
		p.Sure = "sure"
		names := make([]string, len(r.Missing))
		for k, m := range r.Missing {
			names[k] = m.Name
		}
		sys := r.MissingSys
		if sys == "" {
			sys = "System32"
		}
		are := map[bool]string{true: "is", false: "are"}[len(names) == 1]
		p.Cause = "Its program needs " + andList(names) + ", which " + are + " not in its folder, in " + sys + " or on the PATH, so it ended before it ran."
		for _, m := range r.Missing {
			if m.Hint != "" {
				p.Cause += " " + m.Name + " " + strings.TrimPrefix(m.Hint, "it ") + "."
			}
		}
	case noDLL:
		p.Sure = "sure"
		p.Cause = "Windows did not find a DLL its program needs, so it ended before it ran. Which one is not known: it is loaded late, or needed by another DLL."
	default:
		if mod := systemModule(r); mod != "" {
			p.Sure = "unsure"
			p.Cause = "Windows says it crashed in " + mod + ". Without its crash dump, what called it is not known."
		} else if r.ExitText != "" {
			p.Sure = "unsure"
			p.Cause = "It ended with " + r.ExitText + ". Without a crash dump, where it crashed is not known."
		} else {
			p.Cause = "How it ended is not known."
		}
	}
	if p.Cause != "" {
		p.Cause = strings.ToUpper(p.Cause[:1]) + p.Cause[1:]
	}

	// what happened just before
	if tr != nil {
		p.Before = append(p.Before, fmt.Sprintf("%s before: %s sent the command \"%s\" (RCON).", capFirst(ago(r.Time.Sub(tr.Time))), tr.By, tr.Command))
	}
	gone := map[string]bool{}
	for _, x := range noSuchMaps(r.Console) {
		gone[strings.ToLower(x)] = true
	}
	nextGone := gone[strings.ToLower(r.NextMap)]
	switch {
	case neverUp:
		// starting: loading its first map, not changing map
	case r.NextMap != "" && nextGone:
		p.Before = append(p.Before, "It was changing map, to "+r.NextMap+", a map its console said it does not have.")
	case r.NextMap != "":
		p.Before = append(p.Before, "It was changing map, to "+r.NextMap+".")
	case r.MapChange:
		p.Before = append(p.Before, "It was changing map.")
	}
	for _, x := range noSuchMaps(r.Console) {
		if !strings.EqualFold(x, r.NextMap) && !strings.EqualFold(x, noStartMap) {
			p.Before = append(p.Before, "Its console said the map "+x+" does not exist on this server.")
		}
	}
	if r.InARow > 1 {
		p.Before = append(p.Before, fmt.Sprintf("It is the crash %d in a row.", r.InARow))
	}
	if h != nil && h.Count > 1 {
		s := fmt.Sprintf("The same crash %d times since %s", h.Count, h.First.Local().Format("Jan 2"))
		if len(h.Servers) > 1 {
			s += ", on " + strings.Join(h.Servers, ", ")
		}
		s += "."
		if h.After != "" {
			s += " The first one kept came after: " + h.After + "."
		}
		p.Before = append(p.Before, s)
	}
	if c := changedAddon(r); c != "" {
		p.Before = append(p.Before, c)
	}

	// what to do
	if tr != nil && (r.Time.Sub(tr.Time) <= 10*time.Second) {
		p.Todo = append(p.Todo, fmt.Sprintf("Do not send \"%s\" to this server for now: it crashed %s after it. Send it once more on a test server: if it crashes again, that command is the trigger, and the details below are what its fix needs.", tr.Command, ago(r.Time.Sub(tr.Time))))
	}
	game, url := "the game's developers", ""
	if g != nil {
		game, url = "the developers of "+g.Name, g.URL
	}
	if u := valveIssues[r.Game]; u != "" {
		game, url = "Valve", u
	}
	switch {
	case noDLL && installParts(r.Missing) != nil:
		p.Todo = append(p.Todo, "Install "+andList(installParts(r.Missing))+" on this machine (above), then click Start: until then the manager does not start it again, as a restart or a check of its files can not bring it back.")
	case noDLL && len(r.Missing) > 0:
		for _, m := range r.Missing {
			if m.Hint != "" {
				p.Todo = append(p.Todo, "Install what "+m.Name+" is part of (above) on this machine, then start it again.")
			} else {
				p.Todo = append(p.Todo, "Put back "+m.Name+": Verify files brings back a file of the game; one of an addon comes back by installing that addon again.")
			}
		}
	case noDLL:
		p.Todo = append(p.Todo, "Start its program by hand on this machine (double-click it in its folder): Windows then names the DLL it did not find. Most often it is the Visual C++ runtime, Media Foundation (Windows Server), or a file of an addon.")
	case noStartMap != "":
		p.Todo = append(p.Todo, "Change its start map (Settings) to a map it has, or add the file of "+noStartMap+". Until then it can not start: the manager does not start it again by itself.")
	case r.Killed:
		p.Todo = append(p.Todo, "If it gets stuck again, look at what it was doing: the end of its console and logs below (a plugin's loop, a map loading, a database wait).")
	case d != nil && d.Suspect != "" && (p.Sure == "sure" || p.Sure == "likely"):
		p.Todo = append(p.Todo, "Update "+d.Suspect+", or remove it for a while, and see whether the crashes stop.")
		p.Todo = append(p.Todo, "If they go on, report it to its author (\"Report it\" below).")
		p.ReportTo = "the author of " + d.Suspect
	case d != nil && d.Code != "" && p.Sure == "sure" && len(d.Loaded) == 0:
		p.Todo = append(p.Todo, "No addon was loaded: the bug is the game's. Report it to "+game+" (\"Report it\" below).")
		p.ReportTo, p.ReportAt = game, url
	case d != nil && d.Code != "" && len(d.Loaded) > 0:
		p.Todo = append(p.Todo, "To know whether an addon is involved, run it once without its addons (a copy of it is enough) and do what it was doing. If it crashes the same way, the bug is the game's: report it to "+game+" (\"Report it\" below).")
		p.ReportTo, p.ReportAt = game+", if it also crashes without addons", url
	case d != nil && d.Code != "":
		p.Todo = append(p.Todo, "The bug is likely the game's: report it to "+game+" (\"Report it\" below).")
		p.ReportTo, p.ReportAt = game, url
	}
	switch {
	case neverUp:
		// starting: loading its first map, not changing map
	case r.NextMap != "" && nextGone:
		p.Todo = append(p.Todo, "Remove "+r.NextMap+" from the map cycle (Maps tab) and the votes, or add its file.")
	case r.NextMap != "" && !strings.Contains(r.NextMap, "/"):
		p.Todo = append(p.Todo, "Try the map "+r.NextMap+" alone (changelevel "+r.NextMap+"): a broken map file crashes a server as it loads.")
	}
	if d == nil && !r.Killed && !noDLL && r.Exit != nil && strings.HasPrefix(r.ExitCode, "0x") {
		p.Todo = append(p.Todo, "Run the manager as an administrator, or as its service: Windows then writes a crash dump of each crash, which tells much more (what was calling).")
	}
	return p
}

// andList: "a, b and c"
func andList(xs []string) string {
	if len(xs) < 2 {
		return strings.Join(xs, "")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func systemModule(r *CrashReport) string {
	for _, s := range r.System {
		if s.Module != "" {
			return s.Module
		}
	}
	return ""
}

// changedAddon: the suspect's files changed in the week before the run (a plugin updated, added)
func changedAddon(r *CrashReport) string {
	d := r.Dump
	if d == nil || d.Suspect == "" || d.Sure == "unsure" {
		return ""
	}
	folder := ""
	for _, l := range d.Loaded {
		if l.Label == d.Suspect || l.Module == d.Suspect {
			folder = addonFolder(l.Path)
			break
		}
	}
	if folder == "" {
		return ""
	}
	for _, c := range r.Changes {
		if strings.Contains(strings.ToLower(c.Text), "addons/"+folder) {
			return fmt.Sprintf("%s was changed before this run: %s (%s).", d.Suspect, c.Text, c.Time.Local().Format("Jan 2 15:04"))
		}
	}
	return ""
}

var addonFolderRe = regexp.MustCompile(`(?i)[\\/]addons[\\/]([^\\/]+)`)

func addonFolder(path string) string {
	if m := addonFolderRe.FindStringSubmatch(path); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// summaryOf writes the few lines a bug report needs, to paste (a GitHub issue, a forum post).
func summaryOf(r *CrashReport, h *CrashHistory, game string) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	p("Game:       %s%s, dedicated server (%s)", game, prefixed(" ", r.Version), osOf(r))
	if r.OS != "" {
		p("System:     %s", r.OS)
	}
	if r.Manager != "" {
		p("Manager:    SnipeZilla Server Manager %s", r.Manager)
	}
	if r.Killed {
		p("What:       stuck (%s), ended", r.Why)
	} else if d := r.Dump; d != nil && d.Code != "" {
		where := "outside any module"
		if d.Module != "" {
			where = d.Module + "+" + d.Offset
		}
		p("Crash:      %s %s%s, in %s", d.Code, firstPart(d.Exception), prefixed(", ", d.Access), where)
		if f := topFunction(d); f != "" {
			p("Function:   %s", f)
		}
		if !chainKnown(d) && d.ChainEnd != "" {
			p("Call chain: not known (%s)", d.ChainEnd)
		}
		if chainKnown(d) {
			var c []string
			for _, f := range d.Chain {
				c = append(c, f.Module+"+"+f.Offset)
			}
			p("Call chain: %s%s", strings.Join(c, " < "), map[bool]string{true: " (cut short: " + d.ChainEnd + ")", false: ""}[d.ChainEnd != ""])
		}
		if ad := familiesLine(crashinfo.Families(d.Loaded)); ad != "" {
			p("Addons:     %s", ad)
		} else {
			p("Addons:     none loaded")
		}
		if pl := loggedPlugins(r); len(pl) > 0 {
			p("Plugins:    %s (CounterStrikeSharp's log)", strings.Join(pl, ", "))
		}
	} else if r.ExitCode != "" {
		p("Crash:      exit %s, %s (no crash dump)", r.ExitCode, firstPart(r.ExitText))
	}
	if tr := trigger(r); tr != nil {
		p("Before:     RCON \"%s\", %s before the crash", tr.Command, ago(r.Time.Sub(tr.Time)))
	}
	state := fmt.Sprintf("after %s", fmtDur(time.Duration(r.Uptime)*time.Second))
	if r.Map != "" {
		state += fmt.Sprintf(", map %s, %d players", r.Map, r.Players-r.Bots)
	}
	if r.NextMap != "" {
		state += ", changing to " + r.NextMap
	}
	p("When:       %s, %s", r.Time.Format("2006-01-02 15:04 MST"), state)
	if h != nil && h.Count > 1 {
		p("Seen:       %d times since %s", h.Count, h.First.Format("2006-01-02"))
	}
	return strings.TrimRight(b.String(), "\n")
}

func prefixed(pre, s string) string {
	if s == "" {
		return ""
	}
	return pre + s
}

// topFunction: the function it crashed in, when known
func topFunction(d *crashinfo.DumpAnalysis) string {
	for _, fs := range [][]crashinfo.Frame{d.Chain, d.Frames} {
		if len(fs) > 0 {
			return fs[0].Function
		}
	}
	return ""
}

func osOf(r *CrashReport) string {
	if d := r.Dump; d != nil && d.Arch != "" {
		return "Windows " + d.Arch
	}
	if strings.HasSuffix(strings.ToLower(r.Program), ".exe") {
		return "Windows"
	}
	return "Linux"
}

// foldRepeats folds runs of lines alike but for their numbers ("Long frame: 22.43ms…") into the first and a count.
func foldRepeats(lines []string) []string {
	norm := func(s string) string { return digitsRe.ReplaceAllString(s, "#") }
	var out []string
	for k := 0; k < len(lines); {
		j := k + 1
		for j < len(lines) && norm(lines[j]) == norm(lines[k]) {
			j++
		}
		out = append(out, lines[k])
		if n := j - k - 1; n >= 2 {
			out = append(out, fmt.Sprintf("   … %d more lines like it", n))
		} else if n == 1 {
			out = append(out, lines[k+1])
		}
		k = j
	}
	return out
}

var digitsRe = regexp.MustCompile(`\d+(\.\d+)?`)

var sureWords = map[string]string{"sure": "sure", "likely": "likely, not proven", "unsure": "not sure"}

// Text writes the report as plain text, to save or to paste: the part in plain words, then the developers' part.
func (v *CrashView) Text() string {
	r, pl := v.CrashReport, v.Plain
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	head := func(t string) { p(""); p("%s", t); p("%s", strings.Repeat("-", len(t))) }
	p("SnipeZilla Server Manager: crash report")
	p("%s (%s), %s", r.Name, r.Game, r.Time.Format("2006-01-02 15:04:05 MST"))

	head("WHAT HAPPENED")
	p("%s", pl.Headline)
	if pl.Cause != "" {
		if w := sureWords[pl.Sure]; w != "" {
			p("Cause (%s): %s", w, pl.Cause)
		} else {
			p("Cause: %s", pl.Cause)
		}
	}
	if pl.Error != "" {
		p("Error: %s", pl.Error)
	}
	if r.Next != "" {
		p("Then: %s.", r.Next)
	}
	if len(pl.Before) > 0 {
		head("WHAT WAS GOING ON")
		for _, x := range pl.Before {
			p("- %s", x)
		}
	}
	if len(pl.Todo) > 0 {
		head("WHAT TO DO")
		for k, x := range pl.Todo {
			p("%d. %s", k+1, x)
		}
	}
	if pl.ReportTo != "" {
		head("REPORT IT")
		p("To %s.", pl.ReportTo)
		if pl.ReportAt != "" {
			p("Where: %s", pl.ReportAt)
		}
		p("Paste the summary below, and attach the crash dump (in the report's zip).")
	}

	p("")
	p("")
	p("==================== FOR THE DEVELOPERS ====================")
	head("Summary")
	p("%s", v.Summary)

	if d := r.Dump; d != nil && d.Code != "" {
		head(fmt.Sprintf("Crash dump (%s, %s, %s)", filepath.Base(d.File), d.Arch, fmt.Sprintf("%d thread%s", d.Threads, plural(d.Threads))))
		p("Exception: %s %s", d.Code, d.Exception)
		if d.Access != "" {
			p("           %s", d.Access)
		}
		if d.Module != "" {
			p("In:        %s+%s%s", d.Module, d.Offset, prefixed("  ", topFunction(d)))
			if d.Path != "" || d.Version != "" {
				p("           %s%s", d.Path, prefixed(", version ", d.Version))
			}
		} else {
			p("In:        %s (code outside any module)", d.Address)
		}
		if d.Suspect != "" {
			p("Verdict:   %s (%s): %s", d.Suspect, sureWords[orUnsure(d.Sure)], d.Why)
		} else if d.Why != "" {
			p("Verdict:   (%s) %s", sureWords[orUnsure(d.Sure)], d.Why)
		}
		if chainKnown(d) {
			p("")
			p("Call chain of the crashed thread (unwound with the modules' unwind data; the crash first):")
			for k, f := range d.Chain {
				p("  #%-2d %s", k, frameLine(f))
			}
			if d.ChainEnd != "" {
				p("  (stops here: %s)", d.ChainEnd)
			}
		}
		if len(d.Frames) > 0 {
			p("")
			if chainKnown(d) {
				p("Modules found on the crashed thread's stack (a scan: some were not calling, the order is not sure):")
			} else {
				p("Modules found on the crashed thread's stack (a scan, no call chain: some were not calling, the order is not sure):")
				if d.ChainEnd != "" {
					p("  (no call chain: %s)", d.ChainEnd)
				}
			}
			for _, f := range d.Frames {
				p("  %s", frameLine(f))
			}
		}
		if len(v.AddonsLoaded) > 0 {
			p("")
			p("Addons loaded:")
			for _, f := range v.AddonsLoaded {
				x := fmt.Sprintf("%d module%s", f.Modules, plural(f.Modules))
				if f.Runtime != "" {
					x = f.Runtime + ", " + x
				}
				p("  %-28s %-14s %s", f.Name, f.Version, x)
				if len(f.Parts) > 0 {
					p("  %-28s %s", "", strings.Join(f.Parts, ", "))
				}
			}
			if len(v.Plugins) > 0 {
				p("  Plugins loaded this run (CounterStrikeSharp's log): %s", strings.Join(v.Plugins, ", "))
			}
		}
		if d.Note != "" {
			p("")
			p("Note: %s", d.Note)
		}
	} else if r.ExitCode != "" {
		head("Exit")
		p("Exit code: %s, %s", r.ExitCode, r.ExitText)
		for _, m := range r.Missing {
			if m.Hint != "" {
				p("Missing:   %s (%s)", m.Name, m.Hint)
			} else {
				p("Missing:   %s", m.Name)
			}
		}
	}
	if r.Killed {
		head("Ended by the manager")
		p("%s", r.Why)
		for _, w := range r.Windows {
			p("Window: %s", w)
		}
	}
	for _, s := range r.System {
		head(s.Source + ", " + s.Time.Local().Format("15:04:05"))
		p("%s", s.Text)
		if s.Meaning != "" {
			p("%s", s.Meaning)
		}
		if s.ModPath != "" {
			p("Module: %s", s.ModPath)
		}
	}
	if len(r.Commands) > 0 {
		head("Commands sent (RCON, this run, the 5 minutes before)")
		for _, c := range r.Commands {
			by := c.By
			if by == "" {
				by = "the manager"
			}
			x := ""
			if c.Routine {
				x = "  (routine)"
			}
			if c.Error != "" {
				x += "  error: " + c.Error
			}
			d := "<1s"
			if r.Time.Sub(c.Time) >= time.Second {
				d = ago(r.Time.Sub(c.Time))
			}
			p("  %s  -%-8s %-24s %s%s", c.Time.Local().Format("15:04:05"), d, by, c.Command, x)
		}
	}
	head("The server")
	if r.Hostname != "" {
		p("Hostname:  %s", r.Hostname)
	}
	if r.OS != "" {
		p("System:    %s", r.OS)
	}
	if r.Program != "" {
		p("Program:   %s (PID %d)", r.Program, r.PID)
	}
	if r.Version != "" {
		p("Version:   %s", r.Version)
	}
	p("Ran for:   %s", fmtDur(time.Duration(r.Uptime)*time.Second))
	if r.Map != "" {
		of := ""
		if r.MaxPlayers > 0 {
			of = fmt.Sprintf(" of %d", r.MaxPlayers)
		}
		p("Map:       %s, %d players (%d bots)%s", r.Map, r.Players, r.Bots, of)
	}
	if r.NextMap != "" {
		p("Changing:  to %s", r.NextMap)
	}
	if len(r.PlayerList) > 0 {
		p("Players:   %s", strings.Join(r.PlayerList, ", "))
	}
	if r.Memory > 0 {
		p("Last use:  CPU %.1f %%, memory %.0f MB", r.CPU, float64(r.Memory)/1048576)
	}
	if len(r.Changes) > 0 {
		head("Changed in the week before this run")
		for _, c := range r.Changes {
			p("  %s  %s", c.Time.Local().Format("2006-01-02 15:04"), c.Text)
		}
	}
	if len(r.Dumps) > 0 {
		head("Crash dumps")
		for _, d := range r.Dumps {
			p("  %s (%d KB)", d.Path, d.Size/1024)
		}
	}
	for _, l := range r.Logs {
		head("End of " + l.Path + " (this run)")
		for _, x := range l.Lines {
			p("  %s", x)
		}
	}
	head("Last lines of the console (repeated lines folded)")
	if r.ConsoleNote != "" {
		p("  (%s)", r.ConsoleNote)
	}
	for _, l := range v.Short {
		p("  %s", l)
	}
	if len(v.Short) == 0 {
		p("  (nothing)")
	}
	for _, n := range r.Notes {
		p("")
		p("Note: %s", n)
	}
	return b.String()
}

func orUnsure(s string) string {
	if s == "" {
		return "unsure"
	}
	return s
}

func frameLine(f crashinfo.Frame) string {
	l := fmt.Sprintf("%-28s +%-10s %-7s", f.Module, f.Offset, f.Class)
	if f.Function != "" {
		l += " " + f.Function
	}
	if f.Label != "" {
		l += "   <- " + f.Label
	} else if f.Role != "" {
		l += "   (" + f.Role + ")"
	}
	return l
}

func familiesLine(fs []crashinfo.AddonFamily) string {
	var out []string
	for _, f := range fs {
		x := strings.TrimSpace(f.Name + " " + f.Version)
		var more []string
		if f.Runtime != "" {
			more = append(more, f.Runtime)
		}
		more = append(more, f.Parts...)
		if len(more) > 0 {
			x += " (" + strings.Join(more, ", ") + ")"
		}
		out = append(out, x)
	}
	return strings.Join(out, ", ")
}

var cssLoadedRe = regexp.MustCompile(`\(cssharp:PluginContext\) Finished loading plugin (.+?)\s*$`)

// loggedPlugins: the CounterStrikeSharp plugins its log says were loaded, this run
func loggedPlugins(r *CrashReport) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range r.Logs {
		for _, x := range l.Lines {
			if m := cssLoadedRe.FindStringSubmatch(x); m != nil && !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// chainKnown: the call chain goes past the crash itself
func chainKnown(d *crashinfo.DumpAnalysis) bool {
	return len(d.Chain) > 1 || len(d.Chain) == 1 && d.ChainEnd == ""
}
