package link

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

// Hub is the side that shows the others: it forwards its users' requests to the nodes, and keeps what they push.
type Hub struct {
	st *Store

	// OnEvents receives the events a node pushed (their server IDs already with the node's prefix).
	OnEvents func(node Node, evs []events.Event)
	// OnJoin: a node joined with its code. OnOnline: a node connected, or its connection ended.
	OnJoin   func(node Node)
	OnOnline func(node Node, online bool)

	mu      sync.Mutex
	conns   map[string]*conn     // the nodes connected now
	remotes map[string]*Remote   // what each node pushed last
	pend    map[string]*pending  // requests waiting for a node's answer
	tried   map[string]time.Time // failed joins, by node ID: slowed down

	// watched: someone has this hub's web page open. The nodes are told: they send their state every 2 s while it
	// is, every 30 s when it is not (their events go at once either way).
	watched atomic.Bool
}

// SetWatched says whether someone has the hub's web page open; the nodes are told when it changes.
func (h *Hub) SetWatched(on bool) {
	if h.watched.Swap(on) == on {
		return
	}
	h.mu.Lock()
	ids := make([]string, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		go h.send(id, "watch", map[string]bool{"on": on})
	}
}

// Remote is what a node pushed last.
type Remote struct {
	Version string            `json:"version"`
	Machine json.RawMessage   `json:"machine"`
	Games   json.RawMessage   `json:"games"`
	Servers []json.RawMessage `json:"-"` // with the node's prefix and its ID in "machine"
	IDs     []string          `json:"-"` // their IDs, with the prefix
	At      time.Time         `json:"at"`
}

type conn struct {
	out   chan []byte
	done  chan struct{}
	since time.Time
}

type pending struct {
	node     string
	body     io.ReadCloser
	activity atomic.Int64 // when the node last read some of the body (unix nano)
	bodyDone atomic.Bool  // ... all of it
	head     chan resHead
	data     chan []byte
	end      chan struct{}
	gone     chan struct{}
	once     sync.Once
}

type resHead struct {
	Status int                 `json:"status"`
	Header map[string][]string `json:"header"`
}

// request is what the hub sends the node to do.
type request struct {
	ID     string              `json:"id"`
	Method string              `json:"method"`
	Path   string              `json:"path"` // with its query
	Header map[string][]string `json:"header"`
	Length int64               `json:"length"` // of its body: -1 not known, 0 none
	User   User                `json:"user"`
}

// NewHub starts the hub side.
func NewHub(st *Store) *Hub {
	return &Hub{st: st, conns: map[string]*conn{}, remotes: map[string]*Remote{}, pend: map[string]*pending{}, tried: map[string]time.Time{}}
}

/* ---------------------------------------------------------------------------------------------------------------
 * Nodes: added, removed, their state
 * ------------------------------------------------------------------------------------------------------------- */

var errLoopbackHub = errors.New("localhost is this machine itself: the other machine can not reach the hub there. " +
	"Put the hub's address on the internet in This hub's address (https://your-domain/), then make the code")

// Add makes a node and the code it joins with (good once, for 30 minutes).
func (h *Hub) Add(name, hubURL string) (Node, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Node{}, "", errors.New("give the machine a name: US East, EU…")
	}
	u, err := HubURL(hubURL)
	if err != nil {
		return Node{}, "", err
	}
	if loopback(u) {
		return Node{}, "", errLoopbackHub
	}
	sec := secret()
	var n Node
	err = h.st.Update(func(f *File) error {
		if f.Hub != nil {
			return fmt.Errorf("this manager is itself linked to %s: unlink it first to make it a hub", f.Hub.Hub)
		}
		for _, o := range f.Nodes {
			if strings.EqualFold(o.Name, name) {
				return fmt.Errorf("there is already a machine named %s", o.Name)
			}
		}
		n = Node{ID: NodeID(name, func(id string) bool { _, ok := f.Node(id); return ok }), Name: name,
			PairHash: hash(sec), PairUntil: now().Add(30 * time.Minute).Unix(), Added: now().Unix()}
		f.Nodes = append(f.Nodes, n)
		return nil
	})
	if err != nil {
		return Node{}, "", err
	}
	return n, pairCode{URL: u, ID: n.ID, Secret: sec, Hub: h.st.Get().Name}.String(), nil
}

