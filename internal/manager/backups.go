package manager

import (
	"archive/zip"
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
	"github.com/SnipeZilla/szm/internal/games"
)

// Backups: a zip of each server's settings and plugins (in its game folder: cfg, addons…), each day at the time of
// the settings, before an update, an addons push and a restore, and when asked. The newest are kept.

// Backup is a backup of a server.
type Backup struct {
	Name   string    `json:"name"` // 20261013-053000-daily.zip (daily, weekly, monthly, manual, before-…)
	Time   time.Time `json:"time"`
	Size   int64     `json:"size"`
	Reason string    `json:"reason"` // daily, weekly, monthly, manual, before-update, before-addons, before-restore
}

var backupNameRe = regexp.MustCompile(`^(\d{8}-\d{6})-([a-z-]+)\.zip$`)

// BackupsDir is where a server's backups go.
func (m *Manager) BackupsDir(id string) string {
	m.Cfg.RLock()
	dir := strings.TrimSpace(m.Cfg.Backups.Dir)
	m.Cfg.RUnlock()
	if dir == "" {
		dir = filepath.Join(m.DataDir, "backups")
	}
	return filepath.Join(dir, safeName(id))
}

// BackupsRoot is the folder of every server's backups folder.
func (m *Manager) BackupsRoot() string { return filepath.Dir(m.BackupsDir("x")) }

// BackupFolders gives the names of the servers' folders in the backups folder: their IDs (also the IDs of servers
// removed since, whose backups stay).
func (m *Manager) BackupFolders() []string {
	entries, _ := os.ReadDir(m.BackupsRoot())
	out := []string{}
	for _, e := range entries {
		if e.IsDir() && safeName(e.Name()) == e.Name() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Backups lists a server's backups, the newest first.
func (m *Manager) Backups(id string) []Backup {
	entries, _ := os.ReadDir(m.BackupsDir(id))
	out := []Backup{}
	for _, e := range entries {
		b, ok := backupOf(e)
		if ok {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name > out[b].Name })
	return out
}

func backupOf(e fs.DirEntry) (Backup, bool) {
	mm := backupNameRe.FindStringSubmatch(e.Name())
	if mm == nil || e.IsDir() {
		return Backup{}, false
	}
	t, err := time.ParseInLocation("20060102-150405", mm[1], time.Local)
	if err != nil {
		return Backup{}, false
	}
	info, err := e.Info()
	if err != nil {
		return Backup{}, false
	}
	return Backup{Name: e.Name(), Time: t, Size: info.Size(), Reason: mm[2]}, true
}

// BackupPath gives the file of a backup (an error for a name that is not one).
func (m *Manager) BackupPath(id, name string) (string, error) {
	if !backupNameRe.MatchString(name) {
		return "", errors.New("no such backup")
	}
	p := filepath.Join(m.BackupsDir(id), name)
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("no such backup")
	}
	return p, nil
}

// DeleteBackup removes a backup.
func (m *Manager) DeleteBackup(id, name, by string) error {
	p, err := m.BackupPath(id, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	if s := m.Cfg.Server(id); s != nil {
		m.Log.Add(events.Info, id, s.Name, "Backup %s removed by %s", name, by)
	}
	return nil
}

// BackupNow backs a server up (asked from the web page), in the background.
func (m *Manager) BackupNow(id, by string) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	if exe, _ := m.exePath(s, g); exe == "" {
		return errors.New("the server is not installed")
	}
	go func() {
		if b, err := m.backup(i, s, g, "manual"); err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Backup failed: %v", err)
		} else {
			m.Log.Add(events.Info, s.ID, s.Name, "Backed up by %s: %s (%s)", by, b.Name, fmtBytes(b.Size))
		}
	}()
	return nil
}

// backupBefore backs up before a change (update, addons, restore), when the settings say so. A failed backup is
// said, and the change goes on.
func (m *Manager) backupBefore(i *Instance, s config.Server, g *games.Game, reason string) {
	m.Cfg.RLock()
	on := m.Cfg.Backups.Before
	m.Cfg.RUnlock()
	if !on {
		return
	}
	if exe, _ := m.exePath(s, g); exe == "" {
		return
	}
	b, err := m.backup(i, s, g, reason)
	if err != nil {
		m.Log.Add(events.Error, s.ID, s.Name, "Backup before the change failed (the change goes on): %v", err)
		return
	}
	i.say(fmt.Sprintf("[backup] %s (%s)", b.Name, fmtBytes(b.Size)))
}

