package manager

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
)

// The console log is looked for next to the program when the game does not write it where it should; a server
// that writes none at all is told about once, after a minute.
func TestConsoleLogWhere(t *testing.T) {
	cat, err := games.LoadOrBuiltin(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := cat["cs2"]
	dir := t.TempDir()
	s := config.Server{ID: "cs2-1", InstallDir: dir, ConsoleLog: true}
	if p := consolePath(s, g); p != filepath.Join(dir, "game", "csgo", "console.log") {
		t.Fatal(p)
	}
	exeDir := filepath.Dir(filepath.Join(dir, g.ExeCandidates()[len(g.ExeCandidates())-1]))
	os.MkdirAll(exeDir, 0o755)
	os.WriteFile(filepath.Join(exeDir, "console.log"), []byte("hello\n"), 0o644)
	if p := consolePath(s, g); p != filepath.Join(exeDir, "console.log") {
		t.Fatal("next to the program:", p)
	}
	os.MkdirAll(filepath.Join(dir, "game", "csgo"), 0o755)
	os.WriteFile(filepath.Join(dir, "game", "csgo", "console.log"), nil, 0o644)
	if p := consolePath(s, g); p != filepath.Join(dir, "game", "csgo", "console.log") {
		t.Fatal("where it should be:", p)
	}

	// no log at all, running for two minutes: said once
	empty := config.Server{ID: "cs2-2", InstallDir: t.TempDir(), ConsoleLog: true}
	i := newInstance("cs2-2")
	i.pid, i.since = 1234, time.Now().Add(-2*time.Minute)
	m := &Manager{}
	m.readConsole(i, empty, g)
	m.readConsole(i, empty, g)
	n := 0
	for _, l := range i.console.all() {
		if strings.Contains(l, "no console log (game/csgo/console.log)") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("said %d times: %q", n, i.console.all())
	}
}

func TestNewRows(t *testing.T) {
	r := func(a ...string) []string { return a }
	cases := []struct{ prev, cur, want []string }{
		{nil, r("a", "b"), r("a", "b")},                                         // first look: all
		{r("a", "b"), r("a", "b"), r()},                                         // nothing new
		{r("a", "b"), r("a", "b", "c", "d"), r("c", "d")},                       // new rows below
		{r("p", "a", "b", "c"), r("a", "b", "c", "d", "e"), r("d", "e")},        // scrolled: the top row gone
		{r("x", "y", "z", "same"), r("x", "y", "z", "same", "same"), r("same")}, // a row repeated
		{r("a", "b", "c"), r("q", "r"), r("q", "r")},                            // cleared: all
	}
	for n, c := range cases {
		got := newRows(c.prev, c.cur)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%d: %q", n, got)
		}
	}
}

// The window copied into the log: only its new rows; after a restart of the manager, not again what the log has;
// a new start of the server (a new, empty window) from its first row.
func TestCaptureWindow(t *testing.T) {
	cat, _ := games.LoadOrBuiltin(t.TempDir())
	g := cat["cs2"]
	s := config.Server{ID: "cs2-1", InstallDir: t.TempDir(), ConsoleLog: true}
	logPath := filepath.Join(s.InstallDir, "game", "csgo", "console.log")
	var window []string
	var fail error
	readWindow, windowCapture = func(pid, rows int) ([]string, error) { return window, fail }, true
	defer func() { readWindow, windowCapture = proc.ReadConsoleWindow, runtime.GOOS == "windows" }()
	m := &Manager{}
	i := newInstance("cs2-1")
	i.pid = 100
	step := func(rows ...string) {
		window = append(window, rows...)
		m.copyWindow(i, s, g, i.pid)
	}
	step("Loading map de_dust2", "Server is hibernating")
	step()
	step("Player connected", "Player connected")
	b, _ := os.ReadFile(logPath)
	if string(b) != "Loading map de_dust2\nServer is hibernating\nPlayer connected\nPlayer connected\n" {
		t.Fatalf("%q", b)
	}
	// the manager starts again: a new Instance, the same server running, its window as it was plus one row
	i = newInstance("cs2-1")
	i.pid = 100
	step("Map changed")
	b, _ = os.ReadFile(logPath)
	if !strings.HasSuffix(string(b), "Player connected\nMap changed\n") || strings.Count(string(b), "Loading map") != 1 {
		t.Fatalf("after a restart of the manager: %q", b)
	}
	// the server starts again: a new window
	i.pid, window = 200, nil
	step("Loading map de_dust2")
	b, _ = os.ReadFile(logPath)
	if strings.Count(string(b), "Loading map") != 2 {
		t.Fatalf("new start: %q", b)
	}
	// a window that can not be read: said once
	fail = errors.New("access denied")
	step()
	step()
	n := 0
	for _, l := range i.console.all() {
		if strings.Contains(l, "can not be read: access denied") {
			n++
		}
	}
	if n != 1 {
		t.Fatal(n)
	}
}
