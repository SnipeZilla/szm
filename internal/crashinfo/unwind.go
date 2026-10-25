package crashinfo

import (
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The real call chain of the crashed thread (x64): each function's unwind data (the .pdata and .xdata of its module,
// read from the file on disk) says how it set up its stack, so the caller of each frame is known for sure, as a
// debugger finds it without symbols. A scan of the stack, by contrast, also finds leftovers of earlier calls.

// peInfo is what a module's file tells: its functions (begin, end, unwind data), its exported names, its stamp.
type peInfo struct {
	stamp   uint32
	funcs   []rtFunc
	exports []export // by address
	secs    []peSec
}

type rtFunc struct{ begin, end, unwind uint32 }

type export struct {
	rva  uint32
	name string
}

type peSec struct {
	va   uint32
	data []byte
}

var errNoUnwind = errors.New("no unwind data")

// readPE reads the parts of a module's file the unwinding needs (x64 only).
func readPE(path string) (*peInfo, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return nil, errNoUnwind
	}
	p := &peInfo{stamp: f.FileHeader.TimeDateStamp}
	for _, s := range f.Sections {
		if s.Size == 0 {
			continue
		}
		b, err := s.Data()
		if err != nil {
			continue
		}
		p.secs = append(p.secs, peSec{va: s.VirtualAddress, data: b})
	}
	if len(oh.DataDirectory) > 3 {
		dd := oh.DataDirectory[3] // the exception directory: RUNTIME_FUNCTION entries, sorted
		if b := p.at(dd.VirtualAddress, dd.Size); b != nil {
			for k := 0; k+12 <= len(b); k += 12 {
				p.funcs = append(p.funcs, rtFunc{binary.LittleEndian.Uint32(b[k:]), binary.LittleEndian.Uint32(b[k+4:]), binary.LittleEndian.Uint32(b[k+8:])})
			}
		}
	}
	if len(oh.DataDirectory) > 0 {
		p.exports = p.readExports(oh.DataDirectory[0])
	}
	return p, nil
}

// at gives n bytes at an address of the module (nil when they are not in its file).
func (p *peInfo) at(rva, n uint32) []byte {
	for _, s := range p.secs {
		if rva >= s.va && uint64(rva)+uint64(n) <= uint64(s.va)+uint64(len(s.data)) {
			return s.data[rva-s.va : rva-s.va+n]
		}
	}
	return nil
}

