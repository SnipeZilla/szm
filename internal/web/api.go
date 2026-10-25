package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/manager"
	"github.com/SnipeZilla/szm/internal/notify"
	"github.com/SnipeZilla/szm/internal/proc"
)

// gameView is a game of the catalog as the page needs it.
type gameView struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Short      string   `json:"short"`
	Icon       string   `json:"icon,omitempty"`
	Engine     string   `json:"engine"`
	DefaultMap string   `json:"default_map"`
	DefaultMax int      `json:"default_maxplayers"`
	Metamod    bool     `json:"metamod"`
	Login      bool     `json:"login"`
	Exe        []string `json:"exe"` // the programs its server starts with, on this system
	// What its servers have (the games of other engines have not all of it)
	Query    string `json:"query"` // "" (strict), try, none
	Rcon     string `json:"rcon"`  // source, webrcon, telnet, none
	Notes    string `json:"notes,omitempty"`
	Say      bool   `json:"say"`      // it can write to its players (warnings)
	Password bool   `json:"password"` // its command line takes a join password
	GSLT     bool   `json:"gslt"`     // ... a login token
	Map      bool   `json:"map"`      // ... a map (a world)
	Log      bool   `json:"log"`      // it writes a console log the manager shows
}

// hasPlaceholder: whether a game's command line uses one ({password}, {gslt}); the Source engines' take all.
func hasPlaceholder(g *games.Game, ph string) bool {
	if !g.IsOther() {
		return true
	}
	for _, list := range append([][]string{g.Args}, mapValues(g.ArgsOS)...) {
		for _, a := range list {
			if strings.Contains(a, ph) {
				return true
			}
		}
	}
	return false
}

