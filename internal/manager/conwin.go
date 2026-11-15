package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
)

// CS2 on Windows ignores -condebug and prints only into its console window: the manager adds the rows of that
// window to its console log (game/csgo/console.log), as -condebug would. The Console tab, the crash reports and the
// rest read the log as for any other game.
//
// A reader stays with the window for as long as the server runs (proc.FollowConsoleWindow) and gives its rows as
// they are finished: they are in the console within a moment, and no program is started for it again and again.
// Should the reader not stay (it ends at once, three times in a row), the window is read every second instead, the
// way it was before there was a reader: the last rows of the window, and those that are new added to the log.

const (
	windowRows  = 400             // rows read each time: more than a server prints in 1 s
	windowTail  = 30              // the last rows kept, to find where the new ones start
	followRetry = 5 * time.Second // a reader that ended is started again after this long
	followShort = 5 * time.Second // ... and one that lasted less than this long did not stay
	followTries = 3               // readers that did not stay, in a row, before the window is read every second
)

// the tests replace them
var (
	readWindow    = proc.ReadConsoleWindow
	followWindow  = proc.FollowConsoleWindow
	windowCapture = runtime.GOOS == "windows" // elsewhere the server's output goes to its log directly
)

func (m *Manager) captureWindow(i *Instance, s config.Server, g *games.Game) {
	if !windowCapture || !s.ConsoleLog || !g.ConsoleCapture {
		return
	}
	now := time.Now()
	i.mu.Lock()
	pid := i.pid
	if pid <= 0 {
		i.mu.Unlock()
		return
	}
	if i.winFollow == pid {
		i.mu.Unlock()
		return // its reader is with it
	}
	if i.winOneShot != pid && !i.winBusy && (i.winFollowFor != pid || now.Sub(i.winFollowAt) >= followRetry) {
		i.winFollow, i.winFollowFor, i.winFollowAt = pid, pid, now
		i.mu.Unlock()
		go m.followWindow(i, s, g, pid)
		return
	}
	// No reader (it did not stay, or is started again in a moment): the window is read now, once a second. Never
	// both at a time: each would add the rows the other one adds.
	due := !i.winBusy && now.Sub(i.winAt) >= 1*time.Second
	if due {
		i.winAt, i.winBusy = now, true
	}
	i.mu.Unlock()
	if !due {
		return
	}
	go func() {
		defer func() { i.mu.Lock(); i.winBusy = false; i.mu.Unlock() }()
		m.copyWindow(i, s, g, pid)
		m.readLog(i, s, g) // shown now, not at the next look
	}()
}

// followWindow keeps a reader on the server's window until the server ends, and adds its rows to the log as they
// come.
func (m *Manager) followWindow(i *Instance, s config.Server, g *games.Game, pid int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// A reader that stays when its server is gone (it leaves by itself within a moment) is ended.
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		gone := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			i.mu.Lock()
			same := i.pid == pid
			i.mu.Unlock()
			if same {
				gone = 0
			} else if gone++; gone >= 3 {
				cancel()
				return
			}
		}
	}()
	started := time.Now()
	whole := true // its first rows are the window's last ones: the log may have some
	err := followWindow(ctx, pid, windowRows, func(rows []string) {
		m.windowRows(i, s, g, pid, rows, whole)
		whole = false
		m.readLog(i, s, g)
	})
	i.mu.Lock()
	if i.winFollow == pid {
		i.winFollow = 0
	}
	running := i.pid == pid
	tell := false
	switch {
	case !running: // its server ended, and it with it (a reader of the server's last start says nothing of this one)
	case time.Since(started) >= followShort:
		i.winFollowFails = 0
	default:
		// it did not stay
		if i.winFollowFails++; i.winFollowFails >= followTries {
			i.winOneShot, i.winFollowFails = pid, 0
			tell = err != nil
		}
	}
	i.mu.Unlock()
	if tell {
		i.say(fmt.Sprintf("[szm] The reader of its console window does not stay (%v): the window is read every second instead.", err))
	}
}

// copyWindow reads the last rows of the window once, and adds the new ones to the log.
func (m *Manager) copyWindow(i *Instance, s config.Server, g *games.Game, pid int) {
	rows, err := readWindow(pid, windowRows)
	if err != nil {
		i.mu.Lock()
		tell := i.winErrPid != pid
		i.winErrPid = pid
		i.mu.Unlock()
		if tell {
			i.say(fmt.Sprintf("[szm] Its console window can not be read: %v", err))
		}
		return
	}
	m.windowRows(i, s, g, pid, rows, true)
}

// windowRows adds rows of the server's window to its console log. whole: they are the last rows of the window, of
// which the log may have some already (only the new ones are added); else they are new, all of them.
func (m *Manager) windowRows(i *Instance, s config.Server, g *games.Game, pid int, rows []string, whole bool) {
	path := g.Path(s.InstallDir, g.ConsoleLog)
	i.mu.Lock()
	tail, same := i.winTail, i.winPid == pid
	i.mu.Unlock()
	fresh := rows
	if whole {
		if !same {
			// a server the manager found running (the manager started again): its log has the window's lines already
			tail = fileTail(path, windowTail)
		}
		fresh = newRows(tail, rows)
	}
	if len(fresh) > 0 {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			f.WriteString(strings.Join(fresh, "\n") + "\n")
			f.Close()
		}
	}
	keep := rows
	if !whole {
		keep = append(append(make([]string, 0, len(tail)+len(rows)), tail...), rows...)
	}
	if len(keep) > windowTail {
		keep = keep[len(keep)-windowTail:]
	}
	i.mu.Lock()
	if len(keep) > 0 || !same {
		i.winTail = append([]string(nil), keep...)
	}
	i.winPid = pid
	i.mu.Unlock()
}

// newRows gives the rows of the window after those seen last time (prev: the last rows then): proc.RowsAfter.
func newRows(prev, cur []string) []string {
	rows, _ := proc.RowsAfter(prev, cur)
	return rows
}

// fileTail gives the last n lines of a file (its last 64 KB at most).
func fileTail(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	off := max(0, st.Size()-64<<10)
	buf := make([]byte, st.Size()-off)
	k, _ := f.ReadAt(buf, off)
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(buf[:k]), "\r\n", "\n"), "\n"), "\n")
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // cut in the middle
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
