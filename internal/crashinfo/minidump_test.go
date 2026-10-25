package crashinfo

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

// dumpBuilder writes a small minidump, as Windows would, for the tests.
type dumpBuilder struct {
	b       []byte
	streams [][3]uint32 // type, size, rva
}

func (w *dumpBuilder) at() uint32   { return uint32(len(w.b)) }
func (w *dumpBuilder) u16(v uint16) { w.b = binary.LittleEndian.AppendUint16(w.b, v) }
func (w *dumpBuilder) u32(v uint32) { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *dumpBuilder) u64(v uint64) { w.b = binary.LittleEndian.AppendUint64(w.b, v) }
func (w *dumpBuilder) zero(n int)   { w.b = append(w.b, make([]byte, n)...) }
func (w *dumpBuilder) str(s string) uint32 {
	r := w.at()
	u := utf16.Encode([]rune(s))
	w.u32(uint32(len(u) * 2))
	for _, c := range u {
		w.u16(c)
	}
	return r
}
func (w *dumpBuilder) stream(t uint32, start uint32) {
	w.streams = append(w.streams, [3]uint32{t, w.at() - start, start})
}

type tmod struct {
	name       string
	base, size uint64
}

func buildDump(arch uint16, mods []tmod, unloaded []tmod, code uint32, addr uint64, params []uint64, sp uint64, stack []uint64) []byte {
	w := &dumpBuilder{}
	w.zero(32) // header, written at the end
	ptr := 8
	if arch == 0 {
		ptr = 4
	}
	// names
	names := make([]uint32, len(mods))
	for k, m := range mods {
		names[k] = w.str(m.name)
	}
	unames := make([]uint32, len(unloaded))
	for k, m := range unloaded {
		unames[k] = w.str(m.name)
	}
	// system info
	s := w.at()
	w.u16(arch)
	w.zero(54)
	w.stream(mdSystemInfo, s)
	// modules
	s = w.at()
	w.u32(uint32(len(mods)))
	for k, m := range mods {
		w.u64(m.base)
		w.u32(uint32(m.size))
		w.u32(0)
		w.u32(0)
		w.u32(names[k])
		w.u32(0xFEEF04BD)
		w.u32(0)
		w.u32(1<<16 | 2)
		w.u32(3<<16 | 4)
		w.zero(52 - 16)
		w.zero(8 + 8 + 16)
	}
	w.stream(mdModuleList, s)
	if len(unloaded) > 0 {
		s = w.at()
		w.u32(12)
		w.u32(24)
		w.u32(uint32(len(unloaded)))
		for k, m := range unloaded {
			w.u64(m.base)
			w.u32(uint32(m.size))
			w.u32(0)
			w.u32(0)
			w.u32(unames[k])
		}
		w.stream(mdUnloadedModule, s)
	}
	// stack memory and context
	stackStart := sp - 16
	mem := w.at()
	w.zero(16)
	for _, v := range stack {
		if ptr == 8 {
			w.u64(v)
		} else {
			w.u32(uint32(v))
		}
	}
	memSize := w.at() - mem
	ctx := w.at()
	w.zero(0x4D0)
	if ptr == 8 {
		binary.LittleEndian.PutUint64(w.b[ctx+0x98:], sp)
	} else {
		binary.LittleEndian.PutUint32(w.b[ctx+0xC4:], uint32(sp))
	}
	// threads
	s = w.at()
	w.u32(1)
	w.u32(42)
	w.zero(12)
	w.u64(0)
	w.u64(stackStart)
	w.u32(memSize)
	w.u32(mem)
	w.u32(0x4D0)
	w.u32(ctx)
	w.stream(mdThreadList, s)
	// exception
	s = w.at()
	w.u32(42)
	w.u32(0)
	w.u32(code)
	w.u32(0)
	w.u64(0)
	w.u64(addr)
	w.u32(uint32(len(params)))
	w.u32(0)
	for k := 0; k < 15; k++ {
		if k < len(params) {
			w.u64(params[k])
		} else {
			w.u64(0)
		}
	}
	w.u32(0x4D0)
	w.u32(ctx)
	w.stream(mdException, s)
	// directory and header
	dir := w.at()
	for _, st := range w.streams {
		w.u32(st[0])
		w.u32(st[1])
		w.u32(st[2])
	}
	binary.LittleEndian.PutUint32(w.b[0:], 0x504D444D)
	binary.LittleEndian.PutUint32(w.b[4:], 0xA793)
	binary.LittleEndian.PutUint32(w.b[8:], uint32(len(w.streams)))
	binary.LittleEndian.PutUint32(w.b[12:], dir)
	return w.b
}

const inst = `E:\Servers\CS2 - Server 1`

