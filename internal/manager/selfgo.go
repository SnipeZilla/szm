package manager

import (
	"runtime"
	"runtime/metrics"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/proc"
)

// SelfGo tells where the manager's memory goes: what Go holds (its heap and stacks, without what it gave back to
// the system), its goroutines, and the threads and handles of the process. Memory that grows while Go's does not
// is the system's (a DLL, a driver), not a leak of the manager.
type SelfGo struct {
	Mem        uint64 `json:"mem"`
	Heap       uint64 `json:"heap"` // of Mem: the objects in use
	Goroutines int    `json:"goroutines"`
	Threads    int    `json:"threads,omitempty"`
	GoThreads  int    `json:"go_threads"` // of Threads: those Go made (the others are Windows' own, for the manager)
	Handles    int    `json:"handles,omitempty"`
	// the rest of Mem, for the memory log: freed but kept (free), room in the pages in use (unused), the stacks,
	// the runtime's own (metadata, profiling, other)
	Free, Unused, Stacks, Runtime uint64 `json:"-"`
}

var (
	selfMu   sync.Mutex
	selfLast SelfGo
	selfAt   time.Time
)

var selfSamples = []string{
	"/memory/classes/total:bytes",
	"/memory/classes/heap/released:bytes",
	"/memory/classes/heap/objects:bytes",
	"/sched/goroutines:goroutines",
	"/memory/classes/heap/free:bytes",
	"/memory/classes/heap/unused:bytes",
	"/memory/classes/heap/stacks:bytes",
	"/memory/classes/os-stacks:bytes",
}

// selfGo reads them, at most every 10 seconds (counting the threads looks at every thread of the machine).
// The threads Go made come from runtime.ThreadCreateProfile, not runtime/pprof: that package would turn on the
// memory profiling, whose records grow as the manager runs.
func selfGo() *SelfGo {
	selfMu.Lock()
	defer selfMu.Unlock()
	if time.Since(selfAt) < 10*time.Second {
		g := selfLast
		return &g
	}
	s := make([]metrics.Sample, len(selfSamples))
	for i, n := range selfSamples {
		s[i].Name = n
	}
	metrics.Read(s)
	u := func(i int) uint64 {
		if s[i].Value.Kind() == metrics.KindUint64 {
			return s[i].Value.Uint64()
		}
		return 0
	}
	g := SelfGo{Mem: u(0) - u(1), Heap: u(2), Goroutines: int(u(3)), Free: u(4), Unused: u(5), Stacks: u(6) + u(7)}
	if known := g.Heap + g.Free + g.Unused + g.Stacks; g.Mem > known {
		g.Runtime = g.Mem - known
	}
	g.Threads, g.Handles = proc.SelfCounts()
	g.GoThreads, _ = runtime.ThreadCreateProfile(nil)
	selfLast, selfAt = g, time.Now()
	return &g
}
