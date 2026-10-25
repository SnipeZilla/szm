package manager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
	"github.com/SnipeZilla/szm/internal/query"
	"github.com/SnipeZilla/szm/internal/steam"
)

var (
	errBusy         = errors.New("the server is busy")
	errNotInstalled = errors.New("not installed")
	errUnknown      = errors.New("no such server")
	errPortBusy     = errors.New("port in use")
)

// begin marks a server busy with an operation; false when it already is.
func (i *Instance) begin(op string) (context.Context, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.busy != "" {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	i.busy, i.busySince, i.cancel, i.lastError = op, time.Now(), cancel, ""
	return ctx, true
}

// setStage says what the operation waits for or does now ("" when steamcmd's progress says it).
func (i *Instance) setStage(s string) {
	i.mu.Lock()
	i.stage = s
	i.mu.Unlock()
}

func (i *Instance) end(err error) {
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	i.busy, i.cancel, i.steamJob, i.stage, i.copyProg = "", nil, false, "", nil
	if err != nil {
		i.lastError = err.Error()
	}
	i.mu.Unlock()
}

func (m *Manager) lookup(id string) (config.Server, *games.Game, *Instance, error) {
	s, ok := m.server(id)
	if !ok {
		return s, nil, nil, errUnknown
	}
	g := m.Catalog()[s.Game]
	if g == nil {
		return s, nil, nil, fmt.Errorf("unknown game %q", s.Game)
	}
	return s, g, m.instance(id), nil
}

func (m *Manager) setDesired(i *Instance, s config.Server, run bool) {
	i.mu.Lock()
	i.desired = run
	i.mu.Unlock()
	d := "stopped"
	if run {
		d = "running"
	}
	m.state.update(s.ID, func(v *savedServer) { v.Desired = d })
}

/* ---------------------------------------------------------------------------------------------------------------
 * Start, stop, restart
 * ------------------------------------------------------------------------------------------------------------- */

// Start starts a server now (and forgets its crashes).
func (m *Manager) Start(id, by string) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	i.mu.Lock()
	if i.busy != "" {
		i.mu.Unlock()
		return fmt.Errorf("%w (%s)", errBusy, i.busy)
	}
	if i.pid > 0 {
		i.mu.Unlock()
		return errors.New("it is already running")
	}
	i.failed, i.crashes, i.nextStart, i.lastError, i.repairWanted = false, 0, time.Time{}, "", false
	i.mu.Unlock()
	m.setDesired(i, s, true)
	if by != "" {
		m.Log.Add(events.Info, s.ID, s.Name, "Start asked by %s", by)
	}
	if err := m.startProcess(i, s, g); err != nil {
		if errors.Is(err, errBusy) {
			// The watch, seeing it wanted, started it at the same moment: it is starting, all is well
			i.mu.Lock()
			starting := i.busy == "start" || i.pid > 0
			i.mu.Unlock()
			if starting {
				return nil
			}
		}
		m.failStart(i, s, err)
		return err
	}
	return nil
}

// startProcess starts the server's program, unless an operation is running.
func (m *Manager) startProcess(i *Instance, s config.Server, g *games.Game) error {
	if _, ok := i.begin("start"); !ok {
		return errBusy
	}
	err := m.launch(i, s, g)
	i.end(nil)
	return err
}

// launch starts the server's program. The caller holds the operation.
func (m *Manager) launch(i *Instance, s config.Server, g *games.Game) error {
	i.mu.Lock()
	running := i.pid > 0
	i.mu.Unlock()
	if running {
		return nil
	}
	exe, expected := m.exePath(s, g)
	if exe == "" {
		return fmt.Errorf("%w: %s is missing (install the server first)", errNotInstalled, expected)
	}
	// Already running (started by hand, by the old manager, or before the manager lost track of it)? Watch that
	// one instead of starting a second copy.
	if pid := proc.FindServer(exeNames(s, g, exe), s.InstallDir, s.IP, s.Port); pid > 0 {
		if m.takeOver(i, s, g, exe, pid, time.Now()) {
			m.Log.Add(events.Info, s.ID, s.Name, "Already running (PID %d): the manager watches it", pid)
			return nil
		}
	}
	if err := portFree(s, g); err != nil {
		return err
	}
	m.applyVanilla(s, g) // without its addons while it waits for a fix, or with them again
	if changed, err := m.patchMetamod(s, g); err != nil {
		m.Log.Add(events.Error, s.ID, s.Name, "Metamod: %v", err)
	} else if changed {
		m.Log.Add(events.Info, s.ID, s.Name, "Metamod added back to gameinfo.gi")
	}
	m.openConsole(i, s, g, false)
	m.newConfig(i, s, g)
	m.setRconConfig(i, s, g)

	m.Cfg.RLock()
	prio := m.Cfg.Priority
	m.Cfg.RUnlock()
	if s.Priority != "" {
		prio = s.Priority
	}
	args := s.BuildArgs(g)
	// Linux: srcds_run and hlds_run start the server again by themselves after a crash; the manager does it
	if b := strings.ToLower(filepath.Base(exe)); (strings.HasPrefix(b, "srcds_run") || strings.HasPrefix(b, "hlds_run")) && !hasArg(args, "-norestart") {
		args = append([]string{"-norestart"}, args...)
	}
	m.linkSteamClient(i) // Linux: Steam's library where the servers look for it
	spec := proc.Spec{Exe: exe, Args: args, Dir: filepath.Dir(exe), Priority: prio}
	spec.Env = gameEnv(g, s.InstallDir)
	if s.ConsoleLog && g.ConsoleCapture {
		if runtime.GOOS == "windows" {
			spec.WideConsole = true // read by captureWindow
		} else {
			spec.Output = g.Path(s.InstallDir, g.ConsoleLog)
		}
	}
	p, err := proc.Start(spec)
	if err != nil {
		return err
	}
	now := time.Now()
	i.mu.Lock()
	i.pid, i.proc, i.exited, i.since, i.watchFrom = p.Pid, p, proc.Exited(p, true), now, now
	i.quietUp = false
	i.status, i.info, i.players, i.queryFails, i.lastQuery = StatusStarting, nil, nil, 0, now
	i.lastInfo, i.lastPlayers, i.crashWindows = nil, nil, nil
	i.expectExit, i.crashWhy, i.lastError, i.lastStartErr, i.startMapGone, i.cantStart = false, "", "", "", "", ""
	i.cpu, i.lastCPUAt = 0, time.Time{}
	i.mu.Unlock()
	i.say("── " + now.Format("15:04:05") + " started: " + s.HideSecrets(config.Redact(config.QuoteArgs(append([]string{filepath.Base(exe)}, args...)))))
	m.state.update(s.ID, func(v *savedServer) {
		v.Desired, v.PID, v.Exe, v.Started = "running", p.Pid, exe, now.Unix()
	})
	m.Log.Add(events.Info, s.ID, s.Name, "Starting (PID %d)", p.Pid)
	return nil
}

