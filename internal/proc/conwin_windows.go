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
	procGetConsoleProcList  = k32.NewProc("GetConsoleProcessList")
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
	h, err := openScreen()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	scr := &conScreen{h: h}
	_, last, err := scr.size() // rows before the cursor's are finished
	if err != nil {
		return nil, err
	}
	if scr.width <= 0 || last <= 0 {
		return nil, nil
	}
	first := max(0, last-rows)
	return scr.rows(first, last-first)
}

// openScreen opens the screen of the console this program is attached to.
func openScreen() (windows.Handle, error) {
	name, _ := windows.UTF16PtrFromString("CONOUT$")
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("open its screen: %v", err)
	}
	return h, nil
}

// conScreen is the screen of the attached console (rowScreen).
type conScreen struct {
	h     windows.Handle
	width int      // of its buffer, in characters: known after size
	buf   []uint16 // kept from a read to the next
}

func (c *conScreen) size() (height, cursor int, err error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(c.h, &info); err != nil {
		return 0, 0, fmt.Errorf("read its screen size: %v", err)
	}
	c.width = int(info.Size.X)
	return int(info.Size.Y), int(info.CursorPosition.Y), nil
}

// screenRows: rows read by one call of ReadConsoleOutputCharacterW at most (more are read by several).
const screenRows = 400

func (c *conScreen) rows(first, n int) ([]string, error) {
	if c.width <= 0 {
		if _, _, err := c.size(); err != nil {
			return nil, err
		}
	}
	width := c.width
	out := make([]string, 0, max(n, 0))
	if width <= 0 {
		return out, nil
	}
	for n > 0 {
		k := min(n, screenRows)
		chars := width * k
		if cap(c.buf) < chars {
			c.buf = make([]uint16, chars)
		}
		buf := c.buf[:chars]
		var read uint32
		coord := uintptr(uint32(uint16(0)) | uint32(uint16(first))<<16) // COORD{X: 0, Y: first}, by value
		// ReadConsoleOutputCharacterW(screen, chars, length, COORD, &read): it goes on row after row
		if r, _, e := procReadConsoleOutputCh.Call(uintptr(c.h), uintptr(unsafe.Pointer(&buf[0])), uintptr(chars), coord, uintptr(unsafe.Pointer(&read))); r == 0 {
			return nil, fmt.Errorf("read its screen: %v", e)
		}
		buf = buf[:min(int(read), chars)]
		got := 0
		for j := 0; j+width <= len(buf); j += width {
			out = append(out, strings.TrimRight(string(utf16.Decode(buf[j:j+width])), " \x00"))
			got++
		}
		if got < k {
			break // the end of the buffer
		}
		first, n = first+k, n-k
	}
	return out, nil
}

// followEvery is how often the reader of a console window looks at it.
const followEvery = 100 * time.Millisecond

// FollowConsoleWindow gives the rows of a process's console window as they are finished, until the process ends or
// ctx does: first its last rows (rows of them at most), then each time the new ones only. Through a helper (szm
// __console <pid> <rows> follow) that stays attached to the server's console: one program for as long as the
// server runs, and its lines within a moment, instead of a new one every second.
func FollowConsoleWindow(ctx context.Context, pid, rows int, each func(rows []string)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "__console", strconv.Itoa(pid), strconv.Itoa(rows), "follow")
	RunHiddenCmd(cmd)
	// its input is a pipe the manager never writes to: it ends when the manager does, and the helper leaves then
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return err
	}
	rerr := readBatches(out, each)
	if rerr != nil {
		_ = cmd.Process.Kill()
	}
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if msg := strings.TrimPrefix(strings.TrimSpace(errb.String()), "Error: "); msg != "" {
		return errors.New(msg)
	}
	if rerr != nil {
		return rerr
	}
	return werr
}

// FollowConsole is the helper: it attaches to the console of pid and writes its rows to w as they are finished
// (writeBatch), until the process ends, the manager does (its input closes), or w can not be written.
func FollowConsole(pid, rows int, w io.Writer) error {
	procFreeConsole.Call() // CREATE_NO_WINDOW may still give the helper a console of its own: one only at a time
	if r, _, e := procAttachConsole.Call(uintptr(uint32(pid))); r == 0 {
		return fmt.Errorf("attach to the console of %d: %v", pid, e)
	}
	defer procFreeConsole.Call()
	procSetConsoleCtrlHandl.Call(0, 1) // a Ctrl+C sent to the server (Interrupt) is not for this helper
	h, err := openScreen()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	go func() {
		// The manager ended (or let go): leave the server's console, which must not stay open for the helper.
		_, _ = io.Copy(io.Discard, os.Stdin)
		procFreeConsole.Call()
		os.Exit(0)
	}()
	scr := &conScreen{h: h}
	t := newRowTracker(rows)
	var buf bytes.Buffer
	for {
		// Asked before the look: the rows a server wrote as it ended are still read.
		gone := !inConsole(pid)
		batch, err := t.poll(scr)
		if err != nil {
			return err
		}
		if gone {
			// the row it was writing has no line end and never will
			if _, cur, err := scr.size(); err == nil && cur >= 0 {
				if last, err := scr.rows(cur, 1); err == nil && len(last) == 1 && last[0] != "" {
					batch = append(batch, last[0])
				}
			}
		}
		if len(batch) > 0 {
			if err := writeBatch(w, &buf, batch); err != nil {
				return nil // the manager is gone
			}
		}
		if gone {
			return nil
		}
		time.Sleep(followEvery)
	}
}

// inConsole tells whether a process is still one of those of the console this program is attached to.
func inConsole(pid int) bool {
	list := make([]uint32, 64)
	n, _, _ := procGetConsoleProcList.Call(uintptr(unsafe.Pointer(&list[0])), uintptr(len(list)))
	if n == 0 {
		return false
	}
	if int(n) > len(list) {
		return true // more than fit: not known, taken for there
	}
	for _, p := range list[:n] {
		if int(p) == pid {
			return true
		}
	}
	return false
}
