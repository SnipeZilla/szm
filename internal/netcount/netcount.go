// Package netcount counts the manager's own network traffic: the bytes of every connection it opens or accepts (its
// web interface, the link between a hub and its nodes, Steam's API, RCON, the queries, the notifications). It is
// what Resource Monitor shows for szm, without relying on the system to say which program a packet was for.
package netcount

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var in, out atomic.Uint64

// Totals gives the bytes received and sent since the manager started.
func Totals() (uint64, uint64) { return in.Load(), out.Load() }

type conn struct{ net.Conn }

func (c conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	in.Add(uint64(n))
	return n, err
}

func (c conn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	out.Add(uint64(n))
	return n, err
}

// Conn counts a connection's traffic.
func Conn(c net.Conn) net.Conn {
	if c == nil {
		return nil
	}
	return conn{c}
}

type listener struct{ net.Listener }

func (l listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return Conn(c), nil
}

// Listener counts the traffic of the connections a listener accepts.
func Listener(l net.Listener) net.Listener { return listener{l} }

var dialer = &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}

// DialContext dials and counts the connection (for an http.Transport).
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := dialer.DialContext(ctx, network, addr)
	return Conn(c), err
}

// DialTimeout is net.DialTimeout, counted.
func DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	c, err := net.DialTimeout(network, addr, timeout)
	return Conn(c), err
}

var once sync.Once

// CountHTTP has the default HTTP transport (the clients without one of their own) count its connections.
func CountHTTP() {
	once.Do(func() {
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			t.DialContext = DialContext
		}
	})
}

// Rate turns the totals into bytes per second, from one reading to the next.
type Rate struct {
	mu        sync.Mutex
	at        time.Time
	lastIn    uint64
	lastOut   uint64
	inPS, out float64
}

// Read gives the bytes per second received and sent since the last reading (one second at least apart: a closer one
// gives the last rate again).
func (r *Rate) Read() (float64, float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, o := Totals()
	now := time.Now()
	if r.at.IsZero() {
		r.at, r.lastIn, r.lastOut = now, i, o
		return 0, 0
	}
	if d := now.Sub(r.at).Seconds(); d >= 1 {
		r.inPS, r.out = float64(i-r.lastIn)/d, float64(o-r.lastOut)/d
		r.at, r.lastIn, r.lastOut = now, i, o
	}
	return r.inPS, r.out
}
