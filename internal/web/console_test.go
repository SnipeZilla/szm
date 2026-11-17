package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/link"
	"github.com/SnipeZilla/szm/internal/manager"
)

type sseEvent struct {
	name, id string
	lines    []string
}

// readEvents sends the events of a console stream to a channel until it ends.
func readEvents(t *testing.T, body io.Reader) chan sseEvent {
	out := make(chan sseEvent, 256)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(body)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		var ev sseEvent
		for sc.Scan() {
			l := sc.Text()
			switch {
			case l == "":
				if ev.name != "" {
					out <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(l, "event: "):
				ev.name = l[7:]
			case strings.HasPrefix(l, "id: "):
				ev.id = l[4:]
			case strings.HasPrefix(l, "data: "):
				if ev.name != "ping" {
					if err := json.Unmarshal([]byte(l[6:]), &ev.lines); err != nil {
						t.Errorf("data %q: %v", l, err)
					}
				}
			}
		}
	}()
	return out
}

func consoleServer(t *testing.T) (*manager.Manager, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "szm.json"))
	cfg.Servers = []*config.Server{{ID: "css-1", Name: "Test", Game: "css", InstallDir: filepath.Join(dir, "css"), ConsoleLog: true}}
	cat, _ := games.LoadOrBuiltin(t.TempDir())
	m := manager.New(cfg, cat, dir)
	s, err := New(m)
	if err != nil {
		t.Fatal(err)
	}
	h := s.routes()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// signed in, as a request of a hub is
		h.ServeHTTP(w, r.WithContext(link.WithUser(r.Context(), link.User{Name: "admin", Rights: "a"})))
	}))
	t.Cleanup(ts.Close)
	return m, ts
}

// say adds two lines to the console: a command (not sent: the server has no RCON password), and why.
func sayTwo(m *manager.Manager, k int) { _, _ = m.Rcon("css-1", fmt.Sprintf("cmd %d", k)) }

