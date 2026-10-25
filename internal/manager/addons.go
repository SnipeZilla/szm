package manager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
)

// Addons are packages: one folder per brand and version (metamod2, counterstrikesharp, sourcemod, amxmodx...),
// each mirroring the game folder of a server (addons/metamod/..., cfg/sourcemod/...). A server picks the packages
// it gets; a push copies what changed into its game folder, keeping its plugin configs (AddonsKeep).

var packageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

// AddonsRoot is the folder of the packages.
func (m *Manager) AddonsRoot() string {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	if m.Cfg.AddonsDir != "" {
		return m.Cfg.AddonsDir
	}
	return filepath.Join(m.DataDir, "addons")
}

// PackagePath is the folder of a package; an error for a name that is not one.
func (m *Manager) PackagePath(name string) (string, error) {
	if !packageName.MatchString(name) {
		return "", errors.New("a package name has letters, digits, - _ and . (metamod2, counterstrikesharp)")
	}
	return filepath.Join(m.AddonsRoot(), name), nil
}

// Package is a package as the web interface shows it.
type Package struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Files    int             `json:"files"`
	Bytes    int64           `json:"bytes"`
	Modified int64           `json:"modified,omitempty"` // newest file (unix seconds)
	Top      []string        `json:"top"`                // what is in it: addons/metamod, cfg/sourcemod...
	Servers  []string        `json:"servers"`            // IDs of the servers that get it
	Update   AddonUpdateInfo `json:"update"`             // its new versions
}

