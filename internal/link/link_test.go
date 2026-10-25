package link

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func init() { AllowLoopback = true } // the test hubs are on 127.0.0.1

func TestLoopback(t *testing.T) {
	AllowLoopback = false
	defer func() { AllowLoopback = true }()
	for u, want := range map[string]bool{"http://localhost:8888/": true, "http://127.0.0.1:8888/": true, "http://[::1]:8888/": true,
		"https://z.sniper.tf/": false, "http://192.168.1.10:8888/": false} {
		if loopback(u) != want {
			t.Error(u)
		}
	}
	h := NewHub(must(Open(t.TempDir())))
	if _, _, err := h.Add("US East", "http://localhost:8888/"); err == nil || !strings.Contains(err.Error(), "localhost is this machine") {
		t.Fatal(err)
	}
	c := NewClient(must(Open(t.TempDir())))
	code := pairCode{URL: "http://localhost:8888/", ID: "us-east", Secret: "x", Hub: "SZ"}.String()
	if _, err := c.Join(context.Background(), code); err == nil || !strings.Contains(err.Error(), "this machine itself") {
		t.Fatal(err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestHubURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://manager.example.com":           "https://manager.example.com/",
		"https://example.com/szm/index.html":    "https://example.com/szm/",
		"http://192.168.1.10:8888/":             "http://192.168.1.10:8888/",
		"http://localhost:8888":                 "http://localhost:8888/",
		"https://example.com/szm/#/settings/x":  "https://example.com/szm/",
		"https://example.com/szm/?a=1#/servers": "https://example.com/szm/",
	} {
		if got, err := HubURL(in); err != nil || got != want {
			t.Errorf("%s: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"http://69.169.101.210:8888/", "http://example.com/", "ftp://x", "example.com"} {
		if _, err := HubURL(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestNodeID(t *testing.T) {
	taken := map[string]bool{"france": true}
	if id := NodeID("France", func(s string) bool { return taken[s] }); id != "france-2" {
		t.Fatal(id)
	}
	if id := NodeID("US East #1", func(string) bool { return false }); id != "us-east-1" {
		t.Fatal(id)
	}
	if id := NodeID("éé", func(string) bool { return false }); id != "machine" {
		t.Fatal(id)
	}
}

// a hub and a node, linked through real HTTP
func linked(t *testing.T, nodeHandler http.Handler) (*Hub, *httptest.Server, *Client, func()) {
	t.Helper()
	hst, _ := Open(t.TempDir())
	hub := NewHub(hst)
	mux := http.NewServeMux()
	hub.Routes(mux)
	mux.HandleFunc("/fwd/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/fwd")
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		hub.Forward(w, r, "france", path, User{Name: "admin (US)", Rights: "a"})
	})
	srv := httptest.NewServer(mux)
	_, code, err := hub.Add("France", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	nst, _ := Open(t.TempDir())
	node := NewClient(nst)
	node.Handler = nodeHandler
	node.State = func() map[string]any {
		return map[string]any{"servers": []map[string]any{{"id": "css-1", "name": "FR 1"}}, "version": "test"}
	}
	if _, err := node.Join(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	if _, err := node.Join(context.Background(), code); err == nil {
		t.Fatal("a code works twice")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go node.Run(ctx)
	for k := 0; k < 100; k++ {
		if on, _ := hub.Online("france"); on && hub.Remote("france") != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if on, _ := hub.Online("france"); !on {
		t.Fatal("not connected")
	}
	return hub, srv, node, func() { cancel(); srv.Close() }
}

func TestForward(t *testing.T) {
	big := bytes.Repeat([]byte("0123456789"), 300_000) // 3 MB: several pieces
	nm := http.NewServeMux()
	nm.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		w.Header().Set("X-Test", "yes")
		fmt.Fprintf(w, "hello %s %s", u.Name, r.URL.Query().Get("q"))
	})
	nm.HandleFunc("POST /api/echo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		io.Copy(w, r.Body)
	})
	nm.HandleFunc("GET /api/big", func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
	nm.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		for k := 0; ; k++ {
			fmt.Fprintf(w, "line %d\n", k)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})
	hub, srv, _, stop := linked(t, nm)
	defer stop()

	if rm := hub.Remote("france"); len(rm.IDs) != 1 || rm.IDs[0] != "france~css-1" || !strings.Contains(string(rm.Servers[0]), `"machine":"france"`) {
		t.Fatalf("%+v", rm)
	}
	resp, err := http.Get(srv.URL + "/fwd/api/hello?q=1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "hello admin (US) 1" || resp.Header.Get("X-Test") != "yes" {
		t.Fatalf("%q %v", b, resp.Header)
	}
	// an upload: its body comes from the hub
	resp, err = http.Post(srv.URL+"/fwd/api/echo", "application/octet-stream", bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated || !bytes.Equal(b, big) {
		t.Fatalf("echo: %d, %d bytes", resp.StatusCode, len(b))
	}
	// a download in pieces
	resp, _ = http.Get(srv.URL + "/fwd/api/big")
	b, _ = io.ReadAll(resp.Body)
	if !bytes.Equal(b, big) {
		t.Fatalf("big: %d bytes", len(b))
	}
	// a stream: lines arrive as they come; the browser leaving stops it on the node
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/fwd/api/stream", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rd := bufio.NewReader(resp.Body)
	for k := 0; k < 3; k++ {
		if l, err := rd.ReadString('\n'); err != nil || !strings.HasPrefix(l, "line ") {
			t.Fatalf("%q %v", l, err)
		}
	}
	cancel()
	resp.Body.Close()
	// not through the link: its own endpoints, other paths
	resp, _ = http.Get(srv.URL + "/fwd/api/link/feed")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatal("link endpoint through the link:", resp.Status)
	}
}

func TestOfflineAndRemoved(t *testing.T) {
	nm := http.NewServeMux()
	hub, srv, node, stop := linked(t, nm)
	defer stop()
	if err := hub.Remove("france"); err != nil {
		t.Fatal(err)
	}
	resp, _ := http.Get(srv.URL + "/fwd/api/x")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal(resp.Status)
	}
	for k := 0; k < 100 && !strings.Contains(node.Status().Error, "does not know"); k++ {
		time.Sleep(20 * time.Millisecond)
	}
	if st := node.Status(); st.Connected || !strings.Contains(st.Error, "does not know") {
		t.Fatalf("%+v", st)
	}
	// a forged token
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/link/feed", nil)
	req.Header.Set("Authorization", "Bearer france.nope")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal(resp.Status)
	}
}

// An upload slower than the limits, but going on, is answered; one the node stops reading is not waited for forever.
func TestSlowUpload(t *testing.T) {
	readLimit, workLimit, waitCheck = 300*time.Millisecond, 300*time.Millisecond, 50*time.Millisecond
	defer func() { readLimit, workLimit, waitCheck = 2*time.Minute, 30*time.Minute, 5*time.Second }()
	nm := http.NewServeMux()
	nm.HandleFunc("POST /api/slow", func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		fmt.Fprint(w, n)
	})
	nm.HandleFunc("POST /api/stuck", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 10)
		r.Body.Read(buf)
		<-r.Context().Done() // reads a little, then nothing
	})
	_, srv, _, stop := linked(t, nm)
	defer stop()
	// 1.5 s of body, a little every 100 ms: five times the limits
	pr, pw := io.Pipe()
	go func() {
		for k := 0; k < 15; k++ {
			pw.Write(bytes.Repeat([]byte("x"), 1000))
			time.Sleep(100 * time.Millisecond)
		}
		pw.Close()
	}()
	resp, err := http.Post(srv.URL+"/fwd/api/slow", "application/octet-stream", pr)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(b) != "15000" {
		t.Fatalf("%d %q", resp.StatusCode, b)
	}
	pr2, pw2 := io.Pipe()
	go func() {
		for k := 0; k < 100; k++ {
			if _, err := pw2.Write(bytes.Repeat([]byte("x"), 1000)); err != nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		pw2.Close()
	}()
	start := time.Now()
	resp, err = http.Post(srv.URL+"/fwd/api/stuck", "application/octet-stream", pr2)
	if err == nil && resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("stuck: %d", resp.StatusCode)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("waited too long")
	}
}

