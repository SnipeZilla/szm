package manager

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/netmon"
	"github.com/SnipeZilla/szm/internal/proc"
	"github.com/SnipeZilla/szm/internal/query"
	"github.com/SnipeZilla/szm/internal/steam"
)

// Status of a server, as the web interface shows it.
const (
	StatusStopped      = "stopped"
	StatusStarting     = "starting"
	StatusRunning      = "running"
	StatusStopping     = "stopping"
	StatusCrashed      = "crashed" // waiting to be started again
	StatusFailed       = "failed"  // crashed too often: the manager gave up
	StatusNotInstalled = "not_installed"
)

// Instance is a server while the manager runs: its process and what is known of it.
type Instance struct {
	ID string

	mu         sync.Mutex
	status     string
	desired    bool // it should run
	pid        int
	proc       *os.Process
	exited     *proc.Exit
	since      time.Time // started
	expectExit bool      // the manager is stopping it
	// stopTyped: who typed its game's stop command in its console (#shutdown, quit), until when its end is taken
	// for a stop, not a crash
	stopTyped, stopTypedCmd string
	stopTypedUntil          time.Time
	crashes                 int      // in a row
	crashWhy                string   // why the manager killed it (frozen)
	crashWhyKill            bool     // ... it did (a crash report says "stuck")
	crashWindows            []string // the windows it showed when it was ended (an error box)
	nextStart               time.Time
	failed                  bool
	lastError               string
	lastStartErr            string // why the last start failed (not cleared by the next operation): said once

	busy      string    // the operation running: install, update, restart, stop
	warnUntil time.Time // players are being warned until then
	warnWhat  string    // "updates", "restarts", "stops"
	busySince time.Time
	cancel    func()
	steamJob  bool // the operation is waiting for or running steamcmd

	info        *query.Info
	lastInfo    *query.Info    // its last answer, kept while it does not answer (for a crash report)
	lastPlayers []query.Player // ... and its players then
	hostname    string         // the server's hostname, kept in state.json
	steamCreds  steamCreds     // a Steam account's password and Steam Guard code, in memory only, until a download works
	loginNeed   string         // what Steam asked for at the last download: password, bad_password, guard, 2fa
	players     []query.Player
	queryFails  int
	lastQuery   time.Time
	querying    bool
	lastAnswer  time.Time
	quietUp     bool      // found running when the manager started: its first answer is not news
	foreign     bool      // runs as another Windows user: watched by its PID, stopped through RCON only
	watchFrom   time.Time // when the manager started or found it: how long it has been silent

	version      string
	versionNum   int64
	updateWanted bool  // Steam has a newer version
	updateTo     int64 // the version Steam asks for
	repairWanted bool  // validate the files at the next start (after crashes)
	updateStuck  int64 // a version Steam asked for that an update did not bring: not asked again

	lastRestartDay string

	cpu       float64 // percent of the whole machine
	mem       uint64
	lastCPU   time.Duration
	lastCPUAt time.Time

	console            *ring
	consoleN           int64     // lines added since the manager started: where a reader is
	consoleOff         int64     // read up to here in the console log
	consoleMissingSaid time.Time // the start (since) for which "no console log" was said
	// its console window, copied to its log (CS2 on Windows: conwin.go)
	winAt        time.Time
	winBusy      bool
	winTail      []string
	winPid       int
	winErrPid    int
	goodCPU      float64 // the last use measured before a sudden drop (the process ending): for crash reports
	goodMem      uint64
	consoleSub   map[chan string]struct{}
	hidden       []hiddenLine         // lines already shown (an RCON answer) or not to show: skipped if the log has them too
	sent         []SentCommand        // the last commands sent through RCON (commands.go)
	noMap        map[string]time.Time // maps its console said it does not have (nomap.go)
	noMapNew     []string             // ... not said in its activity yet
	badStart     string               // it was ended as its start map is not there: not restarted
	stage        string               // what its operation waits for or does now, in words ("Copying files")
	copyProg     *steam.Progress      // the copy from the shared copy, while it runs
	startMapGone string               // ... and so it waits: shown until it is started again
	// cantStart: why it can not start until something is installed on Windows (a DLL it needs is not there), and
	// so it waits: shown until it is started again
	cantStart string

	backupMu sync.Mutex // one backup at a time

	// The size of its folder, measured while its Overview is open
	size   int64
	sizeAt time.Time
	sizing bool
}

type steamCreds struct{ password, guard, twoFactor string }

type hiddenLine struct {
	text  string
	until time.Time
}

// hide keeps lines out of the console for a while, should the console log have them too. The lock must not be held.
func (i *Instance) hide(lines []string, d time.Duration) {
	until := time.Now().Add(d)
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			i.hidden = append(i.hidden, hiddenLine{t, until})
		}
	}
	if len(i.hidden) > 2000 {
		i.hidden = i.hidden[len(i.hidden)-2000:]
	}
}

