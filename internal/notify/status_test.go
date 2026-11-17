package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDiscord is a webhook as Discord answers it: messages posted, edited and deleted, each with its attachments;
// an edit keeps the attachments it names, and no other.
type fakeDiscord struct {
	t        *testing.T
	mu       sync.Mutex
	next     int
	messages map[string]*fakeMessage
	slow     int // answers "slow down" this many times
	calls    []string
}

type fakeMessage struct {
	Payload map[string]any
	Files   map[string]string // attachment number → file name
	Data    map[string][]byte // file name → content
	Edits   int
}

func newFakeDiscord(t *testing.T) (*fakeDiscord, hook) {
	f := &fakeDiscord{t: t, next: 1000, messages: map[string]*fakeMessage{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, hook{base: srv.URL + "/api/v10/webhooks/42/secret", query: url.Values{}}
}

func (f *fakeDiscord) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if f.slow > 0 {
		f.slow--
		w.Header().Set("Retry-After", "0.01")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v10/webhooks/42/secret")
	if !ok {
		http.Error(w, `{"message":"Unknown Webhook"}`, http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(rest, "/messages/")
	var payload map[string]any
	uploads := map[string][]byte{} // "files[n]" → content
	if r.Method != http.MethodDelete {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				f.t.Errorf("multipart: %v", err)
			}
			_ = json.Unmarshal([]byte(r.FormValue("payload_json")), &payload)
			for name, hs := range r.MultipartForm.File {
				fh, _ := hs[0].Open()
				uploads[name], _ = io.ReadAll(fh)
			}
		} else {
			_ = json.NewDecoder(r.Body).Decode(&payload)
		}
	}
	var m *fakeMessage
	switch {
	case r.Method == http.MethodPost && rest == "":
		if r.URL.Query().Get("wait") != "true" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		f.next++
		id = fmt.Sprint(f.next)
		m = &fakeMessage{Files: map[string]string{}, Data: map[string][]byte{}}
		f.messages[id] = m
	case r.Method == http.MethodPatch && f.messages[id] != nil:
		m = f.messages[id]
		m.Edits++
	case r.Method == http.MethodDelete && f.messages[id] != nil:
		delete(f.messages, id)
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		http.Error(w, `{"message":"Unknown Message","code":10008}`, http.StatusNotFound)
		return
	}
	m.Payload = payload
	// the attachments named stay (by their number), those sent are added; the others go
	files, data := map[string]string{}, map[string][]byte{}
	list, _ := payload["attachments"].([]any)
	for _, x := range list {
		a := x.(map[string]any)
		switch n := a["id"].(type) {
		case string:
			name, ok := m.Files[n]
			if !ok {
				http.Error(w, `{"message":"Invalid Form Body"}`, http.StatusBadRequest)
				return
			}
			files[n], data[name] = name, m.Data[name]
		case float64:
			up, ok := uploads[fmt.Sprintf("files[%d]", int(n))]
			if !ok {
				http.Error(w, `{"message":"Invalid Form Body"}`, http.StatusBadRequest)
				return
			}
			f.next++
			name := a["filename"].(string)
			files[fmt.Sprint(f.next)], data[name] = name, up
		}
	}
	m.Files, m.Data = files, data
	var atts []map[string]any
	for n, name := range files {
		atts = append(atts, map[string]any{"id": n, "filename": name})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "attachments": atts})
}

func (f *fakeDiscord) message(id string) *fakeMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messages[id]
}

func embedOf(m *fakeMessage) map[string]any {
	return m.Payload["embeds"].([]any)[0].(map[string]any)
}

