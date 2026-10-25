package crashinfo

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePE writes a small 64-bit Windows program (or DLL) that imports dlls: enough for its import directory.
func writePE(t *testing.T, path string, dlls ...string) {
	t.Helper()
	const va = 0x1000
	var sec bytes.Buffer
	names := []byte{}
	nameAt := make([]uint32, len(dlls))
	descSize := 20 * (len(dlls) + 1)
	for k, d := range dlls {
		nameAt[k] = va + uint32(descSize+len(names))
		names = append(append(names, d...), 0)
	}
	thunkAt := va + uint32(descSize+len(names))
	for k := range dlls {
		_ = binary.Write(&sec, binary.LittleEndian, [5]uint32{0, 0, 0, nameAt[k], thunkAt})
	}
	sec.Write(make([]byte, 20))
	sec.Write(names)
	sec.Write(make([]byte, 16)) // the (empty) thunks
	for sec.Len()%0x200 != 0 {
		sec.WriteByte(0)
	}
	var b bytes.Buffer
	dos := make([]byte, 0x40)
	copy(dos, "MZ")
	binary.LittleEndian.PutUint32(dos[0x3c:], 0x40)
	b.Write(dos)
	b.WriteString("PE\x00\x00")
	_ = binary.Write(&b, binary.LittleEndian, pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_AMD64, NumberOfSections: 1,
		SizeOfOptionalHeader: uint16(binary.Size(pe.OptionalHeader64{})), Characteristics: 0x22})
	oh := pe.OptionalHeader64{Magic: 0x20b, ImageBase: 0x140000000, SectionAlignment: 0x1000, FileAlignment: 0x200,
		SizeOfImage: va + uint32(sec.Len()), SizeOfHeaders: 0x200, NumberOfRvaAndSizes: 16}
	oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT] = pe.DataDirectory{VirtualAddress: va, Size: uint32(descSize)}
	_ = binary.Write(&b, binary.LittleEndian, oh)
	sh := pe.SectionHeader32{VirtualSize: uint32(sec.Len()), VirtualAddress: va, SizeOfRawData: uint32(sec.Len()), PointerToRawData: 0x200, Characteristics: 0x40000040}
	copy(sh.Name[:], ".idata")
	_ = binary.Write(&b, binary.LittleEndian, sh)
	for b.Len() < 0x200 {
		b.WriteByte(0)
	}
	b.Write(sec.Bytes())
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 0xC0000135: which DLL Windows does not find, looked for where it looks, its own DLLs' needs too.
func TestMissingDLLs(t *testing.T) {
	root := t.TempDir()
	win := filepath.Join(root, "Windows")
	app := filepath.Join(root, "DayZServer")
	path := filepath.Join(root, "tools")
	for _, d := range []string{filepath.Join(win, "System32"), filepath.Join(win, "WinSxS", "amd64_microsoft.vc90.crt_1fc8b3b9a1e18e3b_9.0.30729.9625"), app, path} {
		os.MkdirAll(d, 0o755)
	}
	for _, f := range []string{filepath.Join(win, "System32", "KERNEL32.dll"), filepath.Join(win, "System32", "user32.dll"), filepath.Join(win, "WinSxS", "amd64_microsoft.vc90.crt_1fc8b3b9a1e18e3b_9.0.30729.9625", "msvcr90.dll"), filepath.Join(path, "steam_api64.dll")} {
		os.WriteFile(f, []byte("x"), 0o644)
	}
	exe := filepath.Join(app, "DayZServer_x64.exe")
	writePE(t, exe, "kernel32.dll", "api-ms-win-crt-runtime-l1-1-0.dll", "MFReadWrite.dll", "own.dll", "user32.dll", "msvcr90.dll", "steam_api64.dll")
	writePE(t, filepath.Join(app, "own.dll"), "KERNEL32.DLL", "VCRUNTIME140.dll", "mfreadwrite.dll")
	if got := importedDLLs(exe); len(got) != 7 || got[2] != "MFReadWrite.dll" {
		t.Fatal(got)
	}
	got := MissingDLLs(exe, win, []string{path})
	if strings.Join(got, ",") != "MFReadWrite.dll,VCRUNTIME140.dll" {
		t.Fatal(got)
	}
	// all there: nothing said
	os.WriteFile(filepath.Join(win, "System32", "mfreadwrite.dll"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(app, "vcruntime140.dll"), []byte("x"), 0o644)
	if got := MissingDLLs(exe, win, []string{path}); got != nil {
		t.Fatal(got)
	}
	// not a program
	if MissingDLLs(filepath.Join(path, "steam_api64.dll"), win, nil) != nil || MissingDLLs(filepath.Join(root, "none.exe"), win, nil) != nil {
		t.Fatal("not a program")
	}
	if !IsX64(exe) {
		t.Fatal("x64")
	}

	if h := DLLHint("MFReadWrite.dll", true, true); !strings.Contains(h, "Install-WindowsFeature Server-Media-Foundation") {
		t.Fatal(h)
	}
	if h := DLLHint("MFPlat.DLL", true, false); !strings.Contains(h, "Media Feature Pack") {
		t.Fatal(h)
	}
	for name, want := range map[string]string{"VCRUNTIME140_1.dll": "2015-2022 runtime (x64)", "msvcp140.dll": "2015-2022", "MSVCR120.dll": "2013",
		"msvcp100.dll": "2010", "d3dx9_43.dll": "DirectX", "XINPUT1_3.dll": "DirectX", "steam_api64.dll": "", "msvcp140_atomic_wait.dll": "2015-2022"} {
		h := DLLHint(name, true, true)
		if (want == "") != (h == "") || !strings.Contains(h, want) {
			t.Errorf("%s: %q", name, h)
		}
	}
	if h := DLLHint("vcruntime140.dll", false, true); !strings.Contains(h, "vc_redist.x86.exe") {
		t.Fatal(h)
	}
	for name, want := range map[string]string{"XAPOFX1_5.dll": "the DirectX End-User Runtime (June 2010)", "mfplat.dll": "Media Foundation",
		"msvcp140.dll": "the Visual C++ 2015-2022 runtime (x64)", "steam_api64.dll": "", "own.dll": ""} {
		if got := DLLPart(name, true); got != want {
			t.Errorf("%s: %q", name, got)
		}
	}
}
