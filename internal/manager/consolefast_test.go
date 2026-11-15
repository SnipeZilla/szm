package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/proc"
)

// The ring keeps the last lines in their order, before and after it is full.
func TestRing(t *testing.T) {
	r := newRing(5)
	if got := r.all(); len(got) != 0 {
		t.Fatal(got)
	}
	for k := 1; k <= 3; k++ {
		r.add(fmt.Sprint(k))
	}
	if got := strings.Join(r.all(), ""); got != "123" {
		t.Fatal(got)
	}
	if got := strings.Join(r.last(2), ""); got != "23" {
		t.Fatal(got)
	}
	for k := 4; k <= 12; k++ {
		r.add(fmt.Sprint(k))
		want := ""
		for j := max(1, k-4); j <= k; j++ {
			want += fmt.Sprint(j) + ","
		}
		if got := strings.Join(r.all(), ",") + ","; got != want {
			t.Fatalf("after %d: %s, want %s", k, got, want)
		}
		if got := r.last(1); len(got) != 1 || got[0] != fmt.Sprint(k) {
			t.Fatalf("last after %d: %q", k, got)
		}
		if got := strings.Join(r.last(3), ","); got != fmt.Sprintf("%d,%d,%d", k-2, k-1, k) {
			t.Fatalf("last 3 after %d: %s", k, got)
		}
	}
	if r.len() != 5 || len(r.last(99)) != 5 || len(r.last(0)) != 0 || len(r.last(-1)) != 0 {
		t.Fatal(r.len())
	}
}

func conManager(t *testing.T, game string) (*Manager, config.Server, *games.Game, *Instance) {
	t.Helper()
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "szm.json"))
	s := &config.Server{ID: game + "-1", Name: "Test", Game: game, InstallDir: filepath.Join(dir, game), ConsoleLog: true}
	cfg.Servers = []*config.Server{s}
	cat, _ := games.LoadOrBuiltin(t.TempDir())
	m := New(cfg, cat, dir)
	g := cat[game]
	if g == nil {
		t.Fatal("no game " + game)
	}
	return m, *s, g, m.instance(s.ID)
}

// A reader gets every line by its number, however many come at once; one too far behind is told how many it missed;
// one of another run of the manager starts over.
func TestConsoleByNumbers(t *testing.T) {
	m, s, _, i := conManager(t, "css")
	c := m.SubscribeConsole(s.ID)
	defer m.UnsubscribeConsole(s.ID, c)
	lines, n, missed := m.ConsoleFrom(s.ID, -1)
	if len(lines) != 0 || n != 0 || missed != 0 {
		t.Fatal(lines, n, missed)
	}
	// 1500 lines at once: with lines pushed to the reader one by one, those over its 256 waiting were dropped
	burst := make([]string, 1500)
	for k := range burst {
		burst[k] = fmt.Sprintf("line %d", k)
	}
	i.say(burst...)
	select {
	case <-c:
	case <-time.After(time.Second):
		t.Fatal("not woken")
	}
	lines, next, missed := m.ConsoleFrom(s.ID, n)
	if len(lines) != 1500 || next != 1500 || missed != 0 || lines[0] != "line 0" || lines[1499] != "line 1499" {
		t.Fatal(len(lines), next, missed)
	}
	// nothing new: nothing, and not woken
	if lines, n2, _ := m.ConsoleFrom(s.ID, next); len(lines) != 0 || n2 != next {
		t.Fatal(lines, n2)
	}
	select {
	case <-c:
		t.Fatal("woken for nothing")
	default:
	}
	// many says, one wake-up kept, every line there
	for k := 0; k < 300; k++ {
		i.say(fmt.Sprintf("more %d", k))
	}
	lines, next2, _ := m.ConsoleFrom(s.ID, next)
	if len(lines) != 300 || lines[299] != "more 299" || next2 != 1800 {
		t.Fatal(len(lines), next2)
	}
	// too far behind: the lines kept, and how many are gone
	i.say(make([]string, consoleKeep)...)
	lines, next3, missed := m.ConsoleFrom(s.ID, 100)
	if len(lines) != consoleKeep || next3 != 1800+consoleKeep || missed != 1700 {
		t.Fatal(len(lines), next3, missed)
	}
	// a number of another run (over the last), a new reader: all kept, nothing "missed"
	if lines, _, missed := m.ConsoleFrom(s.ID, next3+50); len(lines) != consoleKeep || missed != 0 {
		t.Fatal(len(lines), missed)
	}
	if lines, _, missed := m.ConsoleFrom(s.ID, -1); len(lines) != consoleKeep || missed != 0 {
		t.Fatal(len(lines), missed)
	}
	// as before for the callers of ConsoleSince
	if lines, n := m.ConsoleSince(s.ID, next3-2); len(lines) != 2 || n != next3 {
		t.Fatal(len(lines), n)
	}
}

