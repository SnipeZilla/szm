package web

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/kv"
)

// The file manager: everything stays inside the server's folder.

func (s *Server) serverDir(r *http.Request) (config.Server, *games.Game, error) {
	sv := s.M.Cfg.Server(r.PathValue("id"))
	if sv == nil {
		return config.Server{}, nil, fail(http.StatusNotFound, "No such server.")
	}
	s.M.Cfg.RLock()
	cp := *sv
	s.M.Cfg.RUnlock()
	return cp, s.M.Catalog()[cp.Game], nil
}

// fileRoot is the folder the file manager works in: a server's, or an addon package's.
type fileRoot struct {
	InstallDir, ID, Name string
}

func (s *Server) fileRoot(r *http.Request) (fileRoot, error) {
	if strings.HasPrefix(r.URL.Path, "/api/cache/") {
		dir := s.M.CacheDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fileRoot{}, fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
		}
		return fileRoot{InstallDir: dir, Name: "shared copies"}, nil
	}
	if pkg := r.PathValue("pkg"); pkg != "" {
		p, err := s.M.PackagePath(pkg)
		if err != nil {
			return fileRoot{}, fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return fileRoot{}, fail(http.StatusNotFound, "No such package.")
		}
		return fileRoot{InstallDir: p, Name: "addon package " + pkg}, nil
	}
	sv, _, err := s.serverDir(r)
	if err != nil {
		return fileRoot{}, err
	}
	return fileRoot{InstallDir: sv.InstallDir, ID: sv.ID, Name: sv.Name}, nil
}