var cs2mods = []tmod{
	{inst + `\game\bin\win64\engine2.dll`, 0x7FF800000000, 0x1000000},
	{inst + `\game\csgo\bin\win64\server.dll`, 0x7FF810000000, 0x4000000},
	{inst + `\game\csgo\addons\metamod\bin\win64\metamod.2.cs2.dll`, 0x7FF820000000, 0x100000},
	{inst + `\game\csgo\addons\counterstrikesharp\bin\win64\counterstrikesharp.dll`, 0x7FF830000000, 0x200000},
	{inst + `\game\csgo\addons\MatchZy\bin\win64\matchzy.dll`, 0x7FF840000000, 0x100000},
	{`C:\Windows\System32\ntdll.dll`, 0x7FF850000000, 0x200000},
}

func TestDumpCrashInGameWithPluginOnStack(t *testing.T) {
	stack := []uint64{0x1234, 0x7FF810001000, 0xDEAD, 0x7FF820000500, 0x7FF840000700, 0x7FF830000100, 0x7FF800000900, 0x7FF850000010}
	b := buildDump(9, cs2mods, nil, 0xC0000005, 0x7FF8101A2B3C, []uint64{0, 0x10}, 0x5000, stack)
	a, err := analyze(b, inst)
	if err != nil {
		t.Fatal(err)
	}
	if a.Arch != "x64" || a.Module != "server.dll" || a.Offset != "0x1A2B3C" || a.Class != "game" || a.Version != "1.2.3.4" {
		t.Fatalf("%+v", a)
	}
	if a.Access != "reading a null pointer (address 0x10)" || !a.Null {
		t.Fatal(a.Access)
	}
	var mods []string
	for _, f := range a.Frames {
		mods = append(mods, f.Module)
	}
	if strings.Join(mods, " ") != "server.dll metamod.2.cs2.dll matchzy.dll counterstrikesharp.dll engine2.dll ntdll.dll" {
		t.Fatal(mods)
	}
	// MatchZy, a plugin, before the frameworks (Metamod is on almost every call)
	// without a call chain (the modules' files are not on this disk), it is named, as unsure
	if a.Suspect != "MatchZy" || !strings.Contains(a.Why, "found on the crashed thread's stack") || a.Sure != "unsure" || len(a.Addons) != 3 {
		t.Fatalf("%q %q %+v", a.Suspect, a.Why, a.Addons)
	}
}

