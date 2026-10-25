package web

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// The shared copies of the games (servers that update from a copy): which folder is which game, and its files
// (the file manager under /api/cache/).

type cacheCopy struct {
	Folder  string   `json:"folder"`
	Games   []string `json:"games"`
	Keys    []string `json:"keys"`
	Servers []string `json:"servers"` // the servers that update from it
	Exists  bool     `json:"exists"`
}

func (s *Server) cacheInfo(w http.ResponseWriter, r *http.Request, u *User) error {
	dir := s.M.CacheDir()
	byFolder := map[string]*cacheCopy{}
	cat := s.M.Catalog()
	for _, g := range cat.Sorted() {
		f := filepath.Base(s.M.CacheFor(g))
		c := byFolder[f]
		if c == nil {
			c = &cacheCopy{Folder: f, Games: []string{}, Keys: []string{}, Servers: []string{}}
			byFolder[f] = c
		}
		c.Games = append(c.Games, g.Name)
		c.Keys = append(c.Keys, g.Key)
	}
	s.M.Cfg.RLock()
	for _, sv := range s.M.Cfg.Servers {
		if sv.UpdateMode != "cache" {
			continue
		}
		if g := cat[sv.Game]; g != nil {
			if c := byFolder[filepath.Base(s.M.CacheFor(g))]; c != nil {
				c.Servers = append(c.Servers, sv.Name)
			}
		}
	}
	s.M.Cfg.RUnlock()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c := byFolder[e.Name()]
		if c == nil {
			c = &cacheCopy{Folder: e.Name(), Games: []string{}, Keys: []string{}, Servers: []string{}}
			byFolder[e.Name()] = c
		}
		c.Exists = true
	}
	out := []cacheCopy{}
	for _, c := range byFolder {
		if c.Exists || len(c.Servers) > 0 {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Exists != out[b].Exists {
			return out[a].Exists
		}
		return out[a].Folder < out[b].Folder
	})
	return writeJSON(w, map[string]any{"dir": dir, "copies": out})
}
