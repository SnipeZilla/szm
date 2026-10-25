package crashinfo

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// testdata/unwind: crashy.dll (crashy.c, built with clang for Windows x64) run in an emulator (emulate.py) until it
// read a null pointer: framed -> outer -> big -> leaf_read. crash.json is the state then: registers, stack.
type crashState struct {
	Base       uint64   `json:"base"`
	Size       uint32   `json:"size"`
	Stamp      uint32   `json:"stamp"`
	Rip        uint64   `json:"rip"`
	Regs       []uint64 `json:"regs"`
	StackStart uint64   `json:"stack_start"`
	Stack      string   `json:"stack"`
}

// writeDump writes a minidump of that crash: one module (dllPath, with stamp), one thread and its stack, the
// exception (reading address 0) with the registers.
func writeDump(t *testing.T, st crashState, dllPath string, stamp uint32) string {
	stack, _ := hex.DecodeString(st.Stack)
	var b []byte
	u32 := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }
	u64 := func(v uint64) { b = binary.LittleEndian.AppendUint64(b, v) }
	pad := func(n int) { b = append(b, make([]byte, n)...) }
	// header + directory of 4 streams, filled in after
	pad(32 + 4*12)
	type stream struct{ typ, size, rva uint32 }
	var streams []stream
	begin := func() uint32 { return uint32(len(b)) }
	// system info
	r := begin()
	b = binary.LittleEndian.AppendUint16(b, 9) // x64
	pad(54)
	streams = append(streams, stream{mdSystemInfo, 56, r})
	// the module's name
	name := begin()
	w := utf16.Encode([]rune(dllPath))
	u32(uint32(2 * len(w)))
	for _, c := range w {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	pad(2)
	// module list
	r = begin()
	u32(1)
	u64(st.Base)
	u32(st.Size)
	u32(0)
	u32(stamp)
	u32(name)
	pad(108 - 24)
	streams = append(streams, stream{mdModuleList, begin() - r, r})
	// the stack
	stackRva := begin()
	b = append(b, stack...)
	// the context (CONTEXT, x64)
	ctx := begin()
	c := make([]byte, 0x4D0)
	binary.LittleEndian.PutUint32(c[0x30:], 0x10001F)
	for k, v := range st.Regs {
		binary.LittleEndian.PutUint64(c[0x78+8*k:], v)
	}
	binary.LittleEndian.PutUint64(c[0xF8:], st.Rip)
	b = append(b, c...)
	// thread list
	r = begin()
	u32(1)
	u32(42)
	pad(12)
	u64(0)
	u64(st.StackStart)
	u32(uint32(len(stack)))
	u32(stackRva)
	u32(0x4D0)
	u32(ctx)
	streams = append(streams, stream{mdThreadList, begin() - r, r})
	// exception: thread 42 read address 0
	r = begin()
	u32(42)
	u32(0)
	u32(0xC0000005)
	u32(0)
	u64(0)
	u64(st.Rip)
	u32(2)
	u32(0)
	u64(0)
	u64(0)
	pad(13 * 8)
	u32(0x4D0)
	u32(ctx)
	streams = append(streams, stream{mdException, begin() - r, r})
	binary.LittleEndian.PutUint32(b[0:], 0x504D444D)
	binary.LittleEndian.PutUint32(b[8:], uint32(len(streams)))
	binary.LittleEndian.PutUint32(b[12:], 32)
	for k, s := range streams {
		binary.LittleEndian.PutUint32(b[32+12*k:], s.typ)
		binary.LittleEndian.PutUint32(b[32+12*k+4:], s.size)
		binary.LittleEndian.PutUint32(b[32+12*k+8:], s.rva)
	}
	p := filepath.Join(t.TempDir(), "crashy.4242.dmp")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadCrash(t *testing.T) (crashState, string) {
	var st crashState
	b, err := os.ReadFile("testdata/unwind/crash.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	dll, _ := filepath.Abs("testdata/unwind/crashy.dll")
	return st, dll
}

func TestUnwindChain(t *testing.T) {
	st, dll := loadCrash(t)
	a, err := AnalyzeDump(writeDump(t, st, dll, st.Stamp), "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range a.Chain {
		got = append(got, f.Offset+" "+f.Function)
	}
	// leaf_read (no frame: no name, its bounds are not in the unwind data), big (internal: no name), outer, framed
	want := []string{"0x1000 ", "0x10", "0x10", "0x10"}
	if len(got) != 4 || a.ChainEnd != "" {
		t.Fatalf("chain %q, end %q", got, a.ChainEnd)
	}
	for k := range want {
		if !strings.HasPrefix(got[k], want[k]) {
			t.Fatalf("frame %d: %q (all: %q)", k, got[k], got)
		}
	}
	// each caller frame is inside the function it should be (by the unwind data's bounds)
	in := func(k int, lo, hi uint64) {
		var off uint64
		if _, err := fmtSscan(a.Chain[k].Offset, &off); err != nil || off <= lo || off > hi {
			t.Fatalf("frame %d at %s, not in 0x%X..0x%X", k, a.Chain[k].Offset, lo, hi)
		}
	}
	in(1, 0x1010, 0x1040) // big
	in(2, 0x1040, 0x1090) // outer
	in(3, 0x1090, 0x1100) // framed
	if !strings.HasPrefix(a.Chain[2].Function, "outer+0x") || !strings.HasPrefix(a.Chain[3].Function, "framed+0x") || a.Chain[1].Function != "" {
		t.Fatalf("names: %+v", a.Chain)
	}
}

// A module's file that is not the one that ran (updated since): the chain stops there, it is not guessed.
func TestUnwindChangedFile(t *testing.T) {
	st, dll := loadCrash(t)
	a, err := AnalyzeDump(writeDump(t, st, dll, st.Stamp+1), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Chain) != 1 || !strings.Contains(a.ChainEnd, "changed since") {
		t.Fatalf("%+v %q", a.Chain, a.ChainEnd)
	}
}

func fmtSscan(s string, v *uint64) (int, error) {
	return fmt.Sscanf(s, "0x%X", v)
}

// The unwind codes Microsoft's compiler uses that the clang build above does not: registers saved with mov
// (SAVE_NONVOL, _FAR), a large allocation with a 32-bit size, a chained entry, a crash inside the prolog.
func TestUnwindCodes(t *testing.T) {
	const base = 0x10000000
	// one section at 0x1000 holding the unwind data
	sec := make([]byte, 0x200)
	put := func(off int, b ...byte) { copy(sec[off:], b) }
	// f1 0x2000..0x2100: prolog 0x10: sub rsp,0x10008 (ALLOC_LARGE info=1) at 0x0B; mov [rsp+0x10010],rbx (SAVE_NONVOL_FAR) at 0x0F; push rdi at 0x02
	// codes, last done first: SAVE_NONVOL_FAR rbx (3 slots), ALLOC_LARGE 32-bit (3 slots), PUSH_NONVOL rdi (1 slot)
	put(0x00, 0x01, 0x10, 7, 0x00)
	put(0x04, 0x0F, 0x35, 0x08, 0x00, 0x01, 0x00) // SAVE_NONVOL_FAR rbx(3), offset 0x00010008 (in bytes)
	put(0x0A, 0x0B, 0x11, 0x08, 0x00, 0x01, 0x00) // ALLOC_LARGE info=1, size 0x00010008
	put(0x10, 0x02, 0x70)                         // PUSH_NONVOL rdi(7)
	// f2 0x3000..0x3080: SAVE_NONVOL rsi at [rsp+0x30] (2 slots), ALLOC_SMALL 0x28; then a chained part 0x3080..0x30C0
	put(0x40, 0x01, 0x08, 3, 0x00)
	put(0x44, 0x08, 0x64, 0x06, 0x00) // SAVE_NONVOL rsi(6), offset 6*8 = 0x30
	put(0x48, 0x04, 0x42)             // ALLOC_SMALL 4*8+8 = 0x28
	put(0x60, 0x21, 0x00, 0, 0x00)    // chained: version 1, UNW_FLAG_CHAININFO, no codes
	binary.LittleEndian.PutUint32(sec[0x64:], 0x3000)
	binary.LittleEndian.PutUint32(sec[0x68:], 0x3080)
	binary.LittleEndian.PutUint32(sec[0x6C:], 0x1040)
	p := &peInfo{secs: []peSec{{va: 0x1000, data: sec}}, funcs: []rtFunc{{0x2000, 0x2100, 0x1000}, {0x3000, 0x3080, 0x1040}, {0x3080, 0x30C0, 0x1060}},
		exports: []export{{0x3000, "Tier0_Thing"}}}
	stack := map[uint64]uint64{}
	mem := func(addr uint64, n int) []byte {
		v, ok := stack[addr]
		if !ok || n != 8 {
			return nil
		}
		return binary.LittleEndian.AppendUint64(nil, v)
	}
	// f1, after its prolog: rsp = S; rbx saved at S+0x10008, rdi pushed at S+0x10008 (before the allocation)... laid out:
	// [S .. S+0x10008) the allocation, [S+0x10008] rdi, [S+0x10010] return address; rbx saved at frame+0x10008?
	const S = 0x500000
	stack[S+0x10008] = 0xDDDD // rdi (pushed: right above the allocation)
	stack[S+0x10010] = 0x7777 // return address
	var r regs
	r[regRsp] = S
	// SAVE_NONVOL_FAR reads [frame + 0x10008]: the same slot as rdi here; check it loads into rbx
	ret, ok := p.unwindStep(base, base+0x2050, &r, mem)
	if !ok || ret != 0x7777 || r[7] != 0xDDDD || r[3] != 0xDDDD || r[regRsp] != S+0x10018 {
		t.Fatalf("f1: ret %x ok %v rsp %x rdi %x rbx %x", ret, ok, r[regRsp], r[7], r[3])
	}
	// inside f1's prolog, after the push only (offset 0x05): only PUSH_NONVOL is undone
	r = regs{}
	r[regRsp] = S
	stack[S] = 0xAAAA
	stack[S+8] = 0x8888
	ret, ok = p.unwindStep(base, base+0x2005, &r, mem)
	if !ok || ret != 0x8888 || r[7] != 0xAAAA || r[regRsp] != S+16 {
		t.Fatalf("f1 prolog: ret %x rsp %x rdi %x", ret, r[regRsp], r[7])
	}
	// the chained part of f2: unwound with f2's codes; named after f2's export
	r = regs{}
	r[regRsp] = S
	stack[S+0x30] = 0x5151 // rsi
	stack[S+0x28] = 0x9999 // return address, above the 0x28 allocation
	ret, ok = p.unwindStep(base, base+0x3090, &r, mem)
	if !ok || ret != 0x9999 || r[6] != 0x5151 || r[regRsp] != S+0x30 {
		t.Fatalf("f2 chained: ret %x rsp %x rsi %x", ret, r[regRsp], r[6])
	}
	if n := p.funcName(0x3090); n != "Tier0_Thing+0x90" {
		t.Fatalf("name %q", n)
	}
	if n := p.funcName(0x2050); n != "" {
		t.Fatalf("an internal function got a name: %q", n)
	}
}

// The same crash with the module in an addons folder: a plugin's own crash, sure.
func TestUnwindVerdictAddon(t *testing.T) {
	st, dll := loadCrash(t)
	dir := filepath.Join(t.TempDir(), "game", "csgo", "addons", "myplugin", "bin")
	_ = os.MkdirAll(dir, 0o755)
	b, _ := os.ReadFile(dll)
	p := filepath.Join(dir, "myplugin.dll")
	_ = os.WriteFile(p, b, 0o644)
	a, err := AnalyzeDump(writeDump(t, st, p, st.Stamp), filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Chain) != 4 || a.Suspect != "myplugin" || a.Sure != "sure" || a.Why != "it crashed inside it" {
		t.Fatalf("%d %q %q %q", len(a.Chain), a.Suspect, a.Sure, a.Why)
	}
	// and as a game module: the game's own code, with no addon in the chain
	game := filepath.Join(t.TempDir(), "game", "bin", "win64")
	_ = os.MkdirAll(game, 0o755)
	g := filepath.Join(game, "server.dll")
	_ = os.WriteFile(g, b, 0o644)
	a, _ = AnalyzeDump(writeDump(t, st, g, st.Stamp), filepath.Dir(filepath.Dir(game)))
	if a.Suspect != "" || a.Sure != "sure" || !strings.Contains(a.Why, "no addon in the call chain") {
		t.Fatalf("%q %q %q", a.Suspect, a.Sure, a.Why)
	}
}