// Readers at once with lines coming: each gets them all, in order (run with -race).
func TestConsoleReadersRace(t *testing.T) {
	m, s, _, i := conManager(t, "css")
	const total = 5000
	var wg sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := m.SubscribeConsole(s.ID)
			defer m.UnsubscribeConsole(s.ID, c)
			var n int64
			got := 0
			deadline := time.After(10 * time.Second)
			for got < total {
				lines, next, missed := m.ConsoleFrom(s.ID, n)
				if missed != 0 {
					t.Errorf("missed %d", missed)
					return
				}
				for _, l := range lines {
					if l != fmt.Sprint(got) {
						t.Errorf("got %q, want %d", l, got)
						return
					}
					got++
				}
				n = next
				if got == total {
					return
				}
				select {
				case <-c:
				case <-deadline:
					t.Errorf("only %d lines", got)
					return
				}
			}
		}()
	}
	time.Sleep(20 * time.Millisecond) // the readers are there
	for k := 0; k < total; k += 50 {
		batch := make([]string, 50)
		for j := range batch {
			batch[j] = fmt.Sprint(k + j)
		}
		i.say(batch...)
		time.Sleep(time.Millisecond) // readers keep up: under 2000 lines behind
	}
	wg.Wait()
}

// The log: its new lines only, half a line waited for, more than a piece at once, a new log, the end of a line
// whose start was not read (a server found running).
func TestReadLog(t *testing.T) {
	m, s, g, i := conManager(t, "css")
	path := g.Path(s.InstallDir, g.ConsoleLog)
	os.MkdirAll(filepath.Dir(path), 0o755)
	add := func(text string) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(text)
		f.Close()
	}
	var n int64
	read := func() []string {
		m.readLog(i, s, g)
		lines, next, _ := m.ConsoleFrom(s.ID, n)
		n = next
		return lines
	}
	add("one\r\ntwo\nhal")
	if got := strings.Join(read(), "|"); got != "one|two" {
		t.Fatal(got)
	}
	if got := read(); len(got) != 0 {
		t.Fatal(got)
	}
	add("f\n\n+sv_setsteamaccount 0123456789ABCDEF0123 x\n")
	if got := strings.Join(read(), "|"); got != "half|+sv_setsteamaccount •••••••• x" {
		t.Fatal(got)
	}
	// 700 KB at once (three pieces), lines across the pieces' edges: all of them, once
	var big strings.Builder
	for k := 0; k < 1800; k++ {
		fmt.Fprintf(&big, "L %05d %s\n", k, strings.Repeat("x", 380))
	}
	add(big.String())
	got := read()
	if len(got) != 1800 {
		t.Fatalf("%d lines", len(got))
	}
	for k, l := range got {
		if !strings.HasPrefix(l, fmt.Sprintf("L %05d ", k)) || len(l) != 388 {
			t.Fatalf("line %d: %q", k, l)
		}
	}
	// more than is read at a time (6 MB): the rest at the next reads, nothing lost on the way
	big.Reset()
	for k := 0; k < 15000; k++ {
		fmt.Fprintf(&big, "M %05d %s\n", k, strings.Repeat("y", 390))
	}
	add(big.String())
	seen := 0
	for k := 0; k < 4 && seen < 15000; k++ {
		m.readLog(i, s, g)
		lines, next, missed := m.ConsoleFrom(s.ID, n)
		for j, l := range lines {
			if want := fmt.Sprintf("M %05d ", seen+int(missed)+j); !strings.HasPrefix(l, want) {
				t.Fatalf("got %.12q, want %s", l, want)
			}
		}
		seen += int(missed) + len(lines)
		n = next
	}
	if seen != 15000 {
		t.Fatalf("%d lines of 15000", seen)
	}
	// the server starts a new log
	os.WriteFile(path, []byte("fresh\n"), 0o644)
	if got := strings.Join(read(), "|"); got != "fresh" {
		t.Fatal(got)
	}
	// found running: from the last lines, the first (cut) left out
	add(strings.Repeat("old line\n", 2000))
	m.openConsole(i, s, g, true)
	add("new\n")
	got = read()
	if len(got) < 100 || len(got) > 1000 || got[0] != "old line" || got[len(got)-1] != "new" {
		t.Fatal(len(got), got[0])
	}
	// a fresh start: from the end of what is there
	m.openConsole(i, s, g, false)
	add("after start\n")
	if got := strings.Join(read(), "|"); got != "after start" {
		t.Fatal(got)
	}
}

