package web

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/SnipeZilla/szm/internal/manager"
)

// A server's backups: the list, one made now, downloaded, restored, removed.

func (s *Server) backupList(w http.ResponseWriter, r *http.Request, u *User) error {
	id := r.PathValue("id")
	if s.M.Cfg.Server(id) == nil {
		return fail(http.StatusNotFound, "No such server.")
	}
	s.M.Cfg.RLock()
	b := s.M.Cfg.Backups
	s.M.Cfg.RUnlock()
	return writeJSON(w, map[string]any{"backups": s.M.Backups(id), "dir": s.M.BackupsDir(id), "schedule": b.Schedule, "time": b.Time, "weekday": b.Weekday, "day": b.Day, "keep": b.Keep, "before": b.Before,
		"offsite": s.M.OffsiteStatuses()})
}

func (s *Server) backupNow(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.M.BackupNow(r.PathValue("id"), u.Name); err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// backupGet downloads a backup, or lists its files (?files=1).
func (s *Server) backupGet(w http.ResponseWriter, r *http.Request, u *User) error {
	id, name := r.PathValue("id"), r.PathValue("name")
	if r.URL.Query().Get("files") != "" {
		files, err := s.M.BackupFiles(id, name)
		if err != nil {
			return fail(http.StatusNotFound, "No such backup.")
		}
		return writeJSON(w, map[string]any{"files": files})
	}
	p, err := s.M.BackupPath(id, name)
	if err != nil {
		return fail(http.StatusNotFound, "No such backup.")
	}
	f, err := os.Open(p)
	if err != nil {
		return fail(http.StatusNotFound, "No such backup.")
	}
	defer f.Close()
	st, _ := f.Stat()
	sv := s.M.Cfg.Server(id)
	label := safeFile(id)
	if sv != nil {
		label = safeFile(sv.Name)
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="backup-`+label+`-`+name+`"`)
	http.ServeContent(w, r, name, st.ModTime(), f)
	return nil
}

func (s *Server) backupRestore(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Warn bool `json:"warn"`
	}
	_ = readJSON(r, &in)
	if err := s.M.Restore(r.PathValue("id"), r.PathValue("name"), u.Name, in.Warn); err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) backupDelete(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.M.DeleteBackup(r.PathValue("id"), r.PathValue("name"), u.Name); err != nil {
		return fail(http.StatusBadRequest, "%s.", capitalize(err.Error()))
	}
	return writeJSON(w, map[string]any{"ok": true})
}

// The Backups page: every server's backups (and those of servers removed since), downloaded one by one or together.

type backupGroup struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Game    string           `json:"game,omitempty"`
	Removed bool             `json:"removed,omitempty"` // the server is no longer in the manager
	Backups []manager.Backup `json:"backups"`
}

// backupGroups gives the backups a user sees, by server (the servers in their order, then the removed ones).
func (s *Server) backupGroups(u *User) []backupGroup {
	out := []backupGroup{}
	known := map[string]bool{}
	for _, sv := range s.M.Servers() {
		known[sv.ID] = true
		if !u.Sees(sv.ID) {
			continue
		}
		out = append(out, backupGroup{ID: sv.ID, Name: sv.Name, Game: sv.Game, Backups: s.M.Backups(sv.ID)})
	}
	if u.Can("e") {
		for _, id := range s.M.BackupFolders() {
			if known[id] {
				continue
			}
			if l := s.M.Backups(id); len(l) > 0 {
				out = append(out, backupGroup{ID: id, Name: id, Removed: true, Backups: l})
			}
		}
	}
	return out
}

func (s *Server) allBackups(w http.ResponseWriter, r *http.Request, u *User) error {
	s.M.Cfg.RLock()
	b := s.M.Cfg.Backups
	s.M.Cfg.RUnlock()
	return writeJSON(w, map[string]any{"servers": s.backupGroups(u), "dir": s.M.BackupsRoot(), "schedule": b.Schedule,
		"time": b.Time, "weekday": b.Weekday, "day": b.Day, "keep": b.Keep, "before": b.Before,
		"offsite": s.M.OffsiteStatuses()})
}

// backupsZip gives several backups in one zip, a folder per server: ?all=1 every one the user sees, ?server=<id>
// those of servers, ?b=<id>/<name> the ones picked.
func (s *Server) backupsZip(w http.ResponseWriter, r *http.Request, u *User) error {
	q := r.URL.Query()
	type item struct{ dir, name, path string }
	var items []item
	want := map[string]bool{}
	for _, x := range q["b"] {
		want[x] = true
	}
	servers := map[string]bool{}
	for _, x := range q["server"] {
		servers[x] = true
	}
	all := q.Get("all") != ""
	for _, g := range s.backupGroups(u) {
		dir := safeFile(g.Name)
		if dir == "" || g.Removed {
			dir = safeFile(g.ID)
		}
		if g.Removed {
			dir = "removed-" + dir
		}
		for _, b := range g.Backups {
			if !all && !servers[g.ID] && !want[g.ID+"/"+b.Name] {
				continue
			}
			p, err := s.M.BackupPath(g.ID, b.Name)
			if err != nil {
				continue
			}
			items = append(items, item{dir, b.Name, p})
		}
	}
	if len(items) == 0 {
		return fail(http.StatusNotFound, "No backup to download.")
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="szm-backups-`+time.Now().Format("20060102-1504")+`.zip"`)
	zw := zip.NewWriter(w)
	for _, it := range items {
		f, err := os.Open(it.path)
		if err != nil {
			continue
		}
		st, _ := f.Stat()
		// the backups are zips already: stored as they are
		hd := &zip.FileHeader{Name: it.dir + "/" + it.name, Method: zip.Store}
		if st != nil {
			hd.Modified = st.ModTime()
		}
		dst, err := zw.CreateHeader(hd)
		if err == nil {
			_, err = io.Copy(dst, f)
		}
		f.Close()
		if err != nil {
			return nil // the download was cut: nothing more to say to it
		}
	}
	_ = zw.Close()
	return nil
}

// backupsNow backs up several servers now (all those the user sees, without ids).
func (s *Server) backupsNow(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		IDs []string `json:"ids"`
	}
	_ = readJSON(r, &in)
	pick := map[string]bool{}
	for _, id := range in.IDs {
		pick[id] = true
	}
	started, skipped := 0, []string{}
	for _, sv := range s.M.Servers() {
		if !u.Sees(sv.ID) || (len(pick) > 0 && !pick[sv.ID]) {
			continue
		}
		if err := s.M.BackupNow(sv.ID, u.Name); err != nil {
			skipped = append(skipped, sv.Name+": "+err.Error())
			continue
		}
		started++
	}
	return writeJSON(w, map[string]any{"started": started, "skipped": skipped})
}

// offsiteNow copies the backups to the off-site destinations now (those not there yet).
func (s *Server) offsiteNow(w http.ResponseWriter, r *http.Request, u *User) error {
	if len(s.M.OffsiteStatuses()) == 0 {
		return fail(http.StatusBadRequest, "No off-site destination: add one in Settings, Backups.")
	}
	s.M.SyncOffsite()
	return writeJSON(w, map[string]any{"ok": true})
}