// Stop stops a server; it stays stopped. With warn, players are warned first (not when it is empty).
func (m *Manager) Stop(id, by string, warn bool) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	ctx, ok := i.begin("stop")
	if !ok {
		// Stopping cancels what waits (a crashed server's restart, players' warning)
		i.mu.Lock()
		busy := i.busy
		i.mu.Unlock()
		return fmt.Errorf("%w (%s)", errBusy, busy)
	}
	go func() {
		if warn {
			m.Cfg.RLock()
			secs := m.Cfg.Restart.WarnSeconds
			m.Cfg.RUnlock()
			if werr := m.warnPlayers(ctx, i, s, g, secs, "stops"); werr != nil {
				i.end(nil)
				m.Log.Add(events.Info, s.ID, s.Name, "Stop cancelled")
				return
			}
		}
		m.setDesired(i, s, false)
		i.mu.Lock()
		i.failed, i.crashes, i.repairWanted = false, 0, false
		i.mu.Unlock()
		err := m.stopProcess(ctx, i, s, g)
		i.end(err)
		who := ""
		if by != "" {
			who = " by " + by
		}
		m.Log.Add(events.Stop, s.ID, s.Name, "Stopped%s", who)
	}()
	return nil
}

// stopProcess ends the server's program: asked to end (RCON quit, its stop commands, Ctrl+C), then killed if it is still
// there when its game's wait is over; killed at once when nothing could ask it.
func (m *Manager) stopProcess(ctx context.Context, i *Instance, s config.Server, g *games.Game) error {
	i.mu.Lock()
	pid, exit := i.pid, i.exited
	if pid == 0 || exit == nil {
		i.mu.Unlock()
		return nil
	}
	i.expectExit = true
	i.status = StatusStopping
	i.mu.Unlock()

	wait := g.StopTimeout()
	if !m.askToEnd(i, s, g, pid) {
		wait = 0 // nothing could ask it: waiting would change nothing
	}
	exited := exit.Done()
	select {
	case <-exited:
	case <-time.After(wait):
		kerr := proc.Kill(pid)
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
			i.mu.Lock()
			foreign := i.foreign
			i.expectExit = false
			i.status = StatusRunning
			i.mu.Unlock()
			if foreign || kerr != nil {
				return fmt.Errorf("PID %d would not end: it runs as another Windows user (started by the old manager?) and "+
					"did not quit through RCON. Check its RCON password, or run the manager as administrator or as the service", pid)
			}
			return fmt.Errorf("PID %d would not end", pid)
		}
	}
	i.mu.Lock()
	if i.pid == pid {
		i.pid, i.proc, i.exited, i.info, i.players, i.cpu, i.mem, i.foreign = 0, nil, nil, nil, nil, 0, 0, false
		i.goodCPU, i.goodMem = 0, 0
		i.expectExit = false
		i.status = StatusStopped
	}
	i.mu.Unlock()
	i.say("── " + time.Now().Format("15:04:05") + " stopped")
	m.state.update(s.ID, func(v *savedServer) { v.PID = 0 })
	return nil
}

// gameEnv: the environment a game's program needs ({dir}: the server's folder); a search path (LD_LIBRARY_PATH,
// PATH) keeps what it was after.
func gameEnv(g *games.Game, dir string) []string {
	var out []string
	keys := make([]string, 0, len(g.Env))
	for k := range g.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "LD_LIBRARY_PATH" && runtime.GOOS == "windows" {
			continue // where Linux looks for its libraries
		}
		v := strings.ReplaceAll(g.Env[k], "{dir}", dir)
		if k == "LD_LIBRARY_PATH" || k == "PATH" {
			if old := os.Getenv(k); old != "" {
				v += string(os.PathListSeparator) + old
			}
		}
		out = append(out, k+"="+v)
	}
	return out
}

// errNoRcon: the game has no RCON the manager speaks
var errNoRcon = errors.New("this game has no RCON the manager can use: its console commands are typed in its window")

// sendRcon sends a command to a server with its game's RCON.
func (m *Manager) sendRcon(s config.Server, g *games.Game, command string, timeout time.Duration) (string, error) {
	pw := m.rconPassword(s, g)
	switch g.RconMode() {
	case games.RconWeb:
		return query.WebRcon(rconAddr(s, g), pw, command, timeout)
	case games.RconTelnet:
		return query.Telnet(rconAddr(s, g), pw, command, timeout)
	case games.RconBattlEye:
		out, err := query.BattlEye(rconAddr(s, g), pw, command, timeout)
		if errors.Is(err, query.ErrBattlEyeSilent) {
			if i := m.instance(s.ID); i != nil {
				i.mu.Lock()
				up := time.Since(i.since)
				running := i.pid != 0
				i.mu.Unlock()
				if running && up < 10*time.Minute {
					err = fmt.Errorf("BattlEye's RCon does not answer yet (started %s ago): it opens once the server has finished loading, often a minute or two", fmtDur(up.Round(time.Second)))
				}
			}
		}
		return out, err
	case games.RconNone:
		return "", errNoRcon
	}
	return query.Rcon(rconAddr(s, g), pw, command, g.Engine == games.GoldSrc, timeout)
}

// askToEnd asks a server to save and end: RCON quit (Source), the stop commands of its game (other engines: their
// RCON), else Ctrl+C (their console: most save and quit on it).
// It tells whether it could ask (else it is ended by force at once).
func (m *Manager) askToEnd(i *Instance, s config.Server, g *games.Game, pid int) bool {
	if !g.IsOther() {
		if pw := m.rconPassword(s, g); pw != "" {
			terr := m.rconTarget(i, s, g)
			if terr == nil {
				_, terr = m.sendRcon(s, g, "quit", 3*time.Second)
				i.noteCommand("", "quit", false, terr)
			}
			if terr == nil {
				return true
			}
			i.say("[stop] RCON quit: " + terr.Error() + ": ended by force now")
			return false
		}
		i.say("[stop] no RCON password to send it quit: ended by force now")
		return false
	}
	if len(g.Stop) > 0 && g.RconMode() != games.RconNone && m.rconPassword(s, g) != "" {
		var err error
		for _, c := range g.Stop {
			if _, err = m.sendRcon(s, g, c, 10*time.Second); err != nil {
				break
			}
			i.noteCommand("", c, false, nil)
		}
		if err == nil {
			i.say(fmt.Sprintf("[stop] %s sent: it saves and ends (ended by force after %s)", strings.Join(g.Stop, ", "), fmtDur(g.StopTimeout())))
			return true
		}
		i.say("[stop] RCON: " + err.Error() + ": Ctrl+C instead")
	}
	how, err := proc.Interrupt(pid)
	if err != nil {
		i.say("[stop] " + err.Error() + ": ended by force now")
		return false
	}
	i.say(fmt.Sprintf("[stop] %s: it saves and ends (ended by force after %s)", how, fmtDur(g.StopTimeout())))
	return true
}

