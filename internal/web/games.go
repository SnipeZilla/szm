package web

import (
	"net/http"
	"strings"

	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
)

// The Games page: the games the manager knows, and the ones the admin adds (<data>/games.json).

type gameEntry struct {
	*games.Game
	Builtin bool `json:"builtin"` // comes with the manager
	Custom  bool `json:"custom"`  // in games.json (added, or replacing the built-in one)
	Servers int  `json:"servers"` // how many servers use it
}

func (s *Server) gamesList(w http.ResponseWriter, r *http.Request, u *User) error {
	builtin := games.Builtin()
	custom, err := games.LoadCustom(s.M.DataDir)
	if err != nil {
		custom = map[string]*games.Game{} // shown: file_error
	}
	used := map[string]int{}
	s.M.Cfg.RLock()
	for _, o := range s.M.Cfg.Servers {
		used[o.Game]++
	}
	s.M.Cfg.RUnlock()
	var list []gameEntry
	for _, g := range s.M.Catalog().Sorted() {
		list = append(list, gameEntry{g, builtin[g.Key] != nil, custom[g.Key] != nil, used[g.Key]})
	}
	return writeJSON(w, map[string]any{"games": list, "icons": s.iconNames(), "file": games.CustomPath(s.M.DataDir), "file_error": s.M.GamesError()})
}

// saveGame adds a game, or changes one (a built-in game changed is kept in games.json, replacing it).
func (s *Server) saveGame(w http.ResponseWriter, r *http.Request, u *User) error {
	var g games.Game
	if err := readJSON(r, &g); err != nil {
		return err
	}
	g.Key = strings.ToLower(strings.TrimSpace(r.PathValue("key")))
	if err := g.Check(); err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	custom, err := games.LoadCustom(s.M.DataDir)
	if err != nil {
		return fail(http.StatusInternalServerError, "%s", capitalize(err.Error()))
	}
	_, existed := s.M.Catalog()[g.Key]
	custom[g.Key] = &g
	if err := s.reloadGames(custom); err != nil {
		return err
	}
	verb := "added"
	if existed {
		verb = "changed"
	}
	s.M.Log.Add(events.Manager, "", "", "Game %s (%s) %s by %s", g.Name, g.Key, verb, u.Name)
	return writeJSON(w, map[string]any{"ok": true, "key": g.Key})
}

// deleteGame removes a game added in games.json (a built-in game it replaced comes back).
func (s *Server) deleteGame(w http.ResponseWriter, r *http.Request, u *User) error {
	key := r.PathValue("key")
	custom, err := games.LoadCustom(s.M.DataDir)
	if err != nil {
		return fail(http.StatusInternalServerError, "%s", capitalize(err.Error()))
	}
	g := custom[key]
	if g == nil {
		return fail(http.StatusBadRequest, "This game comes with the manager: it cannot be removed.")
	}
	if games.Builtin()[key] == nil {
		s.M.Cfg.RLock()
		var users []string
		for _, o := range s.M.Cfg.Servers {
			if o.Game == key {
				users = append(users, o.Name)
			}
		}
		s.M.Cfg.RUnlock()
		if len(users) > 0 {
			return fail(http.StatusConflict, "Servers use it: %s. Remove them first.", strings.Join(users, ", "))
		}
	}
	delete(custom, key)
	if err := s.reloadGames(custom); err != nil {
		return err
	}
	s.M.Log.Add(events.Manager, "", "", "Game %s (%s) removed by %s", g.Name, key, u.Name)
	return writeJSON(w, map[string]any{"ok": true})
}

// reloadGames writes games.json and gives the manager the new catalog.
func (s *Server) reloadGames(custom map[string]*games.Game) error {
	if err := games.SaveCustom(s.M.DataDir, custom); err != nil {
		return fail(http.StatusInternalServerError, "%s", capitalize(err.Error()))
	}
	cat, err := games.Load(s.M.DataDir)
	if err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	s.M.SetCatalog(cat)
	s.M.GamesSaved()
	return nil
}
