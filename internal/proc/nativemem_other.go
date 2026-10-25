//go:build !windows

package proc

// NativeMem is the process's memory as Windows sees it: not known here.
type NativeMem struct {
	Heaps         int
	HeapCommitted uint64
	Private       uint64
	Mapped        uint64
	Own           map[string]uint64
}

func Native() (NativeMem, bool) { return NativeMem{}, false }

func (n NativeMem) Grown(before NativeMem, min uint64) []Block { return nil }