// Restart stops and starts a server. With warn (always for automatic restarts), players are warned first.
func (m *Manager) Restart(id, reason, by string, warn bool) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	ctx, ok := i.begin("restart")
	if !ok {
		return errBusy
	}
	m.Cfg.RLock()
	secs := m.Cfg.Restart.WarnSeconds
	m.Cfg.RUnlock()
	go func() {
		var err error
		defer func() { i.end(err) }()
		if by != "" {
			m.Log.Add(events.Restart, s.ID, s.Name, "Restart asked by %s", by)
		}
		if warn || by == "" {
			if werr := m.warnPlayers(ctx, i, s, g, secs, "restarts"); werr != nil {
				m.Log.Add(events.Info, s.ID, s.Name, "Restart cancelled")
				return
			}
		}
		if by == "" {
			m.Log.Add(events.Restart, s.ID, s.Name, "Restarting (%s)", reason)
		}
		m.setDesired(i, s, true)
		i.mu.Lock()
		i.failed, i.crashes = false, 0
		i.mu.Unlock()
		if err = m.stopProcess(ctx, i, s, g); err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Restart: %v", err)
			return
		}
		if err = m.launch(i, s, g); err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Could not start: %v", err)
		}
	}()
	return nil
}

// Cancel stops what a server is busy with (a steamcmd download, the warning of the players).
func (m *Manager) Cancel(id string) error {
	i := m.instance(id)
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cancel == nil {
		return errors.New("nothing to cancel")
	}
	i.cancel()
	return nil
}

