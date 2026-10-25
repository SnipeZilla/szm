package proc

import (
	"sort"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	k32mem                = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessHeaps   = k32mem.NewProc("GetProcessHeaps")
	procHeapSummary       = k32mem.NewProc("HeapSummary")
	procGetMappedFileName = k32mem.NewProc("K32GetMappedFileNameW")
	procQueryWorkingSetEx = k32mem.NewProc("K32QueryWorkingSetEx")
)

// NativeMem is the process's memory as Windows sees it (debug). Own: the bytes Task Manager counts as its memory
// (pages in RAM that no other process shares), by block and in all, measured page by page.
type NativeMem struct {
	Heaps         int
	HeapCommitted uint64
	Private       uint64            // committed private memory, in all
	Mapped        uint64            // committed mapped views, in all
	Own           map[string]uint64 // kind -> bytes Task Manager counts
	blocks        map[uint64]block  // allocation base -> what it is, and its own bytes
	heapBase      map[uint64]int
}

type block struct {
	kind string
	own  uint64
}

type heapSummary struct {
	cb                                         uint32
	_                                          uint32
	allocated, committed, reserved, maxReserve uintptr
}

// the page list given to QueryWorkingSetEx, kept from one reading to the next
var (
	wsMu  sync.Mutex
	wsBuf []wsPage
)

type wsPage struct {
	addr  uintptr
	attrs uintptr // bit 0: in RAM; bit 15: shared
}

type region struct {
	base, alloc uintptr
	size        uintptr
	kind        string
}

// Native reads the process's native memory. ok is false where it is not known.
func Native() (NativeMem, bool) {
	n := NativeMem{Own: map[string]uint64{}, blocks: map[uint64]block{}, heapBase: map[uint64]int{}}
	for _, p := range []*windows.LazyProc{procHeapSummary, procGetProcessHeaps, procQueryWorkingSetEx} {
		if p.Find() != nil {
			return n, false
		}
	}
	var heaps [256]windows.Handle
	c, _, _ := procGetProcessHeaps.Call(uintptr(len(heaps)), uintptr(unsafe.Pointer(&heaps[0])))
	for k := 0; k < int(c) && k < len(heaps); k++ {
		s := heapSummary{cb: uint32(unsafe.Sizeof(heapSummary{}))}
		if r, _, _ := procHeapSummary.Call(uintptr(heaps[k]), 0, uintptr(unsafe.Pointer(&s))); r != 0 {
			n.Heaps++
			n.HeapCommitted += uint64(s.committed)
		}
		n.heapBase[uint64(heaps[k])] = k + 1
	}
	var regions []region
	var mbi windows.MemoryBasicInformation
	for addr := uintptr(0x10000); addr < 0x7FFFFFFF0000; {
		if err := windows.VirtualQuery(addr, &mbi, unsafe.Sizeof(mbi)); err != nil || mbi.RegionSize == 0 {
			break
		}
		if mbi.State == windows.MEM_COMMIT {
			kind := ""
			switch mbi.Type {
			case 0x20000:
				kind = KindPrivate
				n.Private += uint64(mbi.RegionSize)
			case 0x40000:
				kind = KindMapped
				n.Mapped += uint64(mbi.RegionSize)
			case 0x1000000:
				kind = KindImage
			}
			if kind != "" {
				regions = append(regions, region{mbi.BaseAddress, mbi.AllocationBase, mbi.RegionSize, kind})
			}
		}
		next := mbi.BaseAddress + mbi.RegionSize
		if next <= addr {
			break
		}
		addr = next
	}
	// every committed page: in RAM, and not shared?
	wsMu.Lock()
	defer wsMu.Unlock()
	wsBuf = wsBuf[:0]
	for _, r := range regions {
		for a := r.base; a < r.base+r.size; a += 4096 {
			wsBuf = append(wsBuf, wsPage{addr: a})
		}
	}
	if len(wsBuf) == 0 {
		return n, true
	}
	if r, _, _ := procQueryWorkingSetEx.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&wsBuf[0])), uintptr(len(wsBuf))*unsafe.Sizeof(wsPage{})); r == 0 {
		return n, true
	}
	k := 0
	for _, r := range regions {
		var own uint64
		for a := r.base; a < r.base+r.size; a += 4096 {
			if at := wsBuf[k].attrs; at&1 != 0 && at&(1<<15) == 0 {
				own += 4096
			}
			k++
		}
		n.Own[r.kind] += own
		b := n.blocks[uint64(r.alloc)]
		b.kind, b.own = r.kind, b.own+own
		n.blocks[uint64(r.alloc)] = b
	}
	if cap(wsBuf) > 1<<20 { // do not keep a huge list
		wsBuf = nil
	}
	return n, true
}

// Grown lists the blocks whose own bytes (what Task Manager counts) grew from before to now by min or more.
func (n NativeMem) Grown(before NativeMem, min uint64) []Block {
	var out []Block
	for base, b := range n.blocks {
		was := before.blocks[base].own
		if b.own >= was+min {
			x := Block{Base: base, Grew: b.own - was, Size: b.own, New: was == 0, Kind: b.kind, Heap: n.heapBase[base]}
			if b.kind != KindPrivate {
				x.File = mappedFile(base)
			}
			out = append(out, x)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Grew > out[b].Grew })
	return out
}

// mappedFile: the file a view or image maps ("\Device\HarddiskVolume3\…"), "" for shared memory (not a file)
func mappedFile(base uint64) string {
	if procGetMappedFileName.Find() != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	r, _, _ := procGetMappedFileName.Call(uintptr(windows.CurrentProcess()), uintptr(base), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:r])
}
