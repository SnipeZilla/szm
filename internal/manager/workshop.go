package manager

import (
	"errors"
	"fmt"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/workshop"
)

// CS2 Workshop maps of a server: added by their ID or address (one map, or every map of a collection), played now
// (host_workshop_map, by RCON), started on (+host_workshop_map), and a collection the server plays
// (+host_workshop_collection). CS2 downloads them itself: nothing to install.

// WorkshopAdded is what adding gave: the maps added, and the collection when it was one.
type WorkshopAdded struct {
	Added      []config.WorkshopMap `json:"added"`
	Already    int                  `json:"already"`              // maps it had already
	Collection *workshop.Item       `json:"collection,omitempty"` // the input was a collection
}

const maxWorkshop = 300

// editServer changes a server's configuration (the lock held during fn) and saves it.
func (m *Manager) editServer(id string, fn func(s *config.Server) error) (config.Server, error) {
	m.Cfg.Lock()
	var srv *config.Server
	for _, s := range m.Cfg.Servers {
		if s.ID == id {
			srv = s
		}
	}
	if srv == nil {
		m.Cfg.Unlock()
		return config.Server{}, errUnknown
	}
	if err := fn(srv); err != nil {
		m.Cfg.Unlock()
		return config.Server{}, err
	}
	cp := *srv
	m.Cfg.Unlock()
	return cp, m.Cfg.Save()
}

func (m *Manager) cs2(id string) (config.Server, *games.Game, error) {
	s, g, _, err := m.lookup(id)
	if err != nil {
		return s, nil, err
	}
	if g.Engine != games.Source2 {
		return s, nil, errors.New("Workshop maps are for Counter-Strike 2 servers")
	}
	return s, g, nil
}