func openStream(t *testing.T, ctx context.Context, url string, header ...string) chan sseEvent {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	for k := 0; k+1 < len(header); k += 2 {
		req.Header.Set(header[k], header[k+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(resp.Status, resp.Header)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return readEvents(t, resp.Body)
}

func nextEvent(t *testing.T, c chan sseEvent) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-c:
		if !ok {
			t.Fatal("the stream ended")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
	return sseEvent{}
}

// The stream: the lines kept first, then the new ones as they come, each once, however many at a time; after a
// cut, from where the reader was.
func TestConsoleStream(t *testing.T) {
	m, ts := consoleServer(t)
	url := ts.URL + "/api/servers/css-1/console"
	sayTwo(m, 0)
	ctx, cancel := context.WithCancel(context.Background())
	evs := openStream(t, ctx, url)
	ev := nextEvent(t, evs)
	if ev.name != "init" || len(ev.lines) != 2 || ev.lines[0] != "] cmd 0" || ev.id != manager.ConsoleEpoch+"-2" {
		t.Fatalf("%+v", ev)
	}
	// a line shows within a moment
	start := time.Now()
	sayTwo(m, 1)
	ev = nextEvent(t, evs)
	if ev.name != "lines" || ev.lines[0] != "] cmd 1" || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("%+v after %s", ev, time.Since(start))
	}
	got := len(ev.lines)
	for got < 2 {
		got += len(nextEvent(t, evs).lines)
	}
	// 1600 lines as fast as they can be said: every one, in order, in a few sends
	for k := 2; k < 802; k++ {
		sayTwo(m, k)
	}
	want, sends := 2, 0
	last := ""
	for want < 802 {
		ev = nextEvent(t, evs)
		if ev.name != "lines" {
			t.Fatalf("%+v", ev)
		}
		sends++
		for _, l := range ev.lines {
			if strings.HasPrefix(l, "] ") {
				if l != fmt.Sprintf("] cmd %d", want) {
					t.Fatalf("got %q, want cmd %d", l, want)
				}
				want++
			}
		}
		last = ev.id
	}
	if sends > 100 {
		t.Fatalf("%d sends for 1600 lines", sends)
	}
	for last != manager.ConsoleEpoch+"-1604" {
		last = nextEvent(t, evs).id
	}
	cancel() // the connection is cut

	// back from where it was: what it missed only, and no "init" (the page keeps what it shows)
	for k := 900; k < 903; k++ {
		sayTwo(m, k)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	for _, how := range [][]string{{url + "?from=" + last}, {url, "Last-Event-ID", last}} {
		evs = openStream(t, ctx, how[0], how[1:]...)
		ev = nextEvent(t, evs)
		if ev.name != "lines" || len(ev.lines) != 6 || ev.lines[0] != "] cmd 900" || ev.id != manager.ConsoleEpoch+"-1610" {
			t.Fatalf("%v: %s %d lines, id %s", how, ev.name, len(ev.lines), ev.id)
		}
	}
	// nothing missed: an empty "lines" (the page knows the stream is there)
	evs = openStream(t, ctx, url+"?from="+manager.ConsoleEpoch+"-1610")
	if ev = nextEvent(t, evs); ev.name != "lines" || len(ev.lines) != 0 {
		t.Fatalf("%+v", ev)
	}
	// a place of another run of the manager, or one too old: all the lines kept, as "init"
	for _, from := range []string{"zzz-1610", manager.ConsoleEpoch + "-99999", "nonsense"} {
		evs = openStream(t, ctx, url+"?from="+from)
		if ev = nextEvent(t, evs); ev.name != "init" || len(ev.lines) != 1610 {
			t.Fatalf("%s: %s %d", from, ev.name, len(ev.lines))
		}
	}
	for k := 0; k < 300; k++ {
		sayTwo(m, 2000+k)
	}
	evs = openStream(t, ctx, url+"?from="+manager.ConsoleEpoch+"-5")
	if ev = nextEvent(t, evs); ev.name != "init" || len(ev.lines) != 2000 || ev.id != manager.ConsoleEpoch+"-2210" {
		t.Fatalf("%s %d %s", ev.name, len(ev.lines), ev.id)
	}
}

// With nothing to say the stream still gives a sign, so that the page can tell it from one that is gone.
func TestConsolePing(t *testing.T) {
	old := consolePing
	consolePing = 50 * time.Millisecond
	defer func() { consolePing = old }()
	_, ts := consoleServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	evs := openStream(t, ctx, ts.URL+"/api/servers/css-1/console")
	if ev := nextEvent(t, evs); ev.name != "init" {
		t.Fatalf("%+v", ev)
	}
	for k := 0; k < 3; k++ {
		if ev := nextEvent(t, evs); ev.name != "ping" {
			t.Fatalf("%+v", ev)
		}
	}
}

// A page that can not keep a stream open: it asks for the lines after the last it has, and is answered when one
// comes (or after its wait).
func TestConsolePoll(t *testing.T) {
	m, ts := consoleServer(t)
	type answer struct {
		Lines  []string `json:"lines"`
		Next   int64    `json:"next"`
		Missed int64    `json:"missed"`
		Epoch  string   `json:"epoch"`
	}
	get := func(q string) (answer, time.Duration) {
		t.Helper()
		start := time.Now()
		resp, err := http.Get(ts.URL + "/api/servers/css-1/console?" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var a answer
		if err := json.NewDecoder(resp.Body).Decode(&a); err != nil || resp.StatusCode != 200 {
			t.Fatal(resp.Status, err)
		}
		return a, time.Since(start)
	}
	sayTwo(m, 0)
	a, _ := get("since=-1")
	if len(a.Lines) != 2 || a.Next != 2 || a.Epoch != manager.ConsoleEpoch || a.Missed != 0 {
		t.Fatalf("%+v", a)
	}
	// as the page before this one asked (since=0, no wait)
	if a, d := get("since=0"); len(a.Lines) != 2 || a.Next != 2 || d > time.Second {
		t.Fatalf("%+v", a)
	}
	// nothing new: answered after the wait, with nothing
	a, d := get("since=2&wait=0.3&epoch=" + manager.ConsoleEpoch)
	if len(a.Lines) != 0 || a.Next != 2 || d < 250*time.Millisecond || d > 2*time.Second {
		t.Fatalf("%+v after %s", a, d)
	}
	// a line while it waits: answered at once
	go func() { time.Sleep(100 * time.Millisecond); sayTwo(m, 1) }()
	a, d = get("since=2&wait=20&epoch=" + manager.ConsoleEpoch)
	if len(a.Lines) == 0 || a.Lines[0] != "] cmd 1" || d > 2*time.Second {
		t.Fatalf("%+v after %s", a, d)
	}
	// the manager started again since (another run): its lines from the start
	if a, _ := get("since=4&wait=20&epoch=other"); len(a.Lines) != 4 || a.Next != 4 {
		t.Fatalf("%+v", a)
	}
}

// The page a status card's "Join via Steam" opens: for anyone (no session), an address only, nothing else in it.
func TestJoinPage(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "szm.json"))
	cat, _ := games.LoadOrBuiltin(t.TempDir())
	s, err := New(manager.New(cfg, cat, dir))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.routes())
	defer ts.Close()
	get := func(q string) (int, string) {
		resp, err := http.Get(ts.URL + "/join" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, body := get("?a=203.0.113.7:27015")
	if code != 200 || !strings.Contains(body, `href="steam://connect/203.0.113.7:27015"`) || !strings.Contains(body, `url=steam://connect/203.0.113.7:27015"`) {
		t.Fatal(code, body)
	}
	for _, bad := range []string{"", "?a=", "?a=203.0.113.7", "?a=%22%3E%3Cscript%3Ealert(1)%3C/script%3E:1", "?a=x%20y:27015", "?a=evil.example/path:80"} {
		if code, body := get(bad); code != 400 || strings.Contains(body, "<script") {
			t.Errorf("%q: %d %s", bad, code, body)
		}
	}
}
