package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/link"
	"github.com/SnipeZilla/szm/internal/manager"
	"github.com/SnipeZilla/szm/internal/notify"
)

// uiVersion identifies the page's files (app.js, app.css, the logo, the game icons…): a page opened before an
// update of the manager sees it changed, and offers to reload (its old code would talk to the new manager the old
// way, its pictures would be the old ones).
var uiVersion = func() string {
	h := sha256.New()
	_ = fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := static.ReadFile(p)
			io.WriteString(h, p+"\x00")
			h.Write(b)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
}()

// Machines: this manager as a hub (other managers linked to it, their servers on its page) or as a node (linked to
// a hub). See package link.

func (s *Server) initLinks() error {
	st, err := link.Open(s.M.DataDir)
	if err != nil {
		return err
	}
	s.links = st
	s.hub = link.NewHub(st)
	s.hub.OnEvents = func(n link.Node, evs []events.Event) {
		for _, e := range evs {
			s.M.Log.AddRemote(e)
		}
	}
	s.hub.OnJoin = func(n link.Node) {
		s.M.Log.Add(events.Manager, "", "", "Machine %s joined: its servers show here", n.Name)
	}
	// offline for 90 s: said (Discord, XenForo too); back: said when its going was said
	var offMu sync.Mutex
	offTimers, offSaid := map[string]*time.Timer{}, map[string]bool{}
	s.hub.OnOnline = func(n link.Node, online bool) {
		offMu.Lock()
		defer offMu.Unlock()
		if t := offTimers[n.ID]; t != nil {
			t.Stop()
			delete(offTimers, n.ID)
		}
		if online {
			if offSaid[n.ID] {
				delete(offSaid, n.ID)
				s.M.Log.Add(events.Info, "", "", "Machine %s is connected again", n.Name)
			}
			return
		}
		since := time.Now()
		offTimers[n.ID] = time.AfterFunc(90*time.Second, func() {
			if on, _ := s.hub.Online(n.ID); on {
				return
			}
			if _, still := s.links.Get().Node(n.ID); !still {
				return
			}
			offMu.Lock()
			offSaid[n.ID] = true
			delete(offTimers, n.ID)
			offMu.Unlock()
			s.M.Log.Add(events.Error, "", "", "Machine %s is not connected since %s: its manager goes on by itself, but this hub can not see or reach its servers", n.Name, since.Format("15:04"))
		})
	}
	s.M.RemoteServer = s.remoteServer
	s.node = link.NewClient(st)
	s.node.Log = s.M.Log
	s.node.State = func() map[string]any {
		return map[string]any{"servers": s.M.Views(), "machine": s.M.MachineInfo(), "games": s.gameList(), "version": manager.Version}
	}
	return nil
}

// remoteTarget finds whether a request is for a linked machine: /api/m/<machine>/... (its settings, addons…) or a
// path with one of its servers, "<machine>~<server>". It gives the machine and the path on it.
func (s *Server) remoteTarget(r *http.Request) (node, path, server string, ok bool) {
	p := r.URL.EscapedPath()
	if !strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/api/link/") {
		return "", "", "", false
	}
	f := s.links.Get()
	if len(f.Nodes) == 0 {
		return "", "", "", false
	}
	if rest, found := strings.CutPrefix(p, "/api/m/"); found {
		id, sub, _ := strings.Cut(rest, "/")
		if _, known := f.Node(id); known {
			return id, "/api/" + sub, "", true
		}
		return "", "", "", false
	}
	segs := strings.Split(p, "/")
	for k, seg := range segs {
		before, after, cut := strings.Cut(seg, link.Sep)
		if !cut {
			continue
		}
		if _, known := f.Node(before); known && after != "" {
			segs[k] = after
			return before, strings.Join(segs, "/"), seg, true
		}
	}
	return "", "", "", false
}

