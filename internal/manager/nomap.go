package manager

import (
	"github.com/SnipeZilla/szm/internal/config"
	"regexp"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

// The maps a server's console said it does not have. The engine's own words are the proof (a check of its files
// would have to know where each of the games keeps its maps: folders, packed files, the workshop). Three engines,
// three ways to say it:
//
//	CS2:     map de_dust3:  invalid map name               (also changelevel, ds_workshop_changelevel …)
//	Source:  map load failed: de_dust3 not found or invalid    changelevel failed: de_dust3 not found
//	GoldSrc: map change failed: 'de_dust3' not found on server.
var noSuchMapRe = regexp.MustCompile(`(?:^|\s)\w+ (\S+?):\s+invalid map name|(?:map load|changelevel) failed: '?([^'\s]+?)'? not found|map change failed: '([^']+)' not found`)

// noSuchMap: the map a console line says is not there, or "".
func noSuchMap(line string) string {
	if !strings.Contains(line, "invalid map name") && !strings.Contains(line, "not found") {
		return ""
	}
	if m := noSuchMapRe.FindStringSubmatch(line); m != nil {
		return m[1] + m[2] + m[3]
	}
	return ""
}

// noSuchMaps: the maps the console lines say are not there, each once.
func noSuchMaps(console []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range console {
		if n := noSuchMap(l); n != "" && !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			out = append(out, n)
		}
	}
	return out
}

// noteNoMap records a map the console said is not there. The lock is held.
func (i *Instance) noteNoMapLocked(name string, at time.Time) {
	if i.noMap == nil {
		i.noMap = map[string]time.Time{}
	}
	k := strings.ToLower(name)
	if _, ok := i.noMap[k]; !ok {
		i.noMapNew = append(i.noMapNew, name)
	}
	i.noMap[k] = at
}

// sayNoMaps writes in its activity the maps its console said are not there (each once, while the manager runs).
func (m *Manager) sayNoMaps(i *Instance, id, name string) {
	i.mu.Lock()
	news := i.noMapNew
	i.noMapNew = nil
	i.mu.Unlock()
	for _, n := range news {
		m.Log.Add(events.Error, id, name, "Map %s not found: the server does not have it (its console said so).", n)
	}
}

// NoSuchMaps: the maps a server's console said it does not have, with when (lower-case names). A map it has played
// since is not in it.
func (m *Manager) NoSuchMaps(id string) map[string]time.Time {
	i := m.instance(id)
	i.mu.Lock()
	defer i.mu.Unlock()
	out := map[string]time.Time{}
	cur := ""
	if i.info != nil {
		cur = strings.ToLower(i.info.Map)
	}
	for k, t := range i.noMap {
		if k == cur {
			delete(i.noMap, k) // it has it now
			continue
		}
		out[k] = t
	}
	return out
}

// startMapGoneLocked: its start map, when its console said, since it started, that it does not exist and it has not
// answered since (the server waits with no map). The lock is held.
func (i *Instance) startMapGoneLocked(s config.Server) string {
	if s.Map == "" || i.status != StatusStarting || i.lastAnswer.After(i.since) {
		return ""
	}
	if t, ok := i.noMap[strings.ToLower(s.Map)]; ok && !t.Before(i.since) {
		return s.Map
	}
	return ""
}
