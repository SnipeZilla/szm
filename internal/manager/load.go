package manager

import (
	"context"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/proc"
)

// Load is how busy the whole machine is: its CPU, and its network (every program's, on its network cards).
type Load struct {
	CPU float64 `json:"cpu"` // % of the machine
	In  float64 `json:"in"`  // bytes per second
	Out float64 `json:"out"`
	Net bool    `json:"net"` // In and Out are known
}

type loadSampler struct {
	mu        sync.Mutex
	last      *Load
	busy, tot uint64
	in, out   uint64
	netKnown  bool
	at        time.Time
}

// sampleLoad measures the machine every 2 s, for the machine's tile and the top bar.
func (m *Manager) sampleLoad(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		m.measureLoad(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) measureLoad(now time.Time) {
	busy, tot, cpuErr := proc.SystemCPU()
	in, out, netErr := proc.NetTotals()
	if netErr != nil {
		if mon := m.netMonitor(); mon != nil { // Windows: the network count sees every program
			in, out = mon.Totals()
			netErr = nil
		}
	}
	ls := &m.load
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if !ls.at.IsZero() {
		l := &Load{}
		if cpuErr == nil && tot > ls.tot && busy >= ls.busy {
			l.CPU = float64(busy-ls.busy) / float64(tot-ls.tot) * 100
		}
		if dt := now.Sub(ls.at).Seconds(); netErr == nil && ls.netKnown && dt > 0 && in >= ls.in && out >= ls.out {
			l.In, l.Out, l.Net = float64(in-ls.in)/dt, float64(out-ls.out)/dt, true
		}
		if cpuErr == nil {
			ls.last = l
		}
	}
	ls.busy, ls.tot, ls.in, ls.out, ls.netKnown, ls.at = busy, tot, in, out, netErr == nil, now
}

// MachineLoad gives the last measure (nil before the first two).
func (m *Manager) MachineLoad() *Load {
	m.load.mu.Lock()
	defer m.load.mu.Unlock()
	if m.load.last == nil {
		return nil
	}
	l := *m.load.last
	return &l
}
