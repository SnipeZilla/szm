//go:build !windows

package proc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const isWindows = false

// Start starts a server in its own process group, in its folder: it keeps running when the manager stops.
func Start(s Spec) (*os.Process, error) {
	cmd := exec.Command(s.Exe, s.Args...)
	cmd.Dir = s.Dir
	if len(s.Env) > 0 {
		cmd.Env = append(os.Environ(), s.Env...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if s.Output != "" {
		// the program writes into the file itself: it goes on when the manager stops
		_ = os.MkdirAll(filepath.Dir(s.Output), 0o755)
		f, err := os.OpenFile(s.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		cmd.Stdout, cmd.Stderr = f, f
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if n := niceness(s.Priority); n != 0 {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, n)
	}
	return cmd.Process, nil
}

func niceness(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "real time", "realtime", "high", "high priority":
		return -10
	case "above normal", "":
		return -5
	case "below normal":
		return 5
	case "idle", "low":
		return 19
	}
	return 0
}

// RunHiddenCmd: nothing to hide outside Windows.
func RunHiddenCmd(cmd *exec.Cmd) {}

// Alive tells whether a process is running.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	// A zombie (ended, not yet collected) is not running
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err == nil {
		if i := strings.LastIndexByte(string(b), ')'); i > 0 && i+2 < len(b) && b[i+2] == 'Z' {
			return false
		}
	}
	return true
}

// Image gives the full path of a process's program.
func Image(pid int) string {
	p, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return ""
	}
	return p
}

// workDir gives a process's working folder.
func workDir(pid int) string {
	p, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	return p
}

// groupLeader gives the leader of a process's group when it is another process still running, 0 otherwise.
func groupLeader(pid int) int {
	if g, err := procGroup(pid); err == nil && g > 1 && g != pid && Alive(g) {
		return g
	}
	return 0
}

// CommandLine gives a process's command line.
func CommandLine(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", " "))
}

// ProcessName gives the program name of a process, "" when it is not running.
func ProcessName(pid int) string {
	if img := Image(pid); img != "" {
		return filepath.Base(img)
	}
	if f := strings.Fields(CommandLine(pid)); len(f) > 0 {
		return filepath.Base(f[0])
	}
	return ""
}

// PidsByName lists the processes running one of these programs (a script such as srcds_run is found by its
// command line).
func PidsByName(names []string) []int {
	entries, _ := os.ReadDir("/proc")
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if IsProgram(pid, names) {
			out = append(out, pid)
		}
	}
	return out
}

// udpTable lists the UDP ports listened on, IPv4 and IPv6, with their process: /proc/net/udp and udp6 give the
// sockets, the /proc/<pid>/fd of the processes who has them. A server listening on every address may take an IPv6
// socket that serves IPv4 too ([::]:27016, as CS2 does): only udp6 lists it.
func udpTable() []UDPEntry {
	byInode := map[string]*UDPEntry{}
	var out []*UDPEntry
	for _, file := range []string{"/proc/net/udp", "/proc/net/udp6"} {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, e := range parseProcNetUDP(string(b)) {
			byInode[e.inode] = &e.UDPEntry
			out = append(out, &e.UDPEntry)
		}
	}
	if len(out) == 0 {
		return nil
	}
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		fds, _ := os.ReadDir("/proc/" + p.Name() + "/fd")
		for _, fd := range fds {
			l, err := os.Readlink("/proc/" + p.Name() + "/fd/" + fd.Name())
			if err == nil && strings.HasPrefix(l, "socket:[") {
				if e := byInode[strings.TrimSuffix(strings.TrimPrefix(l, "socket:["), "]")]; e != nil && e.PID == 0 {
					e.PID = pid
				}
			}
		}
	}
	res := make([]UDPEntry, 0, len(out))
	for _, e := range out {
		res = append(res, *e)
	}
	return res
}

// Interrupt asks a process (and its group: a start script and its server) to end, as Ctrl+C does: SIGINT, which
// the game servers take as "save and quit".
// It says what it did.
func Interrupt(pid int) (string, error) {
	if pid <= 0 {
		return "", errors.New("no pid")
	}
	if err := syscall.Kill(-pid, syscall.SIGINT); err == nil {
		return "Ctrl+C (SIGINT) sent", nil
	}
	if err := syscall.Kill(pid, syscall.SIGINT); err != nil {
		return "", fmt.Errorf("Ctrl+C (SIGINT): %w", err)
	}
	return "Ctrl+C (SIGINT) sent", nil
}

// Kill ends a process (and its group) at once.
func Kill(pid int) error {
	if pid <= 0 {
		return errors.New("no pid")
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	return syscall.Kill(pid, syscall.SIGKILL)
}

var clkTck = 100.0

// procStat reads a process's /proc/<pid>/stat: its process group, CPU time and resident pages.
func procStat(pid int) (pgrp int, cpu time.Duration, rssPages uint64, err error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, 0, err
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || i+2 >= len(s) {
		return 0, 0, 0, errors.New("bad stat")
	}
	f := strings.Fields(s[i+2:])
	if len(f) < 22 {
		return 0, 0, 0, errors.New("bad stat")
	}
	pgrp, _ = strconv.Atoi(f[2])
	ut, _ := strconv.ParseFloat(f[11], 64)
	st, _ := strconv.ParseFloat(f[12], 64)
	rssPages, _ = strconv.ParseUint(f[21], 10, 64)
	return pgrp, time.Duration((ut + st) / clkTck * float64(time.Second)), rssPages, nil
}

// privateMem is a process's own memory (Private_Clean + Private_Dirty of smaps_rollup), as Task Manager counts it
// on Windows; 0 when the system does not tell.
func privateMem(pid int) uint64 {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/smaps_rollup")
	if err != nil {
		return 0
	}
	var total uint64
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "Private_Clean:") || strings.HasPrefix(l, "Private_Dirty:") {
			if f := strings.Fields(l); len(f) >= 2 {
				kb, _ := strconv.ParseUint(f[1], 10, 64)
				total += kb * 1024
			}
		}
	}
	return total
}

