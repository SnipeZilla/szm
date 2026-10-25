package web

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/link"
)

// Copy and paste between machines (a hub and its nodes): the hub has the source machine pack the files (a tar,
// streamed) and the destination machine unpack them, then, for a cut, the source remove them. Each machine checks
// the user's rights on its own folders.

// machineOfPlace splits a place into its machine and the place as that machine knows it: a server "france~css-1"
// is on france; a package or the shared copies are on the machine the page named.
func (s *Server) machineOfPlace(p placeRef) (string, placeRef) {
	if p.Kind == "server" {
		if before, after, ok := strings.Cut(p.ID, link.Sep); ok {
			if _, known := s.links.Get().Node(before); known {
				return before, placeRef{Kind: p.Kind, ID: after}
			}
		}
		return "", placeRef{Kind: p.Kind, ID: p.ID}
	}
	m := p.Machine
	if _, known := s.links.Get().Node(m); !known {
		m = ""
	}
	return m, placeRef{Kind: p.Kind, ID: p.ID}
}

// linkUser is a user of this manager as another machine (or this one, in-process) is told of them.
func (s *Server) linkUser(u *User, machine string) link.User {
	lu := link.User{Name: u.Name, Rights: u.Rights, Servers: []string{}}
	if machine != "" {
		lu.Name = u.Name + " (" + s.links.Get().Name + ")"
	}
	for _, id := range u.Servers {
		before, after, cut := strings.Cut(id, link.Sep)
		switch {
		case machine == "" && !cut:
			lu.Servers = append(lu.Servers, id)
		case machine != "" && cut && before == machine:
			lu.Servers = append(lu.Servers, after)
		}
	}
	return lu
}

// pipeRW is a ResponseWriter whose body is read as it is written.
type pipeRW struct {
	hdr    http.Header
	pw     *io.PipeWriter
	once   sync.Once
	head   chan struct{}
	status int
}

func (p *pipeRW) Header() http.Header { return p.hdr }
func (p *pipeRW) WriteHeader(code int) {
	p.once.Do(func() { p.status = code; close(p.head) })
}
func (p *pipeRW) Write(b []byte) (int, error) { p.WriteHeader(http.StatusOK); return p.pw.Write(b) }
func (p *pipeRW) Flush()                      {}

// call runs a request of this manager's API on a machine ("" this one), as a user; its answer is read as it comes.
func (s *Server) call(ctx context.Context, machine, method, apiPath string, body io.Reader, u *User) (int, http.Header, io.ReadCloser, error) {
	pr, pw := io.Pipe()
	rw := &pipeRW{hdr: http.Header{}, pw: pw, head: make(chan struct{})}
	length := int64(0)
	if body == nil {
		body = http.NoBody
	} else {
		length = -1
	}
	lu := s.linkUser(u, machine)
	req, err := http.NewRequestWithContext(link.WithUser(ctx, lu), method, "http://szm"+apiPath, body)
	if err != nil {
		return 0, nil, nil, err
	}
	req.ContentLength = length
	req.Header.Set("X-SZM", "1")
	req.Header.Set("Content-Type", "application/octet-stream")
	go func() {
		if machine == "" {
			s.http.Handler.ServeHTTP(rw, req)
		} else {
			s.hub.Forward(rw, req, machine, apiPath, lu)
		}
		rw.WriteHeader(http.StatusOK)
		pw.Close()
	}()
	select {
	case <-rw.head:
	case <-ctx.Done():
		pw.CloseWithError(ctx.Err())
		return 0, nil, nil, ctx.Err()
	}
	return rw.status, rw.hdr, pr, nil
}

// callJSON runs a request and decodes its JSON answer; an error answer becomes an error.
func (s *Server) callJSON(ctx context.Context, machine, method, apiPath string, in, out any, u *User) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = strings.NewReader(string(b))
	}
	status, _, rc, err := s.call(ctx, machine, method, apiPath, body, u)
	if err != nil {
		return err
	}
	defer rc.Close()
	b, _ := io.ReadAll(io.LimitReader(rc, 8<<20))
	if status >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			return &httpError{status, e.Error}
		}
		return &httpError{status, fmt.Sprintf("Error %d.", status)}
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

type pasteReq struct {
	From     placeRef `json:"from"`
	Dir      string   `json:"dir"`
	Names    []string `json:"names"`
	To       placeRef `json:"to"`
	Dest     string   `json:"dest"`
	Cut      bool     `json:"cut"`
	Conflict string   `json:"conflict"` // "" (ask), overwrite, skip, rename
	FromName string   `json:"from_name,omitempty"`
}

