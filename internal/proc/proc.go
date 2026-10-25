// Package proc starts game servers, finds them again after the manager restarts, and reads what they use.
package proc

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Spec is how to start a program.
type Spec struct {
	Exe      string
	Args     []string
	Dir      string // its working folder: the folder of the program (hlds.exe needs it)
	Priority string // real time, high, above normal, normal, below normal, idle
	Env      []string
	Output   string // a file what the program prints (stdout, stderr) is added to, "" none (not on Windows)
	// WideConsole: its console window is read (Windows, ReadConsoleWindow): wide, and long, so that a line stays on one
	// row and the lines are kept
	WideConsole bool
}

// Usage is what a process uses.
type Usage struct {
	CPUTime time.Duration // user + kernel time since it started
	Memory  uint64        // bytes in RAM that are the process's own (private working set, as Task Manager shows)
}

// FindServer looks for a game server already running (started by hand, by the old manager, as another Windows
// user, or before the manager lost track of it): its PID, or 0. names are the program names the game can run
// (srcds.exe, srcds_win64.exe), dir the server's folder.
//
// First, who owns the server's UDP port, if it is one of those programs: Windows tells that to any user, even
// for a process it does not let him look into. Then, as the old manager did, a process of one of those names
// whose command line has the port and the server's folder.
func FindServer(names []string, dir, ip string, port int) int {
	if pid := PortOwner(ip, port); pid > 0 {
		// On Linux the port is the game's own program (srcds_linux64), started by the script the manager runs
		// (srcds_run_64), which leads their process group: that one is the server.
		if l := groupLeader(pid); l > 0 && IsProgram(l, names) && fromDir(l, dir, true) {
			return l
		}
		if IsProgram(pid, names) && fromDir(pid, dir, true) {
			return pid
		}
	}
	ps := strconv.Itoa(port)
	for _, pid := range PidsByName(names) {
		cmd := CommandLine(pid)
		if cmd == "" || !containsWord(cmd, ps) {
			continue
		}
		if fromDir(pid, dir, false) || strings.Contains(strings.ToLower(cmd), strings.ToLower(filepath.Clean(dir))) {
			return pid
		}
	}
	return 0
}

// IsProgram tells whether a process runs one of these programs, a script (srcds_run, cs2.sh) included: its
// program is then bash, the script is on its command line.
func IsProgram(pid int, names []string) bool {
	if NameMatches(ProcessName(pid), names) {
		return true
	}
	f := strings.Fields(CommandLine(pid))
	for i := 0; i < len(f) && i < 2; i++ {
		if NameMatches(f[i], names) {
			return true
		}
	}
	return false
}

// fromDir tells whether a process runs from a server's folder: its program, or its working folder (a script's
// program is bash). unknown: the answer when neither can be read (a process of another Windows user).
func fromDir(pid int, dir string, unknown bool) bool {
	img, wd := Image(pid), workDir(pid)
	if img == "" && wd == "" {
		return unknown
	}
	return (img != "" && inDir(img, dir)) || (wd != "" && inDir(wd, dir))
}

// NameMatches tells whether a program name is one of names (base names, without case).
func NameMatches(name string, names []string) bool {
	if name == "" {
		return false
	}
	for _, n := range names {
		if strings.EqualFold(filepath.Base(n), filepath.Base(name)) {
			return true
		}
	}
	return false
}

func inDir(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	if isWindows {
		return !strings.Contains(rel, ":")
	}
	return true
}

// UDPEntry is a UDP port a process listens on.
type UDPEntry struct {
	IP   net.IP
	Port int
	PID  int
}

// PortOwner gives the PID listening on a UDP port (on that IP, or on every address), 0 when none or unknown.
func PortOwner(ip string, port int) int {
	e, _ := PortEntry(ip, port)
	return e.PID
}

// PortEntry gives who listens on a UDP port, on that IP or on every address (0.0.0.0). The exact IP wins.
func PortEntry(ip string, port int) (UDPEntry, bool) {
	want := net.ParseIP(ip)
	var wild UDPEntry
	found := false
	for _, e := range udpTable() {
		if e.Port != port || e.PID <= 0 {
			continue
		}
		if want != nil && !want.IsUnspecified() && e.IP.Equal(want) {
			return e, true
		}
		if !found && (want == nil || want.IsUnspecified() || e.IP.IsUnspecified()) {
			wild, found = e, true
		}
	}
	return wild, found
}

