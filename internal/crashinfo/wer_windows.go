//go:build windows

package crashinfo

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const werKey = `SOFTWARE\Microsoft\Windows\Windows Error Reporting`

// EnableDumps has Windows Error Reporting write a crash dump of these programs (srcds.exe, cs2.exe…) into a folder
// when they crash, and show no "has stopped working" window (the server ends at once, the manager restarts it).
// It needs the manager to run as an administrator (or the service).
func EnableDumps(folder string, programs []string, keep int) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, werKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("DontShowUI", 1); err != nil {
		return err
	}
	for _, p := range programs {
		if p == "" || strings.ContainsAny(p, `\/`) {
			continue
		}
		pk, _, err := registry.CreateKey(registry.LOCAL_MACHINE, werKey+`\LocalDumps\`+p, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = pk.SetExpandStringValue("DumpFolder", folder)
		if err == nil {
			err = pk.SetDWordValue("DumpCount", uint32(keep))
		}
		if err == nil {
			err = pk.SetDWordValue("DumpType", 1) // a minidump: the threads, their stacks, the modules
		}
		pk.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
