package proc

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

// SystemCPU gives the CPU time of the whole machine since it started: busy and total, in ticks (/proc/stat).
func SystemCPU() (busy, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, errors.New("/proc/stat is empty")
	}
	// cpu  user nice system idle iowait irq softirq steal guest guest_nice
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, errors.New("/proc/stat: no cpu line")
	}
	var idle uint64
	for k, s := range fields[1:] {
		if k >= 8 { // guest times are inside user and nice already
			break
		}
		n, _ := strconv.ParseUint(s, 10, 64)
		total += n
		if k == 3 || k == 4 { // idle, iowait
			idle += n
		}
	}
	return total - idle, total, nil
}

// NetTotals gives the bytes the machine received and sent since it started, on its network cards (/proc/net/dev):
// not the loopback, nor the virtual links of containers, which would count the same traffic twice.
func NetTotals() (in, out uint64, err error) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	in, out = parseNetDev(string(b))
	return in, out, nil
}

func parseNetDev(s string) (in, out uint64) {
	for _, line := range strings.Split(s, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr") {
			continue
		}
		// receive: bytes packets errs drop fifo frame compressed multicast; transmit: bytes ...
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		in += r
		out += t
	}
	return in, out
}

// MountPoint gives the mount point of the file system a path is on (/proc/self/mounts): "/", "/home", "/srv/games".
func MountPoint(path string) string {
	b, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return "/"
	}
	return mountOf(string(b), path)
}

func mountOf(mounts, path string) string {
	best := "/"
	for _, line := range strings.Split(mounts, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		dir := strings.ReplaceAll(f[1], `\040`, " ") // spaces are written \040
		if (path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")) && len(dir) > len(best) {
			best = dir
		}
	}
	return best
}

// SelfCounts gives the threads and the open files (handles) of the manager's own process.
func SelfCounts() (threads, handles int) {
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "Threads:"); ok {
				threads, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
	}
	if fds, err := os.ReadDir("/proc/self/fd"); err == nil {
		handles = len(fds)
	}
	return threads, handles
}
