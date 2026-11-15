package manager

import (
	"context"
	"strconv"
	"time"
)

/* ---------------------------------------------------------------------------------------------------------------
 * The console of a server, for who looks at it
 *
 * The lines of a console are numbered from the manager's start. A reader keeps the number after the last line it
 * has and asks for what came since: nothing is pushed to it, so nothing can be dropped on the way, however many
 * lines come at once and however slow its connection is. It is only woken when there is something new.
 *
 * While someone looks at a console, its log is read ten times a second instead of once (followConsoles): what the
 * server writes shows at once. Nobody looking: once a second, as the rest of the manager needs it.
 * ------------------------------------------------------------------------------------------------------------- */

// ConsoleEpoch names this run of the manager: the lines' numbers start again with it, so a reader that kept a
// number of another run starts over.
var ConsoleEpoch = strconv.FormatInt(time.Now().UnixMilli(), 36)

const (
	followEvery = 100 * time.Millisecond // how often a console someone looks at is read
	watchFor    = 15 * time.Second       // ... and for how long after it was last asked for (a page that polls)
	sureEvery   = time.Second            // how often its log is opened whatever its size in its folder says
)

// Console gives the last lines of a server's console.
func (m *Manager) Console(id string) []string {
	i := m.instance(id)
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.console.all()
}

// ConsoleSince gives the lines added after line n, and the number to ask from next time. A reader that is new
// (n < 0), of another run of the manager, or too far behind (the lines it misses are no longer kept) gets all the
// lines kept.
func (m *Manager) ConsoleSince(id string, n int64) ([]string, int64) {
	lines, next, _ := m.ConsoleFrom(id, n)
	return lines, next
}

// ConsoleFrom is ConsoleSince, and tells how many lines the reader missed (they came and went while it was away).
func (m *Manager) ConsoleFrom(id string, n int64) (lines []string, next, missed int64) {
	i := m.instance(id)
	i.mu.Lock()
	defer i.mu.Unlock()
	first := i.consoleN - int64(i.console.len()) // the number of the oldest line kept
	if n > i.consoleN {
		n = first // a number of another run
	}
	if n < first {
		if n >= 0 {
			missed = first - n
		}
		n = first
	}
	return i.console.last(int(i.consoleN - n)), i.consoleN, missed
}

// SubscribeConsole gives a channel that is sent to when a server's console has new lines (ConsoleSince gives
// them). While it is subscribed the console is read often.
func (m *Manager) SubscribeConsole(id string) chan struct{} {
	i := m.instance(id)
	c := make(chan struct{}, 1)
	i.mu.Lock()
	i.consoleSub[c] = struct{}{}
	i.mu.Unlock()
	m.watchConsole(id)
	return c
}

// UnsubscribeConsole stops a channel of SubscribeConsole.
func (m *Manager) UnsubscribeConsole(id string, c chan struct{}) {
	i := m.instance(id)
	i.mu.Lock()
	delete(i.consoleSub, c)
	i.mu.Unlock()
}

// WatchConsole tells that someone looks at a server's console without staying subscribed (a page that asks again
// and again, a command just sent): it is read often for a while.
func (m *Manager) WatchConsole(id string) {
	i := m.instance(id)
	i.mu.Lock()
	i.watchUntil = time.Now().Add(watchFor)
	i.mu.Unlock()
	m.watchConsole(id)
}

// WaitConsole gives the lines after line n as ConsoleFrom does, waiting up to wait for one when there is none
// yet (or until ctx ends): a page that can not keep a stream open asks this way, and still gets a line as it comes.
func (m *Manager) WaitConsole(ctx context.Context, id string, n int64, wait time.Duration) (lines []string, next, missed int64) {
	if wait <= 0 {
		m.WatchConsole(id)
		return m.ConsoleFrom(id, n)
	}
	c := m.SubscribeConsole(id)
	defer m.UnsubscribeConsole(id, c)
	defer m.WatchConsole(id) // until it asks again
	t := time.NewTimer(wait)
	defer t.Stop()
	for {
		if lines, next, missed = m.ConsoleFrom(id, n); len(lines) > 0 || next != n {
			return
		}
		select {
		case <-c:
		case <-t.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) watchConsole(id string) {
	m.conMu.Lock()
	if m.conWatch == nil {
		m.conWatch = map[string]struct{}{}
	}
	m.conWatch[id] = struct{}{}
	m.conMu.Unlock()
	select {
	case m.conWake <- struct{}{}:
	default:
	}
}

// watched tells whether someone looks at the console now. The lock must be held.
func (i *Instance) watchedLocked(now time.Time) bool {
	return len(i.consoleSub) > 0 || now.Before(i.watchUntil)
}

// followConsoles reads the logs of the consoles someone looks at, many times a second. It sleeps while nobody
// looks at any.
func (m *Manager) followConsoles(ctx context.Context) {
	tick := time.NewTicker(followEvery)
	tick.Stop()
	defer tick.Stop()
	running := false
	var ids []string
	for {
		ids = ids[:0]
		m.conMu.Lock()
		for id := range m.conWatch {
			ids = append(ids, id)
		}
		m.conMu.Unlock()
		if len(ids) == 0 {
			if running {
				tick.Stop()
				running = false
			}
			select {
			case <-ctx.Done():
				return
			case <-m.conWake:
			}
			continue
		}
		if !running {
			tick.Reset(followEvery)
			running = true
		}
		now := time.Now()
		for _, id := range ids {
			i := m.instance(id)
			i.mu.Lock()
			watched, up := i.watchedLocked(now), i.pid > 0
			i.mu.Unlock()
			if !watched {
				// nobody any more, unless one came just now (it adds itself under conMu: seen here, or added after)
				m.conMu.Lock()
				i.mu.Lock()
				watched = i.watchedLocked(time.Now())
				i.mu.Unlock()
				if !watched {
					delete(m.conWatch, id)
				}
				m.conMu.Unlock()
				continue
			}
			if !up {
				continue // not running: only the manager's own lines, which are said as they come
			}
			s, g, _, err := m.lookup(id)
			if err != nil {
				continue
			}
			m.readConsole(i, s, g)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
