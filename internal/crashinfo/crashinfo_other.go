//go:build !windows

package crashinfo

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemRecords finds what the Linux kernel wrote about the crash (segfault, killed for lack of memory) of these
// programs or processes since a moment. Reading the kernel log needs root (or kernel.dmesg_restrict=0).
func SystemRecords(pid int, programs []string, since time.Time) []SystemRecord {
	up := uptime()
	if up <= 0 {
		return nil
	}
	boot := time.Now().Add(-up)
	// Raw system calls: an os.File would wait (in Go's poller) for new records at the end instead of returning
	fd, err := syscall.Open("/dev/kmsg", syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	defer syscall.Close(fd)
	var out []SystemRecord
	buf := make([]byte, 8192)
	pids := []int{pid}
	for n := 0; n < 200000; n++ {
		k, err := syscall.Read(fd, buf)
		if err != nil || k <= 0 {
			if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.EINTR) {
				continue // records overwritten while reading
			}
			break // EAGAIN: the end
		}
		at, text, ok := parseKmsg(string(buf[:k]))
		if !ok {
			continue
		}
		t := boot.Add(at)
		if t.Before(since.Add(-5*time.Second)) || !kernelMatch(text, programs, pids) {
			continue
		}
		out = append(out, SystemRecord{Time: t, Source: "Linux kernel log", Text: text})
	}
	if len(out) > 10 {
		out = out[len(out)-10:]
	}
	return out
}

func uptime() time.Duration {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	s, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

// CrashDumpDirs: Linux writes core files where the kernel's core_pattern says; the server's folder is looked in.
func CrashDumpDirs() []string { return nil }