// warnPlayers tells the players what is coming, until the time is over or the server is empty.
func (m *Manager) warnPlayers(ctx context.Context, i *Instance, s config.Server, g *games.Game, seconds int, what string) error {
	m.Cfg.RLock()
	every := time.Duration(m.Cfg.Updates.SayEvery) * time.Second
	m.Cfg.RUnlock()
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	var lastSay time.Time
	logged := false
	defer func() {
		i.mu.Lock()
		i.warnUntil, i.warnWhat = time.Time{}, ""
		i.mu.Unlock()
	}()
	for {
		i.mu.Lock()
		humans := 0
		if i.info != nil {
			humans = i.info.Players - i.info.Bots
		}
		running := i.pid > 0
		i.mu.Unlock()
		left := time.Until(deadline)
		if !running || humans <= 0 || left <= 0 || m.sayCommand(s, "") == "" {
			return nil // nobody to warn, or no way to (a game without RCON): at once
		}
		if !logged {
			logged = true
			i.mu.Lock()
			i.warnUntil, i.warnWhat = deadline, what
			i.mu.Unlock()
			m.Log.Add(events.Info, s.ID, s.Name, "%d player(s) on it: warning them, the server %s in %s", humans, what, sayDur(left))
		}
		if time.Since(lastSay) >= every && left >= 3*time.Second {
			lastSay = time.Now()
			if c := m.sayCommand(s, fmt.Sprintf("Server %s in %s", what, sayDur(left))); c != "" {
				_, _ = m.Rcon(s.ID, c)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// playerSay says something to a server's players (the tests replace it)
var playerSay = func(m *Manager, s config.Server, msg string) {
	if c := m.sayCommand(s, msg); c != "" {
		_, _ = m.Rcon(s.ID, c)
	}
}

// sayCommand: the RCON command that writes text to a server's players ("" when its game has none).
func (m *Manager) sayCommand(s config.Server, text string) string {
	g := m.Catalog()[s.Game]
	if g == nil || !g.IsOther() {
		return s.SayCommand + " " + text
	}
	if g.Say == "" || g.RconMode() == games.RconNone || s.RconPassword == "" {
		return ""
	}
	return strings.ReplaceAll(g.Say, "{text}", text)
}

// Said to the players of a server whose update waits for the other servers' copies, then when its own begins
var (
	sayQueued = "Server update: the other servers update first, this one follows in a moment"
	sayNow    = "Time's up: the server is updating now"
)

// warnWhileQueued warns a server's players of its update (when warn), from now: the countdown of seconds, then, while
// it waits for its turn to copy, every say_every that it follows the other servers. It returns when the countdown is
// over and its turn has come (the players told), with what gives the turn to the next server.
func (m *Manager) warnWhileQueued(ctx context.Context, i *Instance, s config.Server, warn bool, seconds int) (func(), error) {
	type turn struct {
		next func()
		err  error
	}
	got := make(chan turn, 1)
	go func() {
		next, err := waitCopyTurn(ctx, i)
		got <- turn{next, err}
	}()
	m.Cfg.RLock()
	every := time.Duration(m.Cfg.Updates.SayEvery) * time.Second
	m.Cfg.RUnlock()
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	var lastSay time.Time
	logged, warned := false, false
	var mine *turn
	defer func() {
		i.mu.Lock()
		i.warnUntil, i.warnWhat = time.Time{}, ""
		i.mu.Unlock()
	}()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		i.mu.Lock()
		humans := 0
		if i.info != nil {
			humans = i.info.Players - i.info.Bots
		}
		running := i.pid > 0
		i.mu.Unlock()
		left := time.Until(deadline)
		players := warn && running && humans > 0
		if mine != nil && (!players || left <= 0) {
			if players && warned {
				playerSay(m, s, sayNow)
			}
			return mine.next, nil
		}
		if players {
			if !logged {
				logged = true
				m.Log.Add(events.Info, s.ID, s.Name, "%d player(s) on it: warning them, the server updates in %s", humans, sayDur(left))
			}
			i.mu.Lock()
			if left > 0 {
				i.warnUntil, i.warnWhat = deadline, "updates"
			} else {
				i.warnUntil, i.warnWhat = time.Time{}, ""
			}
			i.mu.Unlock()
			if time.Since(lastSay) >= every && (left >= 3*time.Second || left <= 0) {
				lastSay, warned = time.Now(), true
				msg := "Server updates in " + sayDur(left)
				if left <= 0 {
					msg = sayQueued
				}
				playerSay(m, s, msg)
			}
		}
		select {
		case t := <-got:
			if t.err != nil {
				return nil, t.err
			}
			mine = &t
			continue
		case <-ctx.Done():
			if mine != nil {
				mine.next()
			} else {
				go func() { // its turn may come still: given on at once
					if t := <-got; t.err == nil {
						t.next()
					}
				}()
			}
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

// portFree checks that nothing else listens on the server's port: UDP (the game) and, for Source, TCP (RCON).
func portFree(s config.Server, g *games.Game) error {
	host := s.IP
	if host == "0.0.0.0" {
		host = ""
	}
	addr := net.JoinHostPort(host, strconv.Itoa(s.Port))
	busy := func(proto string) error {
		who := "another program"
		if proto == "UDP" {
			if pid := proc.PortOwner(s.IP, s.Port); pid > 0 {
				if name := proc.ProcessName(pid); name != "" {
					who = fmt.Sprintf("%s (PID %d)", name, pid)
				}
			}
		}
		return fmt.Errorf("%w: %s %s is used by %s, which is not this server. Stop it, or give this server "+
			"another port", errPortBusy, proto, addr, who)
	}
	c, err := net.ListenPacket("udp", addr)
	if err != nil {
		// Windows refuses this test when another program listens on the port of every address (0.0.0.0: CS2 does,
		// whatever its -ip), but the game servers open theirs anyway (SO_REUSEADDR), and Windows gives the packets
		// of this IP to the socket bound to it. Only a program on this very IP takes the port.
		e, ok := proc.PortEntry(s.IP, s.Port)
		if !(ok && host != "" && e.IP.IsUnspecified()) {
			return busy("UDP")
		}
	} else {
		c.Close()
	}
	if g.Engine != games.GoldSrc {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return busy("TCP")
		}
		l.Close()
	}
	return nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * Install and update
 * ------------------------------------------------------------------------------------------------------------- */

// EnsureSteamCmd downloads steamcmd when it is not there yet.
func (m *Manager) EnsureSteamCmd() error { return m.ensureSteamCmd() }

func (m *Manager) ensureSteamCmd() error {
	p := m.SteamCmdPath()
	m.Steam.Path = p
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	m.Log.Add(events.SteamCmd, "", "", "Downloading steamcmd into %s", filepath.Dir(p))
	got, err := steam.Download(filepath.Dir(p))
	if err != nil {
		return fmt.Errorf("download steamcmd: %w", err)
	}
	m.Steam.Path = got
	return nil
}

func (m *Manager) steamTries(g *games.Game) int {
	if g.Engine == games.GoldSrc {
		return 4 // app 90 rarely installs in one go
	}
	return 2
}

func (m *Manager) runSteam(ctx context.Context, i *Instance, s config.Server, g *games.Game, dir string, validate bool, label string) error {
	if err := m.ensureSteamCmd(); err != nil {
		return err
	}
	i.mu.Lock()
	i.steamJob = true
	i.mu.Unlock()
	i.say("── " + time.Now().Format("15:04:05") + " steamcmd: " + label)
	i.mu.Lock()
	cr := i.steamCreds
	i.loginNeed = ""
	i.mu.Unlock()
	job := steam.Job{Dir: dir, AppID: g.ServerAppID, AppSetConfig: g.AppSetConfig, Login: s.SteamLogin, Validate: validate, Label: label,
		Password: cr.password, GuardCode: cr.guard, TwoFactor: cr.twoFactor}
	err := m.Steam.Run(ctx, job, m.steamTries(g), func(line string) {
		l := hideLogin(line, s.SteamLogin)
		if cr.password != "" {
			l = strings.ReplaceAll(l, cr.password, "••••")
		}
		i.say("[steamcmd] " + l)
	})
	i.mu.Lock()
	i.steamJob = false
	var le *steam.LoginError
	switch {
	case errors.As(err, &le):
		i.loginNeed = le.Need
		i.steamCreds.guard, i.steamCreds.twoFactor = "", "" // a code is good once; the password stays for the next try
		if le.Need == "bad_password" || le.Need == "password" {
			i.steamCreds.password = ""
		}
	case err == nil:
		i.steamCreds = steamCreds{} // steamcmd remembers the account now: nothing is kept
	}
	i.mu.Unlock()
	if le != nil {
		i.say("[steamcmd] " + le.Error())
	}
	return err
}

// SteamLogin downloads a server again with a Steam account: the account is saved (not its password), the password
// and the Steam Guard code Steam asked for are kept in memory for this download only. A server not installed yet is
// installed, else updated.
func (m *Manager) SteamLogin(id, account, password, guard, twoFactor, by string) error {
	s, _, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	account = strings.TrimSpace(account)
	if account == "" {
		account = "anonymous"
	}
	if f := strings.Fields(account); len(f) > 0 {
		account = f[0] // the account only: a password never goes into szm.json
	}
	if strings.EqualFold(account, "anonymous") {
		account, password, guard, twoFactor = "anonymous", "", "", ""
	}
	m.Cfg.Lock()
	changed := false
	for _, o := range m.Cfg.Servers {
		if o.ID == id && o.SteamLogin != account {
			o.SteamLogin, changed = account, true
		}
	}
	m.Cfg.Unlock()
	if changed {
		if err := m.Cfg.Save(); err != nil {
			return err
		}
	}
	i.mu.Lock()
	if password != "" {
		i.steamCreds.password = password
	}
	i.steamCreds.guard, i.steamCreds.twoFactor = strings.TrimSpace(guard), strings.TrimSpace(twoFactor)
	installed := i.status != StatusNotInstalled
	i.mu.Unlock()
	if installed {
		return m.Update(s.ID, false, by, false)
	}
	return m.Install(s.ID, by, false)
}

// SetSteamPassword keeps a Steam account's password in memory for the next download of a server (a new server).
func (m *Manager) SetSteamPassword(id, password string) {
	i := m.instance(id)
	i.mu.Lock()
	i.steamCreds.password = password
	i.mu.Unlock()
}

// hideLogin hides a Steam password that steamcmd could repeat.
func hideLogin(line, login string) string {
	f := strings.Fields(login)
	if len(f) > 1 {
		line = strings.ReplaceAll(line, f[1], "••••")
	}
	return line
}

// CacheFor is the folder of a game's shared copy (update mode "cache").
func (m *Manager) CacheFor(g *games.Game) string {
	name := strconv.Itoa(g.ServerAppID)
	if g.AppSetConfig != "" && g.Game != "cstrike" {
		name += "-" + g.Game // GoldSrc mods share app 90: one copy each
	}
	return filepath.Join(m.CacheDir(), name)
}

// copyTurn lets one server at a time copy from a shared copy: the copies go one after the other, each at the disk's
// full speed, and each server is stopped only for its own copy (the others play on meanwhile).
var copyTurn = make(chan struct{}, 1)

// waitCopyTurn waits for the server's turn to copy; done gives it to the next one.
func waitCopyTurn(ctx context.Context, i *Instance) (done func(), err error) {
	select {
	case copyTurn <- struct{}{}:
	default:
		i.setStage("Waiting for its turn to copy: one server at a time")
		select {
		case copyTurn <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return func() { <-copyTurn }, nil
}

func (m *Manager) cacheLock(g *games.Game) func() {
	m.cacheMu.Lock()
	l, ok := m.cacheLks[g.Key]
	if !ok {
		l = new(sync.Mutex)
		m.cacheLks[g.Key] = l
	}
	m.cacheMu.Unlock()
	l.Lock()
	return l.Unlock
}

// Install installs a server's files (a new server, or one whose files are missing).
func (m *Manager) Install(id, by string, startAfter bool) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	ctx, ok := i.begin("install")
	if !ok {
		return errBusy
	}
	go func() {
		var err error
		defer func() { i.end(err) }()
		start := time.Now()
		m.Log.Add(events.Update, s.ID, s.Name, "Installing %s into %s", g.Name, s.InstallDir)
		if s.UpdateMode == "cache" {
			err = m.updateCache(ctx, i, s, g, false, 0)
			if err == nil {
				var next func()
				if next, err = waitCopyTurn(ctx, i); err == nil {
					err = m.copyFromCache(ctx, i, s, g)
					next()
				}
			}
		} else {
			err = m.runSteam(ctx, i, s, g, s.InstallDir, false, "install "+s.Name)
		}
		if err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Install failed: %v", err)
			return
		}
		m.afterInstall(i, s, g)
		m.Log.Add(events.Updated, s.ID, s.Name, "Installed %s %s in %s", g.Short, m.versionOf(i), fmtDur(time.Since(start)))
		if startAfter {
			m.setDesired(i, s, true)
			if err = m.launch(i, s, g); err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Could not start: %v", err)
			}
		}
	}()
	return nil
}

// Update brings a server's files up to date; validate checks every file. A running server is stopped and started
// again; with warn (always for automatic updates) its players are warned first. In shared-copy mode the download
// comes first, while the server still runs: players are warned, and the server stopped, only for the copy.
func (m *Manager) Update(id string, validate bool, by string, warn bool) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	ctx, ok := i.begin("update")
	if !ok {
		return errBusy
	}
	m.Cfg.RLock()
	secs := m.Cfg.Updates.WarnSeconds
	m.Cfg.RUnlock()
	go func() {
		var err error
		defer func() { i.end(err) }()
		start := time.Now()
		i.mu.Lock()
		wasRunning, desired, target := i.pid > 0, i.desired, i.updateTo
		i.mu.Unlock()
		what := "Updating"
		if validate {
			what = "Checking and repairing"
		}
		if by != "" {
			what += " (asked by " + by + ")"
		}
		m.Log.Add(events.Update, s.ID, s.Name, "%s %s", what, g.Short)

		stop := func() error {
			i.mu.Lock()
			running := i.pid > 0
			i.mu.Unlock()
			if !running {
				return nil
			}
			if warn || by == "" {
				i.setStage("")
				if err := m.warnPlayers(ctx, i, s, g, secs, "updates"); err != nil {
					return err
				}
			}
			i.setStage("Stopping it for the copy")
			return m.stopProcess(ctx, i, s, g)
		}

		if s.UpdateMode == "cache" {
			// Download while the server still runs, then stop it only for the copy
			if err = m.updateCache(ctx, i, s, g, validate, target); err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Update failed: %v", err)
				return
			}
			// Its players are warned now, while it waits for its turn to copy (one server at a time); it is stopped when
			// both the warning time is over and its turn has come
			var next func()
			if next, err = m.warnWhileQueued(ctx, i, s, warn || by == "", secs); err != nil {
				m.Log.Add(events.Info, s.ID, s.Name, "Update cancelled")
				return
			}
			i.setStage("Stopping it for the copy")
			if err = m.stopProcess(ctx, i, s, g); err != nil {
				next()
				m.Log.Add(events.Info, s.ID, s.Name, "Update cancelled")
				return
			}
			i.setStage("Backing up before the update")
			m.backupBefore(i, s, g, "before-update")
			err = m.copyFromCache(ctx, i, s, g)
			next()
		} else {
			if err = stop(); err != nil {
				m.Log.Add(events.Info, s.ID, s.Name, "Update cancelled")
				return
			}
			m.backupBefore(i, s, g, "before-update")
			err = m.runSteam(ctx, i, s, g, s.InstallDir, validate, "update "+s.Name)
		}
		if err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Update failed: %v", err)
		} else {
			m.afterInstall(i, s, g)
			i.mu.Lock()
			i.updateWanted, i.repairWanted = false, false
			stuck := target > 0 && i.versionNum > 0 && i.versionNum < target
			if stuck {
				i.updateStuck = target
			}
			i.mu.Unlock()
			if stuck {
				m.Log.Add(events.Error, s.ID, s.Name, "Updated, but its version is still %s while Steam asks for %d. Steam may not have published it yet: try Update or Verify files later.", m.versionOf(i), target)
			} else {
				m.Log.Add(events.Updated, s.ID, s.Name, "Up to date: %s %s (%s)", g.Short, m.versionOf(i), fmtDur(time.Since(start)))
			}
		}
		i.mu.Lock()
		i.repairWanted = false // tried: the crash count decides what comes next
		i.mu.Unlock()
		if wasRunning || desired {
			if lerr := m.launch(i, s, g); lerr != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Could not start: %v", lerr)
				if err == nil {
					err = lerr
				}
			}
		}
	}()
	return nil
}

func (m *Manager) versionOf(i *Instance) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.version == "" || i.version[0] < '0' || i.version[0] > '9' {
		return i.version // "build 12345678"
	}
	return "v" + i.version
}

