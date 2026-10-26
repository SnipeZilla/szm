//go:build !linux && !windows

package proc

import (
	"context"
	"errors"
)

// startConsole: no console of its own here; its output goes through a pipe.
func startConsole(context.Context, string, []string, string) (*Captured, error) {
	return nil, errors.New("not on this system")
}
