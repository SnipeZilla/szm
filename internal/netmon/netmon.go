// Package netmon counts the network traffic of the game servers, per process, the way Task Manager and Resource
// Monitor do: Windows reports every send and receive of every program (ETW, the Microsoft-Windows-Kernel-Network
// provider), with its process ID and size. It needs administrator rights. On Linux the manager reads every packet's
// ports and finds the server that owns them (netmon_linux.go), as root.
package netmon

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrUnsupported: no per-program network counts on this system.
var ErrUnsupported = errors.New("network per program is counted on Windows and Linux only")

// Rate is a program's traffic, in bytes per second.
type Rate struct {
	In  float64 `json:"in"`
	Out float64 `json:"out"`
}

// sampleEvery: rates are worked out over this window
const sampleEvery = 2 * time.Second

type counter struct{ in, out atomic.Uint64 }

// Monitor counts the traffic of the programs it watches.
type Monitor struct {
	watched atomic.Pointer[map[uint32]*counter] // read by every network event: replaced whole, never changed

	wmu sync.Mutex // Watch

	mu     sync.Mutex
	rates  map[uint32]Rate
	prev   map[uint32][2]uint64
	prevAt time.Time

	allIn, allOut atomic.Uint64 // every program's traffic but the loopback's: the machine's (Windows)

	seen    atomic.Uint64 // network events Windows reported (every program)
	counted atomic.Uint64 // ... of the programs watched

	quit     chan struct{}
	quitOnce sync.Once
	stopOS   func() // ends the trace (Windows)
	statsOS  func() (TraceStats, bool)
	ended    chan struct{}
	failMu   sync.Mutex
	fail     error // the trace ended by itself
}

func newMonitor() *Monitor {
	m := &Monitor{rates: map[uint32]Rate{}, prev: map[uint32][2]uint64{}, quit: make(chan struct{}), ended: make(chan struct{})}
	empty := map[uint32]*counter{}
	m.watched.Store(&empty)
	return m
}

// count adds the size of a send (out) or a receive of a program.
func (m *Monitor) count(pid, size uint32, out bool) {
	m.seen.Add(1)
	c := (*m.watched.Load())[pid]
	if c == nil {
		return
	}
	m.counted.Add(1)
	if out {
		c.out.Add(uint64(size))
	} else {
		c.in.Add(uint64(size))
	}
}

// total adds a send or a receive of any program to the machine's traffic.
func (m *Monitor) total(size uint32, out bool) {
	if out {
		m.allOut.Add(uint64(size))
	} else {
		m.allIn.Add(uint64(size))
	}
}

// Totals gives the machine's traffic since the counting started, in bytes: every program's but the loopback's
// (Windows; 0 elsewhere).
func (m *Monitor) Totals() (in, out uint64) { return m.allIn.Load(), m.allOut.Load() }

// Watch sets the programs to count (the servers' processes); the others are ignored.
func (m *Monitor) Watch(pids []uint32) {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	old := *m.watched.Load()
	same := len(old) == len(pids)
	for _, p := range pids {
		if old[p] == nil {
			same = false
		}
	}
	if same {
		return
	}
	next := make(map[uint32]*counter, len(pids))
	for _, p := range pids {
		if c := old[p]; c != nil {
			next[p] = c
		} else {
			next[p] = &counter{}
		}
	}
	m.watched.Store(&next)
}

// Rate gives a program's traffic over the last seconds; false until it has been watched for a moment.
func (m *Monitor) Rate(pid uint32) (Rate, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rates[pid]
	return r, ok
}

// Events tells how many network events Windows reported, and how many were of the servers.
func (m *Monitor) Events() (seen, counted uint64) { return m.seen.Load(), m.counted.Load() }

// Err is why the counting stopped by itself (nil while it runs).
func (m *Monitor) Err() error {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	return m.fail
}

func (m *Monitor) setErr(err error) {
	m.failMu.Lock()
	m.fail = err
	m.failMu.Unlock()
}

// sample works out the rates from the counts since the last sample.
func (m *Monitor) sample(now time.Time) {
	watched := *m.watched.Load()
	m.mu.Lock()
	defer m.mu.Unlock()
	dt := now.Sub(m.prevAt).Seconds()
	rates := make(map[uint32]Rate, len(watched))
	prev := make(map[uint32][2]uint64, len(watched))
	for pid, c := range watched {
		cur := [2]uint64{c.in.Load(), c.out.Load()}
		prev[pid] = cur
		if p, ok := m.prev[pid]; ok && dt > 0 {
			rates[pid] = Rate{In: float64(cur[0]-p[0]) / dt, Out: float64(cur[1]-p[1]) / dt}
		}
	}
	m.rates, m.prev, m.prevAt = rates, prev, now
}

func (m *Monitor) sampler() {
	t := time.NewTicker(sampleEvery)
	defer t.Stop()
	for {
		select {
		case <-m.quit:
			return
		case now := <-t.C:
			m.sample(now)
		}
	}
}

// TraceStats: the buffers Windows holds for the trace (in the manager's memory), and the events it dropped.
type TraceStats struct {
	BufferKB, Buffers, Free, Lost uint32
}

// Stats tells how the trace goes (Windows; false elsewhere).
func (m *Monitor) Stats() (TraceStats, bool) {
	if m.statsOS == nil {
		return TraceStats{}, false
	}
	return m.statsOS()
}

// Close stops counting.
func (m *Monitor) Close() {
	m.quitOnce.Do(func() {
		close(m.quit)
		if m.stopOS != nil {
			m.stopOS()
		}
	})
}
