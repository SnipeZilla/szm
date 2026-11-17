package manager

import (
	"errors"
	"fmt"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/workshop"
)

// CS2 Workshop maps of a server: added by their ID or address (one map, or every map of a collection), played now
// (host_workshop_map, by RCON), started on (the server starts on its usual map, and is changed to the Workshop map,
// by RCON too, once it is up: a server given +host_workshop_map in place of +map has no map until the download
// ends), and collections kept for it, one of which the server plays (+host_workshop_collection). CS2 downloads
// them itself: nothing to install.

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

// workshopSwitch changes a CS2 server that has just come up (on its usual map) to the Workshop map it starts on.
// Tried a few times: its RCON may open a moment after it answers queries. Not for a server whose own options give
// +host_workshop_map: theirs is the one.
func (m *Manager) workshopSwitch(i *Instance, id string) {
	i.mu.Lock()
	since := i.since
	i.mu.Unlock()
	var last error
	for try := 0; try < 4; try++ {
		time.Sleep(time.Duration(3+2*try) * time.Second)
		s, ok := m.server(id)
		g := m.Catalog()[s.Game]
		if !ok || g == nil || g.Engine != games.Source2 || s.WorkshopStart == "" || config.ArgValue(s.Args, "+host_workshop_map") != "" {
			return
		}
		i.mu.Lock()
		gone := i.pid == 0 || !i.since.Equal(since) || i.busy != ""
		i.mu.Unlock()
		if gone {
			return // ended, started again or being stopped meanwhile
		}
		if _, last = m.RconBy(id, "host_workshop_map "+s.WorkshopStart, ""); last == nil {
			m.Log.Add(events.Info, id, s.Name, "Changing to its Workshop start map %s", workshopTitle(s, s.WorkshopStart))
			return
		}
	}
	if s, ok := m.server(id); ok {
		m.Log.Add(events.Error, id, s.Name, "Could not change to its Workshop start map %s: %v. It stays on its usual map (Play, on its Maps tab, changes to it)", workshopTitle(s, s.WorkshopStart), last)
	}
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
		m.Log.Add(events.Info, id, s.Name, "Changes to the Workshop map %s once it is up, from its next start (%s)", workshopTitle(s, wid), by)
	}
	return nil
}

const maxCollections = 50

// collTitle: how a collection is named in the activity and the page ("Winter maps (12 maps)").
func collTitle(c config.WorkshopCollection) string {
	if c.Maps > 0 {
		return fmt.Sprintf("%s (%d maps)", c.Title, c.Maps)
	}
	return c.Title
}

// lookCollection asks Steam for a collection: its name and how many maps it has.
func lookCollection(input string) (config.WorkshopCollection, error) {
	cid := workshop.ParseID(input)
	if cid == "" {
		return config.WorkshopCollection{}, errors.New("paste the collection's ID or address")
	}
	children, ok, err := workshop.Collection(cid)
	if err != nil {
		return config.WorkshopCollection{}, err
	}
	if !ok {
		return config.WorkshopCollection{}, errors.New("this is not a Workshop collection (a map? add it to the list instead)")
	}
	c := config.WorkshopCollection{ID: cid, Title: cid, Maps: len(children)}
	if d, err := workshop.Details([]string{cid}); err == nil && len(d) == 1 && d[0].Title != "" {
		c.Title = d[0].Title
	}
	// its maps: their names and pictures, shown under it
	if len(children) > maxWorkshop {
		children = children[:maxWorkshop]
	}
	for k := 0; k < len(children); k += 100 {
		part, err := workshop.Details(children[k:min(k+100, len(children))])
		if err != nil {
			return c, err
		}
		for _, it := range part {
			if it.AppID != 0 && it.AppID != 730 {
				continue
			}
			c.List = append(c.List, config.WorkshopMap{ID: it.ID, Title: it.Title, Preview: it.Preview, Size: it.Size, Updated: it.Updated})
		}
	}
	return c, nil
}

// keepCollection puts a collection among those kept for a server (its newer name and size when it is there
// already). The configuration is locked (editServer).
func keepCollection(s *config.Server, c config.WorkshopCollection) error {
	s.WorkshopColls = s.Collections()
	for k := range s.WorkshopColls {
		if s.WorkshopColls[k].ID == c.ID {
			s.WorkshopColls[k] = c
			return nil
		}
	}
	if len(s.WorkshopColls) >= maxCollections {
		return fmt.Errorf("%d collections are kept already: remove one first", maxCollections)
	}
	s.WorkshopColls = append(s.WorkshopColls, c)
	return nil
}

// AddWorkshopCollection keeps a Workshop collection for a server (one for each season, say), to be picked as the
// one it plays. What it plays does not change. One kept already is read again from Steam: its name and its maps.
func (m *Manager) AddWorkshopCollection(id, input, by string) (config.WorkshopCollection, error) {
	if _, _, err := m.cs2(id); err != nil {
		return config.WorkshopCollection{}, err
	}
	c, err := lookCollection(input)
	if err != nil {
		return c, err
	}
	refresh := false // kept already: its name and maps read again
	s, err := m.editServer(id, func(s *config.Server) error {
		for _, k := range s.Collections() {
			refresh = refresh || k.ID == c.ID
		}
		if err := keepCollection(s, c); err != nil {
			return err
		}
		if s.WorkshopColl == c.ID {
			s.WorkshopCollFor = collTitle(c)
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if !refresh {
		m.Log.Add(events.Info, id, s.Name, "Workshop collection %s kept by %s", collTitle(c), by)
	}
	return c, nil
}

// RemoveWorkshopCollection takes a collection off those kept for a server. The one it plays: it plays none, from
// its next start.
func (m *Manager) RemoveWorkshopCollection(id, cid, by string) error {
	played := false
	s, err := m.editServer(id, func(s *config.Server) error {
		out := []config.WorkshopCollection{}
		for _, c := range s.Collections() {
			if c.ID != cid {
				out = append(out, c)
			}
		}
		s.WorkshopColls = out
		if s.WorkshopColl == cid {
			s.WorkshopColl, s.WorkshopCollFor, played = "", "", true
		}
		return nil
	})
	if err == nil && played {
		m.Log.Add(events.Info, id, s.Name, "No Workshop collection any more, from its next start (removed by %s)", by)
	}
	return err
}

// SetWorkshopCollection sets the collection a server plays ("": none), from its next start: one of those kept
// for it, by its ID, or a new one (its ID or address), which is kept too.
func (m *Manager) SetWorkshopCollection(id, input, by string) (string, error) {
	sv, _, err := m.cs2(id)
	if err != nil {
		return "", err
	}
	var c config.WorkshopCollection
	if input != "" {
		cid, kept := workshop.ParseID(input), false
		for _, k := range sv.Collections() {
			if k.ID == cid {
				c, kept = k, true // kept already: Steam is not asked again
			}
		}
		if !kept {
			if c, err = lookCollection(input); err != nil {
				return "", err
			}
		}
	}
	title := ""
	s, err := m.editServer(id, func(s *config.Server) error {
		if c.ID != "" {
			if err := keepCollection(s, c); err != nil {
				return err
			}
			title = collTitle(c)
		}
		s.WorkshopColl, s.WorkshopCollFor = c.ID, title
		return nil
	})
	if err != nil {
		return "", err
	}
	if c.ID == "" {
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
	for _, c := range s.WorkshopColls { // a map of one of its collections
		for _, w := range c.List {
			if w.ID == wid {
				return w.Title
			}
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
