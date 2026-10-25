// Package manager runs the game servers: it starts them, watches them (process and server query), restarts them
// when they crash or freeze, installs and updates them with steamcmd, restarts them on schedule and keeps their
// console.
package manager

import (
	"context"
	"errors"
	"fmt"
	"github.com/SnipeZilla/szm/internal/netcount"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/netmon"
	"github.com/SnipeZilla/szm/internal/notify"
	"github.com/SnipeZilla/szm/internal/proc"
	"github.com/SnipeZilla/szm/internal/query"
	"github.com/SnipeZilla/szm/internal/steam"
)

// Manager runs the servers of a configuration.
type Manager struct {
	Cfg     *config.Config
	cat     atomic.Pointer[games.Catalog] // the games: replaced whole when one is added (Catalog, SetCatalog)
	Log     *events.Log
	Steam   *steam.Runner
	DataDir string
	Started time.Time

	selfNet netcount.Rate // its own traffic per second (MachineInfo)

	state  *stateFile
	addons *addonStates // the addon packages' updates

	// RemoteServer describes a server of a linked machine, for the posts of its events (on a hub)
	RemoteServer func(id string) *notify.Server
	// GameIcon gives the PNG of a game's icon (built in, or <data>/icons/<icon>.png), for Discord
	GameIcon func(icon string) []byte

	mu   sync.Mutex
	inst map[string]*Instance

	cacheMu  sync.Mutex
	dumpsMu  sync.Mutex
	selfMu   sync.Mutex
	selfAt   time.Time     // the manager's own CPU: the last measure
	selfTime time.Duration // ... its CPU time then
	selfPct  float64
	offsite  offsiteState           // the copies of the backups elsewhere
	dumpsErr string                 // why Windows writes no crash dump
	cacheLks map[string]*sync.Mutex // one per game: the shared copy is updated once at a time

	checkMu   sync.Mutex
	lastCheck map[string]time.Time // by game: last time Steam was asked
	checkNow  chan struct{}

	gf gamesFile // games.json, read again when it changes

	backupMu      sync.Mutex
	backupRunning bool   // the daily backups are being made
	backupDay     string // ... were made that day
	logsCleaning  bool   // the servers' logs folders are being cleaned

	netMu    sync.Mutex
	net      *netmon.Monitor // counts the servers' network (nil: not possible here, or not yet)
	netErr   string          // why it is not counted
	netTried time.Time       // last start of the count

	load loadSampler // the machine's CPU and network (load.go)
}

// New prepares a manager. Run starts it.
func New(cfg *config.Config, cat games.Catalog, dataDir string) *Manager {
	m := &Manager{
		Cfg:       cfg,
		Log:       events.New(filepath.Join(dataDir, "logs")),
		Steam:     &steam.Runner{},
		DataDir:   dataDir,
		Started:   time.Now(),
		state:     loadState(filepath.Join(dataDir, "state.json")),
		addons:    loadAddonStates(filepath.Join(dataDir, "addon-updates.json")),
		inst:      map[string]*Instance{},
		cacheLks:  map[string]*sync.Mutex{},
		lastCheck: map[string]time.Time{},
		checkNow:  make(chan struct{}, 1),
	}
	m.Steam.Path = m.SteamCmdPath()
	m.SetCatalog(cat)
	m.gf.stamp = gamesStamp(dataDir)
	return m
}

// Catalog gives the games. The map is never changed: a new one replaces it (SetCatalog).
func (m *Manager) Catalog() games.Catalog {
	if c := m.cat.Load(); c != nil {
		return *c
	}
	return games.Catalog{}
}

// SetCatalog replaces the games (a game was added, changed or removed).
func (m *Manager) SetCatalog(c games.Catalog) { m.cat.Store(&c) }

// SteamCmdPath is where steamcmd is (or will be downloaded).
func (m *Manager) SteamCmdPath() string {
	m.Cfg.RLock()
	p := m.Cfg.SteamCmd
	m.Cfg.RUnlock()
	if p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(m.DataDir, "steamcmd", "steamcmd.exe")
	}
	return filepath.Join(m.DataDir, "steamcmd", "steamcmd.sh")
}

// CacheDir is where the shared copies of the games are.
func (m *Manager) CacheDir() string {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	if m.Cfg.CacheDir != "" {
		return m.Cfg.CacheDir
	}
	return filepath.Join(m.DataDir, "cache")
}

// Run watches the servers until the context ends. The servers keep running after it.
func (m *Manager) Run(ctx context.Context) {
	m.Log.Add(events.Manager, "", "", "Manager started")
	m.SetupDumps()
	m.adoptAll()
	m.startNetwork()
	defer m.stopNetwork()
	go m.forwardToDiscord(ctx)
	go m.forwardToForum(ctx)
	go m.updateChecker(ctx)
	go m.addonChecker(ctx)
	go m.sampleLoad(ctx)
	if m.Debug() {
		go m.logMemory(ctx)
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	slow := 0
	for {
		select {
		case <-ctx.Done():
			m.Log.Add(events.Manager, "", "", "Manager stopped (the servers keep running)")
			return
		case <-tick.C:
		}
		slow++
		stats := slow%5 == 0
		for _, s := range m.servers() {
			m.tickServer(s, stats)
		}
		m.watchNetwork()
		if slow%30 == 0 {
			m.scheduledBackups(time.Now())
		}
		if slow%3 == 0 {
			m.watchGames()
		}
		if slow == 600 || slow%21600 == 0 {
			m.cleanServerLogs()
		}
		if slow%3600 == 0 {
			m.SetupDumps() // a game added since: its program too
		}
		if slow == 300 || (slow%3600 == 0 && m.offsiteFailing()) {
			m.SyncOffsite() // what was missed while the manager was off, or a destination could not be reached
		}
		if slow%3600 == 0 {
			m.Cfg.RLock()
			keep := m.Cfg.Logs.KeepDays
			m.Cfg.RUnlock()
			m.Log.Clean(keep)
		}
	}
}

// servers gives a copy of the servers of the configuration.
func (m *Manager) servers() []config.Server {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	out := make([]config.Server, len(m.Cfg.Servers))
	for i, s := range m.Cfg.Servers {
		out[i] = *s
	}
	return out
}

func (m *Manager) server(id string) (config.Server, bool) {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	for _, s := range m.Cfg.Servers {
		if s.ID == id {
			return *s, true
		}
	}
	return config.Server{}, false
}

func (m *Manager) instance(id string) *Instance {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, ok := m.inst[id]
	if !ok {
		i = newInstance(id)
		if m.state != nil {
			i.hostname = m.state.get(id).Hostname
		}
		m.inst[id] = i
	}
	return i
}

// queryAddr: where a server answers the Steam query (A2S): its port, or the query port of a game of another engine.
func queryAddr(s config.Server, g *games.Game) string {
	if g == nil || g.QueryOffset == 0 {
		return Address(s)
	}
	ip := s.IP
	if ip == "" || ip == "0.0.0.0" {
		ip = "127.0.0.1"
	}
	return net.JoinHostPort(ip, strconv.Itoa(s.Port+g.QueryOffset))
}

// rconAddr: where its RCON listens: its address (Source), or this machine's loopback for a game of another engine
// (their RCON, telnet, listen on every address, or on the loopback only without a password).
func rconAddr(s config.Server, g *games.Game) string {
	if g == nil || !g.IsOther() {
		return Address(s)
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port+g.RconOffset))
}

