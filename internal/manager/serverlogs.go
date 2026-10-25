package manager

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

// The servers' own logs: every folder named "logs" in a server's folder (csgo/logs, addons/sourcemod/logs,
// addons/counterstrikesharp/logs…). The files older than the settings say are deleted by themselves.

// skipDirs are the big folders where no logs folder is: not walked.
var skipDirs = map[string]bool{"maps": true, "materials": true, "models": true, "sound": true, "resource": true,
	"particles": true, "steamapps": true, "bin": true, "dotnet": true}

// CleanLogs deletes the files older than a number of days in the "logs" folders of a folder (and of the folders in
// them). It gives how many and their size.
func CleanLogs(root string, days int, now time.Time) (n int, size int64) {
	if days <= 0 {
		return 0, 0
	}
	cut := now.Add(-time.Duration(days) * 24 * time.Hour)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() || p == root {
			return nil
		}
		name := strings.ToLower(d.Name())
		if skipDirs[name] {
			return fs.SkipDir
		}
		if name != "logs" {
			return nil
		}
		c, sz := cleanLogsDir(p, cut)
		n, size = n+c, size+sz
		return fs.SkipDir
	})
	return n, size
}

// cleanLogsDir deletes the old files of one logs folder (and the folders that are empty after).
func cleanLogsDir(dir string, cut time.Time) (n int, size int64) {
	var dirs []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir {
				dirs = append(dirs, p)
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.ModTime().Before(cut) {
			return nil
		}
		if os.Remove(p) == nil { // a file still open by the server is not: next time
			n++
			size += info.Size()
		}
		return nil
	})
	for k := len(dirs) - 1; k >= 0; k-- {
		_ = os.Remove(dirs[k]) // only when empty
	}
	return n, size
}

// cleanServerLogs cleans the logs folders of every server (in the background, one after the other).
func (m *Manager) cleanServerLogs() {
	m.Cfg.RLock()
	days := m.Cfg.Logs.ServerDays
	m.Cfg.RUnlock()
	if days <= 0 {
		return
	}
	m.backupMu.Lock()
	if m.logsCleaning {
		m.backupMu.Unlock()
		return
	}
	m.logsCleaning = true
	m.backupMu.Unlock()
	go func() {
		defer func() {
			m.backupMu.Lock()
			m.logsCleaning = false
			m.backupMu.Unlock()
		}()
		for _, s := range m.servers() {
			if s.InstallDir == "" {
				continue
			}
			if st, err := os.Stat(s.InstallDir); err != nil || !st.IsDir() {
				continue
			}
			n, size := CleanLogs(s.InstallDir, days, time.Now())
			if n > 0 {
				m.ForgetSize(s.ID)
				m.Log.Add(events.Info, s.ID, s.Name, "Logs older than %d days deleted: %d file%s (%s)", days, n, plural(n), fmtBytes(size))
			}
		}
	}()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
