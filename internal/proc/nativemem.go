package proc

// Memory kinds of a block
const (
	KindPrivate = "private" // allocated memory (Go's heap, Windows' heaps, stacks, buffers)
	KindMapped  = "mapped"  // a view of a file or of shared memory
	KindImage   = "image"   // a program or DLL (its pages written to are its own: data, relocations)
)

// Block is a block of the process's memory whose pages counted as its own grew (NativeMem.Grown).
type Block struct {
	Base, Grew, Size uint64 // Size: its bytes counted as the process's own, now
	New              bool
	Kind             string // private, mapped, image
	Heap             int    // the heap it is (its number), 0: not a heap's first block
	File             string // the file it maps (a mapped view, a program or DLL); "" for shared memory
}

// Owner says, as far as Windows tells, whose block it is: Go reserves its heap at 0x00c0_0000_0000 and the like.
func (b Block) Owner() string {
	switch {
	case b.Kind == KindImage:
		return "pages of " + orUnknown(b.File) + " written to (its data)"
	case b.Kind == KindMapped && b.File != "":
		return "a mapped view of " + b.File
	case b.Kind == KindMapped:
		return "a mapped view of shared memory (not a file)"
	case b.Heap > 0:
		return "a Windows heap (Windows' code, DLLs)"
	case b.Base>>32&0xFF == 0xC0:
		return "Go's heap"
	}
	return "private memory not Go's: reserved by Windows' code or a DLL"
}

func orUnknown(s string) string {
	if s == "" {
		return "a module"
	}
	return s
}
