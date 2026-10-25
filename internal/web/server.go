// Package web is the web interface of the manager: a page (embedded) and the API it calls.
package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SnipeZilla/szm/internal/netcount"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/SnipeZilla/szm/internal/link"
	"github.com/SnipeZilla/szm/internal/manager"
)

//go:embed static
var static embed.FS

// Server is the web interface.
type Server struct {
	viewers atomic.Int32 // web pages open (their live stream): a hub's nodes send their state often only then

	M         *manager.Manager
	users     *Users
	sess      *sessions
	setupCode string
	http      *http.Server
	links     *link.Store  // links.json: the machines linked to this one, or its hub
	hub       *link.Hub    // when other managers are linked to this one
	node      *link.Client // when this one is linked to a hub
}

// New prepares the web interface. When there is no user yet, a setup code is written to setup-code.txt in the
// data folder: whoever creates the first admin from another machine must give it.
func New(m *manager.Manager) (*Server, error) {
	users, err := loadUsers(filepath.Join(m.DataDir, "users.json"))
	if err != nil {
		return nil, fmt.Errorf("users.json: %w", err)
	}
	s := &Server{M: m, users: users, sess: newSessions()}
	if err := s.initLinks(); err != nil {
		return nil, err
	}
	m.GameIcon = s.iconPNG
	_ = os.MkdirAll(s.iconsDir(), 0o755)
	if users.count() == 0 {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		s.setupCode = strings.ToUpper(hex.EncodeToString(b))
		_ = os.WriteFile(filepath.Join(m.DataDir, "setup-code.txt"), []byte(s.setupCode+"\r\n"), 0o600)
	}
	return s, nil
}

// SetupCode is the code to create the first admin ("" once there is one).
func (s *Server) SetupCode() string { return s.setupCode }

// ListenAndServe serves until the context ends.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	s.node.Handler = s.http.Handler // a hub's requests run on this web interface
	go s.node.Run(ctx)
	errc := make(chan error, 1)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() { errc <- s.http.Serve(netcount.Listener(l)) }() // its traffic counted: the manager's own (netcount)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.http.Shutdown(sctx)
		return nil
	}
}

type handler func(w http.ResponseWriter, r *http.Request, u *User) error

// httpError is an error with its HTTP status.
type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func fail(code int, format string, args ...any) error {
	return &httpError{code, fmt.Sprintf(format, args...)}
}