// NewCode makes a fresh code for a node (one that never joined, or to link it again: its old token stops working).
func (h *Hub) NewCode(id, hubURL string) (string, error) {
	u, err := HubURL(hubURL)
	if err != nil {
		return "", err
	}
	if loopback(u) {
		return "", errLoopbackHub
	}
	sec := secret()
	err = h.st.Update(func(f *File) error {
		for k := range f.Nodes {
			if f.Nodes[k].ID == id {
				f.Nodes[k].PairHash, f.Nodes[k].PairUntil = hash(sec), now().Add(30*time.Minute).Unix()
				return nil
			}
		}
		return errors.New("no such machine")
	})
	if err != nil {
		return "", err
	}
	return pairCode{URL: u, ID: id, Secret: sec, Hub: h.st.Get().Name}.String(), nil
}

// Remove forgets a node: its token stops working at once.
func (h *Hub) Remove(id string) error {
	err := h.st.Update(func(f *File) error {
		for k := range f.Nodes {
			if f.Nodes[k].ID == id {
				f.Nodes = append(f.Nodes[:k], f.Nodes[k+1:]...)
				return nil
			}
		}
		return errors.New("no such machine")
	})
	if err != nil {
		return err
	}
	h.mu.Lock()
	if c := h.conns[id]; c != nil {
		close(c.done)
		delete(h.conns, id)
	}
	delete(h.remotes, id)
	h.mu.Unlock()
	return nil
}

// Online tells whether a node is connected now, and since when.
func (h *Hub) Online(id string) (bool, time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c := h.conns[id]; c != nil {
		return true, c.since
	}
	return false, time.Time{}
}

// Remote gives what a node pushed last (nil: nothing yet).
func (h *Hub) Remote(id string) *Remote {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.remotes[id]
}

/* ---------------------------------------------------------------------------------------------------------------
 * What the nodes call (no session: their token)
 * ------------------------------------------------------------------------------------------------------------- */

// Routes adds the hub's side of the link.
func (h *Hub) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/link/join", h.join)
	mux.HandleFunc("GET /api/link/feed", h.auth(h.feed))
	mux.HandleFunc("GET /api/link/body/{rid}", h.auth(h.body))
	mux.HandleFunc("POST /api/link/res/{rid}", h.auth(h.res))
	mux.HandleFunc("POST /api/link/push", h.auth(h.push))
}

func linkErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// auth checks the node's token: "Authorization: Bearer <node ID>.<token>".
func (h *Hub) auth(next func(w http.ResponseWriter, r *http.Request, n Node)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, tok, _ := strings.Cut(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
		n, ok := h.st.Get().Node(id)
		if !ok || !sameHash(tok, n.TokenHash) {
			linkErr(w, http.StatusUnauthorized, "this machine is not linked to this hub (any more)")
			return
		}
		next(w, r, n)
	}
}

