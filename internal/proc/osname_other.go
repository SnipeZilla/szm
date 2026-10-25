//go:build !windows

package proc

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"sync"
)

var osNameOnce = sync.OnceValue(func() string {
	name := ""
	if f, err := os.Open("/etc/os-release"); err == nil {
		s := bufio.NewScanner(f)
		for s.Scan() {
			if v, ok := strings.CutPrefix(s.Text(), "PRETTY_NAME="); ok {
				name = strings.Trim(v, `"'`)
			}
		}
		f.Close()
	}
	if name == "" {
		name = strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:]
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		name += " (kernel " + strings.TrimSpace(string(b)) + ")"
	}
	return name
})

// OSName names the system: "Ubuntu 24.04.1 LTS (kernel 6.8.0-45-generic)".
func OSName() string { return osNameOnce() }
