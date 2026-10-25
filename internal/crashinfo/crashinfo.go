// Package crashinfo gathers what the system knows of a program that crashed: what its exit code means, Windows'
// record of the crash (Application log), the Linux kernel's (segfault, out of memory), the crash dumps it wrote and
// the end of its log files.
package crashinfo

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SystemRecord is what the system wrote down about the crash: on Windows the "Application Error" event, on Linux
// the kernel's line (segfault, out of memory).
type SystemRecord struct {
	Time      time.Time `json:"time"`
	Source    string    `json:"source"`            // "Windows Application log, event 1000", "Linux kernel"
	Program   string    `json:"program,omitempty"` // srcds.exe
	Module    string    `json:"module,omitempty"`  // where it crashed: server.dll, engine.dll, a plugin
	ModPath   string    `json:"module_path,omitempty"`
	ModVer    string    `json:"module_version,omitempty"`
	Exception string    `json:"exception,omitempty"` // 0xc0000005
	Meaning   string    `json:"meaning,omitempty"`   // the exception in words
	Offset    string    `json:"offset,omitempty"`
	Text      string    `json:"text"`
}

// Explain tells what an exit code means. windows: the code is a Windows one; signal: Linux's signal, if any.
func Explain(code int64, signal string, core, windows bool) string {
	if signal != "" {
		w := signals[signal]
		if w == "" {
			w = "ended by " + signal
		}
		if core {
			w += " (a core dump was written)"
		}
		return w
	}
	if windows {
		if w, ok := ntStatus[uint32(code)]; ok {
			return w
		}
		if uint32(code) >= 0xC0000000 {
			return "a Windows error (" + Hex(code) + ")"
		}
	}
	switch code {
	case 0:
		return "it closed itself normally (code 0): \"quit\" or \"exit\" typed in its console, or the game ended it"
	case 1:
		if windows {
			return "code 1: an error the server stopped on, or it was ended (Task Manager, the manager)"
		}
		return "code 1: an error the server stopped on"
	}
	if !windows && code > 128 && code < 160 {
		// a shell script (srcds_run) gives 128 + the signal of the program it ran
		if n, ok := signalByNumber[int(code-128)]; ok {
			return signals[n] + " (in the program srcds_run started)"
		}
	}
	return "code " + strconv.FormatInt(code, 10)
}

// Hex writes a Windows code as 0xC0000005.
func Hex(code int64) string { return fmt.Sprintf("0x%08X", uint32(code)) }

var ntStatus = map[uint32]string{
	0xC0000005: "access violation: it tried to use memory it has no access to (a bug in the game, a plugin or an addon)",
	0xC0000374: "heap corruption: its memory was damaged (often a plugin or an addon)",
	0xC00000FD: "stack overflow: endless recursion, often in a plugin or a script",
	0xC0000409: "it stopped itself on a fatal check (stack buffer overrun / fail fast), often a failed assertion or damaged memory",
	0x80000003: "a breakpoint or failed assertion in the game",
	0xC000001D: "illegal instruction: the CPU does not support something the program uses, or the program is damaged",
	0xC0000094: "division by zero",
	0xC0000095: "integer overflow",
	0xC000008E: "floating-point division by zero",
	0xC0000017: "out of memory",
	0xC000012D: "out of memory: Windows' page file is full",
	0xC0000135: "a DLL it needs is missing (an addon, or the Visual C++ runtime)",
	0xC0000139: "a DLL does not match the program (entry point not found): an addon built for another version",
	0xC0000142: "a DLL failed to start",
	0xC000007B: "a 32-bit / 64-bit mix: an addon or DLL built for the other kind",
	0xE06D7363: "a C++ error nobody handled (in the game or a plugin)",
	0xC000013A: "closed by Ctrl+C or its console window being closed",
	0x40010004: "ended by a debugger or by Windows",
	0xC0000420: "an assertion failed",
	0xC0000006: "a page of the program could not be read from disk (disk error, or the files changed while it ran)",
	0xC0000602: "it stopped itself on a fatal error (fail fast)",
	0xCFFFFFFF: "it stopped itself (an application hang or fatal error)",
}

var signals = map[string]string{
	"SIGSEGV": "segmentation fault: it tried to use memory it has no access to (a bug in the game, a plugin or an addon)",
	"SIGABRT": "it aborted itself (a failed assertion or a fatal error it found)",
	"SIGKILL": "killed at once (by the manager, by hand, or by Linux when memory ran out)",
	"SIGBUS":  "bus error: bad memory access (often a file changed or removed while it ran)",
	"SIGFPE":  "arithmetic error (division by zero)",
	"SIGILL":  "illegal instruction: the CPU does not support something the program uses",
	"SIGTERM": "asked to end (SIGTERM)",
	"SIGINT":  "interrupted (Ctrl+C)",
	"SIGHUP":  "its terminal closed (SIGHUP)",
	"SIGTRAP": "a breakpoint or failed assertion",
	"SIGQUIT": "asked to quit (SIGQUIT)",
}

var signalByNumber = map[int]string{11: "SIGSEGV", 6: "SIGABRT", 9: "SIGKILL", 7: "SIGBUS", 8: "SIGFPE", 4: "SIGILL", 15: "SIGTERM", 5: "SIGTRAP"}

/* ---------- Windows' Application log ---------- */

