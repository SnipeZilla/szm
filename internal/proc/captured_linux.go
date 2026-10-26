//go:build linux

package proc

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"
)

// startConsole runs a program in a terminal of its own (a pseudo terminal): it prints each line at once, as to a
// terminal. Its output is read from the terminal's other end until every program on it has ended.
func startConsole(ctx context.Context, exe string, args []string, dir string) (*Captured, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Captured, error) {
		m.Close()
		return nil, err
	}
	var unlock int32
	if err := ioctl(m.Fd(), syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		return fail(err)
	}
	var n uint32
	if err := ioctl(m.Fd(), syscall.TIOCGPTN, unsafe.Pointer(&n)); err != nil {
		return fail(err)
	}
	s, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return fail(err)
	}
	ws := struct{ row, col, x, y uint16 }{60, 250, 0, 0} // wide: its lines are not wrapped
	_ = ioctl(s.Fd(), syscall.TIOCSWINSZ, unsafe.Pointer(&ws))

	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.Env = append(os.Environ(), "TERM=dumb")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0} // its own session: ended together
	if err := cmd.Start(); err != nil {
		s.Close()
		return fail(err)
	}
	s.Close() // the program has it: once it (and what it started) has ended, reading gives EIO
	pid := cmd.Process.Pid
	kill := func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			kill()
		case <-done:
		}
	}()
	return &Captured{
		Output:  &vtFilter{r: ptyReader{m}},
		Console: true,
		wait: func() error {
			err := cmd.Wait()
			close(done)
			m.Close()
			return err
		},
		kill: kill,
	}, nil
}

// ptyReader: the end of a terminal's output (EIO, every program on it ended) is its EOF.
type ptyReader struct{ f *os.File }

func (r ptyReader) Read(p []byte) (int, error) {
	n, err := r.f.Read(p)
	if err != nil && errors.Is(err, syscall.EIO) {
		return n, io.EOF
	}
	return n, err
}

func ioctl(fd uintptr, req uint, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(req), uintptr(arg)); e != 0 {
		return e
	}
	return nil
}