// pasteAcross: a paste whose source or destination is a linked machine (true when it was one, and answered).
func (s *Server) pasteAcross(w http.ResponseWriter, r *http.Request, u *User, in pasteReq) (bool, error) {
	if len(s.links.Get().Nodes) == 0 {
		return false, nil
	}
	fm, from := s.machineOfPlace(in.From)
	tm, to := s.machineOfPlace(in.To)
	if fm == "" && tm == "" {
		return false, nil
	}
	ctx := r.Context()
	in.From, in.To = from, to
	if fm == tm {
		// both on the same linked machine: it does it
		var out map[string]any
		if err := s.callJSON(ctx, fm, http.MethodPost, "/api/paste", in, &out, u); err != nil {
			return true, err
		}
		return true, writeJSON(w, out)
	}
	if len(in.Names) == 0 {
		return true, fail(http.StatusBadRequest, "Nothing to paste.")
	}
	// what is already there
	if in.Conflict == "" {
		var ex struct {
			Exists []string `json:"exists"`
		}
		if err := s.callJSON(ctx, tm, http.MethodPost, "/api/paste/check", map[string]any{"to": to, "dest": in.Dest, "names": in.Names}, &ex, u); err != nil {
			return true, err
		}
		if len(ex.Exists) > 0 {
			sort.Strings(ex.Exists)
			return true, writeJSON(w, map[string]any{"conflicts": ex.Exists})
		}
	}
	// the source packs, the destination unpacks: in the background (a big copy takes minutes), followed by the page
	fromName := s.machineLabel(fm) + ": " + s.placeLabel(fm, from)
	toName := s.machineLabel(tm) + ": " + s.placeLabel(tm, to)
	job := s.newPasteJob(fromName, toName)
	job.owner = u.Name
	go s.runPaste(job, fm, tm, from, to, in, u)
	return true, writeJSON(w, map[string]any{"job": job.ID})
}