// A card is posted once, then edited in place; its icon and chart are sent with it, and kept by an edit that
// does not change them.
func TestCardPostEditDelete(t *testing.T) {
	f, h := newFakeDiscord(t)
	c := Card{Name: "CS2 - 1", Hostname: "SnipeZilla | *Dust2* 24/7", Game: "Counter-Strike 2", Address: "1.2.3.4:27015",
		Icon: []byte("PNG icon"), IconName: "cs2.png", State: CardOnline, Map: "de_dust2", Players: 12, Max: 32, Bots: 2,
		Since: time.Unix(1700000000, 0), Join: "https://example.com/join?a=1.2.3.4:27015", Chart: []byte("PNG chart 1")}
	msg, err := h.post("SZM", c)
	if err != nil {
		t.Fatal(err)
	}
	m := f.message(msg.ID)
	if m == nil || msg.Files["cs2.png"] == "" || msg.Files[chartName] == "" || len(msg.Files) != 2 {
		t.Fatalf("%+v", msg)
	}
	if string(m.Data["cs2.png"]) != "PNG icon" || string(m.Data[chartName]) != "PNG chart 1" || m.Payload["username"] != "SZM" {
		t.Fatalf("%v %q", m.Payload, m.Data)
	}
	e := embedOf(m)
	if e["title"] != `SnipeZilla \| \*Dust2\* 24/7` || !strings.HasPrefix(e["description"].(string), "🟢 **Online** · <t:1700000000:R>") {
		t.Fatalf("%v", e)
	}
	if e["author"].(map[string]any)["icon_url"] != "attachment://cs2.png" || e["image"].(map[string]any)["url"] != "attachment://"+chartName {
		t.Fatalf("%v", e)
	}
	fields := e["fields"].([]any)
	got := ""
	for _, x := range fields {
		fd := x.(map[string]any)
		got += fd["name"].(string) + "=" + fd["value"].(string) + ";"
	}
	if got != "Map=`de_dust2`;Players=`12 / 32` + 2 bots;Address=`1.2.3.4:27015`\n[Join via Steam](https://example.com/join?a=1.2.3.4:27015);" {
		t.Fatal(got)
	}

	// the players change: an edit with no file, the two attachments kept
	c.Players = 14
	iconID, chartID := msg.Files["cs2.png"], msg.Files[chartName]
	msg2, err := h.edit(msg, c, false)
	if err != nil {
		t.Fatal(err)
	}
	if msg2.ID != msg.ID || msg2.Files["cs2.png"] != iconID || msg2.Files[chartName] != chartID || m.Edits != 1 {
		t.Fatalf("%+v", msg2)
	}
	if !strings.Contains(fmt.Sprint(embedOf(m)["fields"]), "14 / 32") {
		t.Fatal(embedOf(m))
	}
	// a new chart: sent, in the place of the old one; the icon stays
	c.Chart = []byte("PNG chart 2")
	msg3, err := h.edit(msg2, c, true)
	if err != nil {
		t.Fatal(err)
	}
	if msg3.Files["cs2.png"] != iconID || msg3.Files[chartName] == chartID || len(msg3.Files) != 2 || string(m.Data[chartName]) != "PNG chart 2" {
		t.Fatalf("%+v %q", msg3, m.Data)
	}
	// offline: no map, no players
	c.State, c.Since = CardOffline, time.Time{}
	if _, err := h.edit(msg3, c, false); err != nil {
		t.Fatal(err)
	}
	e = embedOf(m)
	if e["description"] != "🔴 **Offline**" || len(e["fields"].([]any)) != 1 || int(e["color"].(float64)) != 0xE5383B {
		t.Fatalf("%v", e)
	}
	// deleted in Discord by someone: the edit says so (the card is posted again); deleting it twice is no error
	if err := h.delete(msg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.edit(msg3, c, false); err != ErrGone {
		t.Fatal(err)
	}
	if err := h.delete(msg.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.messages) != 0 {
		t.Fatal(f.messages)
	}
}

// Told to slow down, Discord is asked once more after the wait it gives; an answer that says no is told in its
// words, without the webhook's secret.
func TestCardSlowDownAndErrors(t *testing.T) {
	f, h := newFakeDiscord(t)
	f.slow = 1
	msg, err := h.post("", Card{Name: "x", State: CardStarting})
	if err != nil || msg.ID == "" {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Fatal(f.calls)
	}
	f.slow = 2
	if _, err := h.edit(msg, Card{Name: "x", State: CardOnline}, false); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatal(err)
	}
	bad := hook{base: strings.Replace(h.base, "/42/secret", "/42/wrong", 1), query: url.Values{}}
	_, err = bad.post("", Card{Name: "x", State: CardOnline})
	if err == nil || !strings.Contains(err.Error(), "Unknown Webhook") || strings.Contains(err.Error(), "wrong") {
		t.Fatal(err)
	}
	gone := hook{base: "http://127.0.0.1:1/api/v10/webhooks/42/secret", query: url.Values{}}
	if _, err = gone.post("", Card{Name: "x"}); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

func TestHookOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://discord.com/api/webhooks/123/abc-DEF_9":                      "https://discord.com/api/v10/webhooks/123/abc-DEF_9",
		"https://discord.com/api/webhooks/123/abc/":                           "https://discord.com/api/v10/webhooks/123/abc",
		"https://ptb.discord.com/api/webhooks/123/abc?wait=true&thread_id=77": "https://ptb.discord.com/api/v10/webhooks/123/abc",
		"https://discord.com/api/webhooks/123":                                "",
		"https://example.com/api/webhooks/123/abc":                            "",
	} {
		h, err := hookOf(in)
		if (err != nil) != (want == "") || h.base != want {
			t.Errorf("%s: %q %v", in, h.base, err)
		}
	}
	h, _ := hookOf("https://discord.com/api/webhooks/123/abc?thread_id=77")
	if got := h.url("/messages/5", false); got != "https://discord.com/api/v10/webhooks/123/abc/messages/5?thread_id=77" {
		t.Fatal(got)
	}
	if got := h.url("", true); got != "https://discord.com/api/v10/webhooks/123/abc?thread_id=77&wait=true" {
		t.Fatal(got)
	}
	if HookID("https://discord.com/api/webhooks/123/abc") != "123" || HookID("nope") != "" {
		t.Fatal("HookID")
	}
}

// What a card shows tells whether it needs an edit: the same card, the same sum, with or without its chart.
func TestCardSum(t *testing.T) {
	c := Card{Name: "a", State: CardOnline, Players: 3, Max: 10, Address: "1.2.3.4:1"}
	s := CardSum(c)
	c.Chart = []byte("x")
	if CardSum(c) != s {
		t.Fatal("the chart changed the sum")
	}
	c.Players = 4
	if CardSum(c) == s {
		t.Fatal("the players did not")
	}
}

// The chart is a PNG of the size Discord shows well, whatever the hours hold.
func TestChart(t *testing.T) {
	for _, hours := range [][]Hour{nil, make([]Hour, 24), {{Avg: 3.5, Max: 9, Has: true}, {Has: true}, {}}} {
		b := Chart(hours)
		if len(b) < 100 || string(b[1:4]) != "PNG" || len(b) > 20000 {
			t.Fatalf("%d bytes", len(b))
		}
	}
	a := make([]Hour, 24)
	a[23] = Hour{Avg: 2, Max: 4, Has: true}
	b := append([]Hour(nil), a...)
	b[23].Max = 5
	if ChartSum(Chart(a)) == ChartSum(Chart(b)) || ChartSum(Chart(a)) != ChartSum(Chart(a)) {
		t.Fatal("charts not told apart")
	}
}
