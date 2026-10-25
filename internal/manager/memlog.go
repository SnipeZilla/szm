package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/proc"
)

// Debug: szm.json has "debug": true
func (m *Manager) Debug() bool {
	m.Cfg.RLock()
	defer m.Cfg.RUnlock()
	return m.Cfg.Debug
}

// the last things the manager was asked (debug): the memory log names them when the memory jumps
var (
	notesMu sync.Mutex
	notes   []note
)

type note struct {
	t    time.Time
	what string
}

// Note keeps what the manager was just asked (a page's call), for the memory log (debug).
func (m *Manager) Note(what string) {
	notesMu.Lock()
	notes = append(notes, note{time.Now(), what})
	if len(notes) > 50 {
		notes = notes[len(notes)-50:]
	}
	notesMu.Unlock()
}

// recentNotes: the calls of the last d, the latest last
func recentNotes(d time.Duration) []string {
	notesMu.Lock()
	defer notesMu.Unlock()
	var out []string
	for _, n := range notes {
		if time.Since(n.t) <= d {
			out = append(out, n.t.Format("15:04:05")+" "+n.what)
		}
	}
	return out
}

// memLogEvery: how often logs/memory.txt gets a line; memCheckEvery: how often the memory is checked for a jump
var (
	memLogEvery   = 30 * time.Minute
	memCheckEvery = 10 * time.Second
	// selfMemory: the manager's memory, as Task Manager shows it
	selfMemory = func() uint64 {
		if u, err := proc.Stats(os.Getpid()); err == nil {
			return u.Memory
		}
		return 0
	}
)

// logMemory writes what the manager uses into logs/memory.txt, a line every half hour (a minute after it starts,
// then each half hour): its memory as Task Manager shows it, what Go holds of it, its goroutines, threads and
// handles. It checks the memory every 10 seconds too: when it jumps, a line says so at once, with what the manager
// did and was asked just before. The file keeps its last lines only.
func (m *Manager) logMemory(ctx context.Context) {
	path := filepath.Join(m.DataDir, "logs", "memory.txt")
	line := func(mem uint64, head string) string {
		g := selfGo()
		etw := ""
		if mon := m.netMonitor(); mon != nil {
			if st, ok := mon.Stats(); ok {
				etw = fmt.Sprintf(" · network count: %d buffers of %d KB (%d free), %d events lost", st.Buffers, st.BufferKB, st.Free, st.Lost)
			}
		}
		if n, ok := proc.Native(); ok {
			etw += fmt.Sprintf(" · Windows heaps %.1f MB (%d) · counted as its own: private %.1f MB, mapped %.1f MB, programs and DLLs %.1f MB",
				mb(n.HeapCommitted), n.Heaps, mb(n.Own[proc.KindPrivate]), mb(n.Own[proc.KindMapped]), mb(n.Own[proc.KindImage]))
		}
		return fmt.Sprintf("%s  %smemory %5.1f MB · Go %5.1f MB (in use %.1f, free %.1f, unused %.1f, stacks %.1f, runtime %.1f) · %d goroutines · %d threads (%d made by Go) · %d handles · up %s%s\n",
			time.Now().Format("2006-01-02 15:04:05"), head, mb(mem), mb(g.Mem), mb(g.Heap), mb(g.Free), mb(g.Unused), mb(g.Stacks), mb(g.Runtime),
			g.Goroutines, g.Threads, g.GoThreads, g.Handles, time.Since(m.Started).Round(time.Second), etw)
	}
	tick := time.NewTicker(memCheckEvery)
	defer tick.Stop()
	start := time.Now()
	next := start.Add(time.Minute)
	last := selfMemory()
	// what Windows' code holds in the process, read each check: a jump says which of it grew (Windows only)
	native, _ := proc.Native()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		mem := selfMemory()
		if mem > last+4<<20 {
			// a jump: when, and what happened in the 30 seconds before
			out := line(mem, fmt.Sprintf("JUMP +%.1f MB in 10 s · ", mb(mem-last)))
			for _, e := range m.Log.Recent("", 20) {
				if time.Since(e.Time) <= 30*time.Second {
					out += "    event " + e.Time.Format("15:04:05") + " " + e.Kind + " " + e.Name + ": " + e.Text + "\n"
				}
			}
			for _, n := range recentNotes(30 * time.Second) {
				out += "    asked " + n + "\n"
			}
			if now, ok := proc.Native(); ok {
				out += fmt.Sprintf("    Windows heaps %.1f → %.1f MB · counted as its own: private %.1f → %.1f MB, mapped %.1f → %.1f MB, programs and DLLs %.1f → %.1f MB\n",
					mb(native.HeapCommitted), mb(now.HeapCommitted), mb(native.Own[proc.KindPrivate]), mb(now.Own[proc.KindPrivate]),
					mb(native.Own[proc.KindMapped]), mb(now.Own[proc.KindMapped]), mb(native.Own[proc.KindImage]), mb(now.Own[proc.KindImage]))
				for k, b := range now.Grown(native, 1<<20) {
					if k == 5 {
						break
					}
					what := "grew"
					if b.New {
						what = "new"
					}
					out += fmt.Sprintf("    +%.1f MB at 0x%X (%s, now %.1f MB): %s\n", mb(b.Grew), b.Base, what, mb(b.Size), b.Owner())
				}
			}
			appendCapped(path, out, 256<<10)
		}
		if n, ok := proc.Native(); ok {
			native = n
		}
		last = mem
		if time.Now().After(next) {
			appendCapped(path, line(mem, ""), 256<<10)
			next = next.Add(memLogEvery)
			if next.Before(time.Now()) {
				next = time.Now().Add(memLogEvery)
			}
		}
	}
}

func mb(b uint64) float64 { return float64(b) / (1 << 20) }

// appendCapped adds a line to a file that keeps its last half when it grows past max bytes.
func appendCapped(path, line string, max int64) {
	if st, err := os.Stat(path); err == nil && st.Size() > max {
		if b, err := os.ReadFile(path); err == nil {
			b = b[len(b)/2:]
			for i, c := range b {
				if c == '\n' {
					b = b[i+1:]
					break
				}
			}
			_ = os.WriteFile(path, b, 0o644)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.WriteString(line)
	f.Close()
}