// jail gives the full path of a path inside a folder; it never leaves the folder.
func jail(root, rel string) (string, error) {
	rel = strings.ReplaceAll(rel, `\`, "/")
	if strings.Contains(rel, ":") || strings.ContainsRune(rel, 0) {
		return "", fail(http.StatusBadRequest, "Bad path.")
	}
	clean := filepath.Clean("/" + rel) // "/../.." becomes "/"
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func relOf(root, full string) string {
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

type fileEntry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Text  bool   `json:"text"` // can be opened in the editor
}

var textExt = map[string]bool{
	".cfg": true, ".txt": true, ".ini": true, ".log": true, ".json": true, ".vdf": true, ".res": true, ".gi": true,
	".lst": true, ".xml": true, ".yml": true, ".yaml": true, ".sp": true, ".inc": true, ".smx.txt": true, ".kv": true,
	".properties": true, ".rc": true, ".bat": true, ".cmd": true, ".sh": true, ".md": true, ".csv": true,
	".phrases.txt": true, ".db.txt": true, ".conf": true, ".toml": true, ".lua": true, ".nut": true, ".js": true,
	".htm": true, ".html": true, ".css": true, ".inf": true, ".mapcycle": true, ".wiki": true, ".rad": true,
}

func isTextName(name string) bool {
	return textExt[strings.ToLower(filepath.Ext(name))] || !strings.Contains(name, ".")
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	dir, err := jail(sv.InstallDir, r.URL.Query().Get("path"))
	if err != nil {
		return err
	}
	list, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fail(http.StatusNotFound, "This folder does not exist (is the server installed?).")
		}
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	out := []fileEntry{}
	for _, e := range list {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fe := fileEntry{Name: e.Name(), Dir: e.IsDir(), MTime: info.ModTime().Unix()}
		if !e.IsDir() {
			fe.Size = info.Size()
			fe.Text = isTextName(e.Name()) && info.Size() <= 4<<20
		}
		out = append(out, fe)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	res := map[string]any{"path": relOf(sv.InstallDir, dir), "entries": out, "root": sv.InstallDir}
	if strings.EqualFold(filepath.Base(dir), "logs") && dir != filepath.Clean(sv.InstallDir) {
		// a logs folder: shown newest first, and its old files can be deleted by themselves
		s.M.Cfg.RLock()
		res["logs_days"] = s.M.Cfg.Logs.ServerDays
		s.M.Cfg.RUnlock()
		res["logs"] = r.PathValue("id") != ""
	}
	return writeJSON(w, res)
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	p, err := jail(sv.InstallDir, r.URL.Query().Get("path"))
	if err != nil {
		return err
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return fail(http.StatusNotFound, "No such file.")
	}
	if r.URL.Query().Get("download") == "1" {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(p)}))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, filepath.Base(p), st.ModTime(), f)
		return nil
	}
	if st.Size() > 4<<20 {
		return fail(http.StatusRequestEntityTooLarge, "Too big to edit here (%d MB): download it.", st.Size()>>20)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if bytes.IndexByte(b[:min(len(b), 8192)], 0) >= 0 {
		return fail(http.StatusUnsupportedMediaType, "This is not a text file: download it.")
	}
	crlf := bytes.Contains(b, []byte("\r\n"))
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	return writeJSON(w, map[string]any{"path": relOf(sv.InstallDir, p), "content": text, "crlf": crlf, "mtime": st.ModTime().UnixMilli()})
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		CRLF    bool   `json:"crlf"`
		MTime   int64  `json:"mtime"` // the version the editor opened: refused when the file changed since
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	p, err := jail(sv.InstallDir, in.Path)
	if err != nil || p == filepath.Clean(sv.InstallDir) {
		return fail(http.StatusBadRequest, "Bad path.")
	}
	if st, err := os.Stat(p); err == nil && in.MTime > 0 && st.ModTime().UnixMilli() != in.MTime {
		return fail(http.StatusConflict, "The file changed on the disk since you opened it. Open it again.")
	}
	text := strings.ReplaceAll(in.Content, "\r\n", "\n")
	if in.CRLF {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := config.WriteFileAtomic(p, []byte(text), 0o644); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	st, _ := os.Stat(p)
	s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s saved %s", u.Name, relOf(sv.InstallDir, p))
	return writeJSON(w, map[string]any{"ok": true, "mtime": st.ModTime().UnixMilli()})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	p, err := jail(sv.InstallDir, r.URL.Query().Get("path"))
	if err != nil || p == filepath.Clean(sv.InstallDir) {
		return fail(http.StatusBadRequest, "Bad path.")
	}
	if err := os.RemoveAll(p); err != nil {
		return fail(http.StatusConflict, "%s", capitalize(err.Error()))
	}
	s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s deleted %s", u.Name, relOf(sv.InstallDir, p))
	if id := r.PathValue("id"); id != "" {
		s.M.ForgetSize(id)
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// deleteMany deletes several files or folders at once (the ones ticked in the file manager).
func (s *Server) deleteMany(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(req.Paths) == 0 {
		return fail(http.StatusBadRequest, "Nothing to delete.")
	}
	n, where := 0, ""
	var failed []string
	for _, rel := range req.Paths {
		p, err := jail(sv.InstallDir, rel)
		if err != nil || p == filepath.Clean(sv.InstallDir) {
			failed = append(failed, rel+": bad path")
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			failed = append(failed, filepath.Base(p)+": "+err.Error())
			continue
		}
		n++
		where = relOf(sv.InstallDir, filepath.Dir(p))
	}
	if n > 0 {
		in := ""
		if where != "" {
			in = " in " + where
		}
		s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s deleted %d file%s%s", u.Name, n, map[bool]string{true: "", false: "s"}[n == 1], in)
		if id := r.PathValue("id"); id != "" {
			s.M.ForgetSize(id)
		}
	}
	return writeJSON(w, map[string]any{"deleted": n, "failed": failed})
}

func (s *Server) mkdir(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	var in struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	p, err := jail(sv.InstallDir, in.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) rename(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	from, err1 := jail(sv.InstallDir, in.From)
	to, err2 := jail(sv.InstallDir, in.To)
	root := filepath.Clean(sv.InstallDir)
	if err1 != nil || err2 != nil || from == root || to == root {
		return fail(http.StatusBadRequest, "Bad path.")
	}
	if _, err := os.Stat(to); err == nil {
		return fail(http.StatusConflict, "%s already exists.", filepath.Base(to))
	}
	if err := os.Rename(from, to); err != nil {
		return fail(http.StatusConflict, "%s", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// upload saves the files of a form into a folder of the server (streamed: no size limit but the disk).
func (s *Server) upload(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, err := s.fileRoot(r)
	if err != nil {
		return err
	}
	dir, err := jail(sv.InstallDir, r.URL.Query().Get("path"))
	if err != nil {
		return err
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return fail(http.StatusBadRequest, "Bad upload.")
	}
	var names []string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(http.StatusBadRequest, "Upload: %v", err)
		}
		name := filepath.Base(strings.ReplaceAll(part.FileName(), `\`, "/"))
		if part.FormName() != "file" || name == "" || name == "." || name == "/" {
			part.Close()
			continue
		}
		target, err := jail(sv.InstallDir, relOf(sv.InstallDir, filepath.Join(dir, name)))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		tmp := target + ".szm-upload"
		f, err := os.Create(tmp)
		if err != nil {
			return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
		}
		_, cerr := io.Copy(f, part)
		f.Close()
		part.Close()
		if cerr != nil {
			os.Remove(tmp)
			return fail(http.StatusBadRequest, "Upload of %s: %v", name, cerr)
		}
		if err := os.Rename(tmp, target); err != nil {
			os.Remove(tmp)
			return fail(http.StatusConflict, "%s", capitalize(err.Error()))
		}
		names = append(names, name)
	}
	if len(names) > 0 {
		s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s uploaded %s into /%s", u.Name, strings.Join(names, ", "), relOf(sv.InstallDir, dir))
	}
	if id := r.PathValue("id"); id != "" {
		s.M.ForgetSize(id)
	}
	return writeJSON(w, map[string]any{"ok": true, "files": names})
}

/* ---------------------------------------------------------------------------------------------------------------
 * Maps and map cycle
 * ------------------------------------------------------------------------------------------------------------- */

func mapcyclePath(sv config.Server, g *games.Game) (string, bool) {
	if g.Engine == games.Source2 || g.IsOther() {
		return "", false // CS2: map groups, in gamemodes_server.txt (gameModesPath)
	}
	cands := []string{g.GameDir + "/cfg/mapcycle.txt", g.GameDir + "/mapcycle.txt"}
	if g.Engine == games.GoldSrc {
		cands = []string{g.GameDir + "/mapcycle.txt"}
	}
	for _, c := range cands {
		if p := g.Path(sv.InstallDir, c); fileExists(p) {
			return c, true
		}
	}
	return cands[0], true
}

// gameModesPath: CS2's map groups (the maps it rotates, and the vote at the end of a match offers), with the
// group a server plays: +mapgroup of its launch options, mg_custom when it has none yet.
func gameModesPath(g *games.Game) string { return g.GameDir + "/gamemodes_server.txt" }

// (or, when it has none, the first of its file: the one a save then has it play)
func mapGroupOf(sv config.Server, names []string) string {
	if v := config.ArgValue(sv.Args, "+mapgroup"); v != "" {
		return v
	}
	if len(names) > 0 {
		return names[0]
	}
	return "mg_custom"
}

var mapGroupRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// cvarKeys are the server.cfg settings the Set up page asks for, in plain words.
var cvarKeys = []string{"hostname", "sv_password"}

func cvarRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?im)^[ \t]*` + regexp.QuoteMeta(key) + `[ \t]+("[^"\r\n]*"|[^\r\n]*?)[ \t]*(//[^\r\n]*)?\r?$`)
}

