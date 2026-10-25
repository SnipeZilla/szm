package manager

import (
	"errors"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/netmon"
)

// NetStatus tells whether the servers' network is counted, and how it goes.
type NetStatus struct {
	On      bool   `json:"on"`
	Error   string `json:"error,omitempty"`   // why it is not counted
	Seen    uint64 `json:"seen,omitempty"`    // network events Windows reported (every program)
	Counted uint64 `json:"counted,omitempty"` // ... of the servers
}

// netRetry: a count that could not start, or that Windows ended, is tried again after this
const netRetry = time.Minute

// startNetwork starts counting the servers' network. It always runs: it needs the manager to run as administrator
// (or as the service); otherwise the Overview shows the server's PID, and why.
func (m *Manager) startNetwork() {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	if m.net != nil {
		return
	}
	m.netTried = time.Now()
	mon, err := netmon.Start()
	if err != nil {
		if m.netErr != err.Error() && !errors.Is(err, netmon.ErrUnsupported) {
			m.Log.Add(events.Manager, "", "", "The network of the servers is not counted: %v", err)
		}
		m.netErr = err.Error()
		return
	}
	if m.netErr != "" {
		m.Log.Add(events.Manager, "", "", "The network of the servers is counted again")
	}
	m.net, m.netErr = mon, ""
}

// stopNetwork ends the count (the manager stops).
func (m *Manager) stopNetwork() {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	if m.net != nil {
		m.net.Close()
		m.net = nil
	}
}

func (m *Manager) netMonitor() *netmon.Monitor {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	return m.net
}

// watchNetwork has the running servers' processes counted, each second. Should Windows end the count, it says so.
func (m *Manager) watchNetwork() {
	mon := m.netMonitor()
	if mon == nil {
		m.netMu.Lock()
		retry := m.netErr != netmon.ErrUnsupported.Error() && time.Since(m.netTried) >= netRetry
		m.netMu.Unlock()
		if retry {
			m.startNetwork()
		}
		return
	}
	if err := mon.Err(); err != nil {
		m.netMu.Lock()
		if m.net == mon {
			m.net.Close()
			m.net, m.netErr, m.netTried = nil, err.Error(), time.Now()
		}
		m.netMu.Unlock()
		m.Log.Add(events.Error, "", "", "%v. It starts again by itself in a minute.", err)
		return
	}
	m.mu.Lock()
	list := make([]*Instance, 0, len(m.inst))
	for _, i := range m.inst {
		list = append(list, i)
	}
	m.mu.Unlock()
	var pids []uint32 // the servers' (the manager counts its own traffic itself: netcount)
	for _, i := range list {
		i.mu.Lock()
		if i.pid > 0 {
			pids = append(pids, uint32(i.pid))
		}
		i.mu.Unlock()
	}
	mon.Watch(pids)
}

// NetworkStatus tells whether the network is counted.
func (m *Manager) NetworkStatus() NetStatus {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	if m.net == nil {
		return NetStatus{Error: m.netErr}
	}
	seen, counted := m.net.Events()
	return NetStatus{On: true, Seen: seen, Counted: counted}
}