// updateCache brings the shared copy of a game up to date (once, when several servers wait for it).
func (m *Manager) updateCache(ctx context.Context, i *Instance, s config.Server, g *games.Game, validate bool, target int64) error {
	i.setStage("Waiting for the shared copy: another server's steamcmd is updating it")
	unlock := m.cacheLock(g)
	defer unlock()
	i.setStage("")
	dir := m.CacheFor(g)
	if !validate && target > 0 {
		if _, have := installedVersion(g, dir); have >= target {
			i.say("[cache] the shared copy is already up to date")
			return nil
		}
	}
	return m.runSteam(ctx, i, s, g, dir, validate, "shared copy of "+g.Short)
}

// afterInstall: version, Metamod, a server.cfg for a new server.
func (m *Manager) afterInstall(i *Instance, s config.Server, g *games.Game) {
	m.readVersion(i, s, g)
	i.mu.Lock()
	i.sizeAt = time.Time{} // its folder is measured again
	i.mu.Unlock()
	if _, err := m.patchMetamod(s, g); err != nil {
		m.Log.Add(events.Error, s.ID, s.Name, "Metamod: %v", err)
	}
	m.writeDefaultConfig(s, g)
	i.mu.Lock()
	if i.status == StatusNotInstalled {
		i.status = StatusStopped
	}
	i.lastError = ""
	i.mu.Unlock()
}

/* ---------------------------------------------------------------------------------------------------------------
 * Copy from the shared copy
 * ------------------------------------------------------------------------------------------------------------- */