// unhidden drops from lines of the console log those hidden. The lock must be held.
func (i *Instance) unhidden(lines []string) []string {
	if len(i.hidden) == 0 {
		return lines
	}
	now := time.Now()
	live := i.hidden[:0]
	for _, h := range i.hidden {
		if now.Before(h.until) {
			live = append(live, h)
		}
	}
	i.hidden = live
	out := lines[:0:0]
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if quietRconRe.MatchString(t) {
			continue // the manager's own stats and status: not what anyone typed
		}
		k := -1
		for n, h := range i.hidden {
			// the log can put a time in front of the line
			if t == h.text || (len(h.text) >= 6 && strings.HasSuffix(t, h.text)) {
				k = n
				break
			}
		}
		if k < 0 {
			out = append(out, l)
			continue
		}
		i.hidden = append(i.hidden[:k], i.hidden[k+1:]...)
	}
	return out
}

func newInstance(id string) *Instance {
	return &Instance{ID: id, status: StatusStopped, console: newRing(600), consoleSub: map[chan string]struct{}{}}
}

// View is a server as the web interface shows it.
type View struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Game     string          `json:"game"`
	GameName string          `json:"game_name"`
	Engine   string          `json:"engine"`
	Status   string          `json:"status"`
	Busy     string          `json:"busy,omitempty"`
	Progress *steam.Progress `json:"progress,omitempty"`
	// Stage: what its operation waits for or does now, in words: "Waiting for the shared copy…", "Copying files"
	Stage     string         `json:"stage,omitempty"`
	Desired   bool           `json:"desired"`
	PID       int            `json:"pid,omitempty"`
	Foreign   bool           `json:"foreign,omitempty"` // started as another Windows user: limited control
	Started   int64          `json:"started,omitempty"` // unix seconds
	Info      *query.Info    `json:"info,omitempty"`
	Players   []query.Player `json:"players,omitempty"`
	Hostname  string         `json:"hostname,omitempty"`   // as the server last said it
	LoginNeed string         `json:"login_need,omitempty"` // what Steam wants before the download goes on
	SteamUser string         `json:"steam_user,omitempty"` // the Steam account it downloads with (not its password)
	CPU       float64        `json:"cpu"`
	Memory    uint64         `json:"memory"`
	Version   string         `json:"version,omitempty"`
	Update    bool           `json:"update"` // an update is waiting
	Crashes   int            `json:"crashes"`
	NextStart int64          `json:"next_start,omitempty"`
	Error     string         `json:"error,omitempty"`
	// StartMapGone: it can not start, its start map is not on the server (its console said so)
	StartMapGone string   `json:"start_map_gone,omitempty"`
	CantStart    string   `json:"cant_start,omitempty"` // it can not start until something is installed on Windows (a DLL)
	WarnUntil    int64    `json:"warn_until,omitempty"` // players warned: the server stops then (unix seconds)
	WarnWhat     string   `json:"warn_what,omitempty"`
	Address      string   `json:"address"`
	ConsoleLog   bool     `json:"console_log"`
	Restart      string   `json:"restart"` // the mode in force
	AutoUpdate   bool     `json:"autoupdate"`
	UpdateMode   string   `json:"update_mode"`
	InstallDir   string   `json:"install_dir"`
	Notes        string   `json:"notes,omitempty"`
	Packages     []string `json:"packages,omitempty"` // addon packages it gets
	// Vanilla: started without its addons while it waits for a fix of this package; CanVanilla: its game can
	Vanilla    string       `json:"vanilla,omitempty"`
	CanVanilla bool         `json:"can_vanilla,omitempty"`
	Net        *netmon.Rate `json:"net,omitempty"` // its network, bytes per second (Windows, as administrator)
}

/* ---------------------------------------------------------------------------------------------------------------
 * Console: the last lines of the server's console log and of the operations, and who listens to them
 * ------------------------------------------------------------------------------------------------------------- */

type ring struct {
	lines []string
	next  int
	full  bool
}

func newRing(n int) *ring { return &ring{lines: make([]string, n)} }

func (r *ring) add(s string) {
	r.lines[r.next] = s
	r.next++
	if r.next == len(r.lines) {
		r.next, r.full = 0, true
	}
}

func (r *ring) all() []string {
	if !r.full {
		return append([]string(nil), r.lines[:r.next]...)
	}
	out := make([]string, 0, len(r.lines))
	out = append(out, r.lines[r.next:]...)
	return append(out, r.lines[:r.next]...)
}

// say adds lines to the console and hands them to the listeners. The lock must not be held.
func (i *Instance) say(lines ...string) {
	i.mu.Lock()
	subs := make([]chan string, 0, len(i.consoleSub))
	for c := range i.consoleSub {
		subs = append(subs, c)
	}
	for _, l := range lines {
		i.console.add(l)
		if n := noSuchMap(l); n != "" {
			i.noteNoMapLocked(n, time.Now())
		}
	}
	i.consoleN += int64(len(lines))
	i.mu.Unlock()
	for _, l := range lines {
		for _, c := range subs {
			select {
			case c <- l:
			default:
			}
		}
	}
}