// Address to reach a server (its IP, or this machine when it listens on all).
func Address(s config.Server) string {
	ip := s.IP
	if ip == "" || ip == "0.0.0.0" {
		ip = "127.0.0.1"
	}
	return net.JoinHostPort(ip, strconv.Itoa(s.Port))
}

func (m *Manager) exePath(s config.Server, g *games.Game) (found, expected string) {
	if s.Exe != "" {
		p := s.Exe
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.InstallDir, p)
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, p
		}
		return "", p
	}
	return g.FindExe(s.InstallDir)
}

// exeNames are the program names a server of this game can run under (srcds.exe and srcds_win64.exe, and the
// one the server says): a copy started elsewhere may use the other one.
func exeNames(s config.Server, g *games.Game, exe string) []string {
	names := []string{}
	if exe != "" {
		names = append(names, filepath.Base(exe))
	}
	if s.Exe != "" {
		names = append(names, filepath.Base(s.Exe))
	}
	for _, c := range g.ExeCandidates() {
		names = append(names, filepath.Base(c))
	}
	// On Linux the scripts start the game's own program, which is the one on the port
	if runtime.GOOS != "windows" {
		for _, n := range names {
			switch b := strings.ToLower(n); {
			case strings.HasPrefix(b, "srcds_run"):
				names = append(names, "srcds_linux", "srcds_linux64")
			case strings.HasPrefix(b, "hlds_run"):
				names = append(names, "hlds_linux")
			}
		}
	}
	return names
}

/* ---------------------------------------------------------------------------------------------------------------
 * Finding the servers again when the manager starts
 * ------------------------------------------------------------------------------------------------------------- */

func (m *Manager) adoptAll() {
	for _, s := range m.servers() {
		m.adopt(s)
	}
}

func (m *Manager) adopt(s config.Server) {
	i := m.instance(s.ID)
	g := m.Catalog()[s.Game]
	saved := m.state.get(s.ID)
	desired := s.AutoStart
	switch saved.Desired {
	case "running":
		desired = true
	case "stopped":
		desired = false
	}

	i.mu.Lock()
	i.desired = desired
	i.lastRestartDay = saved.LastRestartDay
	i.mu.Unlock()

	if g == nil {
		return
	}
	exe, _ := m.exePath(s, g)
	m.readVersion(i, s, g)
	if exe == "" {
		i.mu.Lock()
		i.status = StatusNotInstalled
		i.mu.Unlock()
		return
	}
	pid := 0
	names := exeNames(s, g, exe)
	if saved.PID > 0 && proc.Alive(saved.PID) && proc.IsProgram(saved.PID, names) {
		pid = saved.PID
	} else {
		pid = proc.FindServer(names, s.InstallDir, s.IP, s.Port)
	}
	if pid == 0 {
		return
	}
	started := time.Now()
	if saved.PID == pid && saved.Started > 0 {
		started = time.Unix(saved.Started, 0)
	}
	if m.takeOver(i, s, g, exe, pid, started) {
		m.Log.Add(events.Info, s.ID, s.Name, "Found running (PID %d)", pid)
	}
}

// takeOver watches a server process the manager did not start (it was running already).
// One of another Windows user (the old manager ran as SYSTEM) may not be opened by a manager that is not
// administrator: it is then watched by its PID, and stopped through RCON only.
func (m *Manager) takeOver(i *Instance, s config.Server, g *games.Game, exe string, pid int, started time.Time) bool {
	if !proc.Alive(pid) {
		return false
	}
	var exited *proc.Exit
	p, err := os.FindProcess(pid)
	foreign := err != nil
	if foreign {
		p, exited = nil, proc.WatchPID(pid)
	} else {
		exited = proc.Exited(p, false)
	}
	i.mu.Lock()
	i.pid, i.proc, i.exited, i.since, i.watchFrom, i.foreign = pid, p, exited, started, time.Now(), foreign
	i.status = StatusStarting
	i.desired = true
	i.quietUp = true
	i.mu.Unlock()
	m.openConsole(i, s, g, true)
	m.state.update(s.ID, func(v *savedServer) { v.Desired, v.PID, v.Exe, v.Started = "running", pid, exe, started.Unix() })
	return true
}

/* ---------------------------------------------------------------------------------------------------------------
 * Watching a server, every second
 * ------------------------------------------------------------------------------------------------------------- */

var backoff = []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute, 10 * time.Minute}

