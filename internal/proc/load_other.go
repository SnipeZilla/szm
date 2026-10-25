//go:build !linux && !windows

package proc

import "errors"

func SystemCPU() (busy, total uint64, err error) { return 0, 0, errors.New("not on this system") }
func NetTotals() (in, out uint64, err error)     { return 0, 0, errors.New("not on this system") }

func MountPoint(path string) string { return "/" }

func SelfCounts() (threads, handles int) { return 0, 0 }