func (p *peInfo) u32(rva uint32) (uint32, bool) {
	b := p.at(rva, 4)
	if b == nil {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

func (p *peInfo) cstr(rva uint32) string {
	for _, s := range p.secs {
		if rva >= s.va && rva < s.va+uint32(len(s.data)) {
			b := s.data[rva-s.va:]
			if k := strings.IndexByte(string(b[:min(len(b), 512)]), 0); k >= 0 {
				return string(b[:k])
			}
		}
	}
	return ""
}

func (p *peInfo) readExports(dd pe.DataDirectory) []export {
	if dd.VirtualAddress == 0 || dd.Size < 40 {
		return nil
	}
	h := p.at(dd.VirtualAddress, 40)
	if h == nil {
		return nil
	}
	nFuncs, nNames := binary.LittleEndian.Uint32(h[20:]), binary.LittleEndian.Uint32(h[24:])
	funcs, names, ords := binary.LittleEndian.Uint32(h[28:]), binary.LittleEndian.Uint32(h[32:]), binary.LittleEndian.Uint32(h[36:])
	var out []export
	for k := uint32(0); k < nNames && k < 1<<16; k++ {
		nameRva, ok1 := p.u32(names + 4*k)
		ob := p.at(ords+2*k, 2)
		if !ok1 || ob == nil {
			continue
		}
		ord := uint32(binary.LittleEndian.Uint16(ob))
		if ord >= nFuncs {
			continue
		}
		rva, ok := p.u32(funcs + 4*ord)
		if !ok || (rva >= dd.VirtualAddress && rva < dd.VirtualAddress+dd.Size) { // a forwarder, not code
			continue
		}
		if n := p.cstr(nameRva); n != "" {
			out = append(out, export{rva, n})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].rva < out[b].rva })
	return out
}

// function finds the function an address (relative) is in: its entry, following chained entries to the first one.
func (p *peInfo) function(rva uint32) *rtFunc {
	k := sort.Search(len(p.funcs), func(i int) bool { return p.funcs[i].end > rva })
	if k == len(p.funcs) || rva < p.funcs[k].begin {
		return nil
	}
	return &p.funcs[k]
}

// funcName names the function an address is in, only when the module exports a function starting exactly where it
// starts: "V_strncpy+0x23". "" otherwise (an internal function: no name is guessed).
func (p *peInfo) funcName(rva uint32) string {
	f := p.function(rva)
	if f == nil {
		return ""
	}
	begin := p.primaryBegin(f)
	k := sort.Search(len(p.exports), func(i int) bool { return p.exports[i].rva >= begin })
	if k < len(p.exports) && p.exports[k].rva == begin {
		return fmt.Sprintf("%s+0x%X", demangle(p.exports[k].name), rva-begin)
	}
	return ""
}

// primaryBegin: the start of the function a (possibly chained) entry belongs to
func (p *peInfo) primaryBegin(f *rtFunc) uint32 {
	begin, uw := f.begin, f.unwind
	for depth := 0; depth < 8; depth++ {
		h := p.at(uw&^1, 4)
		if h == nil || h[0]>>3&0x4 == 0 { // not UNW_FLAG_CHAININFO
			return begin
		}
		n := uint32(h[2])
		off := (uw &^ 1) + 4 + 2*((n+1)&^1)
		b := p.at(off, 12)
		if b == nil {
			return begin
		}
		begin, uw = binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[8:])
	}
	return begin
}

// regs are the x64 integer registers, in the order of the unwind codes: rax rcx rdx rbx rsp rbp rsi rdi r8…r15.
type regs [16]uint64

const regRsp = 4

// memReader reads the crashed process's memory as the dump kept it.
type memReader func(addr uint64, n int) []byte

func readU64(mem memReader, addr uint64) (uint64, bool) {
	b := mem(addr, 8)
	if b == nil {
		return 0, false
	}
	return binary.LittleEndian.Uint64(b), true
}

