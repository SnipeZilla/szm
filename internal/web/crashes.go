package web

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/manager"
)

// A server's crash reports: the list, one report (JSON, or as text to save), and removing them.

func (s *Server) crashList(w http.ResponseWriter, r *http.Request, u *User) error {
	if s.M.Cfg.Server(r.PathValue("id")) == nil {
		return fail(http.StatusNotFound, "No such server.")
	}
	list := s.M.Crashes(r.PathValue("id"))
	if list == nil {
		list = []manager.CrashSummary{}
	}
	return writeJSON(w, map[string]any{"crashes": list})
}

func (s *Server) crashGet(w http.ResponseWriter, r *http.Request, u *User) error {
	rep, err := s.M.CrashView(r.PathValue("id"), r.PathValue("report"))
	if err != nil {
		return fail(http.StatusNotFound, "No such crash report.")
	}
	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="crash-`+safeFile(rep.Name)+`-`+rep.ID+`.txt"`)
		_, _ = w.Write([]byte(rep.Text()))
		return nil
	}
	return writeJSON(w, rep)
}

func (s *Server) crashDelete(w http.ResponseWriter, r *http.Request, u *User) error {
	sv := s.M.Cfg.Server(r.PathValue("id"))
	if sv == nil {
		return fail(http.StatusNotFound, "No such server.")
	}
	if err := s.M.DeleteCrashes(r.PathValue("id")); err != nil {
		return fail(http.StatusInternalServerError, "%s", capitalize(err.Error()))
	}
	s.M.Log.Add(events.Manager, sv.ID, sv.Name, "Crash reports removed by %s", u.Name)
	return writeJSON(w, map[string]any{"ok": true})
}

// safeFile keeps the letters, digits, - and _ of a name, for a file name.
func safeFile(n string) string {
	out := make([]rune, 0, len(n))
	for _, c := range n {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		case c == ' ':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "server"
	}
	return string(out)
}

// crashDumpFile downloads a crash dump of a report (for a plugin's author, or WinDbg).
func (s *Server) crashDumpFile(w http.ResponseWriter, r *http.Request, u *User) error {
	rep, err := s.M.Crash(r.PathValue("id"), r.PathValue("report"))
	if err != nil {
		return fail(http.StatusNotFound, "No such crash report.")
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 || n >= len(rep.Dumps) {
		return fail(http.StatusNotFound, "No such crash dump.")
	}
	p := rep.Dumps[n].Path
	f, err := os.Open(p)
	if err != nil {
		return fail(http.StatusNotFound, "The crash dump is no longer there (Windows keeps the last 30 of each program).")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return fail(http.StatusNotFound, "No such crash dump.")
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeFile(rep.Name)+`-`+rep.ID+filepath.Ext(p)+`"`)
	http.ServeContent(w, r, filepath.Base(p), st.ModTime(), f)
	return nil
}

// crashZip downloads a report with all it has, for a bug report: the report as text and JSON, and its crash dumps.
func (s *Server) crashZip(w http.ResponseWriter, r *http.Request, u *User) error {
	rep, err := s.M.CrashView(r.PathValue("id"), r.PathValue("report"))
	if err != nil {
		return fail(http.StatusNotFound, "No such crash report.")
	}
	name := "crash-" + safeFile(rep.Name) + "-" + rep.ID
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.zip"`)
	z := zip.NewWriter(w)
	add := func(n string, mod time.Time) (io.Writer, error) {
		return z.CreateHeader(&zip.FileHeader{Name: name + "/" + n, Method: zip.Deflate, Modified: mod})
	}
	if f, err := add("report.txt", rep.Time); err == nil {
		_, _ = io.WriteString(f, strings.ReplaceAll(rep.Text(), "\n", "\r\n"))
	}
	if f, err := add("report.json", rep.Time); err == nil {
		e := json.NewEncoder(f)
		e.SetIndent("", "  ")
		_ = e.Encode(rep.CrashReport)
	}
	for _, d := range rep.Dumps {
		src, err := os.Open(d.Path)
		if err != nil {
			continue // gone: Windows keeps the last ones only
		}
		if st, err := src.Stat(); err == nil && !st.IsDir() {
			if f, err := add(filepath.Base(d.Path), st.ModTime()); err == nil {
				_, _ = io.Copy(f, src)
			}
		}
		src.Close()
	}
	return z.Close()
}