var errForbidden = fail(http.StatusForbidden, "You are not allowed to do this.")

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(static, "static")
	files := http.FileServer(http.FS(sub))
	// The page names its code with the version of the files (app.js?v=…): after an update, no cache on the way (the
	// browser's, IIS's, a CDN's) can give the old code with the new page
	index, _ := static.ReadFile("static/index.html")
	index = bytes.ReplaceAll(index, []byte(`src="app.js"`), []byte(`src="app.js?v=`+uiVersion+`"`))
	index = bytes.ReplaceAll(index, []byte(`href="app.css"`), []byte(`href="app.css?v=`+uiVersion+`"`))
	page := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	}
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			page(w)
			return
		}
		switch ext := strings.ToLower(path.Ext(r.URL.Path)); {
		case r.URL.Path == "/", ext == ".js", ext == ".css", ext == ".html":
			// The page and its code come from szm.exe: after an update, the browser must not keep the old ones
			w.Header().Set("Cache-Control", "no-cache")
		default:
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		files.ServeHTTP(w, r)
	}))

	// Game icons: built in, or added in <data>/icons
	mux.HandleFunc("GET /games/{file}", s.serveIcon)

	// Without a session
	mux.HandleFunc("GET /api/hello", s.hello)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/logout", s.logout)
	s.hub.Routes(mux) // the linked machines (their token)

	api := func(pattern string, right string, h handler) {
		mux.HandleFunc(pattern, s.wrap(right, h))
	}
	api("GET /api/state", "", s.state)
	api("GET /api/stream", "", s.stream)
	api("GET /api/logs", "", s.logs)
	api("POST /api/password", "", s.changePassword)

	api("POST /api/servers/{id}/{op}", "", s.serverOp)
	api("GET /api/servers/{id}/console", "g", s.consoleStream)
	api("POST /api/servers/{id}/rcon", "g", s.rcon)
	api("GET /api/servers/{id}/config", "f", s.serverConfig)
	api("GET /api/servers/{id}/programs", "f", s.programs)
	api("GET /api/servers/{id}/usage", "", s.usage)
	api("POST /api/servers/{id}/workshop", "j", s.workshopAdd)
	api("DELETE /api/servers/{id}/workshop/{wid}", "j", s.workshopRemove)
	api("PUT /api/servers/{id}/workshop/start", "f", s.workshopStart)
	api("PUT /api/servers/{id}/workshop/collection", "f", s.workshopCollection)
	api("GET /api/backups", "j", s.allBackups)
	api("GET /api/backups/zip", "j", s.backupsZip)
	api("POST /api/backups/now", "j", s.backupsNow)
	api("POST /api/backups/offsite", "j", s.offsiteNow)
	api("GET /api/backups/{id}/{name}", "j", s.backupGet)
	api("DELETE /api/backups/{id}/{name}", "j", s.backupDelete)
	api("GET /api/servers/{id}/backups", "j", s.backupList)
	api("POST /api/servers/{id}/backups", "j", s.backupNow)
	api("GET /api/servers/{id}/backups/{name}", "j", s.backupGet)
	api("POST /api/servers/{id}/backups/{name}/restore", "j", s.backupRestore)
	api("DELETE /api/servers/{id}/backups/{name}", "j", s.backupDelete)
	api("GET /api/servers/{id}/crashes", "g", s.crashList)
	api("GET /api/servers/{id}/crashes/{report}", "g", s.crashGet)
	api("GET /api/servers/{id}/crashes/{report}/dump/{n}", "g", s.crashDumpFile)
	api("GET /api/servers/{id}/crashes/{report}/zip", "g", s.crashZip)
	api("DELETE /api/servers/{id}/crashes", "f", s.crashDelete)
	api("PUT /api/servers/{id}", "f", s.saveServer)
	api("DELETE /api/servers/{id}", "c", s.deleteServer)
	api("POST /api/servers", "c", s.addServer)
	api("POST /api/preview", "", s.preview)
	api("GET /api/suggest", "c", s.suggest)
	api("GET /api/browse", "c", s.browse)
	api("POST /api/browse/mkdir", "c", s.browseMkdir)
	api("GET /api/inspect", "c", s.inspect)

	api("GET /api/servers/{id}/files", "j", s.listFiles)
	api("GET /api/servers/{id}/file", "j", s.readFile)
	api("PUT /api/servers/{id}/file", "j", s.writeFile)
	api("DELETE /api/servers/{id}/file", "j", s.deleteFile)
	api("POST /api/servers/{id}/delete", "j", s.deleteMany)
	api("POST /api/servers/{id}/upload", "j", s.upload)
	api("POST /api/servers/{id}/mkdir", "j", s.mkdir)
	api("POST /api/servers/{id}/rename", "j", s.rename)
	api("GET /api/servers/{id}/maps", "", s.maps)
	api("PUT /api/servers/{id}/mapcycle", "j", s.saveMapcycle)
	api("GET /api/servers/{id}/setupfiles", "j", s.setupFiles)
	api("GET /api/servers/{id}/cvars", "j", s.cvars)
	api("POST /api/servers/{id}/steamlogin", "h", s.steamLogin)
	api("PUT /api/servers/{id}/cvars", "j", s.saveCvars)

	api("GET /api/cache", "b", s.cacheInfo)
	api("GET /api/cache/files", "b", s.listFiles)
	api("GET /api/cache/file", "b", s.readFile)
	api("PUT /api/cache/file", "b", s.writeFile)
	api("DELETE /api/cache/file", "b", s.deleteFile)
	api("POST /api/cache/delete", "b", s.deleteMany)
	api("POST /api/cache/upload", "b", s.upload)
	api("POST /api/cache/mkdir", "b", s.mkdir)
	api("POST /api/cache/rename", "b", s.rename)
	api("POST /api/paste", "", s.paste)
	api("POST /api/paste/check", "", s.pasteCheck)
	api("POST /api/paste/pack", "", s.pastePack)
	api("POST /api/paste/unpack", "", s.pasteUnpack)
	api("POST /api/paste/remove", "", s.pasteRemove)
	api("GET /api/paste/jobs/{job}", "", s.pasteJobGet)
	api("GET /api/addons", "", s.addonsList)
	api("POST /api/addons/pkg/{pkg}/{op}", "h", s.addonsOp)
	api("PUT /api/addons/pkg/{pkg}/source", "h", s.addonSource)
	api("DELETE /api/addons/pkg/{pkg}", "h", s.deletePackage)
	api("GET /api/addons/pkg/{pkg}/files", "h", s.listFiles)
	api("GET /api/addons/pkg/{pkg}/file", "h", s.readFile)
	api("PUT /api/addons/pkg/{pkg}/file", "h", s.writeFile)
	api("DELETE /api/addons/pkg/{pkg}/file", "h", s.deleteFile)
	api("POST /api/addons/pkg/{pkg}/delete", "h", s.deleteMany)
	api("GET /api/servers/{id}/addon-folders", "h", s.addonFolders)
	api("GET /api/settings", "b", s.getSettings)
	api("PUT /api/settings", "b", s.putSettings)
	api("POST /api/discord/test", "b", s.discordTest)
	api("POST /api/xenforo/check", "b", s.forumCheck)
	api("GET /api/games", "", s.gamesList)
	api("PUT /api/games/{key}", "b", s.saveGame)
	api("DELETE /api/games/{key}", "b", s.deleteGame)
	api("POST /api/icons", "b", s.uploadIcon)
	api("POST /api/steamcmd", "b", s.installSteamCmd)
	api("GET /api/rclone", "b", s.rcloneInfo)
	api("POST /api/rclone", "b", s.rcloneDownload)
	api("POST /api/offsite/test", "b", s.offsiteTest)
	api("POST /api/updates/check", "h", s.checkUpdates)
	api("POST /api/import", "a", s.importOld)

	api("GET /api/users", "d", s.listUsers)
	api("PUT /api/users/{name}", "d", s.putUser)
	api("DELETE /api/users/{name}", "d", s.deleteUser)

	api("GET /api/machines", "", s.machinesInfo)
	api("PUT /api/machines/name", "a", s.machinesName)
	api("POST /api/machines", "a", s.machinesAdd)
	api("POST /api/machines/join", "a", s.machinesJoin)
	api("DELETE /api/machines/hub", "a", s.machinesUnlink)
	api("POST /api/machines/{node}/code", "a", s.machinesCode)
	api("DELETE /api/machines/{node}", "a", s.machinesRemove)

	return secure(s.remote(mux))
}