// backup zips what the settings say of a server's game folder. One at a time per server.
func (m *Manager) backup(i *Instance, s config.Server, g *games.Game, reason string) (Backup, error) {
	i.backupMu.Lock()
	defer i.backupMu.Unlock()
	m.Cfg.RLock()
	include := append([]string(nil), m.Cfg.Backups.Include...)
	exclude := ExcludeMatcher(m.Cfg.Backups.Exclude)
	keep := m.Cfg.Backups.Keep
	m.Cfg.RUnlock()

	root := g.Path(s.InstallDir, g.GameDir)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return Backup{}, fmt.Errorf("its game folder %s is not there", root)
	}
	files := backupFiles(root, include, exclude)
	if len(files) == 0 {
		return Backup{}, errors.New("nothing to back up: none of the backed up files is in its game folder (Settings, Backups)")
	}
	dir := m.BackupsDir(s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Backup{}, err
	}
	now := time.Now().Truncate(time.Second)
	if list := m.Backups(s.ID); len(list) > 0 && !now.After(list[0].Time) {
		now = list[0].Time.Add(time.Second) // two in the same second: the names stay in order
	}
	name := now.Format("20060102-150405") + "-" + reason + ".zip"
	tmp := filepath.Join(dir, name+".tmp")
	f, err := os.Create(tmp)
	if err != nil {
		return Backup{}, err
	}
	zw := zip.NewWriter(f)
	skipped := 0
	for _, rel := range files {
		if err := addToZip(zw, root, rel); err != nil {
			skipped++ // in use, or gone since the list was made
		}
	}
	err = zw.Close()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return Backup{}, err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		os.Remove(tmp)
		return Backup{}, err
	}
	if skipped > 0 {
		i.say(fmt.Sprintf("[backup] %d files could not be read (in use?) and are not in %s", skipped, name))
	}
	m.pruneBackups(s.ID, keep, name)
	m.queueOffsite(s.ID)
	info, _ := os.Stat(filepath.Join(dir, name))
	b := Backup{Name: name, Time: now, Reason: reason}
	if info != nil {
		b.Size = info.Size()
	}
	return b, nil
}

// backupFiles lists the files to back up: the included files and folders of root, without the excluded ones.
func backupFiles(root string, include []string, excluded func(rel string) bool) []string {
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		rel = filepath.ToSlash(rel)
		if !seen[rel] && !excluded(rel) {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	for _, pat := range include {
		pat = strings.Trim(filepath.ToSlash(strings.TrimSpace(pat)), "/")
		if pat == "" || strings.Contains(pat, "..") {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
		for _, p := range matches {
			rel, err := filepath.Rel(root, p)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			st, err := os.Stat(p)
			if err != nil {
				continue
			}
			if !st.IsDir() {
				add(rel)
				continue
			}
			_ = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				r, _ := filepath.Rel(root, q)
				if d.IsDir() {
					if q != p && excluded(filepath.ToSlash(r)) {
						return filepath.SkipDir
					}
					return nil
				}
				if d.Type().IsRegular() {
					add(r)
				}
				return nil
			})
		}
	}
	sort.Strings(out)
	return out
}

func addToZip(zw *zip.Writer, root, rel string) error {
	src, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name, h.Method = rel, zip.Deflate
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

// pruneBackups keeps the newest backups of a server (and always the one just made).
func (m *Manager) pruneBackups(id string, keep int, made string) {
	list := m.Backups(id)
	for k := keep; k < len(list); k++ {
		if list[k].Name != made {
			os.Remove(filepath.Join(m.BackupsDir(id), list[k].Name))
		}
	}
}

// Restore puts a backup's files back into the server's game folder: a running server is stopped first (players
// warned when asked) and started again after. The files it has now are backed up first (before-restore). Files the
// backup does not have are left as they are.
func (m *Manager) Restore(id, name, by string, warn bool) error {
	s, g, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	p, err := m.BackupPath(id, name)
	if err != nil {
		return err
	}
	ctx, ok := i.begin("restore")
	if !ok {
		return errBusy
	}
	m.Cfg.RLock()
	secs := m.Cfg.Restart.WarnSeconds
	m.Cfg.RUnlock()
	go func() {
		var err error
		defer func() { i.end(err) }()
		i.mu.Lock()
		running, desired := i.pid > 0, i.desired
		i.mu.Unlock()
		m.Log.Add(events.Restart, s.ID, s.Name, "Restoring the backup %s (asked by %s)", name, by)
		if running {
			if warn {
				if werr := m.warnPlayers(ctx, i, s, g, secs, "restarts"); werr != nil {
					m.Log.Add(events.Info, s.ID, s.Name, "Restore cancelled")
					return
				}
			}
			if err = m.stopProcess(ctx, i, s, g); err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Restore: %v", err)
				return
			}
		}
		m.backupBefore(i, s, g, "before-restore")
		var n int
		n, err = restoreZip(ctx, p, g.Path(s.InstallDir, g.GameDir))
		if err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Restore failed after %d files: %v", n, err)
		} else {
			if _, perr := m.patchMetamod(s, g); perr != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Metamod: %v", perr)
			}
			m.Log.Add(events.Updated, s.ID, s.Name, "Restored from %s: %d files put back", name, n)
		}
		m.ForgetSize(s.ID)
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