func mapValues(m map[string][]string) [][]string {
	var out [][]string
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func (s *Server) gameList() []gameView {
	var out []gameView
	for _, g := range s.M.Catalog().Sorted() {
		exe := []string{}
		for _, c := range g.ExeCandidates() {
			exe = append(exe, filepath.ToSlash(c))
		}
		out = append(out, gameView{g.Key, g.Name, g.Short, g.Icon, g.Engine, g.DefaultMap, g.DefaultMax, g.Metamod, g.Login, exe,
			g.QueryMode(), g.RconMode(), g.Notes, g.Say != "", hasPlaceholder(g, "{password}"), hasPlaceholder(g, "{gslt}"), hasPlaceholder(g, "{map}"), g.ConsoleLog != ""})
	}
	return out
}

func (s *Server) visible(u *User) []manager.View {
	var out []manager.View
	for _, v := range s.M.Views() {
		if u.Sees(v.ID) {
			out = append(out, v)
		}
	}
	return out
}

func (s *Server) state(w http.ResponseWriter, r *http.Request, u *User) error {
	return writeJSON(w, map[string]any{
		"servers":  s.allVisible(u),
		"machine":  s.M.MachineInfo(),
		"machines": s.machines(u),
		"ui":       uiVersion,
		"crashes":  s.crashes24h(u),
		"games":    s.allGames(),
		"user":     s.me(u),
		"rights":   RightNames,
		"events":   s.recent(u, 60),
		"warn":     s.warnTimes(),
	})
}

func (s *Server) addonsList(w http.ResponseWriter, r *http.Request, u *User) error {
	if !u.Can("h") && !u.Can("f") {
		return errForbidden
	}
	s.M.Cfg.RLock()
	keep := append([]string(nil), s.M.Cfg.AddonsKeep...)
	s.M.Cfg.RUnlock()
	return writeJSON(w, map[string]any{"packages": s.M.Packages(), "root": s.M.AddonsRoot(), "keep": keep})
}

func (s *Server) addonsOp(w http.ResponseWriter, r *http.Request, u *User) error {
	name := r.PathValue("pkg")
	q := r.URL.Query()
	switch op := r.PathValue("op"); op {
	case "create":
		if err := s.M.CreatePackage(name, u.Name); err != nil {
			return fail(http.StatusConflict, "%s", capitalize(err.Error()))
		}
	case "archive":
		mode := manager.PkgReplace
		switch q.Get("mode") {
		case "update", "merge": // merge: the name of an older page
			mode = manager.PkgUpdate
		case "add":
			mode = manager.PkgAdd
		}
		return s.uploadArchive(w, r, u, name, mode)
	case "import":
		from := q.Get("from")
		if !u.Sees(from) {
			return fail(http.StatusNotFound, "No such server.")
		}
		if err := s.M.ImportPackage(name, from, q.Get("folder"), u.Name); err != nil {
			return fail(http.StatusConflict, "%s", capitalize(err.Error()))
		}
	case "push":
		started, errs := s.M.PushPackage(name, u.Name, q.Get("warn") == "1", u.Sees)
		return writeJSON(w, map[string]any{"started": started, "errors": errs})
	case "check":
		info, err := s.M.CheckAddon(r.Context(), name, u.Name)
		if err != nil && !errors.Is(err, manager.ErrAddonBusy) {
			return writeJSON(w, map[string]any{"update": info, "error": capitalize(err.Error())})
		}
		return writeJSON(w, map[string]any{"update": info})
	case "install":
		if err := s.M.InstallAddon(name, u.Name, false); err != nil {
			return fail(http.StatusConflict, "%s", capitalize(err.Error()))
		}
	case "have":
		if err := s.M.MarkAddonInstalled(name, u.Name); err != nil {
			return fail(http.StatusConflict, "%s", capitalize(err.Error()))
		}
	case "unwatch":
		s.M.StopWatch(name, u.Name)
	case "upload":
		return s.upload(w, r, u)
	case "mkdir":
		return s.mkdir(w, r, u)
	case "rename":
		return s.rename(w, r, u)
	default:
		return fail(http.StatusNotFound, "Unknown operation.")
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// addonSource sets where a package's new versions are found ({"kind": ""} or null: the default its name gives).
func (s *Server) addonSource(w http.ResponseWriter, r *http.Request, u *User) error {
	var src *config.AddonSource
	if err := readJSON(r, &src); err != nil {
		return err
	}
	if src != nil && src.Kind == "" {
		src = nil
	}
	name := r.PathValue("pkg")
	if err := s.M.SetAddonSource(name, src, u.Name); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"update": s.M.AddonUpdate(name)})
}

func (s *Server) deletePackage(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.M.DeletePackage(r.PathValue("pkg"), u.Name); err != nil {
		return fail(http.StatusConflict, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// uploadArchive receives a .zip or .tar.gz (streamed to a temporary file) and puts it into a package.
func (s *Server) uploadArchive(w http.ResponseWriter, r *http.Request, u *User, name, mode string) error {
	if _, err := s.M.PackagePath(name); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return fail(http.StatusBadRequest, "Bad upload.")
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			return fail(http.StatusBadRequest, "No archive in the upload.")
		}
		if part.FormName() != "file" || part.FileName() == "" {
			part.Close()
			continue
		}
		tmp, err := os.CreateTemp("", "szm-archive-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		_, cerr := io.Copy(tmp, part)
		tmp.Close()
		part.Close()
		if cerr != nil {
			return fail(http.StatusBadRequest, "Upload: %v", cerr)
		}
		n, kept, err := s.M.ExtractPackage(name, tmp.Name(), filepath.Base(strings.ReplaceAll(part.FileName(), `\`, "/")), mode, u.Name)
		if err != nil {
			return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
		}
		return writeJSON(w, map[string]any{"ok": true, "files": n, "kept": kept})
	}
}

func (s *Server) addonFolders(w http.ResponseWriter, r *http.Request, u *User) error {
	list, err := s.M.AddonFolders(r.PathValue("id"))
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]any{"folders": list})
}

// warnTimes are how long players are warned before an update, and before a restart or a stop (seconds).
func (s *Server) warnTimes() map[string]int {
	s.M.Cfg.RLock()
	defer s.M.Cfg.RUnlock()
	return map[string]int{"update": s.M.Cfg.Updates.WarnSeconds, "restart": s.M.Cfg.Restart.WarnSeconds}
}

func (s *Server) recent(u *User, n int) []events.Event {
	var out []events.Event
	for _, e := range s.M.Log.Recent("", 500) {
		if s.canSeeEvent(u, e) {
			out = append(out, e)
			if len(out) == n {
				break
			}
		}
	}
	return out
}

// crashes24h: the crashes of the last 24 hours, of the servers the user sees (every machine's on a hub)
func (s *Server) crashes24h(u *User) int {
	return s.M.Log.Crashes24h(func(e events.Event) bool { return s.canSeeEvent(u, e) })
}

func (s *Server) canSeeEvent(u *User, e events.Event) bool {
	if e.Server == "" {
		return u.Can("e")
	}
	return u.Sees(e.Server)
}

// stream sends the state of the servers every 2 s (when it changed) and the events as they come (Server-Sent Events).
func (s *Server) stream(w http.ResponseWriter, r *http.Request, u *User) error {
	fl, ok := w.(http.Flusher)
	if !ok {
		return fail(http.StatusInternalServerError, "streaming not supported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	evs := s.M.Log.Subscribe()
	defer s.M.Log.Unsubscribe(evs)
	s.viewing(true)
	defer s.viewing(false)

	send := func(kind string, v any) bool {
		b, _ := json.Marshal(v)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	var last []byte
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	ping := 0
	for {
		select {
		case <-r.Context().Done():
			return nil
		case e, ok := <-evs:
			if !ok {
				return nil
			}
			if s.canSeeEvent(u, e) && !send("log", e) {
				return nil
			}
		case <-tick.C:
			// The user may have been deleted or changed
			if nu := s.users.get(u.Name); nu == nil {
				return nil
			} else {
				u = nu
			}
			st := map[string]any{"servers": s.allVisible(u), "machine": s.M.MachineInfo(), "machines": s.machines(u), "ui": uiVersion,
				"crashes": s.crashes24h(u)}
			b, _ := json.Marshal(st)
			// Uptime of the manager changes every time: compare without it
			cmp := bytes.ReplaceAll(b, []byte(`"uptime"`), nil)
			ping++
			if bytes.Equal(cmp, last) && ping < 15 {
				continue
			}
			ping = 0
			last = cmp
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b); err != nil {
				return nil
			}
			fl.Flush()
		}
	}
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request, u *User) error {
	day := r.URL.Query().Get("day")
	server := r.URL.Query().Get("server")
	if server != "" && !u.Sees(server) {
		return fail(http.StatusNotFound, "No such server.")
	}
	var list []events.Event
	if day == "" {
		list = s.M.Log.Recent(server, 500)
	} else {
		all, err := s.M.Log.Day(day)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for k := len(all) - 1; k >= 0; k-- {
			if server == "" || all[k].Server == server {
				list = append(list, all[k])
			}
		}
	}
	out := []events.Event{}
	for _, e := range list {
		if s.canSeeEvent(u, e) {
			out = append(out, e)
		}
	}
	return writeJSON(w, map[string]any{"events": out, "days": s.M.Log.Days()})
}

/* ---------------------------------------------------------------------------------------------------------------
 * Servers
 * ------------------------------------------------------------------------------------------------------------- */

func (s *Server) serverOp(w http.ResponseWriter, r *http.Request, u *User) error {
	id, op := r.PathValue("id"), r.PathValue("op")
	warn := r.URL.Query().Get("warn") == "1"
	var err error
	switch op {
	case "start", "stop", "restart", "cancel":
		if !u.Can("i") {
			return errForbidden
		}
		switch op {
		case "start":
			err = s.M.Start(id, u.Name)
		case "stop":
			err = s.M.Stop(id, u.Name, warn)
		case "restart":
			err = s.M.Restart(id, "", u.Name, warn)
		case "cancel":
			err = s.M.Cancel(id)
		}
	case "update", "validate":
		if !u.Can("h") {
			return errForbidden
		}
		err = s.M.Update(id, op == "validate", u.Name, warn)
	case "addons":
		if !u.Can("h") {
			return errForbidden
		}
		err = s.M.PushAddons(id, r.URL.Query().Get("package"), u.Name, warn)
	case "package":
		if !u.Can("f") && !u.Can("h") {
			return errForbidden
		}
		err = s.M.SetPackage(id, r.URL.Query().Get("name"), r.URL.Query().Get("on") == "1", u.Name)
	case "install":
		if !u.Can("c") && !u.Can("h") {
			return errForbidden
		}
		err = s.M.Install(id, u.Name, r.URL.Query().Get("start") == "1")
	default:
		return fail(http.StatusNotFound, "Unknown operation.")
	}
	if err != nil {
		return fail(http.StatusConflict, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (s *Server) consoleStream(w http.ResponseWriter, r *http.Request, u *User) error {
	id := r.PathValue("id")
	if since := r.URL.Query().Get("since"); since != "" {
		n, _ := strconv.ParseInt(since, 10, 64)
		lines, next := s.M.ConsoleSince(id, n)
		return writeJSON(w, map[string]any{"lines": lines, "next": next})
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		return fail(http.StatusInternalServerError, "streaming not supported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	c := s.M.SubscribeConsole(id)
	defer s.M.UnsubscribeConsole(id, c)
	b, _ := json.Marshal(s.M.Console(id))
	fmt.Fprintf(w, "event: init\ndata: %s\n\n", b)
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	batch := []string{}
	flush := time.NewTicker(150 * time.Millisecond)
	defer flush.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case l := <-c:
			batch = append(batch, l)
		case <-flush.C:
			if len(batch) == 0 {
				continue
			}
			b, _ := json.Marshal(batch)
			batch = batch[:0]
			if _, err := fmt.Fprintf(w, "event: lines\ndata: %s\n\n", b); err != nil {
				return nil
			}
			fl.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
			fl.Flush()
		}
	}
}

func (s *Server) rcon(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Command string `json:"command"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	cmd := strings.TrimSpace(in.Command)
	if cmd == "" {
		return fail(http.StatusBadRequest, "Type a command.")
	}
	low := strings.ToLower(cmd)
	if (strings.HasPrefix(low, "rcon_password") || strings.HasPrefix(low, "sv_setsteamaccount")) && !u.Can("f") {
		return errForbidden
	}
	out, err := s.M.RconBy(r.PathValue("id"), cmd, u.Name)
	if err != nil {
		return fail(http.StatusBadGateway, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"output": config.Redact(out)})
}

func (s *Server) serverConfig(w http.ResponseWriter, r *http.Request, u *User) error {
	sv := s.M.Cfg.Server(r.PathValue("id"))
	if sv == nil {
		return fail(http.StatusNotFound, "No such server.")
	}
	s.M.Cfg.RLock()
	cp := *sv
	s.M.Cfg.RUnlock()
	return writeJSON(w, cp)
}

func (s *Server) saveServer(w http.ResponseWriter, r *http.Request, u *User) error {
	var in config.Server
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.ID = r.PathValue("id")
	if errs := s.M.SaveServer(&in, u.Name); len(errs) > 0 {
		return writeErrs(w, errs)
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func writeErrs(w http.ResponseWriter, errs []string) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	return json.NewEncoder(w).Encode(map[string]any{"error": strings.Join(errs, " "), "errors": errs})
}

func (s *Server) addServer(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Server   config.Server `json:"server"`
		Install  bool          `json:"install"`
		Start    bool          `json:"start"`
		Update   bool          `json:"update"`   // already installed in its folder: update it with steamcmd all the same
		Password string        `json:"password"` // of the Steam account: for this download only, never saved
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	sv := in.Server
	sv.ID = ""
	if strings.TrimSpace(sv.InstallDir) != "" {
		sv.InstallDir = fullDir(sv.InstallDir)
		// A folder that already has the game's server: its server.cfg's RCON password is the one it uses
		if s.M.AdoptRcon(&sv) {
			in.Install = false
		}
		if in.Install {
			if f := s.M.InspectFolder(sv.InstallDir, sv.Game); f.Exists && !f.Empty && f.Exe == "" && !f.Installed && sv.Exe == "" {
				return writeErrs(w, []string{fmt.Sprintf("%s is not empty and has no %s server: pick an empty folder, or the folder of a %s server.", sv.InstallDir, gameName(s, sv.Game), gameName(s, sv.Game))})
			}
		}
	}
	if f := strings.Fields(sv.SteamLogin); len(f) > 1 {
		if in.Password == "" {
			in.Password = f[1]
		}
		sv.SteamLogin = f[0] // the account only goes into szm.json
	}
	if errs := s.M.AddServer(&sv, u.Name); len(errs) > 0 {
		return writeErrs(w, errs)
	}
	// Whoever adds a server looks after it
	if !u.Can("e") {
		u.Servers = append(u.Servers, sv.ID)
		_ = s.users.put(*u, "")
	}
	if exe, _ := s.M.ExeOf(sv); exe != "" && !in.Update {
		in.Install = false // already there: added as it is
	}
	if in.Install {
		if in.Password != "" {
			s.M.SetSteamPassword(sv.ID, in.Password)
		}
		if err := s.M.Install(sv.ID, u.Name, in.Start); err != nil {
			return fail(http.StatusConflict, "%s", capitalize(err.Error()))
		}
	}
	return writeJSON(w, map[string]any{"ok": true, "id": sv.ID})
}

// steamLogin downloads a server again with a Steam account and what Steam asked for (password, Steam Guard code).
func (s *Server) steamLogin(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Account   string `json:"account"`
		Password  string `json:"password"`
		Guard     string `json:"guard"`
		TwoFactor string `json:"two_factor"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := s.M.SteamLogin(r.PathValue("id"), in.Account, in.Password, in.Guard, in.TwoFactor, u.Name); err != nil {
		return fail(http.StatusConflict, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request, u *User) error {
	files := r.URL.Query().Get("files") == "1"
	if err := s.M.DeleteServer(r.PathValue("id"), files, u.Name); err != nil {
		return fail(http.StatusConflict, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// preview shows the command line a server would start with.
func (s *Server) preview(w http.ResponseWriter, r *http.Request, u *User) error {
	var in config.Server
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.ID != "" {
		if !u.Sees(in.ID) {
			return fail(http.StatusNotFound, "No such server.")
		}
		if old := s.M.Cfg.Server(in.ID); old != nil {
			s.M.Cfg.RLock()
			stored := *old
			s.M.Cfg.RUnlock()
			if in.Game == "" {
				in = stored // only the ID: the server as it is saved
			}
			in.Game = stored.Game
		}
	}
	g := s.M.Catalog()[in.Game]
	if g == nil {
		return fail(http.StatusBadRequest, "Pick a game.")
	}
	sv := in
	s.M.Prepare(&sv)
	found, expected := s.M.ExeOf(sv)
	exe := found
	if exe == "" {
		exe = expected
	}
	return writeJSON(w, map[string]any{
		"exe":       exe,
		"exe_found": found != "",
		"exe_auto":  sv.Exe == "",
		"dir":       filepath.Dir(exe),
		"command":   sv.HideSecrets(config.Redact(config.QuoteArgs(append([]string{filepath.Base(exe)}, sv.BuildArgs(g)...)))),
	})
}

// suggest proposes a port and a folder for a new server of a game.
func (s *Server) suggest(w http.ResponseWriter, r *http.Request, u *User) error {
	g := s.M.Catalog()[r.URL.Query().Get("game")]
	if g == nil {
		return fail(http.StatusBadRequest, "Pick a game.")
	}
	root := s.M.NewServersRoot()
	used := map[int]bool{}
	dirs := map[string]bool{}
	names := map[string]bool{}
	// The pattern the servers of this game already follow: "Counter-Strike 2 - Server 3" (folder), "CS2 - Server 3" (name)
	folderPrefix, namePrefix := "", ""
	usedOn := map[string]bool{} // "ip:port" taken; "*:port" on every address
	cat := s.M.Catalog()
	s.M.Cfg.RLock()
	for _, o := range s.M.Cfg.Servers {
		ip := o.IP
		if ip == "" || ip == "0.0.0.0" {
			ip = "*"
		}
		for _, p := range config.UsedPorts(o, cat[o.Game]) {
			used[p] = true
			usedOn[net.JoinHostPort(ip, strconv.Itoa(p))] = true
		}
		dirs[strings.ToLower(filepath.Clean(o.InstallDir))] = true
		names[strings.ToLower(o.Name)] = true
		if o.Game != g.Key {
			continue
		}
		if m := serverNumRe.FindStringSubmatch(filepath.Base(o.InstallDir)); m != nil && folderPrefix == "" {
			folderPrefix = m[1]
		}
		if m := serverNumRe.FindStringSubmatch(o.Name); m != nil && namePrefix == "" {
			namePrefix = m[1]
		}
	}
	s.M.Cfg.RUnlock()
	// The address: the machine's first public one where 27015 is free (a machine with several addresses gives each
	// server its own, all on 27015); else a port no server uses, on its first public address. None public (behind a
	// router): every address.
	// (The games of other engines start from their own port, and take the ports after it, their query and RCON ports.)
	ip, port := "", g.FirstPort()
	needs := func(p int) []int { return config.UsedPorts(&config.Server{Port: p}, g) }
	freeOn := func(a string, p int) bool {
		for _, x := range needs(p) {
			if usedOn[net.JoinHostPort(a, strconv.Itoa(x))] || usedOn[net.JoinHostPort("*", strconv.Itoa(x))] {
				return false
			}
		}
		return true
	}
	public := manager.PublicIPs()
	for _, a := range public {
		if freeOn(a, port) {
			ip = a
			break
		}
	}
	if ip == "" {
		free := func(p int) bool {
			for _, x := range needs(p) {
				if used[x] || x > 65535 {
					return false
				}
			}
			return true
		}
		for port < 65535 && !free(port) {
			port += g.PortSpan()
		}
		if len(public) > 0 {
			ip = public[0]
		}
	}
	// Else the game's name for the folder (one Windows accepts: "Counter-Strike: Source" gives "Counter-Strike
	// Source"), its short name for the server
	clean := func(v string) string {
		v = strings.Map(func(r rune) rune {
			if strings.ContainsRune(`<>:"/\|?*`, r) {
				return -1
			}
			return r
		}, v)
		return strings.Join(strings.Fields(v), " ")
	}
	if folderPrefix == "" {
		folderPrefix = clean(g.Name)
	}
	if namePrefix == "" {
		namePrefix = g.Short
	}
	var dir, name string
	for n := 1; n < 1000; n++ {
		dir = filepath.Join(root, fmt.Sprintf("%s - Server %d", folderPrefix, n))
		name = fmt.Sprintf("%s - Server %d", namePrefix, n)
		if dirs[strings.ToLower(dir)] || names[strings.ToLower(name)] {
			continue
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
	}
	return writeJSON(w, map[string]any{
		"ip": ip, "port": port, "install_dir": dir, "map": g.DefaultMap, "maxplayers": g.DefaultMax,
		"name": name, "rcon_password": config.RandomPassword(),
		"root": root, "folder_name": filepath.Base(dir), // where new folders go, and a name for this one
	})
}

// serverNumRe reads "Counter-Strike 2 - Server 3": its prefix and number.
var serverNumRe = regexp.MustCompile(`^(.+?) - Server (\d+)$`)

// browse lists the folders of a folder, to pick where a server goes.
func (s *Server) browse(w http.ResponseWriter, r *http.Request, u *User) error {
	p := r.URL.Query().Get("path")
	// files: the files to list too: "program" (what can start a server), or extensions (.xml,.cfg)
	want := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("files")))
	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Size int64  `json:"size,omitempty"`
	}
	out := []entry{}
	files := []entry{}
	if p == "" {
		for _, root := range proc.Roots() {
			out = append(out, entry{Name: root, Path: root})
		}
		return writeJSON(w, map[string]any{"path": "", "parent": "", "dirs": out, "files": files})
	}
	// A file given (C:\old\config.xml): its folder is listed
	if want != "" {
		if st, err := os.Stat(fullDir(p)); err == nil && !st.IsDir() {
			p = filepath.Dir(fullDir(p))
		}
	}
	exts := []string{}
	for _, x := range strings.Split(want, ",") {
		if x = strings.TrimSpace(x); x != "" && x != "program" {
			if !strings.HasPrefix(x, ".") {
				x = "." + x
			}
			exts = append(exts, x)
		}
	}
	p = fullDir(p)
	if !filepath.IsAbs(p) {
		return fail(http.StatusBadRequest, "A full path, please (E:\\Servers).")
	}
	list, err := os.ReadDir(p)
	if err != nil {
		if os.IsNotExist(err) {
			return fail(http.StatusNotFound, "%s does not exist.", p)
		}
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	for _, e := range list {
		if strings.HasPrefix(e.Name(), "$") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(p, e.Name())
		if e.IsDir() {
			out = append(out, entry{Name: e.Name(), Path: full})
			continue
		}
		if want == "" {
			continue
		}
		ok := want == "program" && manager.IsProgram(full)
		for _, x := range exts {
			if strings.HasSuffix(strings.ToLower(e.Name()), x) {
				ok = true
			}
		}
		if ok {
			var size int64
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
			files = append(files, entry{Name: e.Name(), Path: full, Size: size})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	// A game server's folder (configured, or with steamcmd's files): a new server does not go inside it
	// installed: it has a game server (steamcmd's files, a server program), not one of the manager
	server, installed := "", manager.LooksInstalled(p)
	if o, ok := s.M.Cfg.ServerAt(p); ok {
		server = o.Name
	} else if installed {
		server = "a game server"
	}
	return writeJSON(w, map[string]any{"path": p, "parent": parent, "dirs": out, "files": files, "empty": len(list) == 0, "server": server, "managed": server != "" && server != "a game server", "installed": installed})
}

// inspect tells what a folder holds for a new server of a game: nothing, its server already, another game...
func (s *Server) inspect(w http.ResponseWriter, r *http.Request, u *User) error {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		return fail(http.StatusBadRequest, "Give a folder.")
	}
	p = fullDir(p)
	if !filepath.IsAbs(p) {
		return fail(http.StatusBadRequest, "A full path, please (E:\\Servers).")
	}
	return writeJSON(w, s.M.InspectFolder(p, r.URL.Query().Get("game")))
}

// usage gives a server's network traffic and the size of its folder (for its Overview, while it is open).
func (s *Server) usage(w http.ResponseWriter, r *http.Request, u *User) error {
	us, err := s.M.Usage(r.PathValue("id"))
	if err != nil {
		return fail(http.StatusNotFound, "No such server.")
	}
	return writeJSON(w, us)
}

// programs lists the programs of a server's folder, to pick the one it starts with.
func (s *Server) programs(w http.ResponseWriter, r *http.Request, u *User) error {
	sv := s.M.Cfg.Server(r.PathValue("id"))
	if sv == nil {
		return fail(http.StatusNotFound, "No such server.")
	}
	s.M.Cfg.RLock()
	cp := *sv
	s.M.Cfg.RUnlock()
	found, expected := s.M.ExeOf(cp)
	return writeJSON(w, map[string]any{"dir": cp.InstallDir, "programs": manager.Programs(cp.InstallDir), "found": found, "expected": expected})
}

func gameName(s *Server, key string) string {
	if g := s.M.Catalog()[key]; g != nil {
		return g.Name
	}
	return key
}

// fullDir cleans a folder path; a bare drive ("E:") is its root ("E:\").
func fullDir(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if v := filepath.VolumeName(p); v != "" && v == p {
		p += string(filepath.Separator)
	}
	return p
}

// browseMkdir creates a folder (for a new server) inside another, which is created too if needed.
func (s *Server) browseMkdir(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	parent := fullDir(in.Path)
	name := strings.TrimSpace(in.Name)
	if !filepath.IsAbs(parent) {
		return fail(http.StatusBadRequest, "A full path, please (E:\\Servers).")
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `<>:"/\|?*`) || strings.TrimRight(name, ". ") != name {
		return fail(http.StatusBadRequest, "A folder name cannot have < > : \" / \\ | ? * nor end with a dot or a space.")
	}
	if o, ok := s.M.Cfg.ServerAt(parent); ok {
		return fail(http.StatusBadRequest, "%s is the folder of %s: create the new one next to it, not inside.", parent, o.Name)
	}
	if manager.LooksInstalled(parent) {
		return fail(http.StatusBadRequest, "%s holds a game server: create the new one next to it, not inside.", parent)
	}
	dir := filepath.Join(parent, name)
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return fail(http.StatusConflict, "%s is a file.", dir)
		}
		if list, _ := os.ReadDir(dir); len(list) > 0 {
			return fail(http.StatusConflict, "%s already exists and is not empty: give another name.", dir)
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true, "path": dir})
}

/* ---------------------------------------------------------------------------------------------------------------
 * Settings
 * ------------------------------------------------------------------------------------------------------------- */

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, u *User) error {
	s.M.Cfg.RLock()
	st := s.M.Cfg.Settings
	s.M.Cfg.RUnlock()
	return writeJSON(w, map[string]any{
		"settings":    st,
		"linked_hub":  s.linkedHub(),
		"data_dir":    s.M.DataDir,
		"steamcmd":    s.M.SteamCmdPath(),
		"cache_dir":   s.M.CacheDir(),
		"addons_dir":  s.M.AddonsRoot(),
		"backups_dir": filepath.Join(s.M.DataDir, "backups"),
		"config":      s.M.Cfg.Path(),
		"kinds":       []string{events.Crash, events.Failed, events.Update, events.Updated, events.Restart, events.Start, events.Stop, events.Error, events.Addons, events.Info},
		"priorities":  []string{"real time", "high", "above normal", "normal", "below normal", "idle"},
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, u *User) error {
	s.M.Cfg.RLock()
	st := s.M.Cfg.Settings
	s.M.Cfg.RUnlock()
	if err := readJSON(r, &st); err != nil {
		return err
	}
	var errs []string
	st.OldDiscord, st.OldXenForo = nil, nil
	if st.Discords == nil {
		st.Discords = []config.DiscordHook{}
	}
	if st.Forums == nil {
		st.Forums = []config.ForumPost{}
	}
	for k := range st.Discords {
		d := &st.Discords[k]
		d.Name, d.Webhook, d.Username = strings.TrimSpace(d.Name), strings.TrimSpace(d.Webhook), strings.TrimSpace(d.Username)
		if d.Name == "" {
			d.Name = fmt.Sprintf("Discord %d", k+1)
		}
		if d.Events == nil {
			d.Events = []string{}
		}
		if d.Webhook != "" && !notify.Valid(d.Webhook) {
			errs = append(errs, "Discord \""+d.Name+"\": the webhook must start with https://discord.com/api/webhooks/.")
		}
	}
	for k := range st.Forums {
		x := &st.Forums[k]
		x.Name, x.URL, x.Key, x.Icons = strings.TrimSpace(x.Name), strings.TrimSpace(x.URL), strings.TrimSpace(x.Key), strings.TrimSpace(x.Icons)
		if x.Icons != "" && !notify.ValidURL(x.Icons) {
			errs = append(errs, "XenForo \""+x.Name+"\": the icons' address starts with https:// (https://snipezilla.com/szm-icons/).")
		}
		if x.Name == "" {
			x.Name = fmt.Sprintf("Forum %d", k+1)
		}
		if x.Events == nil {
			x.Events = []string{}
		}
		if x.URL != "" || x.Key != "" {
			switch {
			case !notify.ValidURL(x.URL):
				errs = append(errs, "XenForo \""+x.Name+"\": the forum address starts with https:// (https://snipezilla.com).")
			case x.Key == "":
				errs = append(errs, "XenForo \""+x.Name+"\": the API key is missing.")
			case x.Thread <= 0 && x.Node <= 0:
				errs = append(errs, "XenForo \""+x.Name+"\": give the thread ID (posts as replies) or the forum ID (a new thread each time).")
			}
		}
		if x.User < 0 || x.Thread < 0 || x.Node < 0 || x.Prefix < 0 {
			errs = append(errs, "XenForo \""+x.Name+"\": the IDs are numbers (0 for none).")
		}
	}
	st.Backups.Time, st.Backups.Dir = strings.TrimSpace(st.Backups.Time), strings.TrimSpace(st.Backups.Dir)
	switch st.Backups.Schedule {
	case "manual", "daily", "weekly", "monthly":
	default:
		errs = append(errs, "Backups: by hand, daily, weekly or monthly.")
	}
	if _, _, ok := config.ParseClock(st.Backups.Time); !ok {
		if st.Backups.Schedule == "manual" {
			st.Backups.Time = "05:30"
		} else {
			errs = append(errs, "Backups: the time is HH:MM (24 h).")
		}
	}
	if st.Backups.Weekday < 0 || st.Backups.Weekday > 6 {
		st.Backups.Weekday = 1
	}
	if st.Backups.Day < 1 || st.Backups.Day > 31 {
		errs = append(errs, "Backups: the day of the month is 1 to 31 (31: the last day of each month).")
	}
	if st.Logs.ServerDays < 0 || st.Logs.ServerDays > 3650 {
		errs = append(errs, "Server logs: keep 0 (all) to 3650 days.")
	}
	if st.Backups.Keep < 1 || st.Backups.Keep > 1000 {
		errs = append(errs, "Backups: keep between 1 and 1000 of each server.")
	}
	if st.Backups.Dir != "" && !filepath.IsAbs(st.Backups.Dir) {
		errs = append(errs, "Backups: the folder is a full path ("+`D:\Backups\szm`+"), or empty for the data folder.")
	}
	if len(st.Backups.Include) == 0 {
		errs = append(errs, "Backups: say what is backed up (cfg, addons…).")
	}
	st.Backups.RClone = strings.TrimSpace(st.Backups.RClone)
	if st.Backups.RClone != "" && !filepath.IsAbs(st.Backups.RClone) {
		errs = append(errs, "Backups: rclone's program is a full path, or empty for the one downloaded here.")
	}
	if st.Backups.Offsite == nil {
		st.Backups.Offsite = []config.OffsiteCopy{}
	}
	seen := map[string]bool{}
	for k := range st.Backups.Offsite {
		o := &st.Backups.Offsite[k]
		o.Name, o.Target = strings.TrimSpace(o.Name), strings.TrimSpace(o.Target)
		if o.Name == "" {
			o.Name = fmt.Sprintf("Copy %d", k+1)
		}
		switch {
		case o.Kind != "rclone" && o.Kind != "folder":
			errs = append(errs, "Off-site copy \""+o.Name+"\": rclone or a folder.")
		case o.Target == "":
			errs = append(errs, "Off-site copy \""+o.Name+"\": say where.")
		case o.Kind == "rclone" && !strings.Contains(o.Target, ":"):
			errs = append(errs, "Off-site copy \""+o.Name+"\": an rclone remote and a folder, like gdrive:SZM backups.")
		case o.Kind == "folder" && !filepath.IsAbs(o.Target):
			errs = append(errs, "Off-site copy \""+o.Name+"\": the folder is a full path ("+`G:\My Drive\SZM backups`+").")
		case seen[o.Kind+"|"+o.Target]:
			errs = append(errs, "Off-site copy \""+o.Name+"\": the same place twice.")
		}
		seen[o.Kind+"|"+o.Target] = true
	}
	if _, _, ok := config.ParseClock(st.Restart.Time); !ok {
		errs = append(errs, "The restart time is HH:MM (24 h).")
	}
	st.SteamCmd, st.ServersRoot = strings.TrimSpace(st.SteamCmd), strings.TrimSpace(st.ServersRoot)
	st.CacheDir, st.AddonsDir = strings.TrimSpace(st.CacheDir), strings.TrimSpace(st.AddonsDir)
	for _, f := range []struct{ label, path, example string }{
		{"steamcmd.exe", st.SteamCmd, `C:\steamcmd\steamcmd.exe`},
		{"Folder for new servers", st.ServersRoot, `E:\Servers`},
		{"Shared copies of the games", st.CacheDir, `E:\SteamCache`},
		{"Packages folder (Addons)", st.AddonsDir, `C:\SnipeZilla\addons`},
	} {
		if f.path != "" && !filepath.IsAbs(f.path) {
			errs = append(errs, fmt.Sprintf("%s: \"%s\" is not a full path. Give one with its drive (%s), or empty it for the default.", f.label, f.path, f.example))
		}
	}
	if st.ServersRoot != "" {
		if o, ok := s.M.Cfg.ServerAt(st.ServersRoot); ok {
			errs = append(errs, fmt.Sprintf("Folder for new servers: %s is the folder of %s. Give the folder that holds the servers (%s).", st.ServersRoot, o.Name, filepath.Dir(o.InstallDir)))
		}
	}
	if len(errs) > 0 {
		return writeErrs(w, errs)
	}
	s.M.Cfg.Lock()
	old := s.M.Cfg.Listen
	oldOffsite := fmt.Sprint(s.M.Cfg.Backups.Offsite)
	s.M.Cfg.Settings = st
	s.M.Cfg.Unlock()
	if err := s.M.Cfg.Save(); err != nil {
		return err
	}
	// Load again: the limits of the values are applied
	if c, err := config.Load(s.M.Cfg.Path()); err == nil {
		s.M.Cfg.Lock()
		s.M.Cfg.Settings = c.Settings
		s.M.Cfg.Unlock()
	}
	s.M.Steam.Path = s.M.SteamCmdPath()
	_ = os.MkdirAll(s.M.AddonsRoot(), 0o755)
	s.M.Log.Add(events.Manager, "", "", "Settings changed by %s", u.Name)
	if fmt.Sprint(st.Backups.Offsite) != oldOffsite {
		s.M.SyncOffsite() // a new destination gets the backups there are
	}
	return writeJSON(w, map[string]any{"ok": true, "restart_needed": old != st.Listen})
}

func (s *Server) discordTest(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Webhook  string `json:"webhook"`
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	user := strings.TrimSpace(in.Username)
	text := "Test message from SnipeZilla Server Manager, sent by " + u.Name + "."
	if n := strings.TrimSpace(in.Name); n != "" {
		text = "Test message from SnipeZilla Server Manager to \"" + n + "\", sent by " + u.Name + "."
	}
	e := events.Event{Time: time.Now(), Kind: events.Info, Text: text}
	// Shown as a post about the first server the user sees: its names and game icon
	var sv *notify.Server
	for _, v := range s.M.Views() {
		if u.Sees(v.ID) {
			e.Server, e.Name = v.ID, v.Name
			sv = s.M.DiscordServer(v.ID)
			break
		}
	}
	if err := notify.Send(in.Webhook, user, e, sv); err != nil {
		return fail(http.StatusBadGateway, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// forumCheck checks XenForo settings (not saved yet) without posting anything.
func (s *Server) forumCheck(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		URL    string `json:"url"`
		Key    string `json:"key"`
		User   int    `json:"user"`
		Thread int    `json:"thread"`
		Node   int    `json:"node"`
		Icons  string `json:"icons"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if !notify.ValidURL(in.URL) {
		return fail(http.StatusBadRequest, "The forum address starts with https://.")
	}
	if in.Icons != "" && !notify.ValidURL(in.Icons) {
		return fail(http.StatusBadRequest, "The icons' address starts with https://.")
	}
	// the icon of the first game with one, to try
	iconName, icon := "", []byte(nil)
	for _, g := range s.M.Catalog().Sorted() {
		if g.Icon != "" {
			if b := s.iconPNG(g.Icon); len(b) > 0 {
				iconName, icon = g.Icon+".png", b
				break
			}
		}
	}
	msg, err := notify.CheckForum(notify.Forum{URL: in.URL, Key: in.Key, User: in.User, Thread: in.Thread, Node: in.Node, Icons: strings.TrimSpace(in.Icons)}, iconName, icon)
	if err != nil {
		return fail(http.StatusBadGateway, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true, "message": msg})
}

func (s *Server) installSteamCmd(w http.ResponseWriter, r *http.Request, u *User) error {
	go func() {
		if err := s.M.EnsureSteamCmd(); err != nil {
			s.M.Log.Add(events.Error, "", "", "steamcmd: %v", err)
			return
		}
		s.M.Log.Add(events.SteamCmd, "", "", "steamcmd is ready: %s", s.M.Steam.Path)
	}()
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) checkUpdates(w http.ResponseWriter, r *http.Request, u *User) error {
	s.M.CheckUpdates()
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) importOld(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	rep, err := s.M.ImportOld(in.Path, u.Name)
	if err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, rep)
}

/* ---------------------------------------------------------------------------------------------------------------
 * Users
 * ------------------------------------------------------------------------------------------------------------- */

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, u *User) error {
	return writeJSON(w, map[string]any{"users": s.users.all(), "rights": RightNames})
}

func (s *Server) putUser(w http.ResponseWriter, r *http.Request, u *User) error {
	name := r.PathValue("name")
	var in struct {
		Rights   string   `json:"rights"`
		Servers  []string `json:"servers"`
		Password string   `json:"password"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := validName(name); err != nil {
		return err
	}
	rights := ""
	for _, c := range allRights {
		if strings.ContainsRune(in.Rights, c) {
			rights += string(c)
		}
	}
	// Only an admin makes admins, and gives rights they have
	if !u.Can("a") {
		for _, c := range rights {
			if !u.Can(string(c)) {
				return fail(http.StatusForbidden, "You can not give a right you do not have (%c).", c)
			}
		}
	}
	old := s.users.get(name)
	if old != nil && strings.Contains(old.Rights, "a") && !strings.Contains(rights, "a") && s.users.admins() <= 1 {
		return fail(http.StatusConflict, "This is the last admin.")
	}
	if old != nil && strings.Contains(old.Rights, "a") && !u.Can("a") {
		return errForbidden
	}
	if in.Password != "" && len(in.Password) < 8 {
		return fail(http.StatusBadRequest, "The password needs 8 characters or more.")
	}
	if in.Servers == nil {
		in.Servers = []string{}
	}
	if err := s.users.put(User{Name: strings.TrimSpace(name), Rights: rights, Servers: in.Servers}, in.Password); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	if in.Password != "" && old != nil {
		s.sess.dropUser(name, "")
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, u *User) error {
	name := r.PathValue("name")
	old := s.users.get(name)
	if old == nil {
		return fail(http.StatusNotFound, "No such user.")
	}
	if strings.EqualFold(old.Name, u.Name) {
		return fail(http.StatusConflict, "You can not delete yourself.")
	}
	if strings.Contains(old.Rights, "a") && (!u.Can("a") || s.users.admins() <= 1) {
		return fail(http.StatusConflict, "This user is an admin.")
	}
	if err := s.users.remove(name); err != nil {
		return err
	}
	s.sess.dropUser(name, "")
	return writeJSON(w, map[string]any{"ok": true})
}

// viewing counts the web pages open; the hub tells its nodes when the first opens, and 15 s after the last closed
// (a page reloaded is not a change).
func (s *Server) viewing(open bool) {
	if s.hub == nil {
		return
	}
	if open {
		if s.viewers.Add(1) == 1 {
			s.hub.SetWatched(true)
		}
		return
	}
	if s.viewers.Add(-1) == 0 {
		time.AfterFunc(15*time.Second, func() {
			if s.viewers.Load() == 0 {
				s.hub.SetWatched(false)
			}
		})
	}
}