// secure adds the headers every answer gets.
func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://images.steamusercontent.com https://steamuserimages-a.akamaihd.net https://*.steamstatic.com; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.ServeHTTP(w, r)
	})
}

const cookieName = "szm_session"

func (s *Server) current(r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	name := s.sess.get(c.Value)
	if name == "" {
		return nil
	}
	return s.users.get(name)
}

// wrap checks the session, the right, and that a change comes from the page (X-SZM header: a cross-site form can
// not send it).
func (s *Server) wrap(right string, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := linkedUser(r) // a request of the hub, through the link
		if u == nil {
			u = s.current(r)
		}
		if u == nil {
			writeErr(w, fail(http.StatusUnauthorized, "Please sign in."))
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-SZM") != "1" {
			writeErr(w, fail(http.StatusForbidden, "Missing X-SZM header."))
			return
		}
		if right != "" && !u.Can(right) {
			writeErr(w, errForbidden)
			return
		}
		if s.M.Debug() && !strings.HasSuffix(r.URL.Path, "/stream") && r.URL.Path != "/api/state" {
			s.M.Note(r.Method + " " + r.URL.Path)
		}
		if id := r.PathValue("id"); id != "" && !u.Sees(id) {
			writeErr(w, fail(http.StatusNotFound, "No such server."))
			return
		}
		if err := h(w, r, u); err != nil {
			writeErr(w, err)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	return json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	var he *httpError
	if errors.As(err, &he) {
		code = he.code
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	if err := dec.Decode(v); err != nil {
		return fail(http.StatusBadRequest, "Bad request: %v", err)
	}
	return nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * Sign in
 * ------------------------------------------------------------------------------------------------------------- */

func (s *Server) hello(w http.ResponseWriter, r *http.Request) {
	u := s.current(r)
	out := map[string]any{"setup": s.users.count() == 0, "version": manager.Version, "ui": uiVersion}
	if s.users.count() == 0 {
		out["local"] = isLocal(r)
	}
	if u != nil {
		out["user"] = s.me(u)
	}
	_ = writeJSON(w, out)
}

func (s *Server) me(u *User) map[string]any {
	rights := u.Rights
	if strings.Contains(rights, "a") {
		rights = allRights
	}
	return map[string]any{"name": u.Name, "rights": rights, "servers": u.Servers}
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, name string, remember bool) {
	tok, ttl := s.sess.create(name, remember)
	c := &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: isHTTPS(r),
	}
	if remember {
		c.MaxAge = int(ttl.Seconds())
	}
	http.SetCookie(w, c)
	s.users.touch(name)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ip := clientIP(r)
	if s.sess.throttled(ip) {
		writeErr(w, fail(http.StatusTooManyRequests, "Too many tries. Wait 10 minutes."))
		return
	}
	u := s.users.get(strings.TrimSpace(in.Name))
	if u == nil || !checkPassword(u.Hash, in.Password) {
		if u == nil {
			checkPassword(hashDummy, in.Password) // same time either way
		}
		s.sess.failed(ip)
		writeErr(w, fail(http.StatusUnauthorized, "Wrong name or password."))
		return
	}
	s.startSession(w, r, u.Name, in.Remember)
	_ = writeJSON(w, map[string]any{"user": s.me(u)})
}

var hashDummy = hashPassword("dummy")

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.users.count() > 0 {
		writeErr(w, fail(http.StatusConflict, "The manager already has an admin."))
		return
	}
	var in struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if !isLocal(r) && !strings.EqualFold(strings.TrimSpace(in.Code), s.setupCode) {
		s.sess.failed(clientIP(r))
		writeErr(w, fail(http.StatusForbidden, "Wrong setup code. It is in setup-code.txt, next to szm.exe."))
		return
	}
	if err := validName(in.Name); err != nil {
		writeErr(w, err)
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, fail(http.StatusBadRequest, "The password needs 8 characters or more."))
		return
	}
	if err := s.users.put(User{Name: strings.TrimSpace(in.Name), Rights: "a"}, in.Password); err != nil {
		writeErr(w, err)
		return
	}
	s.setupCode = ""
	_ = os.Remove(filepath.Join(s.M.DataDir, "setup-code.txt"))
	s.startSession(w, r, strings.TrimSpace(in.Name), false)
	u := s.users.get(in.Name)
	_ = writeJSON(w, map[string]any{"user": s.me(u)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.sess.drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	_ = writeJSON(w, map[string]any{"ok": true})
}

func validName(name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 32 {
		return fail(http.StatusBadRequest, "A name has 2 to 32 characters.")
	}
	for _, c := range name {
		if !(c == '-' || c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return fail(http.StatusBadRequest, "A name has letters, digits, - _ and . only.")
		}
	}
	return nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if !checkPassword(u.Hash, in.Old) {
		return fail(http.StatusForbidden, "The current password is wrong.")
	}
	if len(in.New) < 8 {
		return fail(http.StatusBadRequest, "The password needs 8 characters or more.")
	}
	if err := s.users.put(*u, in.New); err != nil {
		return err
	}
	keep := ""
	if c, err := r.Cookie(cookieName); err == nil {
		keep = c.Value
	}
	s.sess.dropUser(u.Name, keep)
	return writeJSON(w, map[string]any{"ok": true})
}
