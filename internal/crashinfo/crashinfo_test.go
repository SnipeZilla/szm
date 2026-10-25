package crashinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAppErrorNamed(t *testing.T) {
	x := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Application Error'/><EventID>1000</EventID><TimeCreated SystemTime='2026-10-13T20:01:02.1234567Z'/></System><EventData>` +
		`<Data Name='AppName'>srcds_win64.exe</Data><Data Name='AppVersion'>0.0.0.0</Data><Data Name='AppTimeStamp'>6512abcd</Data>` +
		`<Data Name='ModuleName'>server.dll</Data><Data Name='ModuleVersion'>1.2.3.4</Data><Data Name='ModuleTimeStamp'>6512abce</Data>` +
		`<Data Name='ExceptionCode'>c0000005</Data><Data Name='FaultingOffset'>00000000001a2b3c</Data><Data Name='ProcessId'>0x1f40</Data>` +
		`<Data Name='ProcessCreationTime'>0x1d9</Data><Data Name='AppPath'>E:\Servers\CSS\srcds_win64.exe</Data><Data Name='ModulePath'>E:\Servers\CSS\cstrike\bin\x64\server.dll</Data></EventData></Event>`
	rec, pid, ok := parseAppError(x)
	if !ok || pid != 8000 || rec.Module != "server.dll" || rec.Exception != "0xC0000005" || !strings.Contains(rec.Meaning, "access violation") {
		t.Fatalf("%v %d %+v", ok, pid, rec)
	}
	if rec.Time.IsZero() || !strings.Contains(rec.Text, "srcds_win64.exe crashed in server.dll (version 1.2.3.4), exception 0xC0000005") {
		t.Fatalf("%+v", rec)
	}
}

func TestParseAppErrorUnnamed(t *testing.T) {
	x := `<Event><System><EventID>1000</EventID><TimeCreated SystemTime='2026-10-13T20:01:02Z'/></System><EventData>` +
		`<Data>hlds.exe</Data><Data>1.0</Data><Data>0</Data><Data>mp.dll</Data><Data>0</Data><Data>0</Data><Data>c00000fd</Data><Data>0x1234</Data><Data>0x10</Data></EventData></Event>`
	rec, pid, ok := parseAppError(x)
	if !ok || pid != 16 || rec.Module != "mp.dll" || !strings.Contains(rec.Meaning, "stack overflow") {
		t.Fatalf("%v %d %+v", ok, pid, rec)
	}
}

func TestExplain(t *testing.T) {
	if !strings.Contains(Explain(0xC0000005, "", false, true), "access violation") {
		t.Fatal("0xC0000005")
	}
	if !strings.Contains(Explain(-1, "SIGSEGV", true, false), "core dump") {
		t.Fatal("SIGSEGV")
	}
	if !strings.Contains(Explain(139, "", false, false), "segmentation fault") {
		t.Fatal("139")
	}
	if !strings.Contains(Explain(0, "", false, true), "normally") {
		t.Fatal("0")
	}
	if Hex(3221225477) != "0xC0000005" {
		t.Fatal(Hex(3221225477))
	}
}

func TestKmsg(t *testing.T) {
	at, text, ok := parseKmsg("6,1234,5678901234,-;srcds_linux[4321]: segfault at 0 ip 00007f sp 00007f error 4 in server_srv.so[7f000+1000]\n SUBSYSTEM=x")
	if !ok || at != 5678901234*time.Microsecond || !strings.HasPrefix(text, "srcds_linux[4321]: segfault") {
		t.Fatal(at, text, ok)
	}
	if !kernelMatch(text, []string{"srcds_run"}, []int{4321}) || !kernelMatch(text, []string{"srcds_linux"}, nil) {
		t.Fatal("no match")
	}
	if kernelMatch("eth0: link up", []string{"srcds_linux"}, []int{4321}) {
		t.Fatal("match")
	}
	if !kernelMatch("Out of memory: Killed process 4321 (srcds_linux) total-vm:1kB", nil, []int{4321}) {
		t.Fatal("oom")
	}
}

func TestFilesAndTail(t *testing.T) {
	dir := t.TempDir()
	start := time.Now().Add(-time.Minute)
	_ = os.WriteFile(filepath.Join(dir, "crash_srcds.mdmp"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	old := filepath.Join(dir, "old.dmp")
	_ = os.WriteFile(old, []byte("x"), 0o644)
	_ = os.Chtimes(old, start.Add(-time.Hour), start.Add(-time.Hour))
	d := FindDumps([]string{dir, dir}, start, time.Now().Add(time.Minute))
	if len(d) != 1 || filepath.Base(d[0].Path) != "crash_srcds.mdmp" {
		t.Fatalf("%+v", d)
	}
	var b strings.Builder
	for k := 0; k < 500; k++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", k%7))
		b.WriteString("\r\n")
	}
	b.WriteString("L Error: last one\n")
	log := filepath.Join(dir, "errors_20261013.log")
	_ = os.WriteFile(log, []byte(b.String()), 0o644)
	if lines := Tail(log, 30); len(lines) != 30 || lines[29] != "L Error: last one" {
		t.Fatalf("%q", lines)
	}
}