// cvars reads the settings of server.cfg the Set up page shows (hostname, join password).
func (s *Server) cvars(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, g, err := s.serverDir(r)
	if err != nil {
		return err
	}
	if g == nil {
		return fail(http.StatusBadRequest, "Unknown game.")
	}
	if g.IsOther() {
		return fail(http.StatusBadRequest, "This game has no server.cfg: its name and password are its settings.")
	}
	b, _ := os.ReadFile(g.Path(sv.InstallDir, g.Config))
	out := map[string]string{}
	for _, k := range cvarKeys {
		out[k] = ""
		if m := cvarRe(k).FindSubmatch(b); m != nil {
			out[k] = strings.Trim(strings.TrimSpace(string(m[1])), `"`)
		}
	}
	return writeJSON(w, map[string]any{"cvars": out, "path": g.Config})
}

// saveCvars writes those settings into server.cfg: the line changed, or added at the end.
func (s *Server) saveCvars(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, g, err := s.serverDir(r)
	if err != nil {
		return err
	}
	if g == nil {
		return fail(http.StatusBadRequest, "Unknown game.")
	}
	if g.IsOther() {
		return fail(http.StatusBadRequest, "This game has no server.cfg: its name and password are its settings.")
	}
	var in map[string]string
	if err := readJSON(r, &in); err != nil {
		return err
	}
	path := g.Path(sv.InstallDir, g.Config)
	b, _ := os.ReadFile(path)
	text := string(b)
	nl := "\r\n"
	if text != "" && !strings.Contains(text, "\r\n") {
		nl = "\n"
	}
	changed := false
	for _, k := range cvarKeys {
		v, ok := in[k]
		if !ok {
			continue
		}
		v = strings.TrimSpace(strings.NewReplacer(`"`, "'", "\r", " ", "\n", " ").Replace(v))
		line := fmt.Sprintf(`%s "%s"`, k, v)
		re := cvarRe(k)
		if loc := re.FindStringIndex(text); loc != nil {
			if strings.HasSuffix(text[loc[0]:loc[1]], "\r") {
				line += "\r" // the line end stays as it was
			}
			if text[loc[0]:loc[1]] != line {
				text = text[:loc[0]] + line + text[loc[1]:]
				changed = true
			}
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += nl
		}
		text += line + nl
		changed = true
	}
	if changed {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := config.WriteFileAtomic(path, []byte(text), 0o644); err != nil {
			return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
		}
		s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s changed %s", u.Name, g.Config)
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// setupFiles lists the files a new server is set up with: its config, map cycle, message of the day...
func (s *Server) setupFiles(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, g, err := s.serverDir(r)
	if err != nil {
		return err
	}
	if g == nil {
		return fail(http.StatusBadRequest, "Unknown game.")
	}
	type file struct {
		Label  string `json:"label"`
		Path   string `json:"path"` // inside the server's folder
		Hint   string `json:"hint"`
		Exists bool   `json:"exists"`
	}
	var out []file
	add := func(label, rel, hint string) {
		for _, f := range out {
			if f.Path == rel {
				return
			}
		}
		out = append(out, file{label, rel, hint, fileExists(g.Path(sv.InstallDir, rel))})
	}
	if g.IsOther() {
		// its own config files: the one its entry names (its name, ports and passwords come from its command line)
		if g.Config != "" {
			add("Server settings", g.Config, "its own settings (the name, ports and passwords of its command line win)")
		}
		return writeJSON(w, map[string]any{"files": out})
	}
	add("Server settings", g.Config, "hostname, rcon_password, sv_password, game rules…")
	switch g.Engine {
	case games.Source2:
		add("Startup commands", g.GameDir+"/cfg/autoexec.cfg", "run once when the server starts")
		add("Map groups", gameModesPath(g), "the maps it rotates and votes on (the Maps tab edits its map group)")
	case games.Source:
		add("Startup commands", g.GameDir+"/cfg/autoexec.cfg", "run once when the server starts")
		add("Message of the day", g.GameDir+"/motd.txt", "shown to players when they join")
	default:
		add("Message of the day", g.GameDir+"/motd.txt", "shown to players when they join")
	}
	if rel, ok := mapcyclePath(sv, g); ok {
		add("Map cycle", rel, "the maps, in order (the Maps tab edits it too)")
	}
	return writeJSON(w, map[string]any{"files": out})
}

func (s *Server) maps(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, g, err := s.serverDir(r)
	if err != nil {
		return err
	}
	if g == nil {
		return fail(http.StatusBadRequest, "Unknown game.")
	}
	found := map[string]bool{}
	for _, d := range []string{g.MapsDir, g.GameDir + "/download/maps"} {
		list, _ := os.ReadDir(g.Path(sv.InstallDir, d))
		for _, e := range list {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			for _, x := range g.MapExt {
				if ext == x {
					name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
					// CS2 keeps other .vpk next to the maps (their parts): only whole maps
					if g.Engine == games.Source2 && (strings.Contains(name, "_vanity") || strings.HasSuffix(name, "_dir") || isPart(name)) {
						continue
					}
					found[name] = true
				}
			}
		}
	}
	list := make([]string, 0, len(found))
	for k := range found {
		list = append(list, k)
	}
	sort.Strings(list)
	out := map[string]any{"maps": list, "cycle_supported": false, "current": ""}
	if g.Engine == games.Source2 {
		ws := sv.Workshop
		if ws == nil {
			ws = []config.WorkshopMap{}
		}
		out["workshop"], out["workshop_map"], out["workshop_collection"], out["workshop_collection_title"] = ws, sv.WorkshopStart, sv.WorkshopColl, sv.WorkshopCollFor
	}
	if g.Engine == games.Source2 {
		rel := gameModesPath(g)
		b, _ := os.ReadFile(g.Path(sv.InstallDir, rel))
		names, groups := kv.MapGroups(string(b))
		cur := mapGroupOf(sv, names)
		for _, n := range names {
			if strings.EqualFold(n, cur) {
				cur = n
			}
		}
		cycle := groups[cur]
		if cycle == nil {
			cycle = []string{}
		}
		if names == nil {
			names = []string{}
		}
		out["cycle_supported"], out["cycle_path"], out["cycle"] = true, rel, cycle
		out["map_group"], out["map_group_set"], out["map_groups"], out["group_maps"] = cur, config.ArgValue(sv.Args, "+mapgroup") != "", names, groups
	}
	if rel, ok := mapcyclePath(sv, g); ok {
		out["cycle_supported"] = true
		out["cycle_path"] = rel
		var cycle []string
		if b, err := os.ReadFile(g.Path(sv.InstallDir, rel)); err == nil {
			for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
				l = strings.TrimSpace(l)
				if l != "" && !strings.HasPrefix(l, "//") {
					cycle = append(cycle, l)
				}
			}
		}
		out["cycle"] = cycle
	}
	if v, ok := s.M.View(sv.ID); ok && v.Info != nil {
		out["current"] = v.Info.Map
	}
	// the maps its console said it does not have, with when
	gone := map[string]int64{}
	for k, t := range s.M.NoSuchMaps(sv.ID) {
		gone[k] = t.Unix()
	}
	out["not_found"] = gone
	return writeJSON(w, out)
}

// isPart: "de_foo_001" style pieces of a map split in several .vpk
func isPart(name string) bool {
	i := strings.LastIndexByte(name, '_')
	if i < 0 || len(name)-i-1 != 3 {
		return false
	}
	for _, c := range name[i+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Server) saveMapcycle(w http.ResponseWriter, r *http.Request, u *User) error {
	sv, g, err := s.serverDir(r)
	if err != nil {
		return err
	}
	if g == nil {
		return fail(http.StatusBadRequest, "Unknown game.")
	}
	var in struct {
		Maps  []string `json:"maps"`
		Group string   `json:"group"` // CS2: the map group
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if g.Engine == games.Source2 {
		return s.saveMapGroup(w, sv, g, u, in.Group, in.Maps)
	}
	rel, ok := mapcyclePath(sv, g)
	if !ok {
		return fail(http.StatusBadRequest, "This game has no mapcycle.txt.")
	}
	var b strings.Builder
	for _, m := range in.Maps {
		m = strings.TrimSpace(m)
		if m == "" || strings.ContainsAny(m, " \t\r\n\"") {
			continue
		}
		b.WriteString(m + "\r\n")
	}
	p := g.Path(sv.InstallDir, rel)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := config.WriteFileAtomic(p, []byte(b.String()), 0o644); err != nil {
		return err
	}
	s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s changed the map cycle (%d maps)", u.Name, len(in.Maps))
	return writeJSON(w, map[string]any{"ok": true, "saved": time.Now().Unix()})
}

// saveMapGroup writes a CS2 server's map group into its gamemodes_server.txt (the rest of the file stays as it was),
// and has the server play it (+mapgroup) from its next start.
func (s *Server) saveMapGroup(w http.ResponseWriter, sv config.Server, g *games.Game, u *User, group string, maps []string) error {
	group = strings.TrimSpace(group)
	if group == "" {
		group = mapGroupOf(sv, nil)
	}
	if !mapGroupRe.MatchString(group) {
		return fail(http.StatusBadRequest, "A map group name has letters, digits and _ only (mg_custom).")
	}
	var list []string
	for _, m := range maps {
		if m = strings.TrimSpace(m); m != "" && !strings.ContainsAny(m, " \t\r\n\"{}") {
			list = append(list, m)
		}
	}
	rel := gameModesPath(g)
	p := g.Path(sv.InstallDir, rel)
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := config.WriteFileAtomic(p, []byte(kv.SetMapGroup(string(b), group, list)), 0o644); err != nil {
		return fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
	}
	s.M.Log.Add(events.Info, sv.ID, sv.Name, "%s changed the map group %s (%d maps)", u.Name, group, len(list))
	changed, err := s.M.SetMapGroup(sv.ID, group, u.Name)
	if err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true, "saved": time.Now().Unix(), "group": group, "args_changed": changed})
}