func TestDumpCrashInsideExtension32(t *testing.T) {
	mods := []tmod{
		{`D:\css\bin\engine.dll`, 0x10000000, 0x400000},
		{`D:\css\cstrike\bin\server.dll`, 0x20000000, 0x800000},
		{`D:\css\cstrike\addons\sourcemod\extensions\dhooks.ext.2.css.dll`, 0x30000000, 0x40000},
	}
	b := buildDump(0, mods, nil, 0xC0000005, 0x30001234, []uint64{1, 0xFFFFFFFF00000000}, 0x9000, []uint64{0x20000100, 0x10000100})
	a, err := analyze(b, `D:\css`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Arch != "x86" || a.Module != "dhooks.ext.2.css.dll" || a.Class != "addon" || a.Suspect != "SourceMod extension dhooks" || a.Why != "it crashed inside it" {
		t.Fatalf("%+v", a)
	}
	if a.Access != "writing a null pointer (address 0x00000000)" {
		t.Fatal(a.Access)
	}
}

func TestDumpUnloadedAndOutside(t *testing.T) {
	gone := []tmod{{inst + `\game\csgo\addons\counterstrikesharp\plugins\old\native.dll`, 0x7FF860000000, 0x10000}}
	b := buildDump(9, cs2mods, gone, 0xC0000005, 0x7FF860000100, []uint64{8, 0x7FF860000100}, 0x5000, nil)
	a, _ := analyze(b, inst)
	if !a.Unloaded || !strings.Contains(a.Why, "unloaded") || a.Suspect != "native.dll" {
		t.Fatalf("%+v", a)
	}
	b = buildDump(9, cs2mods, nil, 0xC0000005, 0x1F00000000, nil, 0x5000, []uint64{0x7FF830000100, 0x7FF810000100})
	a, _ = analyze(b, inst)
	if a.Module != "" || a.Suspect != "CounterStrikeSharp" || !strings.Contains(a.Why, "outside any module") {
		t.Fatalf("%+v", a)
	}
}

func TestNotADump(t *testing.T) {
	if _, err := analyze([]byte("ELF....................................."), ""); err == nil {
		t.Fatal("a core file is not read")
	}
	if DumpPID(`C:\x\srcds.exe.4312.dmp`) != 4312 || DumpPID("crash_2026.mdmp") != 0 {
		t.Fatal("pid")
	}
	if addonLabel(`E:\s\game\csgo\addons\counterstrikesharp\dotnet\shared\Microsoft.NETCore.App\8.0.0\coreclr.dll`) != "CounterStrikeSharp's .NET runtime (it runs its API and plugins)" {
		t.Fatal("dotnet label")
	}
	for in, want := range map[string]string{
		`E:\s\game\csgo\addons\counterstrikesharp\bin\win64\counterstrikesharp.dll`: "CounterStrikeSharp",
		`E:\s\game\csgo\addons\counterstrikesharp\api\CounterStrikeSharp.API.dll`:   "CounterStrikeSharp API",
		`E:\s\game\csgo\addons\counterstrikesharp\api\Serilog.dll`:                  "CounterStrikeSharp API (Serilog)",
		`E:\s\game\csgo\addons\counterstrikesharp\plugins\HLstatsZ\HLstatsZ.dll`:    "CounterStrikeSharp plugin HLstatsZ",
		`E:\s\game\csgo\addons\metamod\bin\win64\metamod.2.cs2.dll`:                 "Metamod:Source",
		`E:\s\game\csgo\addons\metamod\bin\win64\server.dll`:                        "Metamod:Source (server)",
		`D:\css\cstrike\addons\sourcemod\extensions\sdkhooks.ext.2.css.dll`:         "SourceMod extension sdkhooks",
	} {
		if got := addonLabel(in); got != want {
			t.Errorf("%s: %q, not %q", in, got, want)
		}
	}
	if demangle("?Split@CSplitString@@AEAAXPEBDHPEAPEBDH_N@Z") != "CSplitString::Split" || demangle("?f@?$T@H@@QEAAXXZ") != "?f@?$T@H@@QEAAXXZ" || demangle("Plat_FloatTime") != "Plat_FloatTime" {
		t.Error("demangle")
	}
	if addonLabel(`D:\css\cstrike\addons\sourcemod\bin\x64\sourcepawn.vm.dll`) != "SourceMod's SourcePawn VM (it runs the plugins)" {
		t.Fatal("sourcepawn label")
	}
}

// SZM_WRITE_DUMP=<file> go test -run TestWriteSampleDump: writes a sample dump (a crash in server.dll of a Counter-
// Strike: Source server in /srv/css, with a SourceMod extension on the stack), for the tests of the manager.
func TestWriteSampleDump(t *testing.T) {
	out := os.Getenv("SZM_WRITE_DUMP")
	if out == "" {
		t.Skip()
	}
	mods := []tmod{
		{`/srv/css/bin/engine.dll`, 0x10000000, 0x400000},
		{`/srv/css/cstrike/bin/server.dll`, 0x20000000, 0x800000},
		{`/srv/css/cstrike/addons/metamod/bin/server.dll`, 0x28000000, 0x80000},
		{`/srv/css/cstrike/addons/sourcemod/bin/sourcemod.logic.dll`, 0x2C000000, 0x200000},
		{`/srv/css/cstrike/addons/sourcemod/extensions/sdkhooks.ext.2.css.dll`, 0x30000000, 0x40000},
	}
	b := buildDump(0, mods, nil, 0xC0000005, 0x20123456, []uint64{0, 0x24}, 0x9000,
		[]uint64{0x20000100, 0x28000100, 0x30000200, 0x2C000300, 0x10000100})
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Like a real one (CS:S, 32-bit): SourceMod's core crashed, called by the console variables (vstdlib) through Metamod.
func TestDumpSourceModCoreFromCvars(t *testing.T) {
	mods := []tmod{
		{`E:\css\bin\vstdlib.dll`, 0x10000000, 0x40000},
		{`E:\css\cstrike\addons\metamod\bin\metamod.2.css.dll`, 0x20000000, 0x40000},
		{`E:\css\cstrike\addons\sourcemod\bin\sourcemod.2.css.dll`, 0x30000000, 0x80000},
		{`C:\Windows\SysWOW64\ntdll.dll`, 0x40000000, 0x200000},
	}
	b := buildDump(0, mods, nil, 0xC0000005, 0x300133D3, []uint64{0, 4}, 0x9000,
		[]uint64{0x10004310, 0x200018E4, 0x300055B8, 0x40079106})
	a, err := analyze(b, `E:\css`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Suspect != "SourceMod" || !strings.Contains(a.Why, "the core that runs the plugins") || !strings.Contains(a.Why, "console variables and commands (vstdlib)") {
		t.Fatalf("%q %q", a.Suspect, a.Why)
	}
	if a.Frames[1].Role != "console variables and commands" || a.Frames[4].Role != "Windows" {
		t.Fatalf("%+v", a.Frames)
	}
}