// appErrorXML is an "Application Error" event (1000), as EvtRender writes it.
type appErrorXML struct {
	System struct {
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		EventID int `xml:"EventID"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
}

// order of the values of event 1000, for Windows versions that do not name them
var appErrorFields = []string{"AppName", "AppVersion", "AppTimeStamp", "ModuleName", "ModuleVersion", "ModuleTimeStamp",
	"ExceptionCode", "FaultingOffset", "ProcessId", "ProcessCreationTime", "AppPath", "ModulePath", "IntegratorReportId"}

// parseAppError reads an event 1000; pid is the process's ID it was about (0 when it does not say).
func parseAppError(x string) (rec SystemRecord, pid int, ok bool) {
	var ev appErrorXML
	if err := xml.Unmarshal([]byte(x), &ev); err != nil {
		return rec, 0, false
	}
	v := map[string]string{}
	for k, d := range ev.EventData.Data {
		name := d.Name
		if name == "" && k < len(appErrorFields) {
			name = appErrorFields[k]
		}
		v[name] = strings.TrimSpace(d.Value)
	}
	if v["AppName"] == "" {
		return rec, 0, false
	}
	t, _ := time.Parse(time.RFC3339Nano, ev.System.TimeCreated.SystemTime)
	rec = SystemRecord{Time: t, Source: "Windows Application log (Application Error, event 1000)", Program: v["AppName"],
		Module: v["ModuleName"], ModPath: v["ModulePath"], ModVer: v["ModuleVersion"], Offset: v["FaultingOffset"]}
	if c := v["ExceptionCode"]; c != "" {
		if n, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(c), "0x"), 16, 32); err == nil {
			rec.Exception = Hex(int64(n))
			rec.Meaning = Explain(int64(n), "", false, true)
		} else {
			rec.Exception = c
		}
	}
	rec.Text = fmt.Sprintf("%s crashed in %s", rec.Program, orDash(rec.Module))
	if rec.ModVer != "" {
		rec.Text += " (version " + rec.ModVer + ")"
	}
	if rec.Exception != "" {
		rec.Text += ", exception " + rec.Exception
	}
	if rec.Offset != "" {
		rec.Text += ", at offset " + rec.Offset
	}
	if p := v["ProcessId"]; p != "" {
		if n, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(p), "0x"), 16, 64); err == nil {
			pid = int(n)
		}
	}
	return rec, pid, true
}

func orDash(s string) string {
	if s == "" {
		return "an unknown module"
	}
	return s
}

/* ---------- Linux kernel log ---------- */

// parseKmsg reads a record of /dev/kmsg ("6,1234,5678901234,-;text"): its time since boot and its text.
func parseKmsg(rec string) (since time.Duration, text string, ok bool) {
	semi := strings.IndexByte(rec, ';')
	if semi < 0 {
		return 0, "", false
	}
	f := strings.Split(rec[:semi], ",")
	if len(f) < 3 {
		return 0, "", false
	}
	us, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		return 0, "", false
	}
	text = rec[semi+1:]
	if nl := strings.IndexByte(text, '\n'); nl >= 0 {
		text = text[:nl] // the lines after are key=value details
	}
	return time.Duration(us) * time.Microsecond, strings.TrimSpace(text), true
}

// kernelMatch tells whether a kernel line is about one of these programs or processes crashing or being killed.
func kernelMatch(text string, names []string, pids []int) bool {
	l := strings.ToLower(text)
	if !(strings.Contains(l, "segfault") || strings.Contains(l, "general protection") || strings.Contains(l, "killed process") ||
		strings.Contains(l, "out of memory") || strings.Contains(l, " trap ") || strings.Contains(l, "oom-kill")) {
		return false
	}
	for _, p := range pids {
		if p > 0 && (strings.Contains(text, "["+strconv.Itoa(p)+"]") || strings.Contains(l, "process "+strconv.Itoa(p)+" ") ||
			strings.Contains(l, "pid="+strconv.Itoa(p))) {
			return true
		}
	}
	for _, n := range names {
		n = strings.ToLower(filepath.Base(n))
		if n != "" && strings.Contains(l, n) {
			return true
		}
	}
	return false
}

/* ---------- Files ---------- */

// DumpFile is a crash dump written when the server crashed.
type DumpFile struct {
	Path string    `json:"path"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
}

// FindDumps lists the crash dumps (.mdmp, .dmp, core files) in these folders, written between from and to.
func FindDumps(dirs []string, from, to time.Time) []DumpFile {
	var out []DumpFile
	seen := map[string]bool{}
	for _, d := range dirs {
		d = filepath.Clean(d)
		if d == "" || seen[strings.ToLower(d)] {
			continue
		}
		seen[strings.ToLower(d)] = true
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := strings.ToLower(e.Name())
			if !(strings.HasSuffix(n, ".mdmp") || strings.HasSuffix(n, ".dmp") || n == "core" || strings.HasPrefix(n, "core.")) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().Before(from) || info.ModTime().After(to) {
				continue
			}
			out = append(out, DumpFile{Path: filepath.Join(d, e.Name()), Size: info.Size(), Time: info.ModTime()})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Time.Before(out[b].Time) })
	return out
}

// Tail gives the last lines of a file (up to max), from the last 256 KB.
func Tail(path string, max int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	off := st.Size() - 256<<10
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	first := off > 0
	for sc.Scan() {
		if first {
			first = false // a part of a line
			continue
		}
		l := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		lines = append(lines, strings.ToValidUTF8(l, "?"))
		if len(lines) > 4*max {
			lines = lines[len(lines)-max:]
		}
	}
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}

// LogTail is the end of a log file that changed while the server ran (SourceMod's errors, for example).
type LogTail struct {
	Path  string    `json:"path"`
	Time  time.Time `json:"time"`
	Lines []string  `json:"lines"`
}
