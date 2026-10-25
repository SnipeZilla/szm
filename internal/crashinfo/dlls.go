package crashinfo

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Which DLL is missing when a Windows program cannot start (0xC0000135): Windows names it only in a window, and
// the manager starts the servers without one.

// MissingDLLs gives the DLLs a Windows program imports that Windows would not find, and those the DLLs of its own
// folder import: it looks where Windows does (the program's folder, System32 or SysWOW64, Windows, then more, such
// as the PATH). Delay-loaded DLLs are not seen. nil when none is missing or the program cannot be read.
func MissingDLLs(exe, windir string, more []string) []string {
	if _, err := os.Stat(exe); err != nil {
		return nil
	}
	sys := SystemDir(exe, windir)
	appDir := filepath.Dir(exe)
	dirs := append([]string{appDir, sys, filepath.Join(windir, "System"), windir}, more...)
	listed := map[string]map[string]string{} // folder -> lower-case name -> its name
	find := func(name string) string {
		low := strings.ToLower(name)
		for _, d := range dirs {
			if d == "" {
				continue
			}
			names, ok := listed[d]
			if !ok {
				names = map[string]string{}
				if es, err := os.ReadDir(d); err == nil {
					for _, e := range es {
						names[strings.ToLower(e.Name())] = e.Name()
					}
				}
				listed[d] = names
			}
			if n, ok := names[low]; ok {
				return filepath.Join(d, n)
			}
		}
		return ""
	}
	var missing []string
	seen := map[string]bool{}
	var walk func(path string)
	walk = func(path string) {
		libs := importedDLLs(path)
		for _, lib := range libs {
			low := strings.ToLower(lib)
			if lib == "" || seen[low] || apiSetRe.MatchString(low) {
				continue
			}
			seen[low] = true
			switch at := find(lib); {
			case at != "":
				if strings.EqualFold(filepath.Dir(at), appDir) {
					walk(at) // one of the program's own DLLs: what it needs too
				}
			case sideBySideRe.MatchString(low) && inSideBySide(windir, lib):
				// the Visual C++ 2005 / 2008 runtimes are in WinSxS
			default:
				missing = append(missing, lib)
			}
		}
	}
	walk(exe)
	return missing
}

// importedDLLs reads the names of the DLLs a program or DLL imports (its import directory; debug/pe does not give
// them). nil when it cannot be read.
func importedDLLs(path string) []string {
	f, err := pe.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var dir pe.DataDirectory
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		if h.NumberOfRvaAndSizes > pe.IMAGE_DIRECTORY_ENTRY_IMPORT {
			dir = h.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
		}
	case *pe.OptionalHeader32:
		if h.NumberOfRvaAndSizes > pe.IMAGE_DIRECTORY_ENTRY_IMPORT {
			dir = h.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
		}
	}
	if dir.VirtualAddress == 0 {
		return nil
	}
	data := map[*pe.Section][]byte{}
	// at: the bytes from an address of the loaded program on (nil when no section has it)
	at := func(rva uint32) []byte {
		for _, s := range f.Sections {
			if rva >= s.VirtualAddress && rva-s.VirtualAddress < max(s.VirtualSize, s.Size) {
				b, ok := data[s]
				if !ok {
					b, _ = s.Data()
					data[s] = b
				}
				if off := rva - s.VirtualAddress; off < uint32(len(b)) {
					return b[off:]
				}
				return nil
			}
		}
		return nil
	}
	var out []string
	d := at(dir.VirtualAddress)
	for k := 0; len(d) >= 20 && k < 4096; k++ {
		name, thunk := binary.LittleEndian.Uint32(d[12:16]), binary.LittleEndian.Uint32(d[16:20])
		if name == 0 && thunk == 0 {
			break // the empty entry that ends it
		}
		d = d[20:]
		b := at(name)
		if n := bytes.IndexByte(b, 0); n > 0 && n < 260 {
			out = append(out, string(b[:n]))
		}
	}
	return out
}