func (m *Manager) copyFromCache(ctx context.Context, i *Instance, s config.Server, g *games.Game) error {
	m.Cfg.RLock()
	protected := ExcludeMatcher(m.Cfg.CopyExclude)
	m.Cfg.RUnlock()
	// The version file always comes with the update, whatever the patterns say: without it the server would look
	// out of date for ever
	version := strings.ToLower(filepath.ToSlash(g.VersionFile))
	excl := func(rel string) bool {
		return strings.ToLower(filepath.ToSlash(rel)) != version && protected(rel)
	}
	src := m.CacheFor(g)
	i.say(fmt.Sprintf("[copy] %s → %s", src, s.InstallDir))
	i.setStage("")
	started := time.Now()
	st, err := copyTree(ctx, src, s.InstallDir, excl, false, func(done, copied, total int64) {
		p := &steam.Progress{Label: "copy", State: "Copying files from the shared copy", Done: done, Total: total, Started: started.Unix()}
		if total > 0 {
			p.Percent = float64(done) * 100 / float64(total)
		}
		if el := time.Since(started).Seconds(); el >= 2 && copied > 0 {
			p.Speed = float64(copied) / el
			if p.Speed > 0 {
				p.Left = int64(float64(total-done) / p.Speed)
			}
		}
		i.mu.Lock()
		i.copyProg = p
		i.mu.Unlock()
	})
	i.mu.Lock()
	i.copyProg = nil
	i.mu.Unlock()
	i.say(fmt.Sprintf("[copy] %d files copied (%s), %d already up to date, %d kept", st.Copied, fmtBytes(st.Bytes), st.Same, st.Kept))
	for _, e := range st.Errors {
		i.say("[copy] " + e)
	}
	if err != nil {
		return err
	}
	if st.Failed > 0 {
		return fmt.Errorf("%d files could not be copied (first: %s)", st.Failed, st.Errors[0])
	}
	return nil
}

// ExcludeMatcher tells whether a file (its path inside the server's folder, with /) is protected by one of the
// patterns: a name or a path, with wildcards * ? [abc] ("server_*.cfg", "gamemode_*.cfg", "csgo/cfg/*.cfg"). A
// pattern naming a folder protects everything inside it ("csgo/addons"). Without case.
func ExcludeMatcher(patterns []string) func(rel string) bool {
	var pats []string
	for _, p := range patterns {
		p = strings.ToLower(strings.Trim(strings.TrimSpace(filepath.ToSlash(p)), "/"))
		if p != "" {
			if _, err := path.Match(p, ""); err == nil {
				pats = append(pats, p)
			}
		}
	}
	return func(rel string) bool {
		rel = strings.ToLower(strings.Trim(filepath.ToSlash(rel), "/"))
		base := path.Base(rel)
		for _, p := range pats {
			if ok, _ := path.Match(p, base); ok {
				return true
			}
			// The path, or one of its folders
			for sub := rel; sub != "." && sub != ""; sub = path.Dir(sub) {
				if ok, _ := path.Match(p, sub); ok {
					return true
				}
				if !strings.Contains(sub, "/") {
					break
				}
			}
		}
		return false
	}
}

// CopyStats says what a copy did.
type CopyStats struct {
	Copied, Same, Kept, Failed int
	Bytes                      int64
	Errors                     []string // the first ones
}

// CopyTree copies the files of src into dst that differ (size or time). Excluded files are copied only when dst
// does not have them yet. steamcmd's own folder is left out.
func CopyTree(ctx context.Context, src, dst string, isExcluded func(rel string) bool) (CopyStats, error) {
	return copyTree(ctx, src, dst, isExcluded, false, nil)
}

// KeepTree is CopyTree for an addons push: an excluded folder dst already has is left as it is, nothing goes in
// it (a plugin removed from addons/sourcemod/plugins does not come back); one it does not have yet is copied.
func KeepTree(ctx context.Context, src, dst string, isExcluded func(rel string) bool) (CopyStats, error) {
	return copyTree(ctx, src, dst, isExcluded, true, nil)
}

// copyTree copies; progress, when given, hears how far it is: bytes done (copied, or found up to date), bytes
// copied, out of the total (a few times a second).
func copyTree(ctx context.Context, src, dst string, isExcluded func(rel string) bool, wholeDirs bool, progress func(done, copied, total int64)) (CopyStats, error) {
	var st CopyStats
	if _, err := os.Stat(src); err != nil {
		return st, fmt.Errorf("the shared copy is missing: %w", err)
	}
	var total, done, copied int64
	var told time.Time
	tell := func(force bool) {
		if progress != nil && (force || time.Since(told) >= 250*time.Millisecond) {
			told = time.Now()
			progress(done, copied, total)
		}
	}
	if progress != nil {
		filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if rel, _ := filepath.Rel(src, p); d.IsDir() && strings.EqualFold(rel, "steamapps") {
				return filepath.SkipDir
			}
			if d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					total += info.Size()
				}
			}
			return nil
		})
		tell(true)
		defer tell(true)
	}
	fail := func(msg string) {
		st.Failed++
		if len(st.Errors) < 10 {
			st.Errors = append(st.Errors, msg)
		}
	}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			fail(err.Error())
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if strings.EqualFold(first, "steamapps") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if wholeDirs && isExcluded(rel) {
				if ti, err := os.Stat(target); err == nil && ti.IsDir() {
					filepath.WalkDir(path, func(_ string, e fs.DirEntry, err error) error {
						if err == nil && e.Type().IsRegular() {
							st.Kept++
						}
						return nil
					})
					return filepath.SkipDir
				}
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				fail(err.Error())
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			fail(err.Error())
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		tinfo, terr := os.Stat(target)
		if terr == nil && isExcluded(rel) {
			st.Kept++
			done += info.Size()
			tell(false)
			return nil
		}
		if terr == nil && tinfo.Size() == info.Size() && tinfo.ModTime().Unix() == info.ModTime().Unix() {
			st.Same++
			done += info.Size()
			tell(false)
			return nil
		}
		before := done
		if err := copyFile(path, target, info, func(n int64) { done += n; copied += n; tell(false) }); err != nil {
			fail(fmt.Sprintf("%s: %v", rel, err))
			done = before + info.Size()
			return nil
		}
		st.Copied++
		st.Bytes += info.Size()
		return nil
	})
	return st, err
}

var copyBufs = sync.Pool{New: func() any { b := make([]byte, 1<<20); return &b }}

// countWriter tells how many bytes went through it.
type countWriter struct {
	w    io.Writer
	tell func(int64)
}

func (c countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	if c.tell != nil {
		c.tell(int64(n))
	}
	return n, err
}

