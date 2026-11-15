//go:build !windows

package proc

import (
	"context"
	"errors"
	"io"
)

// ConsoleCols: see conwin_windows.go. Elsewhere the servers' output is written to their log directly (Spec.Output).
const ConsoleCols = 0

func ReadConsoleWindow(pid, rows int) ([]string, error) {
	return nil, errors.New("reading a console window is for Windows")
}

func DumpConsole(pid, rows int, w io.Writer) error {
	return errors.New("reading a console window is for Windows")
}

func FollowConsoleWindow(ctx context.Context, pid, rows int, each func(rows []string)) error {
	return errors.New("reading a console window is for Windows")
}

func FollowConsole(pid, rows int, w io.Writer) error {
	return errors.New("reading a console window is for Windows")
}

// SendCtrlC is the Windows helper of Interrupt (not used elsewhere).
func SendCtrlC(int) error { return errors.New("only on Windows") }
