package proc

import (
	"errors"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")

// SystemCPU gives the CPU time of the whole machine since it started: busy and total, in 100 ns (GetSystemTimes,
// whose kernel time includes the idle time).
func SystemCPU() (busy, total uint64, err error) {
	var idle, kernel, user windows.Filetime
	if r, _, e := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); r == 0 {
		return 0, 0, e
	}
	ft := func(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	total = ft(kernel) + ft(user)
	return total - ft(idle), total, nil
}

// NetTotals: on Windows the machine's traffic comes from the network count of the servers (netmon), which sees
// every program's.
func NetTotals() (in, out uint64, err error) {
	return 0, 0, errors.New("counted by netmon on Windows")
}

// MountPoint: the drive of a path, "E:\" (see the Linux one).
func MountPoint(path string) string { return filepath.VolumeName(path) + `\` }

var procGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// SelfCounts gives the threads and the handles of the manager's own process (as Task Manager's Details columns).
func SelfCounts() (threads, handles int) {
	self := windows.GetCurrentProcessId()
	if snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0); err == nil {
		var e windows.ThreadEntry32
		e.Size = uint32(unsafe.Sizeof(e))
		for err := windows.Thread32First(snap, &e); err == nil; err = windows.Thread32Next(snap, &e) {
			if e.OwnerProcessID == self {
				threads++
			}
		}
		windows.CloseHandle(snap)
	}
	var n uint32
	if r, _, _ := procGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n))); r != 0 {
		handles = int(n)
	}
	return threads, handles
}
