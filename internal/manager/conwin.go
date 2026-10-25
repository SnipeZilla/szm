package manager

import (
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

// CS2 on Windows ignores -condebug and prints only into its console window: every 2 s the manager reads the last
// rows of that window and adds the new ones to its console log (game/csgo/console.log), as -condebug would. The
// Console tab, the crash reports and the rest read the log as for any other game.

const (
	windowRows = 400 // rows read each time: more than a server prints in 2 s
	windowTail = 30  // the last rows kept, to find where the new ones start
)

// the tests replace them
var (
	readWindow    = proc.ReadConsoleWindow
	windowCapture = runtime.GOOS == "windows" // elsewhere the server's output goes to its log directly
)

func (m *Manager) captureWindow(i *Instance, s config.Server, g *games.Game) {
	if !windowCapture || !s.ConsoleLog || !g.ConsoleCapture {
		return
	}
	i.mu.Lock()
	pid := i.pid
	due := pid > 0 && !i.winBusy && time.Since(i.winAt) >= 2*time.Second
	if due {
		i.winAt, i.winBusy = time.Now(), true
	}
	i.mu.Unlock()
	if !due {
		return
	}
	go func() {
		defer func() { i.mu.Lock(); i.winBusy = false; i.mu.Unlock() }()
		m.copyWindow(i, s, g, pid)
	}()
}

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
	path := g.Path(s.InstallDir, g.ConsoleLog)
	i.mu.Lock()
	tail, same := i.winTail, i.winPid == pid
	i.mu.Unlock()
	if !same {
		// a server the manager found running (the manager started again): its log has the window's lines already
		tail = fileTail(path, windowTail)
	}
	if fresh := newRows(tail, rows); len(fresh) > 0 {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			f.WriteString(strings.Join(fresh, "\n") + "\n")
			f.Close()
		}
	}
	if len(rows) > windowTail {
		rows = rows[len(rows)-windowTail:]
	}
	i.mu.Lock()
	if len(rows) > 0 || !same {
		i.winTail = append([]string(nil), rows...)
	}
	i.winPid = pid
	i.mu.Unlock()
}

// newRows gives the rows of the window after those seen last time (prev: the last rows then). The window scrolls:
// prev is looked for as a whole, then by its last rows only; not found, every row is new (the window was cleared,
// or more came than read).
func newRows(prev, cur []string) []string {
	if len(prev) == 0 {
		return cur
	}
	for k := len(prev); k >= min(3, len(prev)); k-- {
		seq := prev[len(prev)-k:]
		for e := len(cur) - 1; e >= k-1; e-- {
			match := true
			for j := 0; j < k; j++ {
				if cur[e-k+1+j] != seq[j] {
					match = false
					break
				}
			}
			if match {
				return cur[e+1:]
			}
		}
	}
	return cur
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
