package crashinfo

import (
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

// A crash dump (.dmp, .mdmp: Windows' minidump format, also Breakpad's) read without symbols: where it crashed (the
// module and the offset in it), how (the exception), and which modules were running on the crashed thread (a scan of
// its stack for addresses in their code). The plugins and extensions found there point at the cause.

// DumpAnalysis is what a crash dump tells.
type DumpAnalysis struct {
	File      string  `json:"file"`
	Arch      string  `json:"arch"`                // x64, x86
	Code      string  `json:"code"`                // 0xC0000005
	Exception string  `json:"exception,omitempty"` // in words
	Access    string  `json:"access,omitempty"`    // "reading a null pointer (address 0x10)", "writing address 0x…"
	Null      bool    `json:"null,omitempty"`      // it used a null pointer: something it used was not there
	Address   string  `json:"address"`             // where: 0x7FF8…
	Module    string  `json:"module,omitempty"`    // the module it crashed in: server.dll
	Path      string  `json:"path,omitempty"`
	Version   string  `json:"version,omitempty"`
	Offset    string  `json:"offset,omitempty"` // in the module: 0x1A2B3C
	Class     string  `json:"class,omitempty"`  // addon, game, system, other
	Unloaded  bool    `json:"unloaded,omitempty"`
	Frames    []Frame `json:"frames,omitempty"` // the crashed thread: the crash first, then what called it (likely)
	// Chain: the real call chain of the crashed thread (x64, unwound with the modules' unwind data): the crash
	// first, then its caller, and so on. Empty when it could not be followed (then Frames, a stack scan, is all).
	Chain    []Frame `json:"chain,omitempty"`
	ChainEnd string  `json:"chain_end,omitempty"` // why the chain stops where it does, when it stops early
	Addons   []Frame `json:"addons,omitempty"`    // the addon modules among them, each once, the nearest first
	// Loaded: every addon module the process had loaded, with its version (what a developer asks first)
	Loaded  []Loaded `json:"loaded,omitempty"`
	Suspect string   `json:"suspect,omitempty"`
	Why     string   `json:"why,omitempty"` // why it is named
	// Sure: how much the dump proves it. "sure": it crashed inside it; "likely": it called the code that crashed;
	// "unsure": it was only near (a framework every call goes through, or found on the stack without a call chain)
	Sure    string `json:"sure,omitempty"`
	Threads int    `json:"threads"`
	Note    string `json:"note,omitempty"`
}

// Loaded is an addon module the process had loaded.
type Loaded struct {
	Module  string `json:"module"`
	Label   string `json:"label"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path"`
}

// Frame is a place in a module: module + offset.
type Frame struct {
	Module string `json:"module"`
	Offset string `json:"offset"`
	Class  string `json:"class"`
	Label  string `json:"label,omitempty"` // for an addon: "SourceMod extension dhooks"
	Role   string `json:"role,omitempty"`  // for the game's and Windows' modules: what they do
	Path   string `json:"path,omitempty"`
	// Function: the exported function it is in, when the module exports one starting exactly there: "V_strncpy+0x23"
	Function string `json:"function,omitempty"`
}

type mdModule struct {
	base, size uint64
	stamp      uint32 // TimeDateStamp of its file: the file on disk must be the same one to read its unwind data
	name       string
	version    string
	code       [][2]uint64 // executable ranges, relative (from the file on disk), nil when unknown
	unloaded   bool
}

type minidump struct {
	b       []byte
	streams map[uint32][2]uint32 // type -> size, rva
}

func (d *minidump) u16(off uint32) uint16 {
	if int(off)+2 > len(d.b) {
		return 0
	}
	return binary.LittleEndian.Uint16(d.b[off:])
}
func (d *minidump) u32(off uint32) uint32 {
	if int(off)+4 > len(d.b) {
		return 0
	}
	return binary.LittleEndian.Uint32(d.b[off:])
}
func (d *minidump) u64(off uint32) uint64 {
	if int(off)+8 > len(d.b) {
		return 0
	}
	return binary.LittleEndian.Uint64(d.b[off:])
}

func (d *minidump) str(rva uint32) string {
	n := d.u32(rva)
	if n == 0 || int(rva)+4+int(n) > len(d.b) || n > 4096 {
		return ""
	}
	u := make([]uint16, n/2)
	for k := range u {
		u[k] = d.u16(rva + 4 + uint32(k*2))
	}
	return string(utf16.Decode(u))
}

const (
	mdThreadList     = 3
	mdModuleList     = 4
	mdMemoryList     = 5
	mdException      = 6
	mdSystemInfo     = 7
	mdMemory64List   = 9
	mdUnloadedModule = 14
)

// AnalyzeDump reads a crash dump. installDir is the server's folder (to tell the game's modules from the rest).
func AnalyzeDump(path, installDir string) (*DumpAnalysis, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > 512<<20 {
		return nil, errors.New("too big to read")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	a, err := analyze(b, installDir)
	if a != nil {
		a.File = path
	}
	return a, err
}

func analyze(b []byte, installDir string) (*DumpAnalysis, error) {
	d := &minidump{b: b, streams: map[uint32][2]uint32{}}
	if len(b) < 32 || d.u32(0) != 0x504D444D {
		return nil, errors.New("not a crash dump (minidump)")
	}
	n, dir := d.u32(8), d.u32(12)
	for k := uint32(0); k < n && k < 512; k++ {
		e := dir + k*12
		d.streams[d.u32(e)] = [2]uint32{d.u32(e + 4), d.u32(e + 8)}
	}
	a := &DumpAnalysis{}
	ptr := uint64(8)
	switch d.u16(d.streams[mdSystemInfo][1]) {
	case 0:
		a.Arch, ptr = "x86", 4
	case 9:
		a.Arch = "x64"
	case 12:
		a.Arch = "arm64"
	default:
		a.Arch = "?"
	}
	mods := d.modules(installDir)
	for _, m := range mods {
		if !m.unloaded && classify(m.name, installDir) == "addon" {
			v := m.version
			if v == "0.0.0.0" {
				v = ""
			}
			a.Loaded = append(a.Loaded, Loaded{Module: filepath.Base(winPath(m.name)), Label: addonLabel(m.name), Version: v, Path: m.name})
		}
	}
	if s, ok := d.streams[mdThreadList]; ok {
		a.Threads = int(d.u32(s[1]))
	}
	ex, ok := d.streams[mdException]
	if !ok {
		a.Note = "The dump has no crash in it (written on demand, not by a crash)."
		return a, nil
	}
	r := ex[1]
	tid := d.u32(r)
	code := d.u32(r + 8)
	addr := d.u64(r + 24)
	np := d.u32(r + 32)
	var params []uint64
	for k := uint32(0); k < np && k < 15; k++ {
		params = append(params, d.u64(r+40+k*8))
	}
	ctxSize, ctxRva := d.u32(r+160), d.u32(r+164)
	if ptr == 4 {
		addr &= 0xFFFFFFFF
	}
	a.Code = fmt.Sprintf("0x%08X", code)
	if w, ok := ntStatus[code]; ok {
		a.Exception = w
	}
	a.Address = hexAddr(addr, ptr)
	if ptr == 4 {
		for k := range params {
			params[k] &= 0xFFFFFFFF
		}
	}
	if code == 0xC0000005 && len(params) >= 2 {
		op := map[uint64]string{0: "reading", 1: "writing", 8: "running code at"}[params[0]]
		if op == "" {
			op = "using"
		}
		a.Access = op + " address " + hexAddr(params[1], ptr)
		if params[1] < 0x10000 {
			a.Null = true
			a.Access = op + " a null pointer (address " + hexAddr(params[1], ptr) + ")"
		}
	}
	// where it crashed
	if m := findModule(mods, addr); m != nil {
		a.Module, a.Path, a.Version = filepath.Base(winPath(m.name)), m.name, m.version
		a.Offset = fmt.Sprintf("0x%X", addr-m.base)
		a.Class = classify(m.name, installDir)
		a.Unloaded = m.unloaded
		a.Frames = append(a.Frames, frameOf(m, addr, installDir))
	} else {
		a.Frames = append(a.Frames, Frame{Module: "?", Offset: a.Address, Class: "unknown"})
	}
	// what was running on the crashed thread: return addresses on its stack
	var sp uint64
	if ctxRva != 0 && ctxSize > 0 {
		if ptr == 8 {
			sp = d.u64(ctxRva + 0x98) // CONTEXT.Rsp
		} else {
			sp = uint64(d.u32(ctxRva + 0xC4)) // CONTEXT.Esp
		}
	}
	if stack, base := d.stackOf(tid, sp); stack != nil && sp >= base {
		from := sp - base
		end := uint64(len(stack))
		if end-from > 256<<10 {
			end = from + 256<<10
		}
		last := a.Frames[0].Module
		for off := from; off+ptr <= end && len(a.Frames) < 60; off += ptr {
			var v uint64
			if ptr == 8 {
				v = binary.LittleEndian.Uint64(stack[off:])
			} else {
				v = uint64(binary.LittleEndian.Uint32(stack[off:]))
			}
			m := findModule(mods, v)
			if m == nil || m.unloaded || !m.inCode(v) {
				continue
			}
			f := frameOf(m, v, installDir)
			if f.Module == last {
				continue // the same module again: one line
			}
			last = f.Module
			a.Frames = append(a.Frames, f)
		}
	} else {
		a.Note = "The dump has no stack of the crashed thread: only where it crashed is known."
	}
	if ptr == 8 && ctxRva != 0 && ctxSize >= 0x100 {
		a.Chain, a.ChainEnd = d.unwind(ctxRva, mods, installDir)
	}
	a.Addons = addonsOf(a.Frames)
	a.Suspect, a.Why, a.Sure = suspect(a)
	return a, nil
}

// suspect names what most likely caused the crash, and how sure that is. With the call chain (x64), only what was
// really calling counts; without it (x86, or a chain cut short), what the stack scan found is named, as unsure.
func suspect(a *DumpAnalysis) (string, string, string) {
	if a.Unloaded {
		return a.Module, "it crashed in " + a.Module + " after it was unloaded: a plugin removed (or reloaded) while still in use", "sure"
	}
	frames := a.Frames
	chained := len(a.Chain) > 1 || len(a.Chain) == 1 && a.ChainEnd == ""
	if chained {
		frames = a.Chain
	}
	if len(frames) == 0 {
		return "", "", ""
	}
	top := frames[0]
	if top.Class == "addon" {
		if rank(top) == 1 {
			return top.Label, "it crashed inside it", "sure"
		}
		// a framework (SourceMod's or CounterStrikeSharp's core, Metamod): a plugin of it calling it is the usual cause
		for _, f := range frames[1:] {
			if f.Class == "addon" && rank(f) == 1 {
				return f.Label, "it called " + top.Label + ", which crashed", "likely"
			}
		}
		why := "it crashed inside it. It is the core that runs the plugins: a plugin calling it, or something it reacts to (a map change, a config, a player), is often the real cause"
		if c := callerRole(frames); c != "" {
			why += "; " + c
		}
		return top.Label, why, "likely"
	}
	if a.Module == "" {
		if best := bestAddon(addonsOf(frames)); best != nil {
			return best.Label, "it crashed in code outside any module (made at run time: a .NET or SourcePawn plugin, or a bad jump); " + best.Label + " was found on the crashed thread's stack, but whether it was calling is not known", "unsure"
		}
		return "", "it crashed in code outside any module (made at run time: a .NET or SourcePawn plugin, or a bad jump)", "unsure"
	}
	addons := addonsOf(frames)
	where := "the game's own code (" + a.Module + ")"
	if a.Class == "system" {
		where = "Windows' " + a.Module + ", called by the game"
	}
	if !chained {
		if best := bestAddon(addons); best != nil {
			return best.Label, "it crashed in " + where + "; " + best.Label + " was found on the crashed thread's stack, but the call chain is not known, so whether it was calling is not sure", "unsure"
		}
		return "", "it crashed in " + where + "; no addon was found on its stack", "unsure"
	}
	// the call chain is known: the nearest plugin that called down to the crash
	for _, f := range addons {
		if rank(f) == 1 {
			return f.Label, "it called the code that crashed, in " + where, "likely"
		}
	}
	if len(addons) > 0 {
		f := addons[0]
		why := "it crashed in " + where + ". " + Family(f.Path) + " was in the call chain: it passes the game's calls (commands, events) on to its plugins and back, so it is in most chains; the crash may well be the game's own"
		if strings.Contains(a.ChainEnd, "code made at run time") {
			why += ". The chain starts in code made at run time, called by " + Family(f.Path) + "'s scripted side (its API or a plugin): the call came from there"
		}
		return "", why, "unsure"
	}
	return "", "it crashed in " + where + ", with no addon in the call chain: the game itself, or what it was given (a map, a command, a config)", "sure"
}

// addonsOf: the addons among frames, each once, the nearest first
func addonsOf(frames []Frame) []Frame {
	var out []Frame
	seen := map[string]bool{}
	for _, f := range frames {
		if f.Class == "addon" && !seen[strings.ToLower(f.Path)] {
			seen[strings.ToLower(f.Path)] = true
			out = append(out, f)
		}
	}
	return out
}

// rank: a plugin or extension (1) before the frameworks almost always on the stack: SourceMod's,
// CounterStrikeSharp's and AMX Mod X's cores (2), Metamod (3).
func rank(f Frame) int {
	pkg, rest := addonParts(f.Path)
	low := strings.ToLower(rest)
	switch pkg {
	case "metamod":
		return 3
	case "sourcemod":
		if strings.Contains(low, "/extensions/") {
			return 1
		}
		return 2
	case "counterstrikesharp", "amxmodx":
		if strings.Contains(low, "/plugins/") || strings.Contains(low, "/modules/") {
			return 1
		}
		return 2
	}
	return 1
}

// bestAddon: the nearest addon to the crash of the best rank.
func bestAddon(list []Frame) *Frame {
	var best *Frame
	for k := range list {
		if best == nil || rank(list[k]) < rank(*best) {
			best = &list[k]
		}
	}
	return best
}

func hexAddr(v, ptr uint64) string {
	if ptr == 4 {
		return fmt.Sprintf("0x%08X", v)
	}
	return fmt.Sprintf("0x%X", v)
}

func findModule(mods []*mdModule, addr uint64) *mdModule {
	for _, m := range mods {
		if !m.unloaded && addr >= m.base && addr < m.base+m.size {
			return m
		}
	}
	for _, m := range mods {
		if m.unloaded && addr >= m.base && addr < m.base+m.size {
			return m
		}
	}
	return nil
}

// inCode says whether an address is in the module's code (its executable sections), when they are known.
func (m *mdModule) inCode(addr uint64) bool {
	if m.code == nil {
		return true
	}
	rel := addr - m.base
	for _, c := range m.code {
		if rel >= c[0] && rel < c[1] {
			return true
		}
	}
	return false
}

func frameOf(m *mdModule, addr uint64, installDir string) Frame {
	f := Frame{Module: filepath.Base(winPath(m.name)), Offset: fmt.Sprintf("0x%X", addr-m.base), Class: classify(m.name, installDir), Path: m.name}
	if f.Class == "addon" {
		f.Label = addonLabel(m.name)
	} else {
		f.Role = moduleRoles[strings.ToLower(f.Module)]
	}
	return f
}

// moduleRoles say what the game's and Windows' modules do, to read a stack.
var moduleRoles = map[string]string{
	"vstdlib.dll": "console variables and commands", "libvstdlib.so": "console variables and commands",
	"engine.dll": "the engine", "engine_srv.so": "the engine", "engine2.dll": "the engine", "libengine2.so": "the engine",
	"server.dll": "the game's code", "server.so": "the game's code", "server_srv.so": "the game's code",
	"tier0.dll": "memory, threads", "libtier0.so": "memory, threads",
	"materialsystem.dll": "materials (textures)", "datacache.dll": "models, sounds cache", "studiorender.dll": "models",
	"filesystem_stdio.dll": "files", "dedicated.dll": "the dedicated server", "networksystem.dll": "network",
	"steam_api.dll": "Steam", "steam_api64.dll": "Steam", "steamclient.dll": "Steam", "steamclient64.dll": "Steam",
	"srcds.exe": "the server program", "srcds_win64.exe": "the server program", "cs2.exe": "the server program",
	"hlds.exe": "the server program", "hw.dll": "the engine (GoldSrc)", "swds.dll": "the engine (GoldSrc)",
	"ntdll.dll": "Windows", "kernel32.dll": "Windows", "kernelbase.dll": "Windows", "ucrtbase.dll": "Windows (C runtime)",
	"msvcrt.dll": "Windows (C runtime)", "ws2_32.dll": "Windows (network)",
}

// callerRole says what called the crashed code, when the game did it through a part worth knowing.
func callerRole(frames []Frame) string {
	for _, f := range frames[1:] {
		if f.Class == "addon" && f.Label == frames[0].Label {
			continue
		}
		switch strings.ToLower(f.Module) {
		case "vstdlib.dll", "libvstdlib.so":
			return "it was called by the console variables and commands (vstdlib): a cvar changed or a command ran, as configs do at a map change"
		case "materialsystem.dll", "datacache.dll", "studiorender.dll":
			return "it was called while loading models or materials (" + f.Module + "): often a map's or a plugin's files"
		}
		if f.Class == "addon" {
			return "" // another addon called it: shown on the stack
		}
	}
	return ""
}

// winPath lets filepath.Base cut a Windows path on Linux too.
func winPath(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// classify tells an addon's module (in an addons folder) from the game's, Windows' and the rest.
func classify(path, installDir string) string {
	p := strings.ToLower(winPath(path))
	switch {
	case strings.Contains(p, "/addons/"):
		return "addon"
	case strings.Contains(p, "/windows/system32/") || strings.Contains(p, "/windows/syswow64/") || strings.Contains(p, "/windows/winsxs/"):
		return "system"
	case installDir != "" && strings.HasPrefix(p, strings.ToLower(strings.TrimRight(winPath(installDir), "/"))+"/"):
		return "game"
	}
	return "other"
}

var extRe = regexp.MustCompile(`(?i)\.(ext\.)?(2\.[a-z0-9]+\.)?(dll|so)$`)

// addonParts: an addon module's package (the folder under addons/, lower case) and its path from there.
func addonParts(path string) (string, string) {
	p := winPath(path)
	k := strings.Index(strings.ToLower(p), "/addons/")
	if k < 0 {
		return "", ""
	}
	rest := p[k+len("/addons/"):]
	pkg, _, _ := strings.Cut(rest, "/")
	return strings.ToLower(pkg), rest
}

// Family names an addon package for people: "CounterStrikeSharp", "SourceMod".
func Family(path string) string {
	pkg, rest := addonParts(path)
	switch pkg {
	case "sourcemod":
		return "SourceMod"
	case "counterstrikesharp":
		return "CounterStrikeSharp"
	case "metamod":
		return "Metamod:Source"
	case "amxmodx":
		return "AMX Mod X"
	case "":
		return extRe.ReplaceAllString(filepath.Base(winPath(path)), "")
	}
	first, _, _ := strings.Cut(rest, "/")
	return first
}

// addonLabel names an addon's module for people: "SourceMod extension dhooks", "CounterStrikeSharp". It says what
// the module is, never more (a module of a .NET runtime loaded is not a plugin running).
func addonLabel(path string) string {
	pkg, rest := addonParts(path)
	parts := strings.Split(rest, "/")
	file := extRe.ReplaceAllString(parts[len(parts)-1], "")
	low := strings.ToLower(rest)
	fam := Family(path)
	switch {
	case pkg == "":
		return file
	case pkg == "sourcemod" && strings.Contains(low, "/extensions/"):
		return "SourceMod extension " + file
	case pkg == "sourcemod" && strings.HasPrefix(strings.ToLower(file), "sourcepawn"):
		return "SourceMod's SourcePawn VM (it runs the plugins)"
	case pkg == "counterstrikesharp" && strings.Contains(low, "/plugins/") && len(parts) > 2:
		return "CounterStrikeSharp plugin " + parts[len(parts)-2]
	case pkg == "counterstrikesharp" && strings.Contains(low, "/dotnet/"):
		return "CounterStrikeSharp's .NET runtime (it runs its API and plugins)"
	case pkg == "counterstrikesharp" && strings.Contains(low, "/api/"):
		if strings.EqualFold(file, "CounterStrikeSharp.API") {
			return "CounterStrikeSharp API"
		}
		return "CounterStrikeSharp API (" + file + ")"
	case strings.EqualFold(file, pkg) || pkg == "counterstrikesharp" && strings.Contains(low, "/bin/"):
		return fam
	case pkg == "amxmodx" && strings.Contains(low, "/modules/"):
		return "AMX Mod X module " + file
	case len(parts) > 1:
		return fam + " (" + file + ")"
	}
	return file
}

// modules reads the module list (and the unloaded ones), with the code sections of each from its file on disk.
func (d *minidump) modules(installDir string) []*mdModule {
	var out []*mdModule
	if s, ok := d.streams[mdModuleList]; ok {
		n := d.u32(s[1])
		for k := uint32(0); k < n && k < 4096; k++ {
			e := s[1] + 4 + k*108
			m := &mdModule{base: d.u64(e), size: uint64(d.u32(e + 8)), stamp: d.u32(e + 16), name: d.str(d.u32(e + 20))}
			if d.u32(e+24) == 0xFEEF04BD {
				ms, ls := d.u32(e+32), d.u32(e+36)
				m.version = fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xFFFF, ls>>16, ls&0xFFFF)
			}
			m.code = codeRanges(m.name)
			out = append(out, m)
		}
	}
	if s, ok := d.streams[mdUnloadedModule]; ok {
		hdr, size, n := d.u32(s[1]), d.u32(s[1]+4), d.u32(s[1]+8)
		for k := uint32(0); k < n && k < 1024 && size >= 24; k++ {
			e := s[1] + hdr + k*size
			out = append(out, &mdModule{base: d.u64(e), size: uint64(d.u32(e + 8)), name: d.str(d.u32(e + 20)), unloaded: true})
		}
	}
	return out
}

// codeRanges reads the executable sections of a module's file (nil when it can not be read).
func codeRanges(path string) [][2]uint64 {
	if path == "" {
		return nil
	}
	f, err := pe.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out [][2]uint64
	for _, s := range f.Sections {
		if s.Characteristics&0x20000000 != 0 { // IMAGE_SCN_MEM_EXECUTE
			size := s.VirtualSize
			if size == 0 {
				size = s.Size
			}
			out = append(out, [2]uint64{uint64(s.VirtualAddress), uint64(s.VirtualAddress) + uint64(size)})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stackOf gives the memory of a thread's stack and where it starts.
func (d *minidump) stackOf(tid uint32, sp uint64) ([]byte, uint64) {
	if s, ok := d.streams[mdThreadList]; ok {
		n := d.u32(s[1])
		for k := uint32(0); k < n && k < 65536; k++ {
			e := s[1] + 4 + k*48
			if d.u32(e) != tid {
				continue
			}
			start, size, rva := d.u64(e+24), d.u32(e+32), d.u32(e+36)
			if rva != 0 && size > 0 && int(rva)+int(size) <= len(d.b) {
				return d.b[rva : rva+size], start
			}
		}
	}
	// a full dump: the stack is in the memory lists
	if sp == 0 {
		return nil, 0
	}
	if s, ok := d.streams[mdMemoryList]; ok {
		n := d.u32(s[1])
		for k := uint32(0); k < n && k < 1<<20; k++ {
			e := s[1] + 4 + k*16
			start, size, rva := d.u64(e), d.u32(e+8), d.u32(e+12)
			if sp >= start && sp < start+uint64(size) && int(rva)+int(size) <= len(d.b) {
				return d.b[rva : rva+size], start
			}
		}
	}
	if s, ok := d.streams[mdMemory64List]; ok {
		n, rva := d.u64(s[1]), d.u64(s[1]+8)
		for k := uint64(0); k < n && k < 1<<20; k++ {
			e := s[1] + 16 + uint32(k)*16
			start, size := d.u64(e), d.u64(e+8)
			if sp >= start && sp < start+size && rva+size <= uint64(len(d.b)) {
				return d.b[rva : rva+size], start
			}
			rva += size
		}
	}
	return nil, 0
}

var dumpPidRe = regexp.MustCompile(`\.(\d+)\.(dmp|mdmp)$`)

// DumpPID gives the PID in a dump's name as Windows writes it (srcds.exe.1234.dmp), or 0.
func DumpPID(name string) int {
	m := dumpPidRe.FindStringSubmatch(strings.ToLower(filepath.Base(name)))
	if m == nil {
		return 0
	}
	n := 0
	fmt.Sscan(m[1], &n)
	return n
}

// AddonFamily is an addon as people know it ("CounterStrikeSharp 1.0.377"), from all its modules loaded.
type AddonFamily struct {
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	Runtime string   `json:"runtime,omitempty"` // the .NET it brings: ".NET 10.0.3"
	Modules int      `json:"modules"`
	Parts   []string `json:"parts,omitempty"` // its extensions, plugins or modules loaded: "sdkhooks"
}

var netVersionRe = regexp.MustCompile(`(?i)/Microsoft\.NETCore\.App/([0-9][0-9.]*)/`)

// Families groups the addon modules loaded by addon, each with its version.
func Families(loaded []Loaded) []AddonFamily {
	var out []AddonFamily
	at := map[string]int{}
	for _, l := range loaded {
		name := Family(l.Path)
		k, ok := at[name]
		if !ok {
			k = len(out)
			at[name] = k
			out = append(out, AddonFamily{Name: name})
		}
		f := &out[k]
		f.Modules++
		pkg, rest := addonParts(l.Path)
		low := strings.ToLower(rest)
		file := extRe.ReplaceAllString(filepath.Base(winPath(l.Path)), "")
		switch {
		case strings.Contains(low, "/dotnet/"):
			if m := netVersionRe.FindStringSubmatch(winPath(l.Path)); m != nil {
				f.Runtime = ".NET " + m[1]
			}
			continue
		case rank(Frame{Path: l.Path}) == 1 && pkg != "" && !strings.EqualFold(file, pkg):
			part := strings.TrimPrefix(strings.TrimPrefix(addonLabel(l.Path), name+" "), "extension ")
			f.Parts = append(f.Parts, strings.TrimPrefix(part, "plugin "))
		}
		// its version: the API's (CounterStrikeSharp's own module has none), else its core's, else the first known
		core := strings.EqualFold(file, "CounterStrikeSharp.API") || strings.EqualFold(file, pkg) || strings.HasPrefix(strings.ToLower(file), pkg+".")
		if l.Version != "" && l.Version != "0.0.0.0" && (f.Version == "" || core && !strings.EqualFold(file, pkg+".logic")) {
			if core || f.Version == "" {
				f.Version = l.Version
			}
		}
	}
	return out
}
