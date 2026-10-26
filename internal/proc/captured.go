package proc

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

// Captured is a program whose output is read as it prints (steamcmd).
type Captured struct {
	Output io.Reader // what it prints (its console's control sequences taken out)
	// Console: it runs in a console of its own (a terminal on Linux, a pseudo console on Windows): a question it asks
	// waits for an answer typed there, which nobody gives (its caller ends it)
	Console bool
	wait    func() error
	kill    func()
}

// Wait waits for its end, once its output is read to the end.
func (c *Captured) Wait() error { return c.wait() }

// Kill ends it (and what it started).
func (c *Captured) Kill() { c.kill() }

// StartCaptured starts a program without a window, its output read as it prints. Programs hold back what they
// print to a pipe (steamcmd: on Windows, until it ends): it runs in a console of its own where it can, so that it
// prints each line at once; else through a pipe, a question it asks getting an empty answer.
func StartCaptured(ctx context.Context, exe string, args []string, dir string) (*Captured, error) {
	if c, err := startConsole(ctx, exe, args, dir); err == nil {
		return c, nil
	}
	return startPiped(ctx, exe, args, dir)
}

// startPiped: its output through a pipe, its input empty.
func startPiped(ctx context.Context, exe string, args []string, dir string) (*Captured, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	RunHiddenCmd(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	cmd.Stdin = strings.NewReader("") // a question (a Steam Guard code) gets an empty answer instead of waiting forever
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Captured{Output: out, wait: cmd.Wait, kill: func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}}, nil
}

// vtFilter takes out of a console's output its control sequences (colors, cursor moves, its title): a move to
// another line becomes a line end (\r), a move forward spaces.
type vtFilter struct {
	r     io.Reader
	state int    // 0 text, 1 after ESC, 2 in CSI, 3 in OSC, 4 ESC in OSC, 5 one more byte to pass over
	param []byte // the parameters of a CSI
	buf   []byte // read
	out   []byte // filtered, not given yet
	err   error  // the reader's, once out is given
}

func (f *vtFilter) Read(p []byte) (int, error) {
	for len(f.out) == 0 {
		if f.err != nil {
			return 0, f.err
		}
		if f.buf == nil {
			f.buf = make([]byte, 32*1024)
		}
		n, err := f.r.Read(f.buf)
		f.err = err
		out := f.out[:0]
		for _, b := range f.buf[:n] {
			switch f.state {
			case 0:
				switch b {
				case 0x1b:
					f.state = 1
				case 0x07, 0x00:
				default:
					out = append(out, b)
				}
			case 1:
				switch b {
				case '[':
					f.state, f.param = 2, f.param[:0]
				case ']':
					f.state = 3
				case '(', ')', '*', '+':
					f.state = 5
				default:
					f.state = 0
				}
			case 2:
				if b < 0x40 || b > 0x7e {
					if len(f.param) < 32 {
						f.param = append(f.param, b)
					}
					continue
				}
				f.state = 0
				switch b {
				case 'H', 'f', 'd', 'A', 'B', 'E', 'F':
					out = append(out, '\r')
				case 'G':
					if len(f.param) == 0 || string(f.param) == "1" {
						out = append(out, '\r')
					}
				case 'C':
					k := 1
					if v := atoiSmall(f.param); v > 0 {
						k = min(v, 200)
					}
					out = append(out, []byte(strings.Repeat(" ", k))...)
				}
			case 3:
				switch b {
				case 0x07:
					f.state = 0
				case 0x1b:
					f.state = 4
				}
			case 4:
				if b == '\\' {
					f.state = 0
				} else {
					f.state = 3
				}
			case 5:
				f.state = 0
			}
		}
		f.out = out
	}
	k := copy(p, f.out)
	f.out = f.out[k:]
	return k, nil
}

func atoiSmall(b []byte) int {
	v := 0
	for _, c := range b {
		if c < '0' || c > '9' || v > 100000 {
			return 0
		}
		v = v*10 + int(c-'0')
	}
	return v
}