func (m *Manager) tickServer(s config.Server, stats bool) {
	g := m.Catalog()[s.Game]
	if g == nil {
		return
	}
	i := m.instance(s.ID)
	m.sayNoMaps(i, s.ID, s.Name)
	m.Cfg.RLock()
	mon := m.Cfg.Monitor
	m.Cfg.RUnlock()
	now := time.Now()

	i.mu.Lock()
	// Has it ended?
	if i.exited != nil {
		select {
		case <-i.exited.Done():
			m.reapLocked(i, s)
		default:
		}
	}
	running := i.pid > 0
	busy := i.busy

	if running {
		if i.crashes > 0 && i.status == StatusRunning && now.Sub(i.since) > time.Duration(mon.StableAfter)*time.Second {
			i.crashes = 0
			i.repairWanted = false
		}
		doQuery := !i.querying && now.Sub(i.lastQuery) >= time.Duration(mon.QueryInterval)*time.Second
		if i.status == StatusStarting && !i.querying && now.Sub(i.lastQuery) >= 5*time.Second {
			doQuery = true // ask often while it starts, to show it up quickly
		}
		// A game that is not queried, or does not answer: up once it ran a moment (its process says it runs)
		if mode := g.QueryMode(); mode != games.QueryStrict && i.status == StatusStarting && !i.quietUp {
			grace := 15 * time.Second
			if mode == games.QueryTry {
				grace = time.Duration(mon.StartGrace) * time.Second
			}
			if now.Sub(i.since) >= grace {
				i.status = StatusRunning
				what := "its port is not queried"
				if mode == games.QueryTry {
					what = "it does not answer the Steam query on " + queryAddr(s, g)
				}
				go m.Log.Add(events.Start, s.ID, s.Name, "Up (%s: its players and map are not shown)", what)
			}
		}
		if g.QueryMode() == games.QueryNone {
			doQuery = false
		}
		if doQuery {
			i.querying = true
			i.lastQuery = now
			go m.queryServer(i, s, g, mon)
		}
		pid := i.pid
		i.mu.Unlock()
		if stats {
			m.readStats(i, pid)
		}
		m.readConsole(i, s, g)
		if busy == "" {
			m.schedule(i, s, g)
		}
		return
	}

	// Not running
	if busy != "" {
		i.mu.Unlock()
		return
	}
	if i.status != StatusNotInstalled {
		switch {
		case i.failed:
			i.status = StatusFailed
		case i.desired && i.crashes > 0:
			i.status = StatusCrashed
		default:
			i.status = StatusStopped
		}
	}
	start := i.desired && !i.failed && !now.Before(i.nextStart)
	repair := i.repairWanted
	update := i.updateWanted && m.autoUpdate(s)
	stopped := i.status == StatusStopped && !i.desired
	i.mu.Unlock()

	switch {
	case start && (repair || update):
		m.Update(s.ID, repair, "", true)
	case start:
		if err := m.startProcess(i, s, g); err != nil && !errors.Is(err, errBusy) {
			m.failStart(i, s, err)
		}
	case update && stopped:
		// a stopped server is updated too, so that it starts up to date
		m.Update(s.ID, false, "", true)
	}
}

// reapLocked takes note that the process ended. The lock is held.
func (m *Manager) reapLocked(i *Instance, s config.Server) {
	m.Cfg.RLock()
	giveUp := m.Cfg.Monitor.GiveUpAfter
	m.Cfg.RUnlock()

	uptime := time.Since(i.since).Round(time.Second)
	sn := i.crashSnapLocked(s, m.Catalog()[s.Game])
	i.crashWindows = nil
	i.pid, i.proc, i.exited, i.info, i.players, i.foreign = 0, nil, nil, nil, nil, false
	i.cpu, i.mem, i.goodCPU, i.goodMem = 0, 0, 0, 0
	expected := i.expectExit
	i.expectExit = false
	why := i.crashWhy
	i.crashWhy = ""
	typedBy, typedCmd := i.stopTyped, i.stopTypedCmd
	typed := typedBy != "" && time.Now().Before(i.stopTypedUntil)
	i.stopTyped, i.stopTypedCmd, i.stopTypedUntil = "", "", time.Time{}
	go m.state.update(s.ID, func(v *savedServer) { v.PID = 0 })

	// its stop command typed in its console: stopped, as with Stop
	if typed && !expected && i.busy == "" && i.desired {
		i.desired, i.failed, i.crashes, i.repairWanted = false, false, 0, false
		i.status = StatusStopped
		go func() {
			m.state.update(s.ID, func(v *savedServer) { v.Desired = "stopped" })
			m.Log.Add(events.Stop, s.ID, s.Name, "Stopped by %s (%s in its console) after %s", typedBy, typedCmd, fmtDur(uptime))
		}()
		return
	}

	if expected || i.busy != "" {
		i.status = StatusStopped
		return
	}
	if !i.desired {
		i.status = StatusStopped
		go m.Log.Add(events.Stop, s.ID, s.Name, "Stopped (it was closed outside the manager) after %s", fmtDur(uptime))
		return
	}
	i.crashes++
	if why == "" {
		why = "the process ended"
	}
	sn.why, sn.killed, sn.inARow = why, i.crashWhyKill, i.crashes
	i.crashWhyKill = false
	if bad := i.badStart; bad != "" {
		// it can not start as it is set: not started again
		i.badStart, i.startMapGone = "", bad
		i.failed, i.status = true, StatusFailed
		if sn.g == nil {
			go m.Log.Add(events.Failed, s.ID, s.Name, "Could not start: its start map %s does not exist on this server. Change its start map in Settings, then start it.", bad)
			return
		}
		go m.reportCrash(sn, events.Failed, func(string) string {
			return "Could not start: its start map " + bad + " does not exist on this server. Change its start map in Settings, then start it."
		}, "not started again: its start map must be changed first")
		return
	}
	// A DLL it needs is not there (0xC0000135): what Windows lacks (DirectX, Visual C++…) is not brought back by a
	// restart or a file check, so it waits for it to be installed; a file of the game or an addon, its files are
	// checked at once
	if runtime.GOOS == "windows" && sn.exit.Known && uint32(sn.exit.Code) == 0xC0000135 && !sn.killed && sn.g != nil {
		if exe, _ := m.exePath(s, sn.g); exe != "" {
			sn.missing, sn.missingSys = missingDLLs(exe, sn.g, s.InstallDir)
			sn.missingDone = true
		}
		if cant, what := cantStartFor(sn.missing); cant != "" {
			i.cantStart = cant
			i.failed, i.status = true, StatusFailed
			msg := "Could not start: " + i.cantStart
			go m.reportCrash(sn, events.Failed, func(string) string { return msg }, "not started again: "+what+" must be installed first")
			return
		}
		if len(sn.missing) > 0 && i.crashes == 1 {
			i.repairWanted = true // a file of the game or an addon: its files are checked before it starts again
		}
	}
	// giveUp restarts; the last one checks the files with steamcmd first. One more crash: it gives up
	if i.crashes > giveUp {
		i.failed = true
		i.status = StatusFailed
		n := i.crashes
		if fixes := m.newVersionsFor(s); len(fixes) > 0 {
			go m.installBeforeRestart(s, fixes) // the fix starts it again
		}
		if sn.g == nil {
			go m.Log.Add(events.Failed, s.ID, s.Name, "Crashed %d times in a row, even after checking its files: the manager stopped restarting it. Check its console and logs, then start it again.", n)
			return
		}
		go m.reportCrash(sn, events.Failed, func(cause string) string {
			return fmt.Sprintf("Crashed %d times in a row, even after checking its files (last: %s): the manager stopped restarting it. See the crash report, then start it again.", n, cause)
		}, "the manager gave up restarting it")
		return
	}
	wait := backoff[min(i.crashes-1, len(backoff)-1)]
	// A new version of one of its addons is out (to install): it goes in first, and the push starts the server
	fixes := m.newVersionsFor(s)
	if len(fixes) > 0 && wait < addonCrashWait {
		wait = addonCrashWait
	}
	i.nextStart = time.Now().Add(wait)
	i.status = StatusCrashed
	if i.crashes == giveUp {
		i.repairWanted = true
	}
	n, repair := i.crashes, i.repairWanted
	say := func(cause string) string {
		msg := fmt.Sprintf("Crashed (%s) after %s. Restart in %s", cause, fmtDur(uptime), fmtDur(wait))
		if len(fixes) > 0 {
			msg = fmt.Sprintf("Crashed (%s) after %s. A new version of %s is out: it is installed first, then the server starts again", cause, fmtDur(uptime), strings.Join(fixes, ", "))
		}
		if n > 1 {
			msg += fmt.Sprintf(" (crash %d in a row)", n)
		}
		if repair {
			msg += ", after checking its files with steamcmd"
		}
		return msg + "."
	}
	next := "restart in " + fmtDur(wait)
	if len(fixes) > 0 {
		next = "a new version of " + strings.Join(fixes, ", ") + " installed first, then a restart"
	}
	if repair {
		next += ", after checking its files with steamcmd"
	}
	if len(fixes) > 0 {
		go m.installBeforeRestart(s, fixes)
	}
	if sn.g == nil {
		go m.Log.Add(events.Crash, s.ID, s.Name, "%s", say(why))
		return
	}
	go m.reportCrash(sn, events.Crash, say, next)
}

