//go:build windows

package crashinfo

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wevtapi       = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery  = wevtapi.NewProc("EvtQuery")
	procEvtNext   = wevtapi.NewProc("EvtNext")
	procEvtRender = wevtapi.NewProc("EvtRender")
	procEvtClose  = wevtapi.NewProc("EvtClose")
)

const (
	evtQueryChannelPath      = 0x1
	evtQueryReverseDirection = 0x200
	evtRenderEventXml        = 1
)

// SystemRecords finds what the system wrote about the crash of a process: Windows' "Application Error" event of
// it (by its PID, or by its program's name near the time), written since a moment.
func SystemRecords(pid int, programs []string, since time.Time) []SystemRecord {
	if procEvtQuery.Find() != nil {
		return nil
	}
	ms := time.Since(since).Milliseconds() + 60_000
	q := fmt.Sprintf("*[System[Provider[@Name='Application Error'] and (EventID=1000) and TimeCreated[timediff(@SystemTime) <= %d]]]", ms)
	path, _ := windows.UTF16PtrFromString("Application")
	query, _ := windows.UTF16PtrFromString(q)
	h, _, _ := procEvtQuery.Call(0, uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(query)), evtQueryChannelPath|evtQueryReverseDirection)
	if h == 0 {
		return nil
	}
	defer procEvtClose.Call(h)
	var byPid, byName []SystemRecord
	events := make([]uintptr, 16)
	for n := 0; n < 20; n++ {
		var got uint32
		r, _, _ := procEvtNext.Call(h, uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), 2000, 0, uintptr(unsafe.Pointer(&got)))
		if r == 0 || got == 0 {
			break
		}
		for _, ev := range events[:got] {
			x := render(ev)
			procEvtClose.Call(ev)
			rec, p, ok := parseAppError(x)
			if !ok {
				continue
			}
			switch {
			case pid > 0 && p == pid:
				byPid = append(byPid, rec)
			case p == 0 || pid <= 0:
				for _, name := range programs {
					if equalFold(name, rec.Program) {
						byName = append(byName, rec)
						break
					}
				}
			}
		}
	}
	if len(byPid) > 0 {
		return byPid
	}
	if len(byName) > 1 {
		byName = byName[:1] // the newest
	}
	return byName
}

func render(ev uintptr) string {
	var used, props uint32
	procEvtRender.Call(0, ev, evtRenderEventXml, 0, 0, uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
	if used == 0 {
		return ""
	}
	buf := make([]uint16, used/2+1)
	r, _, _ := procEvtRender.Call(0, ev, evtRenderEventXml, uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// CrashDumpDirs are where Windows Error Reporting keeps crash dumps, when it is set to (LocalDumps).
func CrashDumpDirs() []string {
	var out []string
	if v, ok := windows.Getenv("LOCALAPPDATA"); ok && v != "" {
		out = append(out, v+`\CrashDumps`)
	}
	if v, ok := windows.Getenv("SystemRoot"); ok && v != "" {
		out = append(out, v+`\System32\config\systemprofile\AppData\Local\CrashDumps`)
	}
	return out
}