// restoreZip writes the files of a backup into a folder: how many.
func restoreZip(ctx context.Context, zipPath, root string) (int, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		if f.FileInfo().IsDir() {
			continue
		}
		dst, err := insideDir(root, f.Name)
		if err != nil {
			return n, fmt.Errorf("%s: %w", f.Name, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return n, err
		}
		src, err := f.Open()
		if err != nil {
			return n, err
		}
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			return n, err
		}
		_, err = io.Copy(out, src)
		src.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return n, fmt.Errorf("%s: %w", f.Name, err)
		}
		_ = os.Chtimes(dst, f.Modified, f.Modified)
		n++
	}
	return n, nil
}

// BackupFiles lists what a backup holds (for the page: what a restore puts back).
func (m *Manager) BackupFiles(id, name string) ([]string, error) {
	p, err := m.BackupPath(id, name)
	if err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			out = append(out, path.Clean(f.Name))
		}
	}
	return out, nil
}

// BackupDue says whether a scheduled backup is due on a day ("" when not: by hand, or not that day), and its reason.
func BackupDue(schedule string, weekday, day int, t time.Time) string {
	switch schedule {
	case "daily":
		return "daily"
	case "weekly":
		if int(t.Weekday()) == weekday {
			return "weekly"
		}
	case "monthly":
		last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
		if t.Day() == min(day, last) {
			return "monthly"
		}
	}
	return ""
}

// scheduled says whether a backup was made by the schedule.
func scheduled(reason string) bool {
	return reason == "daily" || reason == "weekly" || reason == "monthly"
}

// scheduledBackups backs up every installed server on the days of the schedule, at its time (in the background, one
// after the other). A server already backed up by the schedule that day is not again, also after the manager restarts.
func (m *Manager) scheduledBackups(now time.Time) {
	m.Cfg.RLock()
	b := m.Cfg.Backups
	m.Cfg.RUnlock()
	reason := BackupDue(b.Schedule, b.Weekday, b.Day, now)
	if reason == "" {
		return
	}
	h, mi, ok := config.ParseClock(b.Time)
	if !ok {
		return
	}
	due := time.Date(now.Year(), now.Month(), now.Day(), h, mi, 0, 0, now.Location())
	if now.Before(due) || now.Sub(due) > 6*time.Hour {
		return
	}
	m.backupMu.Lock()
	if m.backupRunning || m.backupDay == now.Format("2006-01-02") {
		m.backupMu.Unlock()
		return
	}
	m.backupRunning, m.backupDay = true, now.Format("2006-01-02")
	m.backupMu.Unlock()
	go func() {
		defer func() {
			m.backupMu.Lock()
			m.backupRunning = false
			m.backupMu.Unlock()
		}()
		today := now.Format("20060102")
		for _, s := range m.servers() {
			g := m.Catalog()[s.Game]
			if g == nil {
				continue
			}
			if exe, _ := m.exePath(s, g); exe == "" {
				continue
			}
			done := false
			for _, b := range m.Backups(s.ID) {
				if scheduled(b.Reason) && strings.HasPrefix(b.Name, today) {
					done = true
					break
				}
			}
			if done {
				continue
			}
			i := m.instance(s.ID)
			b, err := m.backup(i, s, g, reason)
			if err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "%s backup failed: %v", strings.ToUpper(reason[:1])+reason[1:], err)
				continue
			}
			m.Log.Add(events.Info, s.ID, s.Name, "Backed up: %s (%s)", b.Name, fmtBytes(b.Size))
		}
	}()
}

// Servers gives a copy of the servers of the configuration, in their order.
func (m *Manager) Servers() []config.Server { return m.servers() }