func copyFile(src, dst string, info fs.FileInfo, tell func(int64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".szm-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	buf := copyBufs.Get().(*[]byte)
	_, err = io.CopyBuffer(countWriter{out, tell}, in, *buf)
	copyBufs.Put(buf)
	if err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// sayDur is a time for players to read: "45 seconds", "2 minutes".
func sayDur(d time.Duration) string {
	sec := int(d.Round(time.Second).Seconds())
	if sec >= 90 {
		return fmt.Sprintf("%d minutes", (sec+30)/60)
	}
	if sec >= 60 {
		return "1 minute"
	}
	return fmt.Sprintf("%d seconds", sec)
}

func fmtBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

/* ---------------------------------------------------------------------------------------------------------------
 * Game files the manager looks after
 * ------------------------------------------------------------------------------------------------------------- */

// patchMetamod puts Metamod back into CS2's gameinfo.gi (each update of CS2 removes it). It is done when the
// server has addons/metamod, or when the server says so.
func (m *Manager) patchMetamod(s config.Server, g *games.Game) (bool, error) {
	if !g.Metamod || m.vanillaFor(s) != "" {
		return false, nil
	}
	gameDir := g.Path(s.InstallDir, g.GameDir)
	want := false
	if s.Metamod != nil {
		want = *s.Metamod
	} else if st, err := os.Stat(filepath.Join(gameDir, "addons", "metamod")); err == nil && st.IsDir() {
		want = true
	}
	if !want {
		return false, nil
	}
	return PatchGameinfo(filepath.Join(gameDir, "gameinfo.gi"), g.Game+"/addons/metamod")
}

// PatchGameinfo adds a search path ("csgo/addons/metamod") after the Game_LowViolence line of a gameinfo.gi.
func PatchGameinfo(path, entry string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if bytes.Contains(bytes.ToLower(b), []byte(strings.ToLower(entry))) {
		return false, nil
	}
	nl := "\n"
	if bytes.Contains(b, []byte("\r\n")) {
		nl = "\r\n"
	}
	lines := strings.Split(string(b), nl)
	insert := func(k int, before bool) (bool, error) {
		at := k + 1
		if before {
			at = k
		}
		out := append([]string{}, lines[:at]...)
		out = append(out, "\t\t\tGame\t"+entry)
		out = append(out, lines[at:]...)
		return true, config.WriteFileAtomic(path, []byte(strings.Join(out, nl)), 0o644)
	}
	// Right after Game_LowViolence, as Metamod says; else before the game's own "Game <mod>" line
	for k, l := range lines {
		if strings.Contains(l, "Game_LowViolence") {
			return insert(k, false)
		}
	}
	mod := strings.SplitN(entry, "/", 2)[0]
	for k, l := range lines {
		f := strings.Fields(strings.SplitN(l, "//", 2)[0])
		if len(f) == 2 && strings.EqualFold(f[0], "Game") && strings.EqualFold(strings.Trim(f[1], `"`), mod) {
			return insert(k, true)
		}
	}
	return false, errors.New("gameinfo.gi has neither a Game_LowViolence nor a \"Game " + mod + "\" line: Metamod not added")
}

var rconCfgRe = regexp.MustCompile(`(?im)^\s*rcon_password\s+"?([^"\r\n]*)"?`)

// rconPassword is the server's RCON password: its own, else the one of its server.cfg.
func (m *Manager) rconPassword(s config.Server, g *games.Game) string {
	if s.RconPassword != "" || g.IsOther() {
		return s.RconPassword // the games of other engines get it on their command line
	}
	b, err := os.ReadFile(g.Path(s.InstallDir, g.Config))
	if err != nil {
		return ""
	}
	if mm := rconCfgRe.FindSubmatch(b); mm != nil {
		return strings.TrimSpace(string(mm[1]))
	}
	return ""
}

// writeDefaultConfig gives a new server a server.cfg with its name and RCON password.
func (m *Manager) writeDefaultConfig(s config.Server, g *games.Game) {
	if g.IsOther() {
		return // its config is its own (an .ini, .xml, .json): its command line gives it its name and passwords
	}
	path := g.Path(s.InstallDir, g.Config)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		var cfg strings.Builder
		cfg.WriteString("// server.cfg written by SnipeZilla Server Manager\n")
		fmt.Fprintf(&cfg, "hostname \"%s\"\n", strings.ReplaceAll(s.Name, `"`, "'"))
		if s.RconPassword != "" {
			fmt.Fprintf(&cfg, "rcon_password \"%s\"\n", s.RconPassword)
		}
		cfg.WriteString("sv_lan 0\n")
		if g.Engine != games.Source2 {
			cfg.WriteString("log on\n")
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte(cfg.String()), 0o644)
		return
	}
	// A server.cfg that came with the game, without a password: add the server's
	if err == nil && s.RconPassword != "" && !rconCfgRe.Match(b) {
		b = append(bytes.TrimRight(b, "\r\n"), []byte(fmt.Sprintf("\nrcon_password \"%s\"\n", s.RconPassword))...)
		_ = os.WriteFile(path, b, 0o644)
	}
}

/* ---------------------------------------------------------------------------------------------------------------
 * RCON and the console
 * ------------------------------------------------------------------------------------------------------------- */

// Rcon sends a command to a server; the answer is also added to its console.
func (m *Manager) Rcon(id, command string) (string, error) { return m.RconBy(id, command, "") }

// RconBy is Rcon for a user (by: their name, kept for a crash report; "" for the manager's own).
func (m *Manager) RconBy(id, command, by string) (string, error) {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return "", err
	}
	if terr := m.rconTarget(i, s, g); terr != nil {
		i.say("] "+command, "[rcon] "+terr.Error())
		return "", terr
	}
	i.mu.Lock()
	before := i.consoleN // the console log lines read while RCON answers: not shown twice
	i.mu.Unlock()
	stop := stopsIt(g, command)
	if stop {
		// its stop command: its end is a stop by this user, not a crash (noted first: it may end before RCON
		// answers)
		who := by
		if who == "" {
			who = "the console"
		}
		i.mu.Lock()
		i.stopTyped, i.stopTypedCmd, i.stopTypedUntil = who, strings.TrimSpace(command), time.Now().Add(g.StopTimeout()+time.Minute)
		i.mu.Unlock()
	}
	out, err := m.sendRcon(s, g, command, 5*time.Second)
	i.noteCommand(by, command, false, err)
	i.say("] " + command)
	if err != nil {
		if stop {
			i.mu.Lock()
			i.stopTyped, i.stopTypedCmd, i.stopTypedUntil = "", "", time.Time{}
			i.mu.Unlock()
		}
		i.say("[rcon] " + err.Error())
		return "", err
	}
	if stop {
		i.say("[stop] " + strings.TrimSpace(command) + ": it saves and ends, and is not started again (Start starts it)")
	}
	// The answer is shown as RCON gives it: Source and GoldSrc servers never write it in their console (nor in
	// console.log), and CS2 not always. Should the console log have it too, it is shown once.
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, config.Redact(l))
		}
	}
	if s.ConsoleLog {
		seen, _ := m.ConsoleSince(id, before)
		lines = notIn(lines, seen)
		i.hide(lines, 15*time.Second) // still to come from the log
	}
	switch {
	case len(lines) > 0:
		i.say(lines...)
	case strings.TrimSpace(out) == "":
		i.say("[rcon] (no answer)")
	}
	return out, nil
}