// remote sends the requests for a linked machine to it, as the signed-in user.
func (s *Server) remote(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		node, path, server, ok := s.remoteTarget(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		u := s.current(r)
		if u == nil {
			writeErr(w, fail(http.StatusUnauthorized, "Please sign in."))
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-SZM") != "1" {
			writeErr(w, fail(http.StatusForbidden, "Missing X-SZM header."))
			return
		}
		if server != "" && !u.Sees(server) {
			writeErr(w, fail(http.StatusNotFound, "No such server."))
			return
		}
		if r.URL.RawQuery != "" {
			// the query too may name the machine's servers (?server=us~css-1)
			q := strings.NewReplacer(node+link.Sep, "", node+"%7E", "", node+"%7e", "").Replace(r.URL.RawQuery)
			path += "?" + q
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") && r.ContentLength != 0 {
			// a JSON body too may name the machine's servers ({"id":"us~css-1"}): as the machine knows them
			b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
			if err != nil {
				writeErr(w, fail(http.StatusBadRequest, "Bad request."))
				return
			}
			b = bytes.ReplaceAll(b, []byte(`"`+node+link.Sep), []byte(`"`))
			r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(b)), int64(len(b))
		}
		lu := link.User{Name: u.Name + " (" + s.links.Get().Name + ")", Rights: u.Rights, Servers: []string{}}
		for _, id := range u.Servers {
			if rest, found := strings.CutPrefix(id, node+link.Sep); found {
				lu.Servers = append(lu.Servers, rest)
			}
		}
		s.hub.Forward(w, r, node, path, lu)
	})
}

// remoteServer describes a server of a linked machine for the posts of its events: as it last said.
func (s *Server) remoteServer(id string) *notify.Server {
	node, _, ok := strings.Cut(id, link.Sep)
	if !ok {
		return nil
	}
	rm := s.hub.Remote(node)
	if rm == nil {
		return nil
	}
	for k, rid := range rm.IDs {
		if rid != id {
			continue
		}
		var v struct {
			Name     string `json:"name"`
			Hostname string `json:"hostname"`
			Address  string `json:"address"`
			Game     string `json:"game"`
			GameName string `json:"game_name"`
		}
		if json.Unmarshal(rm.Servers[k], &v) != nil {
			return nil
		}
		out := &notify.Server{Name: v.Name, Hostname: v.Hostname, Address: v.Address, Game: v.GameName}
		if g := s.M.Catalog()[v.Game]; g != nil && g.Icon != "" {
			if b := s.iconPNG(g.Icon); len(b) > 0 {
				out.Icon, out.IconName = b, g.Icon+".png"
			}
		}
		return out
	}
	return nil
}

// linkedUser is the hub's user of a request that came through the link (on a node).
func linkedUser(r *http.Request) *User {
	lu, ok := link.UserFrom(r.Context())
	if !ok {
		return nil
	}
	return &User{Name: lu.Name, Rights: lu.Rights, Servers: lu.Servers}
}

/* ---------------------------------------------------------------------------------------------------------------
 * What the page shows: the servers of every machine
 * ------------------------------------------------------------------------------------------------------------- */

// machineView is a machine in the state: this one first, then the linked ones.
type machineView struct {
	ID      string          `json:"id"` // "" for this machine
	Name    string          `json:"name"`
	Local   bool            `json:"local,omitempty"`
	Online  bool            `json:"online"`
	Since   int64           `json:"since,omitempty"` // connected since, or last heard of (unix seconds)
	Linked  bool            `json:"linked"`          // joined (false: its code was not used yet)
	Version string          `json:"version,omitempty"`
	Machine json.RawMessage `json:"machine,omitempty"`
	Servers int             `json:"servers"`
}

// allVisible gives the servers a user sees: this machine's, then the linked machines'.
func (s *Server) allVisible(u *User) []any {
	out := []any{}
	for _, v := range s.visible(u) {
		out = append(out, v)
	}
	for _, n := range s.links.Get().Nodes {
		rm := s.hub.Remote(n.ID)
		if rm == nil {
			continue
		}
		for k, raw := range rm.Servers {
			if u.Sees(rm.IDs[k]) {
				out = append(out, raw)
			}
		}
	}
	return out
}