// AddWorkshop adds a Workshop map, or the maps of a collection, to a server's list.
func (m *Manager) AddWorkshop(id, input, by string) (WorkshopAdded, error) {
	res := WorkshopAdded{Added: []config.WorkshopMap{}}
	if _, _, err := m.cs2(id); err != nil {
		return res, err
	}
	wid := workshop.ParseID(input)
	if wid == "" {
		return res, errors.New("paste a Workshop map or collection: its ID (3070284539) or its address (https://steamcommunity.com/sharedfiles/filedetails/?id=3070284539)")
	}
	ids := []string{wid}
	children, isColl, err := workshop.Collection(wid)
	if err != nil {
		return res, err
	}
	if isColl {
		if cd, err := workshop.Details([]string{wid}); err == nil && len(cd) == 1 {
			res.Collection = &cd[0]
		} else {
			res.Collection = &workshop.Item{ID: wid, Title: wid}
		}
		ids = children
		if len(ids) > maxWorkshop {
			ids = ids[:maxWorkshop]
		}
	}
	var items []workshop.Item
	for k := 0; k < len(ids); k += 100 {
		part, err := workshop.Details(ids[k:min(k+100, len(ids))])
		if err != nil {
			return res, err
		}
		items = append(items, part...)
	}
	if len(items) == 0 {
		return res, workshop.ErrNotFound
	}
	var maps []config.WorkshopMap
	for _, it := range items {
		if it.AppID != 0 && it.AppID != 730 {
			if !isColl {
				return res, fmt.Errorf("%s is not a Counter-Strike 2 item", it.Title)
			}
			continue
		}
		maps = append(maps, config.WorkshopMap{ID: it.ID, Title: it.Title, Preview: it.Preview, Size: it.Size, Updated: it.Updated})
	}
	s, err := m.editServer(id, func(s *config.Server) error {
		have := map[string]int{}
		for k, w := range s.Workshop {
			have[w.ID] = k
		}
		for _, w := range maps {
			if k, ok := have[w.ID]; ok {
				s.Workshop[k] = w // its newer name and picture
				res.Already++
				continue
			}
			if len(s.Workshop) >= maxWorkshop {
				break
			}
			s.Workshop = append(s.Workshop, w)
			have[w.ID] = len(s.Workshop) - 1
			res.Added = append(res.Added, w)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	switch {
	case res.Collection != nil:
		m.Log.Add(events.Info, id, s.Name, "Workshop collection %s added by %s: %d maps", res.Collection.Title, by, len(res.Added))
	case len(res.Added) == 1:
		m.Log.Add(events.Info, id, s.Name, "Workshop map %s added by %s", res.Added[0].Title, by)
	}
	return res, nil
}

// RemoveWorkshop takes a map off a server's list (and off its start map).
func (m *Manager) RemoveWorkshop(id, wid, by string) error {
	_, err := m.editServer(id, func(s *config.Server) error {
		out := s.Workshop[:0]
		for _, w := range s.Workshop {
			if w.ID != wid {
				out = append(out, w)
			}
		}
		s.Workshop = out
		if s.WorkshopStart == wid {
			s.WorkshopStart = ""
		}
		return nil
	})
	return err
}

// SetWorkshopStart sets the Workshop map a server starts on ("": its usual map). At its next start.
func (m *Manager) SetWorkshopStart(id, wid, by string) error {
	if _, _, err := m.cs2(id); err != nil {
		return err
	}
	if wid != "" && workshop.ParseID(wid) != wid {
		return errors.New("not a Workshop ID")
	}
	s, err := m.editServer(id, func(s *config.Server) error { s.WorkshopStart = wid; return nil })
	if err != nil {
		return err
	}
	if wid == "" {
		m.Log.Add(events.Info, id, s.Name, "Starts on its map %s again (%s)", s.Map, by)
	} else {
		m.Log.Add(events.Info, id, s.Name, "Starts on the Workshop map %s, from its next start (%s)", workshopTitle(s, wid), by)
	}
	return nil
}

// SetWorkshopCollection sets the collection a server plays ("": none). At its next start.
func (m *Manager) SetWorkshopCollection(id, input, by string) (string, error) {
	if _, _, err := m.cs2(id); err != nil {
		return "", err
	}
	cid, title := "", ""
	if input != "" {
		cid = workshop.ParseID(input)
		if cid == "" {
			return "", errors.New("paste the collection's ID or address")
		}
		children, ok, err := workshop.Collection(cid)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New("this is not a Workshop collection (a map? add it to the list instead)")
		}
		title = cid
		if d, err := workshop.Details([]string{cid}); err == nil && len(d) == 1 {
			title = d[0].Title
		}
		title = fmt.Sprintf("%s (%d maps)", title, len(children))
	}
	s, err := m.editServer(id, func(s *config.Server) error { s.WorkshopColl, s.WorkshopCollFor = cid, title; return nil })
	if err != nil {
		return "", err
	}
	if cid == "" {
		m.Log.Add(events.Info, id, s.Name, "No Workshop collection any more, from its next start (%s)", by)
	} else {
		m.Log.Add(events.Info, id, s.Name, "Plays the Workshop collection %s, from its next start (%s)", title, by)
	}
	return title, nil
}

func workshopTitle(s config.Server, wid string) string {
	for _, w := range s.Workshop {
		if w.ID == wid {
			return w.Title
		}
	}
	return wid
}

// SetMapGroup has a CS2 server play a map group of its gamemodes_server.txt (+mapgroup in its launch options), from
// its next start. It tells whether that changed.
func (m *Manager) SetMapGroup(id, group, by string) (bool, error) {
	if _, _, err := m.cs2(id); err != nil {
		return false, errors.New("map groups are for Counter-Strike 2 servers")
	}
	changed := false
	s, err := m.editServer(id, func(s *config.Server) error {
		if config.ArgValue(s.Args, "+mapgroup") != group {
			s.Args, changed = config.SetArg(s.Args, "+mapgroup", group), true
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if changed {
		m.Log.Add(events.Info, id, s.Name, "Plays the map group %s, from its next start (%s)", group, by)
	}
	return changed, nil
}