func (m *Manager) failStart(i *Instance, s config.Server, err error) {
	notInstalled := errors.Is(err, errNotInstalled)
	m.Cfg.RLock()
	giveUp := m.Cfg.Monitor.GiveUpAfter
	m.Cfg.RUnlock()
	i.mu.Lock()
	prev := i.lastStartErr
	i.lastError, i.lastStartErr = err.Error(), err.Error()
	switch {
	case notInstalled:
		i.status, i.desired, i.crashes = StatusNotInstalled, false, 0
	case errors.Is(err, errPortBusy):
		// Not a crash: something else holds the port (the old manager, a copy started by hand). Try again each
		// minute, and say it once.
		i.nextStart = time.Now().Add(time.Minute)
		if prev == err.Error() {
			i.mu.Unlock()
			return
		}
	default:
		i.crashes++
		if i.crashes > giveUp {
			i.status, i.failed = StatusFailed, true
		} else {
			i.status = StatusCrashed
			i.nextStart = time.Now().Add(backoff[min(i.crashes-1, len(backoff)-1)])
		}
	}
	i.mu.Unlock()
	if notInstalled {
		m.state.update(s.ID, func(v *savedServer) { v.Desired = "stopped" })
	}
	m.Log.Add(events.Error, s.ID, s.Name, "Could not start: %v", err)
}

// nameless tells whether the server query gave players without their names: CS2 does, for every player and bot.
func nameless(players []query.Player) bool {
	for _, p := range players {
		if p.Name == "" {
			return true
		}
	}
	return false
}

