package web

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SnipeZilla/szm/internal/events"
)

// Copy (or cut) and paste in the file manager: from a folder of a server, an addon package or the shared copies, into
// another (or the same, the copies then named "… - copy").

// placeRef names a file manager's folder: a server's ("server", its ID), an addon package's ("pkg", its name), or
// the shared copies ("cache").
type placeRef struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Machine string `json:"machine,omitempty"` // on a hub: the linked machine of a package or of the shared copies
}

type place struct {
	root, label, server string // server: its ID (its size is measured again)
}

// placeOf gives the folder of a reference, when the user may change it.
func (s *Server) placeOf(p placeRef, u *User) (place, error) {
	switch p.Kind {
	case "server":
		sv := s.M.Cfg.Server(p.ID)
		if sv == nil || !u.Sees(p.ID) {
			return place{}, fail(http.StatusNotFound, "No such server.")
		}
		if !u.Can("j") {
			return place{}, errForbidden
		}
		s.M.Cfg.RLock()
		pl := place{root: sv.InstallDir, label: sv.Name, server: sv.ID}
		s.M.Cfg.RUnlock()
		return pl, nil
	case "pkg":
		if !u.Can("h") {
			return place{}, errForbidden
		}
		dir, err := s.M.PackagePath(p.ID)
		if err != nil {
			return place{}, fail(http.StatusBadRequest, "%s", capitalize(err.Error()))
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return place{}, fail(http.StatusNotFound, "No such package.")
		}
		return place{root: dir, label: "addon package " + p.ID}, nil
	case "cache":
		if !u.Can("b") {
			return place{}, errForbidden
		}
		return place{root: s.M.CacheDir(), label: "the shared copies"}, nil
	}
	return place{}, fail(http.StatusBadRequest, "Bad place.")
}

func (s *Server) paste(w http.ResponseWriter, r *http.Request, u *User) error {
	var in pasteReq
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if done, err := s.pasteAcross(w, r, u, in); done || err != nil {
		return err // a linked machine's
	}
	from, err := s.placeOf(in.From, u)
	if err != nil {
		return err
	}
	to, err := s.placeOf(in.To, u)
	if err != nil {
		return err
	}
	if len(in.Names) == 0 {
		return fail(http.StatusBadRequest, "Nothing to paste.")
	}
	srcDir, err := jail(from.root, in.Dir)
	if err != nil {
		return err
	}
	dstDir, err := jail(to.root, in.Dest)
	if err != nil {
		return err
	}
	if st, err := os.Stat(dstDir); err != nil || !st.IsDir() {
		return fail(http.StatusNotFound, "The folder to paste into is not there.")
	}
	type job struct{ src, dst, name string }
	var jobs []job
	var conflicts []string
	same := samePath(srcDir, dstDir)
	for _, n := range in.Names {
		if n == "" || n == "." || n == ".." || strings.ContainsAny(n, `/\`) {
			return fail(http.StatusBadRequest, "Bad name: %s.", n)
		}
		src := filepath.Join(srcDir, n)
		if _, err := os.Lstat(src); err != nil {
			return fail(http.StatusNotFound, "%s is no longer there.", n)
		}
		dst := filepath.Join(dstDir, n)
		if inside(src, dst) && !samePath(src, dst) {
			return fail(http.StatusBadRequest, "%s can not go into itself.", n)
		}
		if same {
			if in.Cut {
				continue // moved where it is: nothing to do
			}
			dst = freeName(dstDir, n, " - copy")
		} else if _, err := os.Lstat(dst); err == nil {
			switch in.Conflict {
			case "overwrite":
			case "skip":
				continue
			case "rename":
				dst = freeName(dstDir, n, "")
			default:
				conflicts = append(conflicts, n)
			}
		}
		jobs = append(jobs, job{src, dst, n})
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return writeJSON(w, map[string]any{"conflicts": conflicts})
	}
	files, failed := 0, []string{}
	var bytes int64
	for _, j := range jobs {
		if in.Cut && !pathExists(j.dst) && os.Rename(j.src, j.dst) == nil {
			files++ // same disk: moved at once
			continue
		}
		n, b, err := copyTree(j.src, j.dst)
		files, bytes = files+n, bytes+b
		if err != nil {
			failed = append(failed, j.name+": "+err.Error())
			continue
		}
		if in.Cut {
			if err := os.RemoveAll(j.src); err != nil {
				failed = append(failed, j.name+" (copied, not removed): "+err.Error())
			}
		}
	}
	verb := "copied"
	if in.Cut {
		verb = "moved"
	}
	what := fmt.Sprintf("%d item%s", len(jobs), map[bool]string{true: "", false: "s"}[len(jobs) == 1])
	where := relOf(to.root, dstDir)
	if where == "" {
		where = "its folder"
	}
	srcWhere := from.label
	if samePlace(from, to) {
		srcWhere = relOf(from.root, srcDir)
		if srcWhere == "" {
			srcWhere = "its folder"
		}
	}
	if len(jobs) > 0 {
		s.M.Log.Add(events.Info, to.server, labelOr(to), "%s %s %s from %s to %s", u.Name, verb, what, srcWhere, where)
	}
	if from.server != "" && in.Cut {
		s.M.ForgetSize(from.server)
	}
	if to.server != "" {
		s.M.ForgetSize(to.server)
	}
	return writeJSON(w, map[string]any{"items": len(jobs), "files": files, "bytes": bytes, "failed": failed})
}

func labelOr(p place) string {
	if p.server != "" {
		return p.label
	}
	return ""
}

func samePlace(a, b place) bool { return samePath(a.root, b.root) }

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// inside says whether p is dir or in it.
func inside(dir, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	if err != nil {
		return false
	}
	if os.PathSeparator == '\\' {
		rel2, err := filepath.Rel(strings.ToLower(filepath.Clean(dir)), strings.ToLower(filepath.Clean(p)))
		if err == nil {
			rel = rel2
		}
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func pathExists(p string) bool { _, err := os.Lstat(p); return err == nil }

// freeName gives a name not taken in a folder: "server.cfg" → "server - copy.cfg", "server - copy (2).cfg"…; with no
// suffix: "server (2).cfg".
func freeName(dir, name, suffix string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if ext == name { // ".env"
		base, ext = name, ""
	}
	for k := 1; ; k++ {
		var n string
		switch {
		case suffix == "":
			n = fmt.Sprintf("%s (%d)%s", base, k+1, ext)
		case k == 1:
			n = base + suffix + ext
		default:
			n = fmt.Sprintf("%s%s (%d)%s", base, suffix, k, ext)
		}
		if p := filepath.Join(dir, n); !pathExists(p) {
			return p
		}
	}
}

// copyTree copies a file or a folder (and all in it); over what is there. It gives the files copied and their size.
func copyTree(src, dst string) (int, int64, error) {
	st, err := os.Lstat(src)
	if err != nil {
		return 0, 0, err
	}
	if !st.IsDir() {
		n, err := copyOne(src, dst, st)
		if err != nil {
			return 0, 0, err
		}
		return 1, n, nil
	}
	files := 0
	var total int64
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil // links and the like: left
		}
		n, err := copyOne(p, target, info)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		files++
		total += n
		return nil
	})
	return files, total, err
}

func copyOne(src, dst string, st fs.FileInfo) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".szm-part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, dst)
		if err != nil && pathExists(dst) {
			// Windows: a file in use is not replaced
			err = fmt.Errorf("in use (stop the server first?): %w", err)
		}
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	_ = os.Chtimes(dst, st.ModTime(), st.ModTime())
	return n, nil
}
