package main

import (
	"math"
	"os"
	"runtime/debug"
	"runtime/metrics"
	"time"
)

// leanMemory keeps the manager's memory close to what it really uses. Left alone, Go lets garbage pile up to as much
// as the memory in use before it cleans, and keeps freed memory a while before giving it back to the system: an idle
// manager would show twice what it needs, more or less depending on the Go version that built it.
//
//   - a soft ceiling, set again every 10 s: what the manager needs besides its data (the stacks of its threads: one
//     per running game server, the runtime's own), plus its data in use and half as much again for the garbage
//     (Go's default lets it grow by as much), and never under 32 MB. The garbage is cleaned before it piles up, but
//     the ceiling always leaves room: the cleaning never runs nonstop to reach a ceiling it can not;
//   - every 2 minutes, the freed memory is given back to the system.
//
// A GOMEMLIMIT set in the environment wins.
func leanMemory() {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	const floor = 32 << 20
	debug.SetMemoryLimit(floor)
	samples := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/heap/unused:bytes"},
		{Name: "/memory/classes/heap/free:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
		{Name: "/gc/heap/live:bytes"},
	}
	value := func(k int) int64 {
		if samples[k].Value.Kind() != metrics.KindUint64 {
			return -1
		}
		return int64(samples[k].Value.Uint64())
	}
	go func() {
		for k := 1; ; k++ {
			time.Sleep(10 * time.Second)
			metrics.Read(samples)
			total, objects, unused, free, released, live := value(0), value(1), value(2), value(3), value(4), value(5)
			if total < 0 || objects < 0 || unused < 0 || free < 0 || released < 0 || live < 0 {
				debug.SetMemoryLimit(math.MaxInt64) // not known: no ceiling rather than a wrong one
				continue
			}
			overhead := total - objects - unused - free - released // stacks, the runtime's own
			debug.SetMemoryLimit(max(floor, overhead+live*3/2+4<<20))
			if k%12 == 0 {
				debug.FreeOSMemory()
			}
		}
	}()
}