// pasteJob is a copy between machines going on.
type pasteJob struct {
	ID       string   `json:"id"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Total    int64    `json:"total"` // bytes to copy (0: not known yet)
	Done     int64    `json:"done"`
	Finished bool     `json:"finished"`
	Error    string   `json:"error,omitempty"`
	Items    int      `json:"items"`
	Files    int      `json:"files"`
	Failed   []string `json:"failed"`
	ended    time.Time
	owner    string
}

var pasteJobs = struct {
	sync.Mutex
	m map[string]*pasteJob
}{m: map[string]*pasteJob{}}

func (s *Server) newPasteJob(from, to string) *pasteJob {
	j := &pasteJob{ID: newJobID(), From: from, To: to, Failed: []string{}}
	pasteJobs.Lock()
	defer pasteJobs.Unlock()
	for id, o := range pasteJobs.m { // finished ones are kept 10 minutes
		if o.Finished && time.Since(o.ended) > 10*time.Minute {
			delete(pasteJobs.m, id)
		}
	}
	pasteJobs.m[j.ID] = j
	return j
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// countReader counts what goes through, for the job's progress.
type countReader struct {
	r io.Reader
	j *pasteJob
}

func (c *countReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	pasteJobs.Lock()
	c.j.Done += int64(n)
	pasteJobs.Unlock()
	return n, err
}

func (s *Server) runPaste(j *pasteJob, fm, tm string, from, to placeRef, in pasteReq, u *User) {
	finish := func(err error) {
		pasteJobs.Lock()
		j.Finished, j.ended = true, time.Now()
		if err != nil {
			j.Error = err.Error()
		}
		failed := append([]string(nil), j.Failed...)
		pasteJobs.Unlock()
		if err != nil || len(failed) > 0 {
			msg := ""
			if err != nil {
				msg = err.Error()
			} else {
				msg = strings.Join(failed, "; ")
			}
			s.M.Log.Add(events.Error, "", "", "Copy from %s to %s by %s: %s", j.From, j.To, u.Name, msg)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status, hdr, pack, err := s.call(ctx, fm, http.MethodPost, "/api/paste/pack", strings.NewReader(mustJSON(map[string]any{"from": from, "dir": in.Dir, "names": in.Names})), u)
	if err != nil {
		finish(err)
		return
	}
	defer pack.Close()
	if status >= 300 {
		b, _ := io.ReadAll(io.LimitReader(pack, 64<<10))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &e) != nil || e.Error == "" {
			e.Error = "the files could not be read"
		}
		finish(errors.New(e.Error))
		return
	}
	if t, err := strconv.ParseInt(hdr.Get("X-Total"), 10, 64); err == nil {
		pasteJobs.Lock()
		j.Total = t
		pasteJobs.Unlock()
	}
	q := url.Values{"p": {mustJSON(pasteReq{To: to, Dest: in.Dest, Names: in.Names, Conflict: in.Conflict, Cut: in.Cut, FromName: j.From})}}
	st2, _, res, err := s.call(ctx, tm, http.MethodPost, "/api/paste/unpack?"+q.Encode(), &countReader{r: pack, j: j}, u)
	if err != nil {
		finish(err)
		return
	}
	defer res.Close()
	b, _ := io.ReadAll(io.LimitReader(res, 8<<20))
	var out struct {
		Items  int      `json:"items"`
		Files  int      `json:"files"`
		Failed []string `json:"failed"`
		Error  string   `json:"error"`
	}
	_ = json.Unmarshal(b, &out)
	if st2 >= 300 {
		if out.Error == "" {
			out.Error = fmt.Sprintf("error %d", st2)
		}
		finish(errors.New(out.Error))
		return
	}
	failed := out.Failed
	if failed == nil {
		failed = []string{}
	}
	if in.Cut && len(failed) == 0 && out.Items > 0 {
		if err := s.callJSON(ctx, fm, http.MethodPost, "/api/paste/remove", map[string]any{"from": from, "dir": in.Dir, "names": in.Names}, nil, u); err != nil {
			failed = append(failed, "copied, but not removed from "+j.From+": "+err.Error())
		}
	}
	pasteJobs.Lock()
	j.Items, j.Files, j.Failed = out.Items, out.Files, failed
	pasteJobs.Unlock()
	finish(nil)
}

// pasteJobGet tells how a copy between machines goes.
func (s *Server) pasteJobGet(w http.ResponseWriter, r *http.Request, u *User) error {
	pasteJobs.Lock()
	j := pasteJobs.m[r.PathValue("job")]
	var c pasteJob
	if j != nil {
		c = *j
		c.Failed = append([]string(nil), j.Failed...)
	}
	pasteJobs.Unlock()
	if j == nil || c.owner != u.Name {
		return fail(http.StatusNotFound, "No such copy.")
	}
	return writeJSON(w, c)
}

// placeLabel names a place of a machine: its server's name, "package x", "the shared copies".
func (s *Server) placeLabel(machine string, p placeRef) string {
	switch p.Kind {
	case "pkg":
		return "package " + p.ID
	case "cache":
		return "the shared copies"
	}
	if machine == "" {
		if sv := s.M.Cfg.Server(p.ID); sv != nil {
			s.M.Cfg.RLock()
			defer s.M.Cfg.RUnlock()
			return sv.Name
		}
		return p.ID
	}
	if rm := s.hub.Remote(machine); rm != nil {
		for k, id := range rm.IDs {
			if id == machine+link.Sep+p.ID {
				var v struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(rm.Servers[k], &v) == nil && v.Name != "" {
					return v.Name
				}
			}
		}
	}
	return p.ID
}

func (s *Server) machineLabel(m string) string {
	f := s.links.Get()
	if n, ok := f.Node(m); ok {
		return n.Name
	}
	return f.Name
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// pasteCheck tells which of the names are already in a folder.
func (s *Server) pasteCheck(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		To    placeRef `json:"to"`
		Dest  string   `json:"dest"`
		Names []string `json:"names"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	to, err := s.placeOf(in.To, u)
	if err != nil {
		return err
	}
	dst, err := jail(to.root, in.Dest)
	if err != nil {
		return err
	}
	if st, err := os.Stat(dst); err != nil || !st.IsDir() {
		return fail(http.StatusNotFound, "The folder to paste into is not there.")
	}
	exists := []string{}
	for _, n := range in.Names {
		if okName(n) && pathExists(filepath.Join(dst, n)) {
			exists = append(exists, n)
		}
	}
	return writeJSON(w, map[string]any{"exists": exists})
}

func okName(n string) bool { return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `/\`) }

// pastePack sends files and folders of a place as a tar.
func (s *Server) pastePack(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		From  placeRef `json:"from"`
		Dir   string   `json:"dir"`
		Names []string `json:"names"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	from, err := s.placeOf(in.From, u)
	if err != nil {
		return err
	}
	src, err := jail(from.root, in.Dir)
	if err != nil {
		return err
	}
	for _, n := range in.Names {
		if !okName(n) {
			return fail(http.StatusBadRequest, "Bad name: %s.", n)
		}
		if !pathExists(filepath.Join(src, n)) {
			return fail(http.StatusNotFound, "%s is no longer there.", n)
		}
	}
	var total int64
	for _, n := range in.Names {
		_ = filepath.WalkDir(filepath.Join(src, n), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
					total += info.Size()
				}
			}
			return nil
		})
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("X-Total", strconv.FormatInt(total, 10))
	tw := tar.NewWriter(w)
	for _, n := range in.Names {
		root := filepath.Join(src, n)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !d.IsDir() && !info.Mode().IsRegular() {
				return nil // links and the like: left
			}
			rel, _ := filepath.Rel(src, p)
			hd, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hd.Name = filepath.ToSlash(rel)
			if d.IsDir() {
				hd.Name += "/"
			}
			hd.Uname, hd.Gname, hd.Uid, hd.Gid = "", "", 0, 0
			if err := tw.WriteHeader(hd); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.CopyN(tw, f, info.Size())
			return err
		})
		if err != nil {
			return nil // the answer has begun: the other side sees the tar end short
		}
	}
	return tw.Close()
}