func (h *Hub) join(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		linkErr(w, http.StatusBadRequest, "bad request")
		return
	}
	h.mu.Lock()
	if t := h.tried[in.ID]; now().Sub(t) < 2*time.Second {
		h.mu.Unlock()
		linkErr(w, http.StatusTooManyRequests, "too many tries: wait a moment")
		return
	}
	h.mu.Unlock()
	tok := secret()
	var name string
	err := h.st.Update(func(f *File) error {
		for k := range f.Nodes {
			n := &f.Nodes[k]
			if n.ID != in.ID {
				continue
			}
			if n.PairHash == "" || now().Unix() > n.PairUntil {
				return errors.New("this link code has expired or was used: make a new one on the hub (Settings, Machines)")
			}
			if !sameHash(in.Secret, n.PairHash) {
				return errors.New("this link code is wrong: copy it again")
			}
			n.TokenHash, n.PairHash, n.PairUntil = hash(tok), "", 0
			return nil
		}
		return errors.New("the hub does not know this machine (removed?): make a new code on the hub")
	})
	if err != nil {
		h.mu.Lock()
		h.tried[in.ID] = now()
		h.mu.Unlock()
		linkErr(w, http.StatusForbidden, err.Error())
		return
	}
	name = h.st.Get().Name
	if n, ok := h.st.Get().Node(in.ID); ok && h.OnJoin != nil {
		h.OnJoin(n)
	}
	// a node linked again: its old connection goes
	h.mu.Lock()
	if c := h.conns[in.ID]; c != nil {
		close(c.done)
		delete(h.conns, in.ID)
	}
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tok, "hub": name})
}

