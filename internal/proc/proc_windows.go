//go:build windows

package proc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const isWindows = true

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	iphlpapi                 = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedUdpTable  = iphlpapi.NewProc("GetExtendedUdpTable")
	procGetExtendedTcpTable  = iphlpapi.NewProc("GetExtendedTcpTable")
)

func priorityClass(p string) uint32 {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "real time", "realtime":
		return windows.REALTIME_PRIORITY_CLASS
	case "high", "high priority":
		return windows.HIGH_PRIORITY_CLASS
	case "normal":
		return windows.NORMAL_PRIORITY_CLASS
	case "below normal":
		return windows.BELOW_NORMAL_PRIORITY_CLASS
	case "idle", "low":
		return windows.IDLE_PRIORITY_CLASS
	default:
		return windows.ABOVE_NORMAL_PRIORITY_CLASS
	}
}

// Start starts a server in a console of its own (minimised, without taking the focus), at its priority, in its
// folder. It keeps running when the manager stops.
//
// It is started with CreateProcess itself: os/exec always hands the child standard handles (NUL when there is no
// pipe), and srcds -console then fails at once with "CTextConsoleWin32::GetLine: !GetNumberOfConsoleInputEvents",
// its input being a file instead of its console. Without handles it gets those of its own new console.
func Start(s Spec) (*os.Process, error) {
	exe, err := windows.UTF16PtrFromString(s.Exe)
	if err != nil {
		return nil, err
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{s.Exe}, s.Args...)))
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if s.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(s.Dir); err != nil {
			return nil, err
		}
	}
	flags := uint32(windows.CREATE_NEW_CONSOLE) | priorityClass(s.Priority)
	var env *uint16
	if len(s.Env) > 0 {
		u, err := envBlock(os.Environ(), s.Env)
		if err != nil {
			return nil, err
		}
		env = &u[0]
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}
	si := &windows.StartupInfo{Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_SHOWMINNOACTIVE}
	if s.WideConsole {
		const startfUseCountChars = 0x8 // the size of its console's buffer, in characters
		si.Flags |= startfUseCountChars
		si.XCountChars, si.YCountChars = ConsoleCols, 9999
	}
	si.Cb = uint32(unsafe.Sizeof(*si))
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(exe, cmdline, nil, nil, false, flags, env, dir, si, &pi); err != nil {
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	// os.Process opens its own handle; ours keeps the process known until then, even if it ends at once
	p, err := os.FindProcess(int(pi.ProcessId))
	windows.CloseHandle(pi.Process)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// RunHidden runs a tool (steamcmd) without a window, its output to w.
func RunHiddenCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
}

func open(pid int, access uint32) (windows.Handle, error) {
	if pid <= 0 {
		return 0, errors.New("no pid")
	}
	return windows.OpenProcess(access, false, uint32(pid))
}

// Alive tells whether a process is running. One of another user that may not be opened is running: Windows
// says "access denied" only for a process that exists.
func Alive(pid int) bool {
	h, err := open(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err == windows.ERROR_ACCESS_DENIED {
		return processes()[pid] != ""
	}
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// Image gives the full path of a process's program.
func Image(pid int) string {
	h, err := open(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// CommandLine gives a process's command line ("" when it cannot be read).
func CommandLine(pid int) string {
	h, err := open(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	size := uint32(4096)
	for i := 0; i < 3; i++ {
		buf := make([]byte, size)
		var ret uint32
		err := windows.NtQueryInformationProcess(h, windows.ProcessCommandLineInformation, unsafe.Pointer(&buf[0]), size, &ret)
		if err == nil {
			us := (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0]))
			if us.Buffer == nil || us.Length == 0 {
				return ""
			}
			return windows.UTF16ToString(unsafe.Slice(us.Buffer, us.Length/2))
		}
		if ret > size {
			size = ret
			continue
		}
		return ""
	}
	return ""
}

// processes gives the name of every running program, by PID. No right is needed.
func processes() map[int]string {
	out := map[int]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[int(e.ProcessID)] = windows.UTF16ToString(e.ExeFile[:])
	}
	return out
}

// ProcessName gives the program name of a process (srcds.exe), "" when it is not running.
func ProcessName(pid int) string { return processes()[pid] }

// workDir is not read on Windows (a server's program is in its folder).
func workDir(int) string { return "" }

// groupLeader: Windows has no process groups of that kind; the server is the program itself.
func groupLeader(int) int { return 0 }

// PidsByName lists the processes running one of these programs (by name).
func PidsByName(names []string) []int {
	var out []int
	for pid, name := range processes() {
		if NameMatches(name, names) {
			out = append(out, pid)
		}
	}
	return out
}

// udpTable lists the UDP ports listened on (IPv4), with their process.
func udpTable() []UDPEntry {
	const afInet, udpTableOwnerPID = 2, 1
	size := uint32(64 * 1024)
	for try := 0; try < 4; try++ {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedUdpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, afInet, udpTableOwnerPID, 0)
		switch r {
		case 0:
			return parseUDPTable(buf[:size])
		case uintptr(windows.ERROR_INSUFFICIENT_BUFFER):
			size += 16 * 1024
			continue
		default:
			return nil
		}
	}
	return nil
}

// tcpListeners lists the TCP ports listened on (IPv4), with their process.
func tcpListeners() []UDPEntry {
	const afInet, tcpTableOwnerPIDListener = 2, 3
	size := uint32(64 * 1024)
	for try := 0; try < 4; try++ {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, afInet, tcpTableOwnerPIDListener, 0)
		switch r {
		case 0:
			return parseTCPTable(buf[:size])
		case uintptr(windows.ERROR_INSUFFICIENT_BUFFER):
			size += 16 * 1024
			continue
		default:
			return nil
		}
	}
	return nil
}

