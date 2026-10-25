package link

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SnipeZilla/szm/internal/netcount"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

// Client is the node side: it keeps the link to the hub up, runs the hub's requests and pushes its state.
type Client struct {
	st *Store

	// Handler is this manager's web interface: the hub's requests run on it.
	Handler http.Handler
	// State gives what the hub shows of this machine: its servers ("servers"), machine ("machine"), games
	// ("games"), version ("version").
	State func() map[string]any
	// Log: its events go to the hub.
	Log *events.Log

	http *http.Client

	mu      sync.Mutex
	status  Status
	cancels map[string]context.CancelFunc
	queue   []events.Event // events not at the hub yet
	evSig   chan struct{}
	endSess context.CancelFunc // ends the connection to the hub (linked to another, or unlinked)

	// What the hub of this connection said (its hello, then "watch"): newHub, it takes compressed pushes without the
	// games when they did not change; watched, someone has its web page open (an older hub: always). wake: it just
	// became watched, the state goes at once.
	newHub  atomic.Bool
	watched atomic.Bool
	wake    chan struct{}
}

// dropSession ends the connection to the hub now: Run goes on with the hub of links.json.
func (c *Client) dropSession() {
	c.mu.Lock()
	end := c.endSess
	c.mu.Unlock()
	if end != nil {
		end()
	}
}

// Status is how the link to the hub goes.
type Status struct {
	Connected bool      `json:"connected"`
	Since     time.Time `json:"since,omitempty"` // connected, or failing, since
	Error     string    `json:"error,omitempty"`
}

// NewClient prepares the node side; Run starts it.
func NewClient(st *Store) *Client {
	return &Client{st: st, cancels: map[string]context.CancelFunc{}, evSig: make(chan struct{}, 1), wake: make(chan struct{}, 1),
		http: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: netcount.DialContext, ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout: 90 * time.Second, MaxIdleConnsPerHost: 8, TLSHandshakeTimeout: 15 * time.Second}}}
}

// Status gives how the link goes.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Client) setStatus(connected bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if c.status.Connected != connected || c.status.Since.IsZero() || (msg != "" && c.status.Error == "") {
		c.status.Since = now()
	}
	c.status.Connected, c.status.Error = connected, msg
}