// statusPlayers reads the players from RCON "status" (names, time, ping, bots), for CS2 whose server query hides
// them. Without an RCON password, or when it fails, the query's list stays. Its answer, should the console log
// show it, is kept out of the console.
func (m *Manager) statusPlayers(i *Instance, s config.Server, g *games.Game, fallback []query.Player) []query.Player {
	pw := m.rconPassword(s, g)
	if pw == "" || m.rconTarget(i, s, g) != nil {
		return fallback
	}
	out, err := query.Rcon(Address(s), pw, "status", false, 3*time.Second)
	i.noteCommand("", "status", true, err)
	if err != nil {
		return fallback
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	i.hide(append(lines, "status"), 15*time.Second)
	players, ok := query.ParseStatus(out)
	if !ok {
		return fallback
	}
	return players
}

func (m *Manager) queryServer(i *Instance, s config.Server, g *games.Game, mon config.Monitor) {
	info, err := query.QueryInfo(queryAddr(s, g), 3*time.Second)
	var players []query.Player
	if err == nil && info.Players > 0 {
		players, _ = query.QueryPlayers(queryAddr(s, g), 3*time.Second)
		if g.Engine == games.Source2 && nameless(players) {
			players = m.statusPlayers(i, s, g, players)
		}
	}
	var kill int
	var frozen bool
	i.mu.Lock()
	i.querying = false
	if i.pid == 0 {
		i.mu.Unlock()
		return
	}
	if err == nil {
		first := i.status == StatusStarting && !i.quietUp
		i.quietUp = false
		i.info, i.players, i.queryFails, i.lastAnswer = info, players, 0, time.Now()
		i.lastInfo, i.lastPlayers = info, players
		if name := strings.TrimSpace(info.Name); name != "" && name != i.hostname {
			i.hostname = name
			go m.state.update(s.ID, func(v *savedServer) { v.Hostname = name })
		}
		if i.status == StatusStarting {
			i.status = StatusRunning
		}
		i.mu.Unlock()
		if first {
			m.Log.Add(events.Start, s.ID, s.Name, "Up on %s, map %s", Address(s), info.Map)
		}
		return
	}
	// Its start map is not there: the server waits with no map, answering nothing, until it is ended. Restarting it
	// would do the same: it is ended now, and not started again until its start map is changed.
	if gone := i.startMapGoneLocked(s); gone != "" && i.busy == "" {
		kill = i.pid
		i.crashWhy = fmt.Sprintf("its start map %s does not exist on this server (its console said so)", gone)
		i.crashWhyKill, i.badStart = true, gone
		i.mu.Unlock()
		_ = proc.Kill(kill) // its crash report says it

		return
	}
	if time.Since(i.since) < time.Duration(mon.StartGrace)*time.Second || i.busy != "" || g.QueryMode() != games.QueryStrict {
		if g.QueryMode() != games.QueryStrict {
			i.info = nil // it may not answer at all: never taken as frozen for it
		}
		i.mu.Unlock()
		return
	}
	i.queryFails++
	if i.status == StatusStarting && i.lastAnswer.Before(i.watchFrom) {
		// It never answered since it started. A slow start (a big map, workshop downloads) is waited for; a server
		// still silent long after is stuck (an error window waiting for a click, a crash dialog) and is ended.
		limit := max(3*time.Duration(mon.StartGrace)*time.Second, 5*time.Minute)
		if time.Since(i.watchFrom) < limit {
			i.mu.Unlock()
			return
		}
		kill = i.pid
		i.crashWhy = fmt.Sprintf("no answer in the %s after it started", fmtDur(limit))
		i.crashWhyKill = true
		i.queryFails = 0
		i.mu.Unlock()
		i.noteWindows(kill)
		m.Log.Add(events.Crash, s.ID, s.Name, "Still not answering %s after starting: it is stuck (often an error window on the server). Ending it.", fmtDur(limit))
		_ = proc.Kill(kill)
		return
	}
	if i.queryFails >= mon.QueryFailures {
		frozen = true
		kill = i.pid
		i.crashWhy = fmt.Sprintf("no answer to %d queries", i.queryFails)
		i.crashWhyKill = true
		i.queryFails = 0
	}
	i.info = nil
	i.mu.Unlock()
	if frozen {
		i.noteWindows(kill)
		m.Log.Add(events.Crash, s.ID, s.Name, "Not answering any more (%d queries): ending it", mon.QueryFailures)
		_ = proc.Kill(kill)
	}
}

func (m *Manager) readStats(i *Instance, pid int) {
	u, err := proc.Stats(pid)
	if err != nil {
		return
	}
	now := time.Now()
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.pid != pid {
		return
	}
	if !i.lastCPUAt.IsZero() && u.CPUTime >= i.lastCPU {
		wall := now.Sub(i.lastCPUAt)
		if wall > 0 {
			i.cpu = float64(u.CPUTime-i.lastCPU) / float64(wall) / float64(runtime.NumCPU()) * 100
		}
	}
	if i.mem > 0 && u.Memory >= i.mem/10 {
		i.goodCPU, i.goodMem = i.cpu, u.Memory // what it really used (an ending process reads near nothing)
	} else if i.mem == 0 {
		i.goodCPU, i.goodMem = i.cpu, u.Memory
	}
	i.lastCPU, i.lastCPUAt, i.mem = u.CPUTime, now, u.Memory
}

// installedVersion: the version of a game installed in a folder: its steam.inf (GoldSrc, Source), else the build
// steamcmd installed (other engines).
func installedVersion(g *games.Game, dir string) (string, int64) {
	if g.IsOther() || g.VersionFile == "" {
		return steam.ReadBuild(dir, g.ServerAppID)
	}
	return steam.ReadVersion(g.Path(dir, g.VersionFile))
}

func (m *Manager) readVersion(i *Instance, s config.Server, g *games.Game) {
	text, num := installedVersion(g, s.InstallDir)
	i.mu.Lock()
	i.version, i.versionNum = text, num
	if i.updateWanted && i.updateTo > 0 && num >= i.updateTo {
		i.updateWanted = false
	}
	i.mu.Unlock()
}

func (m *Manager) autoUpdate(s config.Server) bool {
	if s.AutoUpdate != nil {
		return *s.AutoUpdate
	}
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	return m.Cfg.Updates.Auto
}

/* ---------------------------------------------------------------------------------------------------------------
 * Scheduled restarts and waiting updates of a running server
 * ------------------------------------------------------------------------------------------------------------- */

func (m *Manager) restartRule(s config.Server) (mode, at string, onlyEmpty bool) {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	mode, at, onlyEmpty = m.Cfg.Restart.Mode, m.Cfg.Restart.Time, m.Cfg.Restart.OnlyEmpty
	if s.Restart != "" {
		mode = s.Restart
	}
	if s.RestartTime != "" {
		at = s.RestartTime
	}
	return
}

func (m *Manager) schedule(i *Instance, s config.Server, g *games.Game) {
	now := time.Now()
	i.mu.Lock()
	if i.status != StatusRunning || i.busy != "" {
		i.mu.Unlock()
		return
	}
	players := 0
	if i.info != nil {
		players = i.info.Players - i.info.Bots
	}
	since := i.since
	update := i.updateWanted && m.autoUpdate(s)
	lastDay := i.lastRestartDay
	i.mu.Unlock()

	if update {
		m.Update(s.ID, false, "", true)
		return
	}

	mode, at, onlyEmpty := m.restartRule(s)
	switch mode {
	case "daily":
		h, mi, ok := config.ParseClock(at)
		if !ok {
			return
		}
		today := now.Format("2006-01-02")
		target := time.Date(now.Year(), now.Month(), now.Day(), h, mi, 0, 0, now.Location())
		if lastDay == today || now.Before(target) {
			return
		}
		if since.After(target) || now.Sub(target) > 12*time.Hour {
			// Started after today's time, or too late now: today is done
			m.markRestartDay(i, s, today)
			return
		}
		if onlyEmpty && players > 0 {
			return
		}
		m.markRestartDay(i, s, today)
		m.Restart(s.ID, "daily restart", "", true)
	case "auto":
		if now.Sub(since) >= 24*time.Hour && players <= 0 {
			m.Restart(s.ID, "running for 24 hours", "", true)
		}
	}
}

func (m *Manager) markRestartDay(i *Instance, s config.Server, day string) {
	i.mu.Lock()
	i.lastRestartDay = day
	i.mu.Unlock()
	m.state.update(s.ID, func(v *savedServer) { v.LastRestartDay = day })
}

/* ---------------------------------------------------------------------------------------------------------------
 * Steam: is there a new version?
 * ------------------------------------------------------------------------------------------------------------- */

// CheckUpdates asks Steam now (the button of the web interface).
func (m *Manager) CheckUpdates() {
	m.checkMu.Lock()
	m.lastCheck = map[string]time.Time{}
	m.checkMu.Unlock()
	select {
	case m.checkNow <- struct{}{}:
	default:
	}
}

func (m *Manager) updateChecker(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	first := time.After(15 * time.Second)
	for {
		manual := false
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-first:
		case <-m.checkNow:
			manual = true
		}
		m.Cfg.RLock()
		auto, every := m.Cfg.Updates.Auto, time.Duration(m.Cfg.Updates.CheckInterval)*time.Second
		m.Cfg.RUnlock()
		if !auto && !manual {
			// Still check the servers that update themselves on their own setting
			any := false
			for _, s := range m.servers() {
				if s.AutoUpdate != nil && *s.AutoUpdate {
					any = true
				}
			}
			if !any {
				continue
			}
		}
		m.checkAll(every, manual)
	}
}

