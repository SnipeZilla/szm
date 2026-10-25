package manager

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
)

// Usage is what the Overview of a server shows besides its status: the size of its folder.
type Usage struct {
	Size   int64 `json:"size"`    // bytes, 0 while not measured yet
	SizeAt int64 `json:"size_at"` // unix seconds
	Sizing bool  `json:"sizing"`
}

// sizeEvery: a server's folder is measured again after this (while its page is open)
const sizeEvery = 10 * time.Minute

// quietRconRe matches the line a server writes when the manager asks it for its status (CS2's players): kept out of
// the console. Source: rcon from "127.0.0.1:50123": command "status". GoldSrc: Rcon: "rcon 123 "..." status" from "...".
var quietRconRe = regexp.MustCompile(`(?i)(^|: )(rcon from "[^"]*": command "status"|rcon: "rcon \d+ ".*" status" from ")`)

// Usage gives the size of a server's folder, measuring it again when it is old (in the background: a large folder
// shows at the page's next ask).
func (m *Manager) Usage(id string) (Usage, error) {
	s, _, i, err := m.lookup(id)
	if err != nil {
		return Usage{}, err
	}
	sized := m.refreshSize(i, s)
	if sized != nil {
		// Most folders take a moment: the answer waits a little for it, rather than a page later
		select {
		case <-sized:
		case <-time.After(2 * time.Second):
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	u := Usage{Size: i.size, Sizing: i.sizing}
	if !i.sizeAt.IsZero() {
		u.SizeAt = i.sizeAt.Unix()
	}
	return u, nil
}

// refreshSize measures a server's folder again when it is old, in the background: closed when it is done, nil when
// nothing was started.
func (m *Manager) refreshSize(i *Instance, s config.Server) <-chan struct{} {
	i.mu.Lock()
	if i.sizing || (!i.sizeAt.IsZero() && time.Since(i.sizeAt) < sizeEvery) {
		i.mu.Unlock()
		return nil
	}
	i.sizing = true
	i.mu.Unlock()
	done := make(chan struct{})
	go func() {
		n := FolderSize(s.InstallDir)
		i.mu.Lock()
		i.size, i.sizeAt, i.sizing = n, time.Now(), false
		i.mu.Unlock()
		close(done)
	}()
	return done
}

// ForgetSize has a server's folder measured again (after an install, an update, files uploaded or deleted).
func (m *Manager) ForgetSize(id string) {
	i := m.instance(id)
	i.mu.Lock()
	i.sizeAt = time.Time{}
	i.mu.Unlock()
}

// FolderSize adds up the sizes of the files in a folder and its subfolders (links are not followed).
func FolderSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a file or folder that cannot be read: skipped
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}