// unwindStep goes from a frame (rip, its registers) to its caller's. ok is false when the unwind data or the stack
// memory is missing.
func (p *peInfo) unwindStep(base uint64, rip uint64, r *regs, mem memReader) (uint64, bool) {
	rva := uint32(rip - base)
	f := p.function(rva)
	if f == nil {
		// a leaf function (no stack frame of its own): the return address is on top of the stack
		ret, ok := readU64(mem, r[regRsp])
		if !ok {
			return 0, false
		}
		r[regRsp] += 8
		return ret, true
	}
	inProlog := true // only the first (innermost) entry may be in its prolog
	begin, uw := f.begin, f.unwind
	for depth := 0; depth < 8; depth++ {
		h := p.at(uw&^1, 4)
		if h == nil {
			return 0, false
		}
		flags, prolog, n := h[0]>>3, uint32(h[1]), uint32(h[2])
		frameReg, frameOff := h[3]&0x0F, uint64(h[3]>>4)
		codes := p.at((uw&^1)+4, 2*n)
		if codes == nil && n > 0 {
			return 0, false
		}
		offset := rva - begin
		frame := r[regRsp]
		if frameReg != 0 && (!inProlog || offset >= prolog) {
			frame = r[frameReg] - 16*frameOff
		}
		for k := uint32(0); k < n; {
			codeOff, op, info := uint32(codes[2*k]), codes[2*k+1]&0x0F, uint32(codes[2*k+1]>>4)
			slot := func(j uint32) uint32 {
				if 2*(k+j)+2 > uint32(len(codes)) {
					return 0
				}
				return uint32(binary.LittleEndian.Uint16(codes[2*(k+j):]))
			}
			size := uint32(1)
			switch op {
			case 1: // UWOP_ALLOC_LARGE
				size = 2
				if info != 0 {
					size = 3
				}
			case 4, 6, 8: // UWOP_SAVE_NONVOL, UWOP_EPILOG, UWOP_SAVE_XMM128
				size = 2
			case 5, 9: // the _FAR ones
				size = 3
			}
			if inProlog && offset < prolog && codeOff > offset {
				k += size // not done yet at this point of the prolog
				continue
			}
			switch op {
			case 0: // UWOP_PUSH_NONVOL
				v, ok := readU64(mem, r[regRsp])
				if !ok {
					return 0, false
				}
				r[info] = v
				r[regRsp] += 8
			case 1: // UWOP_ALLOC_LARGE
				if info == 0 {
					r[regRsp] += uint64(slot(1)) * 8
				} else {
					r[regRsp] += uint64(slot(1)) | uint64(slot(2))<<16
				}
			case 2: // UWOP_ALLOC_SMALL
				r[regRsp] += uint64(info)*8 + 8
			case 3: // UWOP_SET_FPREG
				r[regRsp] = r[frameReg] - 16*frameOff
				frame = r[regRsp]
			case 4: // UWOP_SAVE_NONVOL
				v, ok := readU64(mem, frame+uint64(slot(1))*8)
				if !ok {
					return 0, false
				}
				r[info] = v
			case 5: // UWOP_SAVE_NONVOL_FAR
				v, ok := readU64(mem, frame+(uint64(slot(1))|uint64(slot(2))<<16))
				if !ok {
					return 0, false
				}
				r[info] = v
			case 10: // UWOP_PUSH_MACHFRAME: an interrupt's frame, with the return address and rsp in it
				at := r[regRsp]
				if info != 0 {
					at += 8
				}
				ret, ok1 := readU64(mem, at)
				sp, ok2 := readU64(mem, at+24)
				if !ok1 || !ok2 {
					return 0, false
				}
				r[regRsp] = sp
				return ret, true
			}
			k += size
		}
		if flags&0x4 == 0 { // no UNW_FLAG_CHAININFO: done
			break
		}
		off := (uw &^ 1) + 4 + 2*((n+1)&^1)
		b := p.at(off, 12)
		if b == nil {
			return 0, false
		}
		begin, uw = binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[8:])
		inProlog = false
	}
	ret, ok := readU64(mem, r[regRsp])
	if !ok {
		return 0, false
	}
	r[regRsp] += 8
	return ret, true
}

// memory gives a reader of the dump's memory: the crashed threads' stacks and the memory lists.
func (d *minidump) memory() memReader {
	type rng struct {
		start uint64
		b     []byte
	}
	var rs []rng
	add := func(start uint64, rva, size uint64) {
		if size > 0 && rva+size <= uint64(len(d.b)) {
			rs = append(rs, rng{start, d.b[rva : rva+size]})
		}
	}
	if s, ok := d.streams[mdThreadList]; ok {
		n := d.u32(s[1])
		for k := uint32(0); k < n && k < 65536; k++ {
			e := s[1] + 4 + k*48
			add(d.u64(e+24), uint64(d.u32(e+36)), uint64(d.u32(e+32)))
		}
	}
	if s, ok := d.streams[mdMemoryList]; ok {
		n := d.u32(s[1])
		for k := uint32(0); k < n && k < 1<<20; k++ {
			e := s[1] + 4 + k*16
			add(d.u64(e), uint64(d.u32(e+12)), uint64(d.u32(e+8)))
		}
	}
	if s, ok := d.streams[mdMemory64List]; ok {
		n, rva := d.u64(s[1]), d.u64(s[1]+8)
		for k := uint64(0); k < n && k < 1<<20; k++ {
			e := s[1] + 16 + uint32(k)*16
			size := d.u64(e + 8)
			add(d.u64(e), rva, size)
			rva += size
		}
	}
	sort.Slice(rs, func(a, b int) bool { return rs[a].start < rs[b].start })
	return func(addr uint64, n int) []byte {
		// the ranges starting at or before addr, the nearest first (a stack can also be in a memory list)
		top := sort.Search(len(rs), func(i int) bool { return rs[i].start > addr }) - 1
		for k := top; k >= 0 && k > top-16; k-- {
			r := rs[k]
			if addr+uint64(n) <= r.start+uint64(len(r.b)) {
				return r.b[addr-r.start : addr-r.start+uint64(n)]
			}
		}
		return nil
	}
}