// Join links this manager to a hub with the code the hub gave.
func (c *Client) Join(ctx context.Context, code string) (HubLink, error) {
	pc, err := parseCode(code)
	if err != nil {
		return HubLink{}, err
	}
	u, err := HubURL(pc.URL)
	if err != nil {
		return HubLink{}, err
	}
	if loopback(u) {
		return HubLink{}, fmt.Errorf("this code points to %s, which is this machine itself: it was made on the hub's page "+
			"opened as localhost. On the hub, put its address on the internet in This hub's address (https://your-domain/), "+
			"then New code", u)
	}
	f := c.st.Get()
	if len(f.Nodes) > 0 {
		return HubLink{}, errors.New("this manager is a hub (it has machines linked to it): it can not be linked to another")
	}
	body, _ := json.Marshal(map[string]string{"id": pc.ID, "secret": pc.Secret})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u+"api/link/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return HubLink{}, fmt.Errorf("the hub could not be reached at %s: %v", u, err)
	}
	defer resp.Body.Close()
	var out struct {
		Token string `json:"token"`
		Hub   string `json:"hub"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out)
	if resp.StatusCode != http.StatusOK || out.Token == "" {
		if out.Error == "" {
			out.Error = fmt.Sprintf("the hub answered %s: is %s the manager's address?", resp.Status, u)
		}
		return HubLink{}, errors.New(out.Error)
	}
	hl := HubLink{URL: u, ID: pc.ID, Token: out.Token, Hub: out.Hub}
	if hl.Hub == "" {
		hl.Hub = pc.Hub
	}
	if err := c.st.Update(func(f *File) error { f.Hub = &hl; return nil }); err != nil {
		return HubLink{}, err
	}
	c.setStatus(false, errors.New("connecting…"))
	c.dropSession() // linked to another hub before: it goes to the new one
	return hl, nil
}

// Unlink forgets the hub.
func (c *Client) Unlink() error {
	c.mu.Lock()
	c.queue = nil
	c.mu.Unlock()
	err := c.st.Update(func(f *File) error { f.Hub = nil; return nil })
	c.dropSession()
	return err
}

// Run keeps the link up while this manager is linked to a hub, until ctx ends.
func (c *Client) Run(ctx context.Context) {
	go c.collect(ctx)
	wait := time.Second
	for ctx.Err() == nil {
		hl := c.st.Get().Hub
		if hl == nil {
			c.mu.Lock()
			c.status = Status{}
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-c.st.changed:
			case <-time.After(5 * time.Second):
			}
			continue
		}
		start := now()
		sctx, end := context.WithCancel(ctx)
		c.mu.Lock()
		c.endSess = end
		c.mu.Unlock()
		err := c.session(sctx, *hl)
		dropped := sctx.Err() != nil
		end()
		if ctx.Err() != nil {
			return
		}
		if dropped { // linked to another hub, or unlinked
			wait = time.Second
			continue
		}
		if errors.Is(err, errUnlinked) {
			c.setStatus(false, errors.New("the hub does not know this machine any more (removed there): link it again with a new code"))
		} else {
			c.setStatus(false, err)
		}
		if now().Sub(start) > time.Minute {
			wait = time.Second // it was up for a while: try again soon
		}
		select {
		case <-ctx.Done():
			return
		case <-c.st.changed:
			wait = time.Second
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

var errUnlinked = errors.New("unlinked")

func (c *Client) newRequest(ctx context.Context, hl HubLink, method, path string, body io.Reader) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, method, hl.URL+path, body)
	req.Header.Set("Authorization", "Bearer "+hl.ID+"."+hl.Token)
	return req
}

// session is one connection: the feed, read until it breaks, and the pushes meanwhile.
func (c *Client) session(ctx context.Context, hl HubLink) error {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := c.http.Do(c.newRequest(sctx, hl, http.MethodGet, "api/link/feed", nil))
	if err != nil {
		return fmt.Errorf("the hub (%s) could not be reached: %v", hl.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnlinked
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return fmt.Errorf("the hub answered %s: is %s still the manager's address?", resp.Status, hl.URL)
	}
	c.setStatus(true, nil)
	c.newHub.Store(false) // until its hello says otherwise
	c.watched.Store(true)

	// no news for a minute (the hub pings every 20 s): the connection is dead
	alive := make(chan struct{}, 1)
	go func() {
		t := time.NewTimer(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-alive:
				t.Reset(time.Minute)
			case <-t.C:
				cancel()
				return
			}
		}
	}()
	go c.pusher(sctx, hl)
	defer c.cancelAll()

	rd := bufio.NewReaderSize(resp.Body, 64<<10)
	var kind string
	var data []byte
	for {
		line, err := rd.ReadBytes('\n')
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("the connection to the hub was lost")
		}
		select {
		case alive <- struct{}{}:
		default:
		}
		line = bytes.TrimRight(line, "\r\n")
		switch {
		case len(line) == 0:
			if kind != "" {
				c.handle(sctx, hl, kind, data)
			}
			kind, data = "", nil
		case bytes.HasPrefix(line, []byte("event: ")):
			kind = string(line[7:])
		case bytes.HasPrefix(line, []byte("data: ")):
			data = append(data, line[6:]...)
		}
	}
}

func (c *Client) handle(ctx context.Context, hl HubLink, kind string, data []byte) {
	switch kind {
	case "req":
		var r request
		if json.Unmarshal(data, &r) == nil && r.ID != "" {
			go c.serve(ctx, hl, r)
		}
	case "cancel":
		var v struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &v) == nil {
			c.mu.Lock()
			if f := c.cancels[v.ID]; f != nil {
				f()
			}
			c.mu.Unlock()
		}
	case "watch":
		var v struct {
			On bool `json:"on"`
		}
		if json.Unmarshal(data, &v) == nil {
			c.setWatched(v.On)
		}
	case "hello":
		var v struct {
			Hub   string `json:"hub"`
			Watch *bool  `json:"watch"` // a hub that says it (it takes compressed pushes)
		}
		if json.Unmarshal(data, &v) == nil && v.Watch != nil {
			c.newHub.Store(true)
			c.setWatched(*v.Watch)
		}
		if json.Unmarshal(data, &v) == nil && v.Hub != "" && v.Hub != hl.Hub {
			_ = c.st.Update(func(f *File) error {
				if f.Hub != nil {
					f.Hub.Hub = v.Hub
				}
				return nil
			})
		}
	}
}

func (c *Client) cancelAll() {
	c.mu.Lock()
	for id, f := range c.cancels {
		f()
		delete(c.cancels, id)
	}
	c.mu.Unlock()
}

// serve runs a hub's request on this manager's web interface and sends the answer back.
func (c *Client) serve(ctx context.Context, hl HubLink, r request) {
	rctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancels[r.ID] = cancel
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		delete(c.cancels, r.ID)
		c.mu.Unlock()
	}()
	rw := newResponder(rctx, c, hl, r.ID, cancel)
	if !strings.HasPrefix(r.Path, "/api/") || strings.HasPrefix(r.Path, "/api/link/") {
		rw.Header().Set("Content-Type", "application/json; charset=utf-8")
		rw.WriteHeader(http.StatusForbidden)
		_, _ = rw.Write([]byte(`{"error":"not through the link"}`))
		rw.finish()
		return
	}
	var body io.Reader = http.NoBody
	if r.Length != 0 {
		body = &lazyBody{open: func() (io.ReadCloser, error) {
			resp, err := c.http.Do(c.newRequest(rctx, hl, http.MethodGet, "api/link/body/"+r.ID, nil))
			if err != nil {
				return nil, err
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, fmt.Errorf("body: %s", resp.Status)
			}
			return resp.Body, nil
		}}
	}
	req, err := http.NewRequestWithContext(WithUser(rctx, r.User), r.Method, "http://hub"+r.Path, body)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		rw.finish()
		return
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	req.Header.Set("X-SZM", "1") // the hub checked it came from its page
	if r.Length > 0 {
		req.ContentLength = r.Length
	} else if r.Length < 0 {
		req.ContentLength = -1
	}
	req.RemoteAddr = "hub:0"
	c.Handler.ServeHTTP(rw, req)
	if lb, ok := body.(*lazyBody); ok && lb.rc != nil {
		lb.rc.Close()
	}
	rw.finish()
}

// lazyBody fetches an upload from the hub when the handler first reads it.
type lazyBody struct {
	open func() (io.ReadCloser, error)
	rc   io.ReadCloser
	err  error
}

func (b *lazyBody) Read(p []byte) (int, error) {
	if b.rc == nil && b.err == nil {
		b.rc, b.err = b.open()
	}
	if b.err != nil {
		return 0, b.err
	}
	return b.rc.Read(p)
}

// responder is the http.ResponseWriter of a hub's request: what is written goes to the hub in pieces, sent one
// after the other by a goroutine (a stream like the console flushes often: its lines go together while a piece is on
// its way). A slow hub makes Write wait.
type responder struct {
	ctx    context.Context
	c      *Client
	hl     HubLink
	id     string
	cancel context.CancelFunc

	hdr       http.Header
	mu        sync.Mutex
	cond      *sync.Cond
	status    int
	buf       []byte
	headSent  bool
	flushWant bool
	ended     bool
	dead      bool
	done      chan struct{}
}

const pieceMax = 1 << 20

func newResponder(ctx context.Context, c *Client, hl HubLink, id string, cancel context.CancelFunc) *responder {
	w := &responder{ctx: ctx, c: c, hl: hl, id: id, cancel: cancel, hdr: http.Header{}, done: make(chan struct{})}
	w.cond = sync.NewCond(&w.mu)
	go w.sender()
	return w
}

func (w *responder) Header() http.Header { return w.hdr }

func (w *responder) WriteHeader(code int) {
	w.mu.Lock()
	if w.status == 0 {
		w.status = code
	}
	w.mu.Unlock()
}

func (w *responder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	for len(w.buf) >= 4*pieceMax && !w.dead {
		w.flushWant = true
		w.cond.Broadcast()
		w.cond.Wait()
	}
	if w.dead {
		return 0, errors.New("the hub's request ended")
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) >= pieceMax {
		w.flushWant = true
		w.cond.Broadcast()
	}
	return len(p), nil
}

func (w *responder) Flush() {
	w.mu.Lock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.flushWant = true
	w.cond.Broadcast()
	w.mu.Unlock()
}

// finish sends what is left and waits until it is sent.
func (w *responder) finish() {
	w.mu.Lock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ended = true
	w.cond.Broadcast()
	w.mu.Unlock()
	<-w.done
}

func (w *responder) sender() {
	defer close(w.done)
	stop := context.AfterFunc(w.ctx, func() {
		w.mu.Lock()
		w.dead = true
		w.cond.Broadcast()
		w.mu.Unlock()
	})
	defer stop()
	for {
		w.mu.Lock()
		for !w.flushWant && !w.ended && !w.dead {
			w.cond.Wait()
		}
		if w.dead {
			w.mu.Unlock()
			return
		}
		n := min(len(w.buf), pieceMax)
		piece := append([]byte(nil), w.buf[:n]...)
		w.buf = w.buf[n:]
		end := w.ended && len(w.buf) == 0
		if len(w.buf) == 0 {
			w.flushWant = false
		}
		var head string
		if !w.headSent {
			b, _ := json.Marshal(resHead{Status: w.status, Header: w.hdr.Clone()})
			head = base64.RawURLEncoding.EncodeToString(b)
			w.headSent = true
		}
		w.cond.Broadcast() // room in the buffer
		w.mu.Unlock()

		path := "api/link/res/" + w.id
		if end {
			path += "?end=1"
		}
		req := w.c.newRequest(context.WithoutCancel(w.ctx), w.hl, http.MethodPost, path, bytes.NewReader(piece))
		req.Header.Set("Content-Type", "application/octet-stream")
		if head != "" {
			req.Header.Set("X-Link-Head", head)
		}
		resp, err := w.c.http.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if err != nil || resp.StatusCode >= 300 {
			// the browser left, or the hub is gone: the handler stops
			w.mu.Lock()
			w.dead = true
			w.cond.Broadcast()
			w.mu.Unlock()
			w.cancel()
			return
		}
		if end {
			return
		}
	}
}

/* ---------------------------------------------------------------------------------------------------------------
 * Pushing the state
 * ------------------------------------------------------------------------------------------------------------- */

// collect keeps this machine's events from the start of Run until the hub has them: those of a link that is down
// go when it is up again (the last 1000).
func (c *Client) collect(ctx context.Context) {
	if c.Log == nil {
		return
	}
	evs := c.Log.Subscribe()
	defer c.Log.Unsubscribe(evs)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-evs:
			if !ok {
				return
			}
			c.mu.Lock()
			if c.st.Get().Hub != nil { // linked: kept for the hub
				c.queue = append(c.queue, e)
				if len(c.queue) > 1000 {
					c.queue = append([]events.Event(nil), c.queue[len(c.queue)-1000:]...)
				}
			}
			c.mu.Unlock()
			select {
			case c.evSig <- struct{}{}:
			default:
			}
		}
	}
}

// setWatched notes whether someone has the hub's web page open; when someone just opened it, the state goes now.
func (c *Client) setWatched(on bool) {
	if !c.watched.Swap(on) && on {
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
}

// pusher sends the hub this machine's state and its events. The events go as they come (and those kept while the
// link was down). The state: every 2 s when it changed while someone has the hub's web page open, every 30 s
// otherwise. To a hub that takes it, it goes compressed, and without the games when they did not change.
// pushEvery: how often the state is checked for changes (while watched); idleEvery: how often it goes anyway, and
// while nobody looks (the tests shorten them)
var pushEvery, idleEvery = 2 * time.Second, 30 * time.Second

func (c *Client) pusher(ctx context.Context, hl HubLink) {
	every, idle := pushEvery, idleEvery
	tick := time.NewTicker(every)
	defer tick.Stop()
	var last, games []byte
	var lastAt time.Time
	send := func(withState bool) {
		c.mu.Lock()
		evs := append([]events.Event(nil), c.queue...)
		c.mu.Unlock()
		body := map[string]any{"events": evs}
		var sentGames []byte
		if withState && c.State != nil {
			for k, v := range c.State() {
				body[k] = v
			}
			if g, ok := body["games"]; ok && c.newHub.Load() {
				b, _ := json.Marshal(g)
				if bytes.Equal(b, games) {
					delete(body, "games") // the hub has them
				} else {
					sentGames = b
				}
			}
		}
		b, _ := json.Marshal(body)
		var rd io.Reader = bytes.NewReader(b)
		gz := c.newHub.Load()
		if gz {
			var z bytes.Buffer
			w := gzip.NewWriter(&z)
			w.Write(b)
			w.Close()
			rd = &z
		}
		req := c.newRequest(ctx, hl, http.MethodPost, "api/link/push", rd)
		req.Header.Set("Content-Type", "application/json")
		if gz {
			req.Header.Set("Content-Encoding", "gzip")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 300 {
			c.mu.Lock()
			c.queue = c.queue[min(len(evs), len(c.queue)):] // those sent; newer ones wait
			c.mu.Unlock()
			if sentGames != nil {
				games = sentGames
			}
		}
	}
	state := func() []byte {
		if c.State == nil {
			return nil
		}
		b, _ := json.Marshal(c.State())
		return bytes.ReplaceAll(b, []byte(`"uptime"`), nil)
	}
	last, lastAt = state(), now()
	send(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.evSig:
			time.Sleep(150 * time.Millisecond) // a few more may follow at once: they go together
			send(false)
		case <-c.wake:
			last, lastAt = state(), now()
			send(true)
		case <-tick.C:
			c.mu.Lock()
			waiting := len(c.queue)
			c.mu.Unlock()
			due := now().Sub(lastAt) >= idle
			if !due && !c.watched.Load() && waiting == 0 {
				continue // nobody looks: every 30 s
			}
			cur := state()
			if !due && bytes.Equal(cur, last) && waiting == 0 {
				continue
			}
			last, lastAt = cur, now()
			send(true)
		}
	}
}
