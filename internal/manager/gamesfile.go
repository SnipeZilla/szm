package manager

import (
	"fmt"
	"os"
	"sync"

	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
)

// games.json next to szm.exe (the games added or changed) is read again as soon as it changes, by hand or by the
// Games page: no restart. A file that cannot be read keeps the games as they were, and says where it is wrong.

type gamesFile struct {
	mu    sync.Mutex
	stamp string // its time and size when last read
	err   string // why it cannot be read ("" when it can)
}

func gamesStamp(dataDir string) string {
	st, err := os.Stat(games.CustomPath(dataDir))
	if err != nil {
		return "none"
	}
	return fmt.Sprint(st.ModTime().UnixNano(), "/", st.Size())
}

// SetGamesError says games.json could not be read at the start (the built-in games are used).
func (m *Manager) SetGamesError(err error) {
	m.gf.mu.Lock()
	m.gf.stamp = gamesStamp(m.DataDir)
	m.gf.err = ""
	if err != nil {
		m.gf.err = err.Error()
	}
	m.gf.mu.Unlock()
	if err != nil {
		m.Log.Add(events.Error, "", "", "%v. The built-in games are used until it is fixed.", err)
	}
}

// GamesSaved notes that the manager itself wrote games.json (the Games page): not read again for that.
func (m *Manager) GamesSaved() {
	m.gf.mu.Lock()
	m.gf.stamp, m.gf.err = gamesStamp(m.DataDir), ""
	m.gf.mu.Unlock()
}

// GamesError is why games.json cannot be read ("" when it can).
func (m *Manager) GamesError() string {
	m.gf.mu.Lock()
	defer m.gf.mu.Unlock()
	return m.gf.err
}

// watchGames reads games.json again when it changed.
func (m *Manager) watchGames() {
	cur := gamesStamp(m.DataDir)
	m.gf.mu.Lock()
	if cur == m.gf.stamp {
		m.gf.mu.Unlock()
		return
	}
	m.gf.stamp = cur
	hadErr := m.gf.err != ""
	m.gf.mu.Unlock()

	cat, err := games.Load(m.DataDir)
	if err != nil {
		m.gf.mu.Lock()
		m.gf.err = err.Error()
		m.gf.mu.Unlock()
		m.Log.Add(events.Error, "", "", "%v. The games stay as they were until it is fixed.", err)
		return
	}
	m.gf.mu.Lock()
	m.gf.err = ""
	m.gf.mu.Unlock()
	before := len(m.Catalog())
	m.SetCatalog(cat)
	switch {
	case hadErr:
		m.Log.Add(events.Manager, "", "", "games.json is right again: %d games", len(cat))
	default:
		m.Log.Add(events.Manager, "", "", "games.json changed: read again, %d games (%+d)", len(cat), len(cat)-before)
	}
}
