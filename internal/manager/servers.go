package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/proc"
)

// Prepare fills what a new server leaves out: ID, map, players, password, folder.
func (m *Manager) Prepare(s *config.Server) {
	g := m.Catalog()[s.Game]
	if g == nil {
		return
	}
	if s.ID == "" {
		s.ID = m.Cfg.NewID(s.Game)
	}
	if s.Map == "" {
		s.Map = g.DefaultMap
	}
	if s.MaxPlayers == 0 {
		s.MaxPlayers = g.DefaultMax
	}
	if s.RconPassword == "" {
		s.RconPassword = config.RandomPassword()
	}
	if s.SayCommand == "" {
		s.SayCommand = "say"
	}
	if s.SteamLogin == "" {
		s.SteamLogin = "anonymous"
	}
	if s.UpdateMode != "cache" {
		s.UpdateMode = "direct"
	}
	if strings.TrimSpace(s.InstallDir) == "" {
		s.InstallDir = filepath.Join(m.NewServersRoot(), s.ID)
	}
	s.InstallDir = strings.TrimRight(filepath.Clean(s.InstallDir), `\/`)
	if s.InstallDir == "" || filepath.VolumeName(s.InstallDir) == s.InstallDir {
		s.InstallDir += string(filepath.Separator) // a drive's root stays E:\ (or /)
	}
	s.Exe = ExeSetting(s.InstallDir, s.Exe)
	s.Name = strings.TrimSpace(s.Name)
}

// NewServersRoot is the folder new servers go in: the setting "Folder for new servers", unless it is empty or
// inside a server's folder; then the folder that holds most servers (E:\ when they are E:\Counter-Strike 2 -
// Server 1...), else <data>\servers.
func (m *Manager) NewServersRoot() string {
	m.Cfg.RLock()
	root := m.Cfg.ServersRoot
	count := map[string]int{}
	best := ""
	for _, o := range m.Cfg.Servers {
		p := filepath.Dir(filepath.Clean(o.InstallDir))
		k := strings.ToLower(p)
		count[k]++
		if best == "" || count[k] > count[strings.ToLower(best)] {
			best = p
		}
	}
	m.Cfg.RUnlock()
	if root != "" {
		if _, inside := m.Cfg.ServerAt(root); !inside {
			return root
		}
	}
	if best != "" {
		if _, inside := m.Cfg.ServerAt(best); !inside {
			return best
		}
	}
	return filepath.Join(m.DataDir, "servers")
}

// LooksInstalled tells whether a folder holds a game server (steamcmd's files, a server program): no new
// server's folder goes inside it.
func LooksInstalled(dir string) bool {
	for _, n := range []string{"steamapps", "srcds.exe", "srcds_win64.exe", "hlds.exe", "srcds_run", "hlds_run", "game/bin", "steam.inf"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(n))); err == nil {
			return true
		}
	}
	return false
}

// AddServer adds a server to the configuration. The errors to show, none when it was added.
func (m *Manager) AddServer(s *config.Server, by string) []string {
	m.Prepare(s)
	if errs := m.Cfg.Validate(s, m.Catalog()); len(errs) > 0 {
		return errs
	}
	m.Cfg.Lock()
	m.Cfg.Servers = append(m.Cfg.Servers, s)
	m.Cfg.Unlock()
	m.Cfg.SortServers()
	if err := m.Cfg.Save(); err != nil {
		return []string{err.Error()}
	}
	i := m.instance(s.ID)
	m.state.update(s.ID, func(v *savedServer) { v.Desired = "stopped" })
	g := m.Catalog()[s.Game]
	if g == nil {
		m.Log.Add(events.Info, s.ID, s.Name, "Server added by %s", by)
		return nil
	}
	exe, _ := m.exePath(*s, g)
	if exe == "" {
		i.mu.Lock()
		i.status = StatusNotInstalled
		i.mu.Unlock()
		m.readVersion(i, *s, g)
		m.Log.Add(events.Info, s.ID, s.Name, "Server added by %s", by)
		return nil
	}
	// Already installed in its folder: added as it is, and taken over if it already runs
	m.Log.Add(events.Info, s.ID, s.Name, "Server added by %s from its folder %s (%s found: nothing to download)", by, s.InstallDir, filepath.Base(exe))
	i.mu.Lock()
	if i.status == StatusNotInstalled {
		i.status = StatusStopped
	}
	i.mu.Unlock()
	m.adopt(*s)
	return nil
}

// AdoptRcon gives a server already installed the RCON password of its server.cfg (the one the server uses): the
// manager needs that one to talk to it.
func (m *Manager) AdoptRcon(s *config.Server) bool {
	g := m.Catalog()[s.Game]
	if g == nil {
		return false
	}
	if exe, _ := m.exePath(*s, g); exe == "" || g.IsOther() {
		return false // the games of other engines get their RCON password on their command line
	}
	if p := existingRcon(*s, g); p != "" && p != s.RconPassword {
		s.RconPassword = p
		return true
	}
	return false
}

// CheckExe tells what is wrong with the program a server is set to start, "" when nothing is.
func (m *Manager) CheckExe(s config.Server) string {
	if strings.TrimSpace(s.Exe) == "" {
		return ""
	}
	if _, err := os.Stat(s.InstallDir); err != nil {
		return "" // not installed yet
	}
	p := s.Exe
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.InstallDir, p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Sprintf("The program %s is not there: pick it again, or empty the field to let the manager find it.", p)
	}
	if st.IsDir() {
		return fmt.Sprintf("%s is a folder: pick the program inside it.", p)
	}
	return ""
}