// A code of another hub moves a linked node to it at once; unlinking ends the connection at once.
func TestMoveToAnotherHub(t *testing.T) {
	hub, srv, node, stop := linked(t, http.NewServeMux())
	defer stop()
	hst2, _ := Open(t.TempDir())
	hub2 := NewHub(hst2)
	mux := http.NewServeMux()
	hub2.Routes(mux)
	srv2 := httptest.NewServer(mux)
	defer srv2.Close()
	_, code, err := hub2.Add("France", srv2.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := node.Join(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	for k := 0; k < 100; k++ {
		on2, _ := hub2.Online("france")
		on1, _ := hub.Online("france")
		if on2 && !on1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	on2, _ := hub2.Online("france")
	on1, _ := hub.Online("france")
	if !on2 || on1 {
		t.Fatalf("new hub %v, old hub %v", on2, on1)
	}
	if err := node.Unlink(); err != nil {
		t.Fatal(err)
	}
	for k := 0; k < 100; k++ {
		if on, _ := hub2.Online("france"); !on {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if on, _ := hub2.Online("france"); on {
		t.Fatal("still connected after unlinking")
	}
	_ = srv
}

// The node's state goes compressed, without the games when they did not change, often only while someone has the
// hub's web page open.
func TestPushWatched(t *testing.T) {
	pushEvery, idleEvery = 50*time.Millisecond, time.Second
	defer func() { pushEvery, idleEvery = 2*time.Second, 30*time.Second }()
	var mu sync.Mutex
	var pushes, gzipped, withGames int
	hst, _ := Open(t.TempDir())
	hub := NewHub(hst)
	mux := http.NewServeMux()
	hub.Routes(mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/link/push" {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			pushes++
			if r.Header.Get("Content-Encoding") == "gzip" {
				gzipped++
				z, _ := gzip.NewReader(bytes.NewReader(b))
				plain, _ := io.ReadAll(z)
				if bytes.Contains(plain, []byte(`"games"`)) {
					withGames++
				}
			}
			mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()
	_, code, _ := hub.Add("France", srv.URL)
	nst, _ := Open(t.TempDir())
	node := NewClient(nst)
	node.Handler = http.NewServeMux()
	var n atomic.Int64
	node.State = func() map[string]any { // its numbers change every time, as CPU and network do
		return map[string]any{"servers": []map[string]any{{"id": "css-1", "cpu": n.Add(1)}}, "games": []string{"css", "tf2"}, "version": "test"}
	}
	if _, err := node.Join(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go node.Run(ctx)
	count := func() (int, int, int) { mu.Lock(); defer mu.Unlock(); return pushes, gzipped, withGames }

	// nobody looks: the first push, then one a second at most
	time.Sleep(700 * time.Millisecond)
	p0, _, _ := count()
	if p0 < 1 || p0 > 2 {
		t.Fatal("pushes while nobody looks:", p0)
	}
	// someone opens the page: at once, then as often as it changes
	hub.SetWatched(true)
	time.Sleep(500 * time.Millisecond)
	p1, gz, games := count()
	if p1-p0 < 5 || gz < p1-1 {
		t.Fatal("pushes while watched:", p0, p1, gz)
	}
	if games != 1 {
		t.Fatal("the games went", games, "times compressed: once is enough")
	}
	if rm := hub.Remote("france"); rm == nil || string(rm.Games) != `["css","tf2"]` {
		t.Fatalf("the hub lost the games: %+v", rm)
	}
	// the page closed: slow again
	hub.SetWatched(false)
	time.Sleep(100 * time.Millisecond)
	p2, _, _ := count()
	time.Sleep(600 * time.Millisecond)
	if p3, _, _ := count(); p3-p2 > 1 {
		t.Fatal("pushes after the page closed:", p3-p2)
	}
}