// TCPListener gives who listens on a TCP port (RCON), on that IP or on every address (0.0.0.0). The exact IP
// wins. Not known (false) where the system does not tell.
func TCPListener(ip string, port int) (UDPEntry, bool) {
	want := net.ParseIP(ip)
	var wild UDPEntry
	found := false
	for _, e := range tcpListeners() {
		if e.Port != port || e.PID <= 0 {
			continue
		}
		if want != nil && !want.IsUnspecified() && e.IP.Equal(want) {
			return e, true
		}
		if !found && (want == nil || want.IsUnspecified() || e.IP.IsUnspecified()) {
			wild, found = e, true
		}
	}
	return wild, found
}

// parseTCPTable reads what GetExtendedTcpTable(TCP_TABLE_OWNER_PID_LISTENER) gives: a count, then rows of state,
// local address, local port, remote address, remote port (network order) and owning PID.
func parseTCPTable(b []byte) []UDPEntry {
	if len(b) < 4 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	var out []UDPEntry
	for k := 0; k < n; k++ {
		r := 4 + k*24
		if r+24 > len(b) {
			break
		}
		out = append(out, UDPEntry{
			IP:   net.IPv4(b[r+4], b[r+5], b[r+6], b[r+7]),
			Port: int(b[r+8])<<8 | int(b[r+9]),
			PID:  int(binary.LittleEndian.Uint32(b[r+20:])),
		})
	}
	return out
}

// parseUDPTable reads what Windows' GetExtendedUdpTable(UDP_TABLE_OWNER_PID) gives: a count, then rows of
// address (network order), port (network order, in the low two bytes) and owning PID.
func parseUDPTable(b []byte) []UDPEntry {
	if len(b) < 4 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	var out []UDPEntry
	for k := 0; k < n; k++ {
		r := 4 + k*12
		if r+12 > len(b) {
			break
		}
		out = append(out, UDPEntry{
			IP:   net.IPv4(b[r], b[r+1], b[r+2], b[r+3]),
			Port: int(b[r+4])<<8 | int(b[r+5]),
			PID:  int(binary.LittleEndian.Uint32(b[r+8:])),
		})
	}
	return out
}

// ExitInfo is how a process ended, when the system tells.
type ExitInfo struct {
	Known  bool   `json:"known"`
	Code   int64  `json:"code"`             // its exit code (Windows: a crash gives its exception, 0xC0000005)
	Signal string `json:"signal,omitempty"` // Linux: the signal that ended it (SIGSEGV)
	Core   bool   `json:"core,omitempty"`   // Linux: a core dump was written
}

// Exit tells when a process ends, and then how.
type Exit struct {
	done chan struct{}
	info ExitInfo
}

// Done closes when the process has ended.
func (e *Exit) Done() <-chan struct{} { return e.done }

// Info tells how it ended (known once Done is closed).
func (e *Exit) Info() ExitInfo {
	select {
	case <-e.done:
		return e.info
	default:
		return ExitInfo{}
	}
}

// WatchPID tells when a process ends, for one the manager may not open (another user's): it looks every 2 s. How it
// ended is not known.
func WatchPID(pid int) *Exit {
	e := &Exit{done: make(chan struct{})}
	go func() {
		defer close(e.done)
		for Alive(pid) {
			time.Sleep(2 * time.Second)
		}
	}()
	return e
}

func containsWord(cmd, w string) bool {
	fields := strings.Fields(cmd)
	parts := strings.Fields(w)
	for i := 0; i+len(parts) <= len(fields); i++ {
		match := true
		for j, p := range parts {
			if !strings.EqualFold(strings.Trim(fields[i+j], `"`), p) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Exited waits for a process to end, in the background: Done closes when it does. Works for a process the manager
// started and for one it found again (Windows tells the exit code of both; Linux of its own children only).
func Exited(p *os.Process, child bool) *Exit {
	e := &Exit{done: make(chan struct{})}
	go func() {
		defer close(e.done)
		if child || isWindows {
			if st, err := p.Wait(); err == nil {
				e.info = exitInfo(st)
				return
			}
		}
		// Not a child of ours (Linux): watch it
		for Alive(p.Pid) {
			time.Sleep(2 * time.Second)
		}
	}()
	return e
}
