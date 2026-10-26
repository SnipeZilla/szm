package proc

import (
	"context"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startConsole runs a program in a pseudo console (Windows 10 1809, Server 2019 and later): it writes as into a
// console window, each line at once (to a pipe, steamcmd holds back all it prints until it ends). The console has no
// window. Its programs are kept in a job: steamcmd starts itself again after updating itself, and its output ends
// once the last of them has ended.
func startConsole(ctx context.Context, exe string, args []string, dir string) (*Captured, error) {
	if procCreatePseudoConsole.Find() != nil {
		return nil, fmt.Errorf("no pseudo console on this Windows")
	}
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, err
	}
	var pc windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 250, Y: 60}, inR, outW, 0, &pc); err != nil {
		for _, h := range []windows.Handle{inR, inW, outR, outW} {
			windows.CloseHandle(h)
		}
		return nil, fmt.Errorf("pseudo console: %w", err)
	}
	windows.CloseHandle(inR) // the console has its own
	windows.CloseHandle(outW)
	out := os.NewFile(uintptr(outR), "console")
	fail := func(err error) (*Captured, error) {
		windows.ClosePseudoConsole(pc)
		windows.CloseHandle(inW)
		out.Close()
		return nil, err
	}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail(err)
	}
	defer attrs.Delete()
	// the console's handle is the attribute's value itself (not a pointer to it)
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&pc)), unsafe.Sizeof(pc)); err != nil {
		return fail(err)
	}
	si := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.Flags = windows.STARTF_USESTDHANDLES // none of the manager's own: those of its console
	exeP, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return fail(err)
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{exe}, args...)))
	if err != nil {
		return fail(err)
	}
	var dirP *uint16
	if dir != "" {
		if dirP, err = windows.UTF16PtrFromString(dir); err != nil {
			return fail(err)
		}
	}
	job, _ := windows.CreateJobObject(nil, nil) // 0 when it can not be made: its first program only is waited for
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(exeP, cmdline, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_SUSPENDED,
		nil, dirP, &si.StartupInfo, &pi); err != nil {
		if job != 0 {
			windows.CloseHandle(job)
		}
		return fail(err)
	}
	if job != 0 && windows.AssignProcessToJobObject(job, pi.Process) != nil {
		windows.CloseHandle(job)
		job = 0
	}
	windows.ResumeThread(pi.Thread)
	windows.CloseHandle(pi.Thread)

	kill := func() {
		if job != 0 {
			_ = windows.TerminateJobObject(job, 1)
		} else {
			_ = windows.TerminateProcess(pi.Process, 1)
		}
	}
	var code uint32
	done := make(chan struct{})
	go func() {
		windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		windows.GetExitCodeProcess(pi.Process, &code)
		for job != 0 && jobActive(job) > 0 { // steamcmd started again after updating itself
			time.Sleep(300 * time.Millisecond)
		}
		windows.ClosePseudoConsole(pc) // its output ends (once read)
		windows.CloseHandle(inW)
		close(done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			kill()
		case <-done:
		}
	}()
	return &Captured{
		Output:  &vtFilter{r: out},
		Console: true,
		wait: func() error {
			<-done
			out.Close()
			windows.CloseHandle(pi.Process)
			if job != 0 {
				windows.CloseHandle(job)
			}
			if code != 0 {
				return fmt.Errorf("exit status %d", code)
			}
			return nil
		},
		kill: kill,
	}, nil
}

// procCreatePseudoConsole: there from Windows 10 1809 (Server 2019) on (windows.CreatePseudoConsole panics without it)
var procCreatePseudoConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole")

// jobActive: how many programs of a job are running.
func jobActive(job windows.Handle) uint32 {
	var info struct {
		TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
		TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
	}
	if windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)), nil) != nil {
		return 0
	}
	return info.ActiveProcesses
}