// feed streams to the node what it must do (Server-Sent Events). A newer connection of the same node replaces it.
func (h *Hub) feed(w http.ResponseWriter, r *http.Request, n Node) {
	fl, ok := w.(http.Flusher)
	if !ok {
		linkErr(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	c := &conn{out: make(chan []byte, 512), done: make(chan struct{}), since: now()}
	h.mu.Lock()
	if old := h.conns[n.ID]; old != nil {
		close(old.done)
	}
	h.conns[n.ID] = c
	h.mu.Unlock()
	if h.OnOnline != nil {
		h.OnOnline(n, true)
	}
	defer func() {
		h.mu.Lock()
		last := h.conns[n.ID] == c
		if last {
			delete(h.conns, n.ID)
		}
		h.mu.Unlock()
		h.dropPending(n.ID)
		if last && h.OnOnline != nil {
			h.OnOnline(n, false)
		}
	}()
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Accel-Buffering", "no")
	// "watch" also tells the node this hub takes compressed pushes, without the games when they did not change
	b, _ := json.Marshal(map[string]any{"hub": h.st.Get().Name, "id": n.ID, "name": n.Name, "watch": h.watched.Load()})
	fmt.Fprintf(w, "event: hello\ndata: %s\n\n", b)
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.done:
			return
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		case f := <-c.out:
			if _, err := w.Write(f); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

// send queues an event for a node.
func (h *Hub) send(id, kind string, v any) error {
	b, _ := json.Marshal(v)
	h.mu.Lock()
	c := h.conns[id]
	h.mu.Unlock()
	if c == nil {
		return errOffline
	}
	select {
	case c.out <- []byte("event: " + kind + "\ndata: " + string(b) + "\n\n"):
		return nil
	case <-c.done:
		return errOffline
	case <-time.After(5 * time.Second):
		return errors.New("the machine does not keep up")
	}
}

var errOffline = errors.New("offline")

func (h *Hub) getPending(rid, node string) *pending {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.pend[rid]; p != nil && p.node == node {
		return p
	}
	return nil
}

// dropPending ends the requests of a node that went away.
func (h *Hub) dropPending(node string) {
	h.mu.Lock()
	var list []*pending
	for _, p := range h.pend {
		if p.node == node {
			list = append(list, p)
		}
	}
	h.mu.Unlock()
	for _, p := range list {
		p.once.Do(func() { close(p.gone) })
	}
}

// body gives the node the body of a request (an upload): straight from the browser.
func (h *Hub) body(w http.ResponseWriter, r *http.Request, n Node) {
	p := h.getPending(r.PathValue("rid"), n.ID)
	if p == nil || p.body == nil {
		linkErr(w, http.StatusGone, "no such request")
		return
	}
	b := p.body
	p.body = nil
	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := io.Copy(w, &activeReader{r: b, p: p}); err == nil {
		p.bodyDone.Store(true)
	}
	p.activity.Store(now().UnixNano())
}

// activeReader notes when the node reads the body: the answer comes after it.
type activeReader struct {
	r io.Reader
	p *pending
}

func (a *activeReader) Read(b []byte) (int, error) {
	n, err := a.r.Read(b)
	a.p.activity.Store(now().UnixNano())
	return n, err
}

// res receives a piece of the node's answer: its head first (X-Link-Head), then the body, in order; end=1 the last.
func (h *Hub) res(w http.ResponseWriter, r *http.Request, n Node) {
	p := h.getPending(r.PathValue("rid"), n.ID)
	if p == nil {
		linkErr(w, http.StatusGone, "the browser left")
		return
	}
	if hv := r.Header.Get("X-Link-Head"); hv != "" {
		var hd resHead
		if b, err := base64.RawURLEncoding.DecodeString(hv); err != nil || json.Unmarshal(b, &hd) != nil {
			linkErr(w, http.StatusBadRequest, "bad head")
			return
		}
		select {
		case p.head <- hd:
		default:
		}
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		linkErr(w, http.StatusBadRequest, "bad body")
		return
	}
	if len(data) > 0 {
		select {
		case p.data <- data:
		case <-p.gone:
			linkErr(w, http.StatusGone, "the browser left")
			return
		case <-time.After(2 * time.Minute):
			linkErr(w, http.StatusGone, "the browser does not read")
			return
		}
	}
	if r.URL.Query().Get("end") == "1" {
		select {
		case <-p.end:
		default:
			close(p.end)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// push receives the node's servers, machine, games and new events.
func (h *Hub) push(w http.ResponseWriter, r *http.Request, n Node) {
	var in struct {
		Version string            `json:"version"`
		Machine json.RawMessage   `json:"machine"`
		Games   json.RawMessage   `json:"games"`
		Servers []json.RawMessage `json:"servers"`
		Events  []events.Event    `json:"events"`
	}
	body := io.Reader(r.Body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		z, err := gzip.NewReader(r.Body)
		if err != nil {
			linkErr(w, http.StatusBadRequest, "bad push")
			return
		}
		defer z.Close()
		body = z
	}
	if err := json.NewDecoder(io.LimitReader(body, 32<<20)).Decode(&in); err != nil {
		linkErr(w, http.StatusBadRequest, "bad push")
		return
	}
	if in.Servers != nil || in.Machine != nil {
		servers := make([]json.RawMessage, 0, len(in.Servers))
		ids := make([]string, 0, len(in.Servers))
		for _, raw := range in.Servers {
			var v map[string]any
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			id, _ := v["id"].(string)
			if id == "" {
				continue
			}
			v["id"], v["machine"], v["machine_name"] = n.ID+Sep+id, n.ID, n.Name
			b, _ := json.Marshal(v)
			servers = append(servers, b)
			ids = append(ids, n.ID+Sep+id)
		}
		h.mu.Lock()
		if in.Games == nil { // the same as last time: not sent again
			if old := h.remotes[n.ID]; old != nil {
				in.Games = old.Games
			}
		}
		h.remotes[n.ID] = &Remote{Version: in.Version, Machine: in.Machine, Games: in.Games, Servers: servers, IDs: ids, At: now()}
		h.mu.Unlock()
	}
	if len(in.Events) > 0 && h.OnEvents != nil {
		for k := range in.Events {
			e := &in.Events[k]
			if e.Server != "" {
				e.Server = n.ID + Sep + e.Server
			}
			e.Machine = n.ID
		}
		h.OnEvents(n, in.Events)
	}
	w.WriteHeader(http.StatusNoContent)
}

/* ---------------------------------------------------------------------------------------------------------------
 * Forwarding a user's request
 * ------------------------------------------------------------------------------------------------------------- */

// hop-by-hop and the hub's own headers: not passed on
var skipHeader = map[string]bool{"Connection": true, "Keep-Alive": true, "Transfer-Encoding": true, "Te": true,
	"Trailer": true, "Upgrade": true, "Proxy-Authorization": true, "Proxy-Authenticate": true, "Cookie": true,
	"Set-Cookie": true, "Authorization": true, "Content-Security-Policy": true, "X-Frame-Options": true,
	"X-Content-Type-Options": true, "Referrer-Policy": true}

// How long the hub waits for a node's answer: as long as the node reads the body (an upload), and readLimit when it
// stops reading it; workLimit after (or without) one. Checked every waitCheck.
var (
	readLimit = 2 * time.Minute
	workLimit = 30 * time.Minute
	waitCheck = 5 * time.Second
)

// Forward has a node answer a user's request: path is the node's own ("/api/servers/css-1/console?x=1").
func (h *Hub) Forward(w http.ResponseWriter, r *http.Request, nodeID, path string, u User) {
	n, ok := h.st.Get().Node(nodeID)
	if !ok {
		linkErr(w, http.StatusNotFound, "No such machine.")
		return
	}
	p := &pending{node: nodeID, head: make(chan resHead, 1), data: make(chan []byte, 8), end: make(chan struct{}),
		gone: make(chan struct{})}
	length := r.ContentLength
	if r.Body != nil && r.Body != http.NoBody && length != 0 {
		p.body = r.Body
	} else {
		length = 0
	}
	rid := newRequestID()
	h.mu.Lock()
	h.pend[rid] = p
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pend, rid)
		h.mu.Unlock()
		p.once.Do(func() { close(p.gone) })
	}()
	hdr := map[string][]string{}
	for k, v := range r.Header {
		if !skipHeader[k] {
			hdr[k] = v
		}
	}
	req := request{ID: rid, Method: r.Method, Path: path, Header: hdr, Length: length, User: u}
	if err := h.send(nodeID, "req", req); err != nil {
		linkErr(w, http.StatusBadGateway, n.Name+" is not connected to this hub right now.")
		return
	}
	cancel := func() { _ = h.send(nodeID, "cancel", map[string]string{"id": rid}) }
	// The answer may take long: an upload is answered once all of it arrived, a paste once it is copied. No limit
	// while the node reads the body; after (or without one), long enough for any work.
	p.activity.Store(now().UnixNano())
	check := time.NewTicker(waitCheck)
	defer check.Stop()
	var hd resHead
	for waiting := true; waiting; {
		select {
		case hd = <-p.head:
			waiting = false
		case <-p.gone:
			linkErr(w, http.StatusBadGateway, n.Name+" went offline.")
			return
		case <-r.Context().Done():
			cancel()
			return
		case <-check.C:
			idle := now().Sub(time.Unix(0, p.activity.Load()))
			limit := workLimit
			if length != 0 && !p.bodyDone.Load() {
				limit = readLimit // the body is not all read: the node stopped reading it
			}
			if idle > limit {
				cancel()
				linkErr(w, http.StatusGatewayTimeout, n.Name+" did not answer in time.")
				return
			}
		}
	}
	for k, v := range hd.Header {
		if !skipHeader[http.CanonicalHeaderKey(k)] {
			w.Header()[http.CanonicalHeaderKey(k)] = v
		}
	}
	if hd.Status == 0 {
		hd.Status = http.StatusOK
	}
	w.WriteHeader(hd.Status)
	fl, _ := w.(http.Flusher)
	write := func(b []byte) bool {
		if _, err := w.Write(b); err != nil {
			return false
		}
		if fl != nil {
			fl.Flush()
		}
		return true
	}
	for {
		select {
		case b := <-p.data:
			if !write(b) {
				cancel()
				return
			}
		case <-p.end:
			for {
				select {
				case b := <-p.data:
					if !write(b) {
						return
					}
				default:
					return
				}
			}
		case <-p.gone:
			return
		case <-r.Context().Done():
			cancel()
			return
		}
	}
}