// While someone looks at a console its log is read at once, not at the manager's next second; nobody: not read by
// the fast reader.
func TestFollowConsoles(t *testing.T) {
	m, s, g, i := conManager(t, "css")
	path := g.Path(s.InstallDir, g.ConsoleLog)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, nil, 0o644)
	i.mu.Lock()
	i.pid, i.since = os.Getpid(), time.Now()
	i.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.followConsoles(ctx)
	add := func(text string) {
		f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
		f.WriteString(text)
		f.Close()
	}
	// nobody looks: nothing read
	add("unseen yet\n")
	time.Sleep(4 * followEvery)
	if lines, _, _ := m.ConsoleFrom(s.ID, -1); len(lines) != 0 {
		t.Fatal(lines)
	}
	// someone does: what is there, then each line as it is written
	c := m.SubscribeConsole(s.ID)
	wait := func(want string) time.Duration {
		t.Helper()
		start := time.Now()
		for {
			select {
			case <-c:
				if lines := m.Console(s.ID); len(lines) > 0 && lines[len(lines)-1] == want {
					return time.Since(start)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%q not read: %q", want, m.Console(s.ID))
			}
		}
	}
	wait("unseen yet")
	for k := 0; k < 5; k++ {
		add(fmt.Sprintf("live %d\n", k))
		if d := wait(fmt.Sprintf("live %d", k)); d > 5*followEvery {
			t.Fatalf("read after %s", d)
		}
	}
	// gone: not read any more; a page that polls (or a command sent): read for a while
	m.UnsubscribeConsole(s.ID, c)
	time.Sleep(2 * followEvery)
	add("nobody\n")
	time.Sleep(4 * followEvery)
	if lines := m.Console(s.ID); lines[len(lines)-1] != "live 4" {
		t.Fatal(lines)
	}
	m.conMu.Lock()
	left := len(m.conWatch)
	m.conMu.Unlock()
	if left != 0 {
		t.Fatal("still watched")
	}
	lines, next, _ := m.WaitConsole(ctx, s.ID, 6, 2*time.Second)
	if len(lines) != 1 || lines[0] != "nobody" || next != 7 {
		t.Fatal(lines, next)
	}
	// a wait with nothing new ends by itself, with nothing
	start := time.Now()
	lines, next, _ = m.WaitConsole(ctx, s.ID, next, 300*time.Millisecond)
	if len(lines) != 0 || next != 7 || time.Since(start) < 250*time.Millisecond {
		t.Fatal(lines, next, time.Since(start))
	}
	// ... and at once with a line that comes meanwhile
	go func() { time.Sleep(50 * time.Millisecond); add("while waiting\n") }()
	start = time.Now()
	lines, _, _ = m.WaitConsole(ctx, s.ID, next, 5*time.Second)
	if len(lines) != 1 || lines[0] != "while waiting" || time.Since(start) > time.Second {
		t.Fatal(lines, time.Since(start))
	}
}