// Kill ends a process at once, and the programs it started (a launcher's server: FactoryServer.exe's).
func Kill(pid int) error {
	for _, c := range descendants(pid) {
		if h, err := open(c, windows.PROCESS_TERMINATE); err == nil {
			_ = windows.TerminateProcess(h, 1)
			windows.CloseHandle(h)
		}
	}
	h, err := open(pid, windows.PROCESS_TERMINATE)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}

// descendants: the processes a process started, and theirs (the deepest first).
func descendants(pid int) []int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	children := map[int][]int{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ParentProcessID != e.ProcessID {
			children[int(e.ParentProcessID)] = append(children[int(e.ParentProcessID)], int(e.ProcessID))
		}
	}
	var out []int
	seen := map[int]bool{pid: true}
	var walk func(int)
	walk = func(p int) {
		born := created(p)
		for _, c := range children[p] {
			// a child born before it has a parent that ended, whose number it now has: not its child
			if !seen[c] && born > 0 && created(c) >= born {
				seen[c] = true
				walk(c)
				out = append(out, c)
			}
		}
	}
	walk(pid)
	return out
}

// created: when a process started (0 when not known).
func created(pid int) int64 {
	h, err := open(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return 0
	}
	return c.Nanoseconds()
}

// memCounters is PROCESS_MEMORY_COUNTERS_EX2: its PrivateWorkingSetSize is the "Memory" column of Task Manager.
// Only Windows 10/11 22H2 (September 2023 update) and Windows Server 2025 fill it; before (Server 2022), it stays 0
// and privateWorkingSet gives it.
type memCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
	PrivateWorkingSetSize      uintptr
	SharedCommitUsage          uint64
}

// Stats reads what a process uses.
func Stats(pid int) (Usage, error) {
	h, err := open(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return Usage{}, err
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return Usage{}, err
	}
	ft := func(f windows.Filetime) time.Duration {
		return time.Duration((int64(f.HighDateTime)<<32 | int64(f.LowDateTime)) * 100)
	}
	var m memCounters
	m.cb = uint32(unsafe.Sizeof(m))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.cb))
	use := Usage{CPUTime: ft(k) + ft(u)}
	if r != 0 {
		// As Task Manager counts it: the private working set, without the DLLs shared with other programs
		use.Memory = uint64(m.PrivateWorkingSetSize)
		if use.Memory == 0 {
			use.Memory = privateWorkingSet(pid)
		}
		if use.Memory == 0 {
			use.Memory = uint64(m.WorkingSetSize) // with the DLLs shared with other programs: more than Task Manager says
		}
	}
	return use, nil
}

type memStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// SystemMemory gives the RAM of the machine: total and free bytes.
func SystemMemory() (total, free uint64) {
	var m memStatus
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return m.TotalPhys, m.AvailPhys
}

// Disk gives the size and the free space of the disk a folder is on.
func Disk(path string) (total, free uint64) {
	p, err := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if err != nil {
		return 0, 0
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return 0, 0
	}
	return total, avail
}

// Roots lists the drives (C:\, E:\...).
func Roots() []string {
	buf := make([]uint16, 256)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0])
	if err != nil {
		return nil
	}
	// The drives are separated by NULs: UTF16ToString would stop at the first one
	var out []string
	start := 0
	for i := 0; i < int(n); i++ {
		if buf[i] == 0 {
			if i > start {
				out = append(out, windows.UTF16ToString(buf[start:i]))
			}
			start = i + 1
		}
	}
	return out
}

func exitInfo(st *os.ProcessState) ExitInfo {
	return ExitInfo{Known: true, Code: int64(uint32(st.ExitCode()))}
}

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procEnumChildWindows         = user32.NewProc("EnumChildWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procSetErrorMode             = kernel32.NewProc("SetErrorMode")
)

// windowsOf collects, for WindowTexts, the visible windows of a process (one search at a time: winMu).
type windowsOf struct {
	pid   uint32
	found []uintptr
	texts []string
}

var (
	winMu  sync.Mutex
	winCur *windowsOf
)