// machines lists the machines, when there are linked ones (nil otherwise: the page shows no machine).
func (s *Server) machines(u *User) []machineView {
	f := s.links.Get()
	if len(f.Nodes) == 0 {
		return nil
	}
	out := []machineView{{Name: f.Name, Local: true, Online: true, Linked: true, Version: manager.Version, Servers: len(s.visible(u))}}
	for _, n := range f.Nodes {
		mv := machineView{ID: n.ID, Name: n.Name, Linked: n.Linked()}
		on, since := s.hub.Online(n.ID)
		mv.Online = on
		if on {
			mv.Since = since.Unix()
		}
		if rm := s.hub.Remote(n.ID); rm != nil {
			mv.Version, mv.Machine = rm.Version, rm.Machine
			if !on {
				mv.Since = rm.At.Unix()
			}
			for _, id := range rm.IDs {
				if u.Sees(id) {
					mv.Servers++
				}
			}
		}
		out = append(out, mv)
	}
	return out
}

// allGames: this machine's games, and those only a linked machine has (a game added there).
func (s *Server) allGames() []gameView {
	out := s.gameList()
	added := false
	have := map[string]bool{}
	for _, g := range out {
		have[g.Key] = true
	}
	for _, n := range s.links.Get().Nodes {
		rm := s.hub.Remote(n.ID)
		if rm == nil || len(rm.Games) == 0 {
			continue
		}
		var gs []gameView
		if json.Unmarshal(rm.Games, &gs) != nil {
			continue
		}
		for _, g := range gs {
			if !have[g.Key] {
				have[g.Key] = true
				out = append(out, g)
				added = true
			}
		}
	}
	if !added {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

/* ---------------------------------------------------------------------------------------------------------------
 * Settings, Machines
 * ------------------------------------------------------------------------------------------------------------- */

func (s *Server) machinesInfo(w http.ResponseWriter, r *http.Request, u *User) error {
	f := s.links.Get()
	out := map[string]any{"name": f.Name, "nodes": s.machines(u)}
	if f.Hub != nil {
		out["hub"] = map[string]any{"url": f.Hub.URL, "name": f.Hub.Hub, "id": f.Hub.ID, "status": s.node.Status()}
	}
	pairing := map[string]int64{}
	for _, n := range f.Nodes {
		if !n.Linked() {
			pairing[n.ID] = n.PairUntil
		}
	}
	out["pairing"] = pairing
	return writeJSON(w, out)
}

func (s *Server) machinesName(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 40 {
		return fail(http.StatusBadRequest, "A name of 1 to 40 characters: US, EU…")
	}
	if err := s.links.Update(func(f *link.File) error { f.Name = name; return nil }); err != nil {
		return err
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) machinesAdd(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	n, code, err := s.hub.Add(in.Name, in.URL)
	if err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	s.M.Log.Add(events.Manager, "", "", "Machine %s added by %s: waiting for it to join", n.Name, u.Name)
	return writeJSON(w, map[string]any{"id": n.ID, "code": code})
}

func (s *Server) machinesCode(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	code, err := s.hub.NewCode(r.PathValue("node"), in.URL)
	if err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"code": code})
}

func (s *Server) machinesRemove(w http.ResponseWriter, r *http.Request, u *User) error {
	id := r.PathValue("node")
	n, ok := s.links.Get().Node(id)
	if !ok {
		return fail(http.StatusNotFound, "No such machine.")
	}
	if err := s.hub.Remove(id); err != nil {
		return err
	}
	s.M.Log.Add(events.Manager, "", "", "Machine %s removed by %s (its manager and servers go on by themselves)", n.Name, u.Name)
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) machinesJoin(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	hl, err := s.node.Join(r.Context(), in.Code)
	if err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	s.M.Log.Add(events.Manager, "", "", "Linked to the hub %s (%s) by %s", hl.Hub, hl.URL, u.Name)
	return writeJSON(w, map[string]any{"ok": true, "hub": hl.Hub})
}

func (s *Server) machinesUnlink(w http.ResponseWriter, r *http.Request, u *User) error {
	f := s.links.Get()
	if f.Hub == nil {
		return fail(http.StatusBadRequest, "This manager is not linked to a hub.")
	}
	if err := s.node.Unlink(); err != nil {
		return err
	}
	s.M.Log.Add(events.Manager, "", "", "Unlinked from the hub %s by %s", f.Hub.Hub, u.Name)
	return writeJSON(w, map[string]any{"ok": true})
}

// linkedHub is the name of the hub this manager is linked to ("" none).
func (s *Server) linkedHub() string {
	if h := s.links.Get().Hub; h != nil {
		return h.Hub
	}
	return ""
}