// otherCheckEvery: how often the games of other engines are checked for an update (steamcmd is run for it)
var otherCheckEvery = 30 * time.Minute

func (m *Manager) checkAll(every time.Duration, manual bool) {
	// The servers of each game, with the version they have
	byGame := map[string][]config.Server{}
	for _, s := range m.servers() {
		if m.Catalog()[s.Game] == nil || (!manual && !m.autoUpdate(s)) {
			continue
		}
		byGame[s.Game] = append(byGame[s.Game], s)
	}
	keys := make([]string, 0, len(byGame))
	for k := range byGame {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := m.Catalog()[key]
		m.checkMu.Lock()
		wait := every
		if g.IsOther() && wait < otherCheckEvery && !manual {
			wait = otherCheckEvery // steamcmd is asked: not every few minutes
		}
		due := time.Since(m.lastCheck[key]) >= wait
		if due {
			m.lastCheck[key] = time.Now()
		}
		m.checkMu.Unlock()
		if !due {
			continue
		}
		// Steam answers per version: ask once per different version in use
		asked := map[int64]bool{}
		var latest int64 // the newest build of a game of another engine
		for _, s := range byGame[key] {
			i := m.instance(s.ID)
			m.readVersion(i, s, g)
			i.mu.Lock()
			num, ver := i.versionNum, i.version
			i.mu.Unlock()
			if num == 0 || asked[num] {
				continue
			}
			asked[num] = true
			var ok bool
			var required int64
			var err error
			if g.IsOther() {
				// its newest build, from steamcmd (once for all the builds in use)
				if latest == 0 {
					latest, err = m.Steam.LatestBuild(context.Background(), g.ServerAppID)
					if errors.Is(err, steam.ErrBusy) {
						m.checkMu.Lock()
						m.lastCheck[key] = time.Time{} // steamcmd is downloading: asked again in a moment
						m.checkMu.Unlock()
					}
				}
				ok, required = num >= latest, latest
			} else {
				ok, required, err = steam.UpToDate(g.AppID, num)
			}
			if err != nil {
				continue // Steam is down or does not know this game: try again later
			}
			if ok {
				continue
			}
			for _, o := range byGame[key] {
				oi := m.instance(o.ID)
				oi.mu.Lock()
				if oi.updateStuck == required {
					oi.mu.Unlock()
					continue
				}
				newNews := oi.versionNum == num && (!oi.updateWanted || oi.updateTo != required)
				if oi.versionNum == num {
					oi.updateWanted, oi.updateTo = true, required
				}
				oi.mu.Unlock()
				if newNews {
					if g.IsOther() {
						m.Log.Add(events.Update, o.ID, o.Name, "Steam has an update for %s (installed %s, newest build %d)", g.Short, ver, required)
					} else {
						m.Log.Add(events.Update, o.ID, o.Name, "Steam has an update for %s (installed %s, required %d)", g.Short, ver, required)
					}
				}
			}
		}
	}
}

/* ---------------------------------------------------------------------------------------------------------------
 * Discord
 * ------------------------------------------------------------------------------------------------------------- */

func (m *Manager) forwardToDiscord(ctx context.Context) {
	c := m.Log.Subscribe()
	defer m.Log.Unsubscribe(c)
	q := newPosters(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-c:
			if !ok {
				return
			}
			if e.Machine != "" && time.Since(e.Time) > 30*time.Minute {
				continue // a linked machine's, arrived late (its link was down): in Activity, not posted
			}
			m.Cfg.RLock()
			hooks := append([]config.DiscordHook(nil), m.Cfg.Discords...)
			m.Cfg.RUnlock()
			for _, d := range hooks {
				if d.Webhook == "" || !config.Wants(d.Events, d.Servers, e.Kind, e.Server) {
					continue
				}
				hook, user, ev := d.Webhook, d.Username, e
				q.post("discord "+hook, func() {
					if err := notify.Send(hook, user, ev, m.DiscordServer(ev.Server)); err != nil {
						// Not logged as an event: an error posted to Discord would loop
						fmt.Fprintln(os.Stderr, "discord:", err)
					}
				})
			}
		}
	}
}

// forwardToForum posts the chosen events on the XenForo forums.
func (m *Manager) forwardToForum(ctx context.Context) {
	notify.OnIconError = func(f notify.Forum, err error) {
		m.Log.Add(events.Manager, "", "", "XenForo (%s): the game icon is left out of the posts: %v. Settings, XenForo, Check says more.", f.URL, err)
	}
	c := m.Log.Subscribe()
	defer m.Log.Unsubscribe(c)
	q := newPosters(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-c:
			if !ok {
				return
			}
			if e.Machine != "" && time.Since(e.Time) > 30*time.Minute {
				continue // a linked machine's, arrived late (its link was down): in Activity, not posted
			}
			for _, x := range m.Forums() {
				f := ForumOf(x)
				if !f.Ready() || !config.Wants(x.Events, x.Servers, e.Kind, e.Server) {
					continue
				}
				ev := e
				q.post(fmt.Sprintf("forum %s %d %d", f.URL, f.Thread, f.Node), func() {
					if err := notify.PostForum(f, ev, m.DiscordServer(ev.Server)); err != nil {
						// Not logged as an event: an error posted to the forum would loop
						fmt.Fprintln(os.Stderr, "xenforo:", err)
					}
				})
			}
		}
	}
}