// unwind follows the crashed thread's calls from its registers at the crash (an x64 CONTEXT), with the unwind data
// of each module's file. It stops when a module's file can not be read (or is not the one that ran), when the stack
// memory is not in the dump, or at the thread's start.
func (d *minidump) unwind(ctxRva uint32, mods []*mdModule, installDir string) ([]Frame, string) {
	var r regs
	for k := 0; k < 16; k++ {
		r[k] = d.u64(ctxRva + 0x78 + uint32(k)*8) // Rax … R15
	}
	rip := d.u64(ctxRva + 0xF8)
	mem := d.memory()
	files := map[string]*peInfo{}
	var out []Frame
	for depth := 0; depth < 64; depth++ {
		look := rip
		if depth > 0 {
			look = rip - 1 // a return address: the call is just before it
		}
		m := findModule(mods, look)
		if m == nil {
			if depth == 0 {
				return nil, "it crashed in code outside any module (made at run time): its callers can not be followed"
			}
			return out, fmt.Sprintf("the next caller is at 0x%X, outside any module: code made at run time (.NET or SourcePawn: a plugin, or its framework's own scripted part)", rip)
		}
		f := frameOf(m, rip, installDir)
		if m.unloaded {
			out = append(out, f)
			return out, f.Module + " was unloaded: its callers can not be followed"
		}
		p, seen := files[m.name]
		if !seen {
			p, _ = readPE(m.name)
			if p != nil && m.stamp != 0 && p.stamp != m.stamp {
				p = nil // the file changed since (an update): its unwind data would be wrong
			}
			files[m.name] = p
		}
		if p != nil {
			f.Function = p.funcName(uint32(look - m.base))
		}
		out = append(out, f)
		if p == nil {
			return out, "the file of " + f.Module + " can not be read, or changed since the crash (an update): the rest of the chain is not known"
		}
		sp := r[regRsp]
		ret, ok := p.unwindStep(m.base, look, &r, mem)
		if !ok {
			return out, "the stack of the crashed thread is not all in the dump: the rest of the chain is not known"
		}
		if ret == 0 {
			return out, "" // the thread's start
		}
		if r[regRsp] <= sp {
			return out, "the chain loops back: stopped there"
		}
		rip = ret
	}
	return out, "more than 64 calls deep: the rest left out"
}

// demangle shortens a Visual C++ name to what a developer reads: ?Split@CSplitString@@AEAAXPEBDHPEAPEBDH_N@Z is
// CSplitString::Split. Special names (constructors, operators: ??...) and C names are left as they are.
func demangle(n string) string {
	if !strings.HasPrefix(n, "?") || strings.HasPrefix(n, "??") {
		return n
	}
	k := strings.Index(n, "@@")
	if k < 0 {
		return n
	}
	parts := strings.Split(n[1:k], "@")
	for _, x := range parts {
		if x == "" || strings.ContainsAny(x, "?$") {
			return n // templates and the like: kept whole
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "::")
}

// ShortFunc shortens a function name of a report made before names were demangled: "?Split@CSplitString@@…+0x43".
func ShortFunc(s string) string {
	name, off, ok := strings.Cut(s, "+")
	if !ok {
		return demangle(s)
	}
	return demangle(name) + "+" + off
}