// SaveServer changes a server. The game and the ID stay; a running server uses the change at its next start.
func (m *Manager) SaveServer(s *config.Server, by string) []string {
	old, ok := m.server(s.ID)
	if !ok {
		return []string{errUnknown.Error()}
	}
	s.Game = old.Game
	// The Workshop maps are set on the Maps tab: a settings page opened before keeps them as they are
	s.Workshop, s.WorkshopStart, s.WorkshopColl, s.WorkshopCollFor = old.Workshop, old.WorkshopStart, old.WorkshopColl, old.WorkshopCollFor
	m.Prepare(s)
	errs := m.Cfg.Validate(s, m.Catalog())
	if e := m.CheckExe(*s); e != "" {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		return errs
	}
	m.Cfg.Lock()
	for k, o := range m.Cfg.Servers {
		if o.ID == s.ID {
			m.Cfg.Servers[k] = s
		}
	}
	m.Cfg.Unlock()
	m.Cfg.SortServers()
	if err := m.Cfg.Save(); err != nil {
		return []string{err.Error()}
	}
	i := m.instance(s.ID)
	if g := m.Catalog()[s.Game]; g != nil {
		exe, _ := m.exePath(*s, g)
		i.mu.Lock()
		if exe == "" && i.pid == 0 {
			i.status = StatusNotInstalled
		} else if exe != "" && i.status == StatusNotInstalled {
			i.status = StatusStopped
		}
		i.mu.Unlock()
		m.readVersion(i, *s, g)
	}
	m.Log.Add(events.Info, s.ID, s.Name, "Settings changed by %s", by)
	return nil
}

// DeleteServer removes a stopped server from the manager; its files too when asked.
func (m *Manager) DeleteServer(id string, files bool, by string) error {
	s, ok := m.server(id)
	if !ok {
		return errUnknown
	}
	i := m.instance(id)
	i.mu.Lock()
	busy, pid := i.busy, i.pid
	i.mu.Unlock()
	if busy != "" || pid > 0 {
		return errors.New("stop the server first")
	}
	if files {
		if err := m.safeToDelete(s); err != nil {
			return err
		}
		// Not one of its programs still running from that folder (started by hand, by the old manager...)
		if g := m.Catalog()[s.Game]; g != nil {
			if exe, _ := m.exePath(s, g); exe != "" {
				if pid := proc.FindServer(exeNames(s, g, exe), s.InstallDir, s.IP, s.Port); pid > 0 {
					return fmt.Errorf("its program is still running (PID %d): stop it first", pid)
				}
			}
		}
	}
	m.Cfg.Lock()
	out := m.Cfg.Servers[:0]
	for _, o := range m.Cfg.Servers {
		if o.ID != id {
			out = append(out, o)
		}
	}
	m.Cfg.Servers = out
	m.Cfg.Unlock()
	if err := m.Cfg.Save(); err != nil {
		return err
	}
	m.state.remove(id)
	m.mu.Lock()
	delete(m.inst, id)
	m.mu.Unlock()
	if files {
		// A game folder can be tens of GB: deleted in the background, the log tells when it is done
		m.Log.Add(events.Info, id, s.Name, "Removed by %s: deleting its folder %s", by, s.InstallDir)
		go func() {
			start := time.Now()
			if err := os.RemoveAll(s.InstallDir); err != nil {
				m.Log.Add(events.Error, id, s.Name, "Its folder %s could not be deleted completely: %v", s.InstallDir, err)
				return
			}
			m.Log.Add(events.Info, id, s.Name, "Folder %s deleted (%s)", s.InstallDir, fmtDur(time.Since(start).Round(time.Second)))
		}()
		return nil
	}
	m.Log.Add(events.Info, id, s.Name, "Removed by %s (its files stay in %s)", by, s.InstallDir)
	return nil
}

// safeToDelete refuses to delete a folder that is a drive, the manager's, another server's, or a parent of one.
func (m *Manager) safeToDelete(s config.Server) error {
	dir := filepath.Clean(s.InstallDir)
	if dir == filepath.Dir(dir) || len(strings.Split(filepath.ToSlash(dir), "/")) < 3 && filepath.VolumeName(dir) == "" {
		return fmt.Errorf("%s is too high up to be deleted", dir)
	}
	inside := func(child, parent string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
	}
	if inside(m.DataDir, dir) {
		return errors.New("this folder holds the manager itself")
	}
	m.Cfg.RLock()
	root := m.Cfg.ServersRoot
	others := []string{}
	for _, o := range m.Cfg.Servers {
		if o.ID != s.ID {
			others = append(others, o.InstallDir)
		}
	}
	m.Cfg.RUnlock()
	if root != "" && inside(root, dir) {
		return errors.New("this folder holds all the servers")
	}
	if inside(m.CacheDir(), dir) {
		return errors.New("this folder holds the shared copies of the games")
	}
	for _, o := range others {
		if inside(o, dir) {
			return fmt.Errorf("this folder holds another server (%s)", o)
		}
	}
	return nil
}