// Packages lists the packages.
func (m *Manager) Packages() []Package {
	root := m.AddonsRoot()
	entries, _ := os.ReadDir(root)
	users := map[string][]string{}
	for _, s := range m.servers() {
		for _, p := range s.Packages {
			users[strings.ToLower(p)] = append(users[strings.ToLower(p)], s.ID)
		}
	}
	out := []Package{}
	for _, e := range entries {
		if !e.IsDir() || !packageName.MatchString(e.Name()) {
			continue
		}
		p := Package{Name: e.Name(), Path: filepath.Join(root, e.Name()), Top: []string{}, Servers: users[strings.ToLower(e.Name())]}
		if p.Servers == nil {
			p.Servers = []string{}
		}
		_ = filepath.WalkDir(p.Path, func(fp string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(p.Path, fp)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if d.IsDir() {
				if len(parts) == 2 && (parts[0] == "addons" || parts[0] == "cfg") {
					p.Top = append(p.Top, filepath.ToSlash(rel))
				}
				return nil
			}
			if info, err := d.Info(); err == nil {
				p.Files++
				p.Bytes += info.Size()
				if t := info.ModTime().Unix(); t > p.Modified {
					p.Modified = t
				}
			}
			return nil
		})
		p.Update = m.AddonUpdate(p.Name)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// CreatePackage makes an empty package.
func (m *Manager) CreatePackage(name, by string) error {
	p, err := m.PackagePath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("the package %s already exists", name)
	}
	if err := os.MkdirAll(filepath.Join(p, "addons"), 0o755); err != nil {
		return err
	}
	m.Log.Add(events.Manager, "", "", "Addon package %s created by %s", name, by)
	return nil
}

// DeletePackage deletes a package (the servers keep the files they got from it).
func (m *Manager) DeletePackage(name, by string) error {
	p, err := m.PackagePath(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(p); err != nil {
		return err
	}
	m.Cfg.Lock()
	for _, s := range m.Cfg.Servers {
		s.Packages = without(s.Packages, name)
	}
	delete(m.Cfg.AddonUpdates, strings.ToLower(name))
	m.Cfg.Unlock()
	_ = m.Cfg.Save()
	m.addons.remove(name)
	m.Log.Add(events.Manager, "", "", "Addon package %s deleted by %s (the servers keep its files)", name, by)
	return nil
}

func without(list []string, name string) []string {
	out := list[:0]
	for _, x := range list {
		if !strings.EqualFold(x, name) {
			out = append(out, x)
		}
	}
	return out
}

// SetPackage gives a package to a server, or takes it away (its files stay on the server).
func (m *Manager) SetPackage(id, name string, on bool, by string) error {
	if _, err := m.PackagePath(name); err != nil {
		return err
	}
	m.Cfg.Lock()
	var srv *config.Server
	for _, s := range m.Cfg.Servers {
		if s.ID == id {
			srv = s
		}
	}
	if srv == nil {
		m.Cfg.Unlock()
		return errUnknown
	}
	srv.Packages = without(srv.Packages, name)
	if on {
		srv.Packages = append(srv.Packages, name)
		sort.Strings(srv.Packages)
	}
	sname := srv.Name
	m.Cfg.Unlock()
	if err := m.Cfg.Save(); err != nil {
		return err
	}
	if on {
		m.Log.Add(events.Info, id, sname, "Gets the addon package %s (%s)", name, by)
	} else {
		m.Log.Add(events.Info, id, sname, "No longer gets the addon package %s (%s)", name, by)
	}
	return nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * Filling a package: an archive, or a server's folder
 * ------------------------------------------------------------------------------------------------------------- */

// How an archive goes into a package
const (
	PkgReplace = "replace" // what the package had is removed first (a new version)
	PkgUpdate  = "update"  // its files are written over the package's; the package's other files stay
	PkgAdd     = "add"     // only its files the package does not have yet are written: nothing is overwritten
)

// ExtractPackage puts an archive (.zip, .tar.gz) into a package, as mode says (PkgReplace, PkgUpdate, PkgAdd). An
// archive that wraps everything in a folder (metamod-2.0/addons/...) is unwrapped, so that addons/ lands at the top
// of the package. Gives the number of files written, and of those kept as the package had them (PkgAdd).
func (m *Manager) ExtractPackage(name, archive, filename, mode, by string) (int, int, error) {
	n, kept, err := m.extractPackage(name, archive, filename, mode, nil)
	if err != nil {
		return n, kept, err
	}
	m.noteArchive(name, filename, mode)
	switch mode {
	case PkgReplace:
		m.Log.Add(events.Manager, "", "", "%s (%d files) replaced the content of the addon package %s, by %s", filename, n, name, by)
	case PkgAdd:
		m.Log.Add(events.Manager, "", "", "%s added %d new files to the addon package %s (%d it had kept as they were), by %s", filename, n, name, kept, by)
	default:
		m.Log.Add(events.Manager, "", "", "%s (%d files) updated the addon package %s (its other files kept), by %s", filename, n, name, by)
	}
	return n, kept, nil
}

// extractPackage puts an archive into a package; keep, when given: the files it matches that the package has are
// kept as they are (counted as kept).
func (m *Manager) extractPackage(name, archive, filename, mode string, keep func(rel string) bool) (int, int, error) {
	replace := mode == PkgReplace
	dst, err := m.PackagePath(name)
	if err != nil {
		return 0, 0, err
	}
	names, open, closeAll, err := openArchive(archive, filename)
	if err != nil {
		return 0, 0, err
	}
	defer closeAll()
	prefix := archivePrefix(names)
	if replace {
		if err := os.RemoveAll(dst); err != nil {
			return 0, 0, err
		}
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, 0, err
	}
	n, kept := 0, 0
	for _, entry := range names {
		rel := strings.TrimPrefix(entry, prefix)
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		target, err := insideDir(dst, rel)
		if err != nil {
			return n, kept, err
		}
		if mode == PkgAdd || (keep != nil && keep(rel)) {
			if _, err := os.Stat(target); err == nil {
				kept++
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return n, kept, err
		}
		rc, mode, err := open(entry)
		if err != nil {
			return n, kept, err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
		if err != nil {
			rc.Close()
			return n, kept, err
		}
		_, err = io.Copy(f, rc)
		rc.Close()
		f.Close()
		if err != nil {
			return n, kept, err
		}
		n++
	}
	return n, kept, nil
}

// insideDir joins a path of an archive to a folder, refusing one that would leave it.
func insideDir(dir, rel string) (string, error) {
	clean := path.Clean("/" + strings.ReplaceAll(rel, `\`, "/"))
	if strings.Contains(clean, ":") {
		return "", fmt.Errorf("bad path in the archive: %s", rel)
	}
	return filepath.Join(dir, filepath.FromSlash(clean)), nil
}

// archivePrefix finds the folder the archive wraps its content in: the part before the shallowest "addons/" or
// "cfg/" (metamod-2.0-git1234/addons/... gives "metamod-2.0-git1234/"; game/csgo/addons/... gives "game/csgo/").
func archivePrefix(names []string) string {
	best, depth := "", -1
	for _, n := range names {
		parts := strings.Split(strings.ReplaceAll(n, `\`, "/"), "/")
		for k, p := range parts[:len(parts)-1] {
			if strings.EqualFold(p, "addons") || strings.EqualFold(p, "cfg") {
				if depth == -1 || k < depth {
					depth, best = k, strings.Join(parts[:k], "/")
				}
				break
			}
		}
	}
	if best == "" {
		return ""
	}
	return best + "/"
}

// openArchive lists the files of a .zip or .tar.gz and opens them.
func openArchive(file, filename string) (names []string, open func(string) (io.ReadCloser, fs.FileMode, error), closeAll func(), err error) {
	low := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(low, ".zip"):
		z, err := zip.OpenReader(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("not a zip file: %w", err)
		}
		byName := map[string]*zip.File{}
		for _, f := range z.File {
			n := strings.ReplaceAll(f.Name, `\`, "/")
			if f.FileInfo().IsDir() {
				continue
			}
			names = append(names, n)
			byName[n] = f
		}
		return names, func(n string) (io.ReadCloser, fs.FileMode, error) {
			f := byName[n]
			rc, err := f.Open()
			return rc, f.Mode().Perm(), err
		}, func() { z.Close() }, nil
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		// A tar is read in one go: extract it to a temporary folder first
		tmp, err := os.MkdirTemp("", "szm-tar-")
		if err != nil {
			return nil, nil, nil, err
		}
		f, err := os.Open(file)
		if err != nil {
			os.RemoveAll(tmp)
			return nil, nil, nil, err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			os.RemoveAll(tmp)
			return nil, nil, nil, fmt.Errorf("not a .tar.gz file: %w", err)
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				os.RemoveAll(tmp)
				return nil, nil, nil, err
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			n := strings.TrimPrefix(strings.ReplaceAll(h.Name, `\`, "/"), "./")
			target, err := insideDir(tmp, n)
			if err != nil {
				os.RemoveAll(tmp)
				return nil, nil, nil, err
			}
			os.MkdirAll(filepath.Dir(target), 0o755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode).Perm()|0o600)
			if err != nil {
				os.RemoveAll(tmp)
				return nil, nil, nil, err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				os.RemoveAll(tmp)
				return nil, nil, nil, err
			}
			names = append(names, n)
		}
		return names, func(n string) (io.ReadCloser, fs.FileMode, error) {
				target, _ := insideDir(tmp, n)
				f, err := os.Open(target)
				if err != nil {
					return nil, 0, err
				}
				st, _ := f.Stat()
				return f, st.Mode().Perm(), nil
			}, func() {
				os.RemoveAll(tmp)
			}, nil
	}
	return nil, nil, nil, errors.New("upload a .zip or .tar.gz archive")
}

// ImportPackage fills a package from a server: its addons/<folder> (and cfg/<folder>, which SourceMod has).
func (m *Manager) ImportPackage(name, fromID, folder, by string) error {
	dst, err := m.PackagePath(name)
	if err != nil {
		return err
	}
	s, g, _, err := m.lookup(fromID)
	if err != nil {
		return err
	}
	if folder == "" || strings.ContainsAny(folder, `/\:`) || folder == "." || folder == ".." {
		return errors.New("pick a folder of addons")
	}
	gameDir := g.Path(s.InstallDir, g.GameDir)
	src := filepath.Join(gameDir, "addons", folder)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("%s has no addons/%s folder", s.Name, folder)
	}
	go func() {
		total := CopyStats{}
		for _, sub := range []string{"addons", "cfg"} {
			from := filepath.Join(gameDir, sub, folder)
			if st, err := os.Stat(from); err != nil || !st.IsDir() {
				continue
			}
			st, err := CopyTree(context.Background(), from, filepath.Join(dst, sub, folder), ExcludeMatcher(nil))
			total.Copied += st.Copied
			total.Bytes += st.Bytes
			total.Failed += st.Failed
			if err != nil {
				m.Log.Add(events.Error, "", "", "Addon package %s from %s: %v", name, s.Name, err)
				return
			}
		}
		if total.Failed > 0 {
			m.Log.Add(events.Error, "", "", "Addon package %s from %s: %d files could not be copied", name, s.Name, total.Failed)
			return
		}
		m.noteArchive(name, "", PkgReplace) // the version it has is not known
		m.Log.Add(events.Manager, "", "", "Addon package %s filled from %s (addons/%s) by %s: %d files (%s)", name, s.Name, folder, by, total.Copied, fmtBytes(total.Bytes))
	}()
	return nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * Pushing packages into servers
 * ------------------------------------------------------------------------------------------------------------- */

// PushAddons copies a server's packages into it (only pkg when given). A running server is stopped for it (its
// players warned first with warn) and started again. The plugin configs it has (AddonsKeep) are kept.
func (m *Manager) PushAddons(id, only, by string, warn bool) error {
	return m.pushAddons(id, only, by, warn, nil)
}

// pushAddons is PushAddons; done, when given, hears how it ended (after the server was started again).
func (m *Manager) pushAddons(id, only, by string, warn bool, done func(error)) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	pkgs := s.Packages
	if only != "" {
		pkgs = []string{only}
	}
	if len(pkgs) == 0 {
		return errors.New("this server gets no addon package: tick some in its settings, or on the Addons page")
	}
	var dirs []string
	for _, p := range pkgs {
		d, err := m.PackagePath(p)
		if err != nil {
			return err
		}
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			return fmt.Errorf("the addon package %s does not exist", p)
		}
		dirs = append(dirs, d)
	}
	if exe, _ := m.exePath(s, g); exe == "" {
		return errors.New("the server is not installed")
	}
	ctx, ok := i.begin("addons")
	if !ok {
		return errBusy
	}
	m.Cfg.RLock()
	secs := m.Cfg.Restart.WarnSeconds
	keep := ExcludeMatcher(m.Cfg.AddonsKeep)
	m.Cfg.RUnlock()
	go func() {
		var err error
		defer func() {
			i.end(err)
			if done != nil {
				done(err)
			}
		}()
		start := time.Now()
		i.mu.Lock()
		running, desired := i.pid > 0, i.desired
		i.mu.Unlock()
		who := ""
		if by != "" {
			who = " (asked by " + by + ")"
		}
		m.Log.Add(events.Update, s.ID, s.Name, "Updating its addons: %s%s", strings.Join(pkgs, ", "), who)
		if running {
			if warn {
				if werr := m.warnPlayers(ctx, i, s, g, secs, "restarts"); werr != nil {
					m.Log.Add(events.Info, s.ID, s.Name, "Addons update cancelled")
					return
				}
			}
			if err = m.stopProcess(ctx, i, s, g); err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Addons: %v", err)
				return
			}
		}
		m.backupBefore(i, s, g, "before-addons")
		dst := g.Path(s.InstallDir, g.GameDir)
		total := CopyStats{}
		for k, src := range dirs {
			i.say(fmt.Sprintf("[addons] %s → %s", pkgs[k], dst))
			var st CopyStats
			st, err = KeepTree(ctx, src, dst, keep)
			i.say(fmt.Sprintf("[addons] %s: %d files copied (%s), %d already up to date, %d kept as the server has them", pkgs[k], st.Copied, fmtBytes(st.Bytes), st.Same, st.Kept))
			for _, e := range st.Errors {
				i.say("[addons] " + e)
			}
			total.Copied += st.Copied
			total.Bytes += st.Bytes
			total.Kept += st.Kept
			total.Failed += st.Failed
			if err == nil && st.Failed > 0 {
				err = fmt.Errorf("%d files of %s could not be copied (first: %s)", st.Failed, pkgs[k], st.Errors[0])
			}
			if err != nil {
				break
			}
		}
		if err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Addons: %v", err)
		} else {
			if _, perr := m.patchMetamod(s, g); perr != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Metamod: %v", perr)
			}
			m.Log.Add(events.Updated, s.ID, s.Name, "Addons up to date (%s): %d files copied (%s), %d kept as the server has them (%s)", strings.Join(pkgs, ", "), total.Copied, fmtBytes(total.Bytes), total.Kept, fmtDur(time.Since(start)))
		}
		if running || desired {
			if lerr := m.launch(i, s, g); lerr != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Could not start: %v", lerr)
				if err == nil {
					err = lerr
				}
			}
		}
	}()
	return nil
}

// PushPackage pushes a package into every server that gets it: the servers it started, and why the others not.
func (m *Manager) PushPackage(name, by string, warn bool, sees func(id string) bool) (started []string, errs []string) {
	for _, s := range m.servers() {
		uses := false
		for _, p := range s.Packages {
			if strings.EqualFold(p, name) {
				uses = true
			}
		}
		if !uses || !sees(s.ID) {
			continue
		}
		if err := m.PushAddons(s.ID, name, by, warn); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
			continue
		}
		started = append(started, s.Name)
	}
	if len(started) > 0 {
		// pushed by hand: a version held back is no longer
		m.addons.update(name, func(v *addonPkgState) {
			if v.Rollout != nil && v.Rollout.Held != "" {
				v.Rollout = nil
			}
		})
	}
	return started, errs
}

// AddonFolders lists the folders in a server's addons (to import one as a package).
func (m *Manager) AddonFolders(id string) ([]string, error) {
	s, g, _, err := m.lookup(id)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(g.Path(s.InstallDir, g.GameDir), "addons"))
	if err != nil {
		return []string{}, nil
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
