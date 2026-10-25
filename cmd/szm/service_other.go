//go:build !windows

package main

import (
	"context"
	"errors"
)

var errNoService = errors.New("services are for Windows: on Linux, run szm from systemd")

func isService() bool                                          { return false }
func runService(string, func(ctx context.Context) error) error { return errNoService }
func installService(string, string) error                      { return errNoService }
func uninstallService(string) error                            { return errNoService }
func startService(string) error                                { return errNoService }
func stopService(string) error                                 { return errNoService }
