package proc

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/windows/registry"
)

var osNameOnce = sync.OnceValue(func() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer k.Close()
	str := func(n string) string { v, _, _ := k.GetStringValue(n); return strings.TrimSpace(v) }
	name, disp, build := str("ProductName"), str("DisplayVersion"), str("CurrentBuildNumber")
	if name == "" {
		name = "Windows"
	}
	// Windows 11 still calls itself "Windows 10" there: its build says which (22000 and up)
	if n, _ := strconv.Atoi(build); n >= 22000 && strings.HasPrefix(name, "Windows 10") {
		name = "Windows 11" + strings.TrimPrefix(name, "Windows 10")
	}
	if disp != "" {
		name += " " + disp
	}
	if build != "" {
		if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
			build += fmt.Sprintf(".%d", ubr)
		}
		name += " (build " + build + ")"
	}
	return name
})

// OSName names the system: "Windows Server 2025 Datacenter 24H2 (build 26100.4061)".
func OSName() string { return osNameOnce() }