// Forums gives the XenForo forums the events go to.
func (m *Manager) Forums() []config.ForumPost {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	return append([]config.ForumPost(nil), m.Cfg.Forums...)
}

// ForumOf is where a forum's posts go.
func ForumOf(x config.ForumPost) notify.Forum {
	return notify.Forum{URL: x.URL, Key: x.Key, User: x.User, Thread: x.Thread, Node: x.Node, Prefix: x.Prefix, Icons: x.Icons}
}

// posters sends the posts of each destination in order, one a second at most (Discord limits a webhook to a few
// posts a second), each destination on its own: a slow one does not hold the others.
type posters struct {
	ctx context.Context
	mu  sync.Mutex
	q   map[string]chan func()
}

func newPosters(ctx context.Context) *posters { return &posters{ctx: ctx, q: map[string]chan func(){}} }

func (p *posters) post(key string, send func()) {
	p.mu.Lock()
	c := p.q[key]
	if c == nil {
		c = make(chan func(), 200)
		p.q[key] = c
		go func() {
			for {
				select {
				case <-p.ctx.Done():
					return
				case f := <-c:
					f()
					time.Sleep(time.Second)
				}
			}
		}()
	}
	p.mu.Unlock()
	select {
	case c <- send:
	default: // far behind (the site is down): this post is dropped rather than pile up
	}
}