// stopsIt tells whether a console command is the one that ends a server of its game (the last of its stop
// commands: #shutdown, quit, DoExit), to be taken for a stop.
func stopsIt(g *games.Game, command string) bool {
	if len(g.Stop) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(command), strings.TrimSpace(g.Stop[len(g.Stop)-1]))
}

// rconTarget checks that the TCP port RCON talks to is this server's own (Source, CS2: GoldSrc speaks RCON over
// UDP). Another server listening on that port of every address (0.0.0.0), or on this IP, would get the commands
// (a "quit" would stop it), and this server, which could not open its RCON port, none.
func (m *Manager) rconTarget(i *Instance, s config.Server, g *games.Game) error {
	if g.Engine == games.GoldSrc || g.IsOther() {
		return nil
	}
	i.mu.Lock()
	pid := i.pid
	i.mu.Unlock()
	if pid <= 0 {
		return nil
	}
	e, ok := proc.TCPListener(s.IP, s.Port)
	if !ok || e.PID == pid {
		return nil // not known (not Windows), or this server's
	}
	who := fmt.Sprintf("PID %d", e.PID)
	if name := proc.ProcessName(e.PID); name != "" {
		who = fmt.Sprintf("%s (PID %d)", name, e.PID)
	}
	where := e.IP.String()
	if e.IP.IsUnspecified() {
		where = "every IP (0.0.0.0)"
	}
	return fmt.Errorf("not sent: TCP port %d, where RCON goes, is held by %s on %s, not by this server (PID %d): "+
		"that program would get the commands. This server could not open its RCON port (\"bind: WSAEACCES\" in its "+
		"console). Give this server, or that one, another port",
		s.Port, who, where, pid)
}

func hasArg(args []string, a string) bool {
	for _, x := range args {
		if strings.EqualFold(x, a) {
			return true
		}
	}
	return false
}

// notIn drops from lines those already in seen (the console log showed them), each once.
func notIn(lines, seen []string) []string {
	if len(seen) == 0 {
		return lines
	}
	left := map[string]int{}
	for _, l := range seen {
		left[strings.TrimSpace(l)]++
	}
	out := lines[:0:0]
	for _, l := range lines {
		t := strings.TrimSpace(l)
		matched := false
		for k, n := range left {
			if n > 0 && (k == t || (len(t) >= 6 && strings.HasSuffix(k, t))) {
				left[k]--
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, l)
		}
	}
	return out
}

func consolePath(s config.Server, g *games.Game) string {
	p := g.Path(s.InstallDir, g.ConsoleLog)
	if fileExists(p) {
		return p
	}
	// Not where the game should write it: next to the program, or at the top of the server's folder
	var alts []string
	if g.Engine == games.GoldSrc {
		// HLDS writes qconsole.log next to hlds.exe, or in the mod's folder for some builds
		alts = append(alts, "qconsole.log", g.GameDir+"/qconsole.log")
	}
	base := path.Base(filepath.ToSlash(g.ConsoleLog))
	for _, exe := range g.ExeCandidates() {
		alts = append(alts, path.Join(path.Dir(filepath.ToSlash(exe)), base))
	}
	alts = append(alts, base, "game/"+base)
	for _, alt := range alts {
		if q := g.Path(s.InstallDir, alt); fileExists(q) {
			return q
		}
	}
	return p
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// openConsole starts reading the console log from its end. At a fresh start a log over 20 MB is set aside.
func (m *Manager) openConsole(i *Instance, s config.Server, g *games.Game, adopt bool) {
	if !s.ConsoleLog || (g.IsOther() && g.ConsoleLog == "") {
		return
	}
	p := consolePath(s, g)
	var off int64
	if st, err := os.Stat(p); err == nil {
		off = st.Size()
		if !adopt && st.Size() > 20<<20 {
			old := strings.TrimSuffix(p, ".log") + ".old.log"
			os.Remove(old)
			if os.Rename(p, old) == nil {
				off = 0
			}
		}
		if adopt {
			off = max(0, off-8<<10) // show the last lines
		}
	}
	i.mu.Lock()
	i.consoleOff = off
	if adopt && off > 0 {
		i.consoleOff = -off // negative: skip to the next line first
	}
	i.mu.Unlock()
}

func (m *Manager) readConsole(i *Instance, s config.Server, g *games.Game) {
	if !s.ConsoleLog || (g.IsOther() && g.ConsoleLog == "") {
		return
	}
	m.captureWindow(i, s, g) // CS2 on Windows: its window to its log
	path := consolePath(s, g)
	st, err := os.Stat(path) // most seconds nothing new: no need to open it
	if err != nil {
		// Running for a minute, -condebug on, and still no log: say so once, rather than a console that stays empty
		i.mu.Lock()
		since, pid := i.since, i.pid
		tell := pid > 0 && !since.IsZero() && time.Since(since) > time.Minute && !i.consoleMissingSaid.Equal(since)
		if tell {
			i.consoleMissingSaid = since
		}
		i.mu.Unlock()
		if tell {
			rel, _ := filepath.Rel(s.InstallDir, path)
			if g.IsOther() {
				i.say(fmt.Sprintf("[szm] After a minute the server has written no console log (%s): only the manager's messages show here.", filepath.ToSlash(rel)))
			} else {
				i.say(fmt.Sprintf("[szm] The server runs with -condebug, but after a minute it has written no console log (%s). "+
					"Only the RCON answers and the manager's messages show here.", filepath.ToSlash(rel)))
			}
		}
		return
	}
	i.mu.Lock()
	off := i.consoleOff
	i.mu.Unlock()
	skipFirst := off < 0
	if skipFirst {
		off = -off
	}
	if st.Size() < off {
		off = 0 // the server started a new log
		skipFirst = false
	}
	if st.Size() == off {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	n := min(st.Size()-off, 256<<10)
	buf := make([]byte, n)
	k, err := f.ReadAt(buf, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return
	}
	buf, n = buf[:k], int64(k)
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		if n < 256<<10 {
			return // half a line: wait for the rest
		}
		end = len(buf) - 1
	}
	chunk := buf[:end+1]
	var lines []string
	first := true
	for len(chunk) > 0 {
		raw := chunk
		if k := bytes.IndexByte(chunk, '\n'); k >= 0 {
			raw, chunk = chunk[:k], chunk[k+1:]
		} else {
			chunk = nil
		}
		if first && skipFirst {
			first = false
			continue
		}
		first = false
		l := strings.TrimRight(string(raw), "\r")
		if l == "" {
			continue
		}
		lines = append(lines, config.Redact(strings.ToValidUTF8(l, "?")))
	}
	i.mu.Lock()
	i.consoleOff = off + int64(end+1)
	lines = i.unhidden(lines)
	i.mu.Unlock()
	if len(lines) > 0 {
		i.say(lines...)
	}
}
