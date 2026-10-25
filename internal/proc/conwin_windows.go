package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The console window of a program that writes no console log of its own (CS2 ignores -condebug, and writes to its
// window, not to its output): its text is read from the window itself. A process can be attached to one console
// only, and the manager may have its own (run in a terminal): a short helper (szm __console <pid> <rows>) attaches to
// the server's console, prints its last lines, and ends.

var (
	k32                     = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole       = k32.NewProc("AttachConsole")
	procFreeConsole         = k32.NewProc("FreeConsole")
	procReadConsoleOutputCh = k32.NewProc("ReadConsoleOutputCharacterW")
	procSetConsoleCtrlHandl = k32.NewProc("SetConsoleCtrlHandler")
	procGenerateConsoleCtrl = k32.NewProc("GenerateConsoleCtrlEvent")
)

// Interrupt asks a server to end as Ctrl+C in its console window does (the games of other engines save and quit on
// it), through a helper (szm __ctrlc <pid>): a Ctrl+C reaches every program of a console, and the manager may have
// its own. A server whose window is not a console (DayZ, Arma: a window of their own) is asked to close it, as its
// X does. It says what it did.
func Interrupt(pid int) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "__ctrlc", strconv.Itoa(pid))
	RunHiddenCmd(cmd)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimPrefix(strings.TrimSpace(errb.String()), "Error: ")
		if msg == "" {
			msg = err.Error()
		}
		if CloseWindows(pid) > 0 {
			return "its window asked to close, as its X does (it has no console for Ctrl+C)", nil
		}
		return "", fmt.Errorf("Ctrl+C: %s; and it has no window to close", msg)
	}
	return "Ctrl+C sent", nil
}

// SendCtrlC is the helper: it attaches to the console of pid, ignores Ctrl+C itself, and sends it.
func SendCtrlC(pid int) error {
	procFreeConsole.Call()
	if r, _, e := procAttachConsole.Call(uintptr(uint32(pid))); r == 0 {
		return fmt.Errorf("attach to the console of %d: %v", pid, e)
	}
	procSetConsoleCtrlHandl.Call(0, 1) // this helper is not ended by it
	const ctrlCEvent = 0
	r, _, e := procGenerateConsoleCtrl.Call(ctrlCEvent, 0)
	time.Sleep(500 * time.Millisecond) // delivered before the helper leaves the console
	procFreeConsole.Call()
	if r == 0 {
		return fmt.Errorf("Ctrl+C: %v", e)
	}
	return nil
}

// ConsoleCols is the width given to the console of a server whose window is read: long lines stay on one row.
const ConsoleCols = 240

// ReadConsoleWindow gives the last rows of a process's console window (the finished lines: the row of the cursor,
// being written, is left out), through the helper.
func ReadConsoleWindow(pid, rows int) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "__console", strconv.Itoa(pid), strconv.Itoa(rows))
	RunHiddenCmd(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	s := strings.TrimSuffix(out.String(), "\n")
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

// DumpConsole is the helper: it attaches to the console of pid and writes its last rows to w, one per line.
func DumpConsole(pid, rows int, w io.Writer) error {
	procFreeConsole.Call() // CREATE_NO_WINDOW may still give the helper a console of its own: one only at a time
	if r, _, e := procAttachConsole.Call(uintptr(uint32(pid))); r == 0 {
		return fmt.Errorf("attach to the console of %d: %v", pid, e)
	}
	lines, err := readAttached(rows)
	procFreeConsole.Call()
	if err != nil {
		return err
	}
	for _, l := range lines {
		io.WriteString(w, l+"\n")
	}
	return nil
}

func readAttached(rows int) ([]string, error) {
	name, _ := windows.UTF16PtrFromString("CONOUT$")
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("open its screen: %v", err)
	}
	defer windows.CloseHandle(h)
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return nil, fmt.Errorf("read its screen size: %v", err)
	}
	width, last := int(info.Size.X), int(info.CursorPosition.Y) // rows before the cursor's are finished
	if width <= 0 || last <= 0 {
		return nil, nil
	}
	first := max(0, last-rows)
	n := width * (last - first)
	buf := make([]uint16, n)
	var read uint32
	coord := uintptr(uint32(uint16(0)) | uint32(uint16(first))<<16) // COORD{X: 0, Y: first}, by value
	// ReadConsoleOutputCharacterW(screen, chars, length, COORD, &read): it goes on row after row
	if r, _, e := procReadConsoleOutputCh.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(n), coord, uintptr(unsafe.Pointer(&read))); r == 0 {
		return nil, fmt.Errorf("read its screen: %v", e)
	}
	buf = buf[:read]
	out := make([]string, 0, last-first)
	for k := 0; k+width <= len(buf); k += width {
		out = append(out, strings.TrimRight(string(utf16.Decode(buf[k:k+width])), " \x00"))
	}
	return out, nil
}