// The reader of a window: its first rows are the window's last ones (the log may have some), the next ones are new;
// one that does not stay is tried again, then the window is read every second as before.
func TestFollowWindow(t *testing.T) {
	m, s, g, i := conManager(t, "cs2")
	logPath := filepath.Join(s.InstallDir, "game", "csgo", "console.log")
	type run struct {
		batches [][]string
		err     error
		stay    chan struct{} // closed: the reader ends
	}
	runs := make(chan run, 8)
	started := make(chan int, 8)
	var oneShots int
	var mu sync.Mutex
	oldRead, oldFollow, oldCapture := readWindow, followWindow, windowCapture
	defer func() { readWindow, followWindow, windowCapture = oldRead, oldFollow, oldCapture }()
	windowCapture = true
	readWindow = func(pid, rows int) ([]string, error) {
		mu.Lock()
		oneShots++
		mu.Unlock()
		return []string{"a", "b", "c", "d", "e", "every second"}, nil
	}
	followWindow = func(ctx context.Context, pid, rows int, each func([]string)) error {
		r := <-runs
		started <- pid
		for _, b := range r.batches {
			each(b)
		}
		if r.stay != nil {
			select {
			case <-r.stay:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return r.err
	}
	i.mu.Lock()
	i.pid, i.since = 100, time.Now()
	i.mu.Unlock()
	// the log has the start of the window already (the manager started again)
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	os.WriteFile(logPath, []byte("a\nb\nc\n"), 0o644)
	m.openConsole(i, s, g, false)

	stay := make(chan struct{})
	runs <- run{batches: [][]string{{"a", "b", "c", "d"}, {"e"}, {"e", "f"}}, stay: stay}
	m.readConsole(i, s, g)
	if pid := <-started; pid != 100 {
		t.Fatal(pid)
	}
	waitLog := func(want string) {
		t.Helper()
		for k := 0; k < 200; k++ {
			if b, _ := os.ReadFile(logPath); string(b) == want {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		b, _ := os.ReadFile(logPath)
		t.Fatalf("log: %q, want %q", b, want)
	}
	waitLog("a\nb\nc\nd\ne\ne\nf\n")
	// its rows are in the console without waiting for the next read
	for k := 0; k < 200 && len(m.Console(s.ID)) < 4; k++ {
		time.Sleep(5 * time.Millisecond)
	}
	if got := strings.Join(m.Console(s.ID), "|"); got != "d|e|e|f" {
		t.Fatal(got)
	}
	// while it stays: no other reader, no read every second
	for k := 0; k < 5; k++ {
		m.readConsole(i, s, g)
	}
	mu.Lock()
	if oneShots != 0 || len(started) != 0 {
		t.Fatal(oneShots, len(started))
	}
	mu.Unlock()
	close(stay)

	// a new start of the server whose reader does not stay: tried again, then read every second
	i.mu.Lock()
	i.pid = 200
	i.mu.Unlock()
	for k := 0; k < followTries; k++ {
		runs <- run{err: errors.New("access denied")}
		i.mu.Lock()
		i.winFollowAt = time.Time{} // (the wait between two tries)
		i.winAt = time.Now()        // (no read every second meanwhile in this test)
		i.mu.Unlock()
		m.readConsole(i, s, g)
		if pid := <-started; pid != 200 {
			t.Fatal(pid)
		}
		for j := 0; j < 200; j++ {
			i.mu.Lock()
			done := i.winFollow == 0
			i.mu.Unlock()
			if done {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	i.mu.Lock()
	oneShot := i.winOneShot
	i.winAt = time.Time{}
	i.mu.Unlock()
	if oneShot != 200 {
		t.Fatal("not read every second after its reader left three times")
	}
	said := 0
	for k := 0; k < 200 && said == 0; k++ {
		for _, l := range m.Console(s.ID) {
			if strings.Contains(l, "does not stay (access denied)") {
				said++
			}
		}
		if said == 0 {
			time.Sleep(5 * time.Millisecond) // said once its reader is noted gone
		}
	}
	if said != 1 {
		t.Fatal(said, m.Console(s.ID))
	}
	m.readConsole(i, s, g)
	waitLog("a\nb\nc\nd\ne\ne\nf\na\nb\nc\nd\ne\nevery second\n")
	mu.Lock()
	if oneShots != 1 || len(started) != 0 {
		t.Fatal(oneShots, len(started))
	}
	mu.Unlock()
	_ = runtime.GOOS
	_ = proc.ConsoleCols
}