var (
	enumTop = windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		w := winCur
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == w.pid {
			if v, _, _ := procIsWindowVisible.Call(hwnd); v != 0 && className(hwnd) != "ConsoleWindowClass" {
				w.found = append(w.found, hwnd)
			}
		}
		return 1
	})
	enumChild = windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		w := winCur
		switch strings.ToLower(className(hwnd)) {
		case "static", "edit", "richedit20w", "richedit50w":
			if t := windowText(hwnd); t != "" {
				w.texts = append(w.texts, t)
			}
		}
		return 1
	})
)

func windowText(hwnd uintptr) string {
	buf := make([]uint16, 8192)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.TrimSpace(windows.UTF16ToString(buf[:n]))
}

func className(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// CloseWindows asks the visible windows of a process to close (WM_CLOSE, as their X does): a server whose window is
// not a console saves and ends this way. How many were asked.
func CloseWindows(pid int) int {
	if pid <= 0 || procEnumWindows.Find() != nil || procPostMessageW.Find() != nil {
		return 0
	}
	winMu.Lock()
	w := &windowsOf{pid: uint32(pid)}
	winCur = w
	procEnumWindows.Call(enumTop, 0)
	winCur = nil
	winMu.Unlock()
	n := 0
	for _, hwnd := range w.found {
		const wmClose = 0x0010
		if r, _, _ := procPostMessageW.Call(hwnd, wmClose, 0, 0); r != 0 {
			n++
		}
	}
	return n
}

// WindowTexts reads the windows a process shows (besides its console): an error box waiting for a click ("Engine
// Error", "Fatal Error"…), its title and its message. Read before a stuck server is ended.
func WindowTexts(pid int) []string {
	if pid <= 0 || procEnumWindows.Find() != nil {
		return nil
	}
	winMu.Lock()
	defer winMu.Unlock()
	w := &windowsOf{pid: uint32(pid)}
	winCur = w
	defer func() { winCur = nil }()
	procEnumWindows.Call(enumTop, 0)
	var out []string
	for _, hwnd := range w.found {
		w.texts = nil
		procEnumChildWindows.Call(hwnd, enumChild, 0)
		title := windowText(hwnd)
		var msg []string
		for _, t := range w.texts {
			if t != title {
				msg = append(msg, t)
			}
		}
		line := strings.TrimSpace(title + ": " + strings.Join(msg, " "))
		line = strings.TrimSuffix(strings.TrimPrefix(line, ": "), ":")
		if line != "" {
			out = append(out, line)
		}
		if len(out) >= 10 {
			break
		}
	}
	return out
}

// NoCrashDialogs keeps Windows from showing "has stopped working" for the servers the manager starts (they inherit
// it): a crashed server ends at once, with its error code, instead of waiting for a click nobody gives. Windows
// still writes the crash in its Application log.
// With wer, Windows Error Reporting is left on (it writes the crash dumps, and is told to show nothing): only the
// "insert a disk" boxes are off.
func NoCrashDialogs(wer bool) {
	const semFailCriticalErrors, semNoGPFaultErrorBox = 0x0001, 0x0002
	mode := uintptr(semFailCriticalErrors | semNoGPFaultErrorBox)
	if wer {
		mode = semFailCriticalErrors
	}
	if procSetErrorMode.Find() == nil {
		procSetErrorMode.Call(mode)
	}
}

// IsAdmin says whether the manager runs as an administrator (elevated, or as the service).
func IsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }

// The private working set of every process, as Task Manager reads it on a Windows without
// PROCESS_MEMORY_COUNTERS_EX2 (NtQuerySystemInformation): read at most once a second, for all the servers at once.
var (
	pwsMu   sync.Mutex
	pwsAt   time.Time
	pwsByID map[int]uint64
	pwsBuf  []byte
)

func privateWorkingSet(pid int) uint64 {
	pwsMu.Lock()
	defer pwsMu.Unlock()
	if time.Since(pwsAt) > time.Second {
		pwsByID, pwsAt = map[int]uint64{}, time.Now()
		if len(pwsBuf) == 0 {
			pwsBuf = make([]byte, 512<<10)
		}
		for try := 0; try < 4; try++ {
			var n uint32
			err := windows.NtQuerySystemInformation(windows.SystemProcessInformation, unsafe.Pointer(&pwsBuf[0]), uint32(len(pwsBuf)), &n)
			if err == windows.STATUS_INFO_LENGTH_MISMATCH {
				pwsBuf = make([]byte, max(int(n), 2*len(pwsBuf))+64<<10)
				continue
			}
			if err != nil {
				break
			}
			for off := 0; off+int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{})) <= len(pwsBuf); {
				p := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&pwsBuf[off]))
				pwsByID[int(p.UniqueProcessID)] = uint64(p.WorkingSetPrivateSize)
				if p.NextEntryOffset == 0 {
					break
				}
				off += int(p.NextEntryOffset)
			}
			break
		}
	}
	return pwsByID[pid]
}