// pasteUnpack writes a tar into a folder: each top name overwritten, skipped or renamed, as asked.
func (s *Server) pasteUnpack(w http.ResponseWriter, r *http.Request, u *User) error {
	var in pasteReq
	if err := json.Unmarshal([]byte(r.URL.Query().Get("p")), &in); err != nil {
		return fail(http.StatusBadRequest, "Bad request.")
	}
	to, err := s.placeOf(in.To, u)
	if err != nil {
		return err
	}
	dst, err := jail(to.root, in.Dest)
	if err != nil {
		return err
	}
	if st, err := os.Stat(dst); err != nil || !st.IsDir() {
		return fail(http.StatusNotFound, "The folder to paste into is not there.")
	}
	wanted := map[string]bool{}
	for _, n := range in.Names {
		if !okName(n) {
			return fail(http.StatusBadRequest, "Bad name: %s.", n)
		}
		wanted[n] = true
	}
	// where each top name goes ("" skipped)
	target := map[string]string{}
	items := 0
	for n := range wanted {
		t := filepath.Join(dst, n)
		if pathExists(t) {
			switch in.Conflict {
			case "skip":
				t = ""
			case "rename":
				t = freeName(dst, n, "")
			}
		}
		target[n] = t
		if t != "" {
			items++
		}
	}
	files, failed := 0, []string{}
	var total int64
	tr := tar.NewReader(r.Body)
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			failed = append(failed, "the copy was cut short: "+err.Error())
			break
		}
		name := path.Clean(strings.TrimSuffix(hd.Name, "/"))
		top, rest, _ := strings.Cut(name, "/")
		if strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") || !wanted[top] {
			failed = append(failed, hd.Name+": not asked")
			continue
		}
		base := target[top]
		if base == "" {
			continue // skipped
		}
		dest := base
		if rest != "" {
			dest = filepath.Join(base, filepath.FromSlash(rest))
		}
		if !inside(dst, dest) {
			failed = append(failed, hd.Name+": not asked")
			continue
		}
		switch hd.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				failed = append(failed, name+": "+err.Error())
			}
		case tar.TypeReg:
			n, err := writeFrom(tr, dest, hd)
			if err != nil {
				failed = append(failed, name+": "+err.Error())
				continue
			}
			files++
			total += n
		}
	}
	verb := "copied"
	if in.Cut {
		verb = "moved"
	}
	where := relOf(to.root, dst)
	if where == "" {
		where = "its folder"
	}
	if items > 0 {
		s.M.Log.Add(events.Info, to.server, labelOr(to), "%s %s %d item%s from %s to %s", u.Name, verb, items,
			map[bool]string{true: "", false: "s"}[items == 1], in.FromName, where)
	}
	if to.server != "" {
		s.M.ForgetSize(to.server)
	}
	return writeJSON(w, map[string]any{"items": items, "files": files, "bytes": total, "failed": failed})
}

// writeFrom writes a file from a reader (a temporary file, then put in place).
func writeFrom(rd io.Reader, dst string, hd *tar.Header) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".szm-part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, rd)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != hd.Size {
		err = errors.New("cut short")
	}
	if err == nil {
		err = os.Rename(tmp, dst)
		if err != nil && pathExists(dst) {
			err = fmt.Errorf("in use (stop the server first?): %w", err)
		}
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	_ = os.Chtimes(dst, hd.ModTime, hd.ModTime)
	return n, nil
}

// pasteRemove removes what was moved to another machine.
func (s *Server) pasteRemove(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		From  placeRef `json:"from"`
		Dir   string   `json:"dir"`
		Names []string `json:"names"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	from, err := s.placeOf(in.From, u)
	if err != nil {
		return err
	}
	src, err := jail(from.root, in.Dir)
	if err != nil {
		return err
	}
	var failed []string
	for _, n := range in.Names {
		if !okName(n) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(src, n)); err != nil {
			failed = append(failed, n+": "+err.Error())
		}
	}
	if from.server != "" {
		s.M.ForgetSize(from.server)
	}
	if len(failed) > 0 {
		return fail(http.StatusConflict, "%s", strings.Join(failed, "; "))
	}
	return writeJSON(w, map[string]any{"ok": true})
}