// DiscordServer is what a Discord post says of a server: its names, game, address and the game's icon. nil for
// an event that is not about a server.
func (m *Manager) DiscordServer(id string) *notify.Server {
	if id == "" {
		return nil
	}
	s, ok := m.server(id)
	if !ok {
		if m.RemoteServer != nil {
			return m.RemoteServer(id) // a server of a linked machine (on a hub)
		}
		return nil
	}
	i := m.instance(id)
	i.mu.Lock()
	host := i.hostname
	i.mu.Unlock()
	out := &notify.Server{Name: s.Name, Hostname: host, Address: Address(s)}
	if g := m.Catalog()[s.Game]; g != nil {
		out.Game = g.Name
		if g.Icon != "" && m.GameIcon != nil {
			if b := m.GameIcon(g.Icon); len(b) > 0 {
				out.Icon, out.IconName = b, g.Icon+".png"
			}
		}
	}
	return out
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

/* ---------------------------------------------------------------------------------------------------------------
 * What the web interface reads
 * ------------------------------------------------------------------------------------------------------------- */

// Views gives the state of every server, in the order of the configuration.
func (m *Manager) Views() []View {
	list := m.servers()
	out := make([]View, 0, len(list))
	for _, s := range list {
		out = append(out, m.view(s))
	}
	return out
}

// View gives the state of a server.
func (m *Manager) View(id string) (View, bool) {
	s, ok := m.server(id)
	if !ok {
		return View{}, false
	}
	return m.view(s), true
}

func (m *Manager) view(s config.Server) View {
	g := m.Catalog()[s.Game]
	i := m.instance(s.ID)
	mode, at, _ := m.restartRule(s)
	if mode == "daily" {
		mode = "daily " + at
	}
	v := View{
		ID: s.ID, Name: s.Name, Game: s.Game, Address: Address(s), ConsoleLog: s.ConsoleLog, Restart: mode,
		AutoUpdate: m.autoUpdate(s), UpdateMode: s.UpdateMode, InstallDir: s.InstallDir, Notes: s.Notes, Packages: s.Packages,
	}
	if s.IP == "" || s.IP == "0.0.0.0" {
		v.Address = net.JoinHostPort("*", strconv.Itoa(s.Port))
	}
	if g != nil {
		v.GameName, v.Engine = g.Name, g.Engine
	}
	v.Vanilla = m.vanillaFor(s)
	v.CanVanilla = CanVanilla(g)
	i.mu.Lock()
	v.Status, v.Busy, v.Desired, v.PID, v.Foreign = i.status, i.busy, i.desired, i.pid, i.foreign
	v.Hostname, v.LoginNeed = i.hostname, i.loginNeed
	if f := strings.Fields(s.SteamLogin); len(f) > 0 && !strings.EqualFold(f[0], "anonymous") {
		v.SteamUser = f[0]
	}
	if i.pid > 0 {
		v.Started = i.since.Unix()
	}
	if i.info != nil {
		cp := *i.info
		v.Info = &cp
		v.Players = append([]query.Player(nil), i.players...)
	}
	v.CPU, v.Memory, v.Version, v.Update, v.Crashes = i.cpu, i.mem, i.version, i.updateWanted, i.crashes
	pid := i.pid
	if i.status == StatusCrashed && !i.nextStart.IsZero() {
		v.NextStart = i.nextStart.Unix()
	}
	v.Error, v.StartMapGone, v.CantStart = i.lastError, i.startMapGone, i.cantStart
	if !i.warnUntil.IsZero() {
		v.WarnUntil, v.WarnWhat = i.warnUntil.Unix(), i.warnWhat
	}
	steamJob := i.steamJob
	v.Stage = i.stage
	if i.copyProg != nil {
		cp := *i.copyProg
		v.Progress = &cp
	}
	i.mu.Unlock()
	if steamJob {
		v.Progress = m.Steam.Current()
	}
	if mon := m.netMonitor(); mon != nil && pid > 0 {
		if r, ok := mon.Rate(uint32(pid)); ok {
			v.Net = &r
		}
	}
	return v
}

// Console gives the last lines of a server's console.
func (m *Manager) Console(id string) []string {
	i := m.instance(id)
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.console.all()
}

// ConsoleSince gives the lines added after line n (0: all it keeps), and the number to ask from next time. For a
// browser that can not keep a live stream open (a proxy holding it back).
func (m *Manager) ConsoleSince(id string, n int64) ([]string, int64) {
	i := m.instance(id)
	i.mu.Lock()
	defer i.mu.Unlock()
	all := i.console.all()
	first := i.consoleN - int64(len(all)) // number of the line before all[0]
	if n < first || n > i.consoleN {
		n = first
	}
	return all[n-first:], i.consoleN
}

// SubscribeConsole gives a channel that receives the new lines of a server's console.
func (m *Manager) SubscribeConsole(id string) chan string {
	i := m.instance(id)
	c := make(chan string, 256)
	i.mu.Lock()
	i.consoleSub[c] = struct{}{}
	i.mu.Unlock()
	return c
}

// UnsubscribeConsole stops a channel of SubscribeConsole.
func (m *Manager) UnsubscribeConsole(id string, c chan string) {
	i := m.instance(id)
	i.mu.Lock()
	delete(i.consoleSub, c)
	i.mu.Unlock()
}

// Machine is what the web interface shows of the machine.
type Machine struct {
	Hostname string       `json:"hostname"`
	OS       string       `json:"os"`
	CPUs     int          `json:"cpus"`
	MemTotal uint64       `json:"mem_total"`
	MemFree  uint64       `json:"mem_free"`
	SelfMem  uint64       `json:"self_mem"`           // what the manager itself uses
	SelfCPU  float64      `json:"self_cpu"`           // its CPU, % of the machine (since the last ask, a few seconds)
	SelfNet  *netmon.Rate `json:"self_net,omitempty"` // its network: what it sends and receives itself
	// HideManager: the manager's own line is hidden (nor, on a hub, its nodes' shown): unless "show_manager": true
	HideManager bool            `json:"hide_manager,omitempty"`
	SelfGo      *SelfGo         `json:"self_go,omitempty"` // where its memory goes, to tell a leak from the system's use
	Uptime      int64           `json:"uptime"`            // of the manager, seconds
	Disks       []Disk          `json:"disks"`
	Steam       *steam.Progress `json:"steam,omitempty"`
	SteamCmd    bool            `json:"steamcmd"` // steamcmd is there
	Version     string          `json:"version"`
	IPs         []string        `json:"ips"`   // the IPv4 addresses of the machine, for a server's IP
	Net         NetStatus       `json:"net"`   // the count of the servers' network
	Admin       bool            `json:"admin"` // the manager runs as an administrator (Windows) or root (Linux)
	Dumps       string          `json:"dumps_error,omitempty"`
	Load        *Load           `json:"load,omitempty"` // the whole machine's CPU and network
}

// localIPs lists the IPv4 addresses of the machine's network cards that are up (not loopback, not 169.254.x.x).
// PublicIPs: the machine's addresses players outside its network reach (not 10.x, 192.168.x, 172.16-31.x,
// 100.64-127.x), sorted.
func PublicIPs() []string {
	var out []string
	for _, ip := range localIPs() {
		if publicIP(net.ParseIP(ip)) {
			out = append(out, ip)
		}
	}
	return out
}

func publicIP(p net.IP) bool {
	if p == nil || p.IsPrivate() || p.IsLoopback() || p.IsLinkLocalUnicast() || p.IsUnspecified() {
		return false
	}
	v4 := p.To4()
	return v4 == nil || !(v4[0] == 100 && v4[1]&0xC0 == 64) // 100.64.0.0/10: a provider's shared network
}

func localIPs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, it := range ifs {
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || ip.IsLinkLocalUnicast() || ip.IsLoopback() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	sort.Slice(out, func(a, b int) bool {
		x, y := net.ParseIP(out[a]).To4(), net.ParseIP(out[b]).To4()
		for k := range x {
			if x[k] != y[k] {
				return x[k] < y[k]
			}
		}
		return false
	})
	return out
}

// Disk is a disk the servers are on.
type Disk struct {
	Path  string `json:"path"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

// Version of the manager, set by main.
var Version = "dev"

// MachineInfo describes the machine.
func (m *Manager) MachineInfo() Machine {
	host, _ := os.Hostname()
	total, free := proc.SystemMemory()
	mi := Machine{
		Hostname: host, OS: runtime.GOOS + "/" + runtime.GOARCH, CPUs: runtime.NumCPU(), MemTotal: total, MemFree: free,
		Uptime: int64(time.Since(m.Started).Seconds()), Steam: m.Steam.Current(), Version: Version, IPs: localIPs(),
	}
	m.Cfg.RLock()
	mi.HideManager = m.Cfg.ManagerHidden()
	m.Cfg.RUnlock()
	if !mi.HideManager {
		if u, err := proc.Stats(os.Getpid()); err == nil {
			mi.SelfMem = u.Memory
			mi.SelfCPU = m.selfCPU(u.CPUTime)
		}
		in, out := m.selfNet.Read() // counted on its own connections (netcount): every system, and exact
		mi.SelfNet = &netmon.Rate{In: in, Out: out}
	}
	mi.Net = m.NetworkStatus()
	mi.Load = m.MachineLoad()
	if m.Debug() && !mi.HideManager {
		mi.SelfGo = selfGo()
	}
	mi.Admin = proc.IsAdmin()
	mi.Dumps = m.DumpsError()
	if _, err := os.Stat(m.SteamCmdPath()); err == nil {
		mi.SteamCmd = true
	}
	seen := map[string]bool{}
	paths := []string{m.DataDir}
	for _, s := range m.servers() {
		paths = append(paths, s.InstallDir)
	}
	for _, p := range paths {
		// each disk once: its drive (E:\), or its mount point on Linux (/, /home, /srv/games)
		name := proc.MountPoint(p)
		if seen[strings.ToUpper(name)] {
			continue
		}
		seen[strings.ToUpper(name)] = true
		if t, f := proc.Disk(p); t > 0 {
			mi.Disks = append(mi.Disks, Disk{Path: name, Total: t, Free: f})
		}
	}
	return mi
}

// selfCPU gives the manager's own CPU use (% of the machine), measured over at least 2 seconds.
func (m *Manager) selfCPU(cpu time.Duration) float64 {
	m.selfMu.Lock()
	defer m.selfMu.Unlock()
	now := time.Now()
	if m.selfAt.IsZero() {
		m.selfAt, m.selfTime = m.Started, 0
	}
	if wall := now.Sub(m.selfAt); wall >= 2*time.Second && cpu >= m.selfTime {
		m.selfPct = float64(cpu-m.selfTime) / float64(wall) / float64(runtime.NumCPU()) * 100
		m.selfAt, m.selfTime = now, cpu
	}
	return m.selfPct
}
