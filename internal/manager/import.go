package manager

import (
	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
)

// ImportOld brings the servers and settings of the old manager's config.xml. Servers it runs are taken over (stop
// the old manager first); those it had stopped stay stopped.
func (m *Manager) ImportOld(path, by string) (*config.ImportReport, error) {
	// Work on a copy, then swap: the manager reads the configuration all the time
	m.Cfg.RLock()
	tmp := &config.Config{Settings: m.Cfg.Settings}
	tmp.Servers = append(tmp.Servers, m.Cfg.Servers...)
	before := len(tmp.Servers)
	m.Cfg.RUnlock()

	rep, err := tmp.ImportOld(path, m.Catalog())
	if err != nil {
		return nil, err
	}
	added := tmp.Servers[before:]
	m.Cfg.Lock()
	m.Cfg.Settings = tmp.Settings
	m.Cfg.Servers = append(m.Cfg.Servers, added...)
	m.Cfg.Unlock()
	m.Cfg.SortServers()
	if err := m.Cfg.Save(); err != nil {
		return rep, err
	}
	m.Steam.Path = m.SteamCmdPath()

	stopped := map[string]bool{}
	for _, id := range rep.Stopped {
		stopped[id] = true
	}
	for _, s := range added {
		d := "running"
		if stopped[s.ID] {
			d = "stopped"
		}
		m.state.update(s.ID, func(v *savedServer) { v.Desired = d })
		m.adopt(*s)
	}
	m.Log.Add(events.Manager, "", "", "%d servers imported from %s by %s", len(rep.Added), path, by)
	return rep, nil
}

// MarkStopped keeps servers stopped at the next start of the manager.
func (m *Manager) MarkStopped(ids []string) {
	for _, id := range ids {
		m.state.update(id, func(v *savedServer) { v.Desired = "stopped" })
	}
}