// groups: the processes of each process group but its leader, from one look at every process of the machine, kept
// 2 seconds: the servers measured in the same round share it (a look per server would cost as many reads of /proc
// as there are processes, times the servers).
var groups struct {
	sync.Mutex
	at      time.Time
	members map[int][]int
}

func groupMembers(leader int) []int {
	groups.Lock()
	defer groups.Unlock()
	if groups.members == nil || time.Since(groups.at) > 2*time.Second {
		m := map[int][]int{}
		if entries, err := os.ReadDir("/proc"); err == nil {
			for _, e := range entries {
				o, err := strconv.Atoi(e.Name())
				if err != nil {
					continue
				}
				if g, err := procGroup(o); err == nil && g != o {
					m[g] = append(m[g], o)
				}
			}
		}
		groups.members, groups.at = m, time.Now()
	}
	return groups.members[leader]
}

// procGroup reads only the process group of a process.
func procGroup(pid int) (int, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 || i+2 >= len(b) {
		return 0, errors.New("bad stat")
	}
	// after ")": state, ppid, pgrp
	f := bytes.SplitN(b[i+2:], []byte(" "), 4)
	if len(f) < 4 {
		return 0, errors.New("bad stat")
	}
	return strconv.Atoi(string(f[2]))
}

// Stats reads what a server uses. A server started by the manager leads its process group: srcds_run (a script)
// and the srcds_linux it runs are counted together.
func Stats(pid int) (Usage, error) {
	pgrp, cpu, rss, err := procStat(pid)
	if err != nil {
		return Usage{}, err
	}
	pids := []int{pid}
	if pgrp == pid {
		for _, o := range groupMembers(pid) {
			if g, c, r, err := procStat(o); err == nil && g == pid {
				cpu += c
				rss += r
				pids = append(pids, o)
			}
		}
	}
	var mem uint64
	for _, p := range pids {
		mem += privateMem(p)
	}
	if mem == 0 {
		mem = rss * uint64(os.Getpagesize())
	}
	return Usage{CPUTime: cpu, Memory: mem}, nil
}

// SystemMemory gives the RAM of the machine: total and free bytes.
func SystemMemory() (total, free uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			free = v * 1024
		}
	}
	return total, free
}

// Disk gives the size and the free space of the disk a folder is on.
func Disk(path string) (total, free uint64) {
	var st syscall.Statfs_t
	for p := path; ; p = filepath.Dir(p) {
		if err := syscall.Statfs(p, &st); err == nil {
			return st.Blocks * uint64(st.Bsize), st.Bavail * uint64(st.Bsize)
		}
		if p == filepath.Dir(p) {
			return 0, 0
		}
	}
}

// Roots lists where folders can be picked from.
func Roots() []string { return []string{"/"} }

// tcpListeners is not read here: the RCON check is for Windows, where servers share ports across IPs.
func tcpListeners() []UDPEntry { return nil }

func exitInfo(st *os.ProcessState) ExitInfo {
	ws, ok := st.Sys().(syscall.WaitStatus)
	if !ok {
		return ExitInfo{Known: true, Code: int64(st.ExitCode())}
	}
	if ws.Signaled() {
		return ExitInfo{Known: true, Code: -1, Signal: signalName(ws.Signal()), Core: ws.CoreDump()}
	}
	return ExitInfo{Known: true, Code: int64(ws.ExitStatus())}
}

func signalName(s syscall.Signal) string {
	names := map[syscall.Signal]string{syscall.SIGSEGV: "SIGSEGV", syscall.SIGABRT: "SIGABRT", syscall.SIGKILL: "SIGKILL",
		syscall.SIGBUS: "SIGBUS", syscall.SIGFPE: "SIGFPE", syscall.SIGILL: "SIGILL", syscall.SIGTERM: "SIGTERM",
		syscall.SIGINT: "SIGINT", syscall.SIGHUP: "SIGHUP", syscall.SIGTRAP: "SIGTRAP", syscall.SIGQUIT: "SIGQUIT"}
	if n, ok := names[s]; ok {
		return n
	}
	return "signal " + strconv.Itoa(int(s))
}

// WindowTexts: no windows to read outside Windows.
func WindowTexts(pid int) []string { return nil }

// NoCrashDialogs: nothing to do outside Windows.
func NoCrashDialogs(wer bool) {}

// IsAdmin says whether the manager runs as root.
func IsAdmin() bool { return os.Geteuid() == 0 }

type procNetEntry struct {
	UDPEntry
	inode string
}

// parseProcNetUDP reads /proc/net/udp or udp6: the local address (8 hex digits for IPv4, 32 for IPv6, each 32-bit
// word in the machine's byte order), the port, the socket's inode.
func parseProcNetUDP(s string) []procNetEntry {
	var out []procNetEntry
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return nil
	}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		host, portHex, ok := strings.Cut(f[1], ":")
		if !ok || (len(host) != 8 && len(host) != 32) {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil {
			continue
		}
		ip := make(net.IP, 0, 16)
		for k := 0; k < len(host); k += 8 {
			w, err := strconv.ParseUint(host[k:k+8], 16, 32)
			if err != nil {
				ip = nil
				break
			}
			ip = append(ip, byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
		}
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil && len(host) == 32 {
			ip = v4 // ::ffff:a.b.c.d: an IPv4 address
		}
		out = append(out, procNetEntry{UDPEntry{IP: ip, Port: int(port)}, f[9]})
	}
	return out
}