var (
	// API sets (api-ms-win-core-…): names Windows maps to its own DLLs, not files
	apiSetRe     = regexp.MustCompile(`^(api|ext)-ms-`)
	sideBySideRe = regexp.MustCompile(`^(msvc[mpr]|atl|mfc|vcomp)(80|90)[a-z]*\.dll$`)
)

func inSideBySide(windir, name string) bool {
	m, _ := filepath.Glob(filepath.Join(windir, "WinSxS", "*", name))
	return len(m) > 0
}

// DLLHint says where a missing DLL comes from, when it is part of something to install ("" when not known). x64:
// the program is 64-bit; server: Windows Server.
func DLLHint(name string, x64, server bool) string {
	low := strings.ToLower(name)
	arch := map[bool]string{true: "x64", false: "x86"}[x64]
	switch {
	case mediaFoundationRe.MatchString(low):
		if server {
			return "it is part of Media Foundation, which Windows Server does not have by default: add it with Install-WindowsFeature Server-Media-Foundation (PowerShell, as administrator), then restart Windows"
		}
		return "it is part of Media Foundation, which the N editions of Windows do not have: add the Media Feature Pack (Settings, Optional features)"
	case directXRe.MatchString(low):
		return "it is part of the DirectX End-User Runtime (June 2010), which Windows does not have by default"
	}
	for _, v := range vcRuntimes {
		if v.re.MatchString(low) {
			return "it is part of the Visual C++ " + v.year + " runtime (" + arch + "), installed with Microsoft's vc_redist." + arch + ".exe"
		}
	}
	return ""
}

// DLLPart names what a missing DLL is part of, when it is something to install on Windows ("" when not known):
// restarting the program, or checking its files, does not bring it back.
func DLLPart(name string, x64 bool) string {
	low := strings.ToLower(name)
	switch {
	case mediaFoundationRe.MatchString(low):
		return "Media Foundation"
	case directXRe.MatchString(low):
		return "the DirectX End-User Runtime (June 2010)"
	}
	for _, v := range vcRuntimes {
		if v.re.MatchString(low) {
			return "the Visual C++ " + v.year + " runtime (" + map[bool]string{true: "x64", false: "x86"}[x64] + ")"
		}
	}
	return ""
}

var (
	mediaFoundationRe = regexp.MustCompile(`^(mfreadwrite|mfplat|mf|mfcore|mfplay|mfsrcsnk|mfsensorgroup|evr|msmpeg2vdec|msmpeg2adec|mfmp4srcsnk|mfasfsrcsnk)\.dll$`)
	directXRe         = regexp.MustCompile(`^(d3dx(9|10|11)_\d+|d3dcompiler_(3\d|4[0-3])|xinput1_[123]|x3daudio1_\d|xapofx1_\d|xaudio2_[0-7])\.dll$`)
	vcRuntimes        = []struct {
		re   *regexp.Regexp
		year string
	}{
		{regexp.MustCompile(`^(vcruntime140(_1)?|msvcp140(_[a-z0-9_]+)?|concrt140|vcomp140|vccorlib140|mfc140[a-z]*)\.dll$`), "2015-2022"},
		{regexp.MustCompile(`^(msvc[pr]120|vcomp120|mfc120[a-z]*)\.dll$`), "2013"},
		{regexp.MustCompile(`^(msvc[pr]110|vcomp110|mfc110[a-z]*)\.dll$`), "2012"},
		{regexp.MustCompile(`^(msvc[pr]100|vcomp100|mfc100[a-z]*)\.dll$`), "2010"},
	}
)

// SystemDir: where Windows takes its own DLLs for a program from: System32, or SysWOW64 for a 32-bit program on
// 64-bit Windows.
func SystemDir(exe, windir string) string {
	if !IsX64(exe) {
		if st, err := os.Stat(filepath.Join(windir, "SysWOW64")); err == nil && st.IsDir() {
			return filepath.Join(windir, "SysWOW64")
		}
	}
	return filepath.Join(windir, "System32")
}

// IsX64 tells whether a Windows program is 64-bit (true when it cannot be read: the usual).
func IsX64(exe string) bool {
	f, err := pe.Open(exe)
	if err != nil {
		return true
	}
	defer f.Close()
	return f.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_I386
}
