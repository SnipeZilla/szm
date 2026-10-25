//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func isService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

type service struct {
	run func(ctx context.Context) error
}

func (s *service) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			cancel()
			if err != nil {
				return true, 1
			}
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(20 * time.Second):
				}
				return false, 0
			}
		}
	}
}

func runService(name string, run func(ctx context.Context) error) error {
	return svc.Run(name, &service{run: run})
}

func installService(name, dataDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("run this as administrator: %w", err)
	}
	defer m.Disconnect()
	if s, err := m.OpenService(name); err == nil {
		s.Close()
		return fmt.Errorf("the service %s is already installed", name)
	}
	s, err := m.CreateService(name, exe, mgr.Config{
		DisplayName:      "SnipeZilla Server Manager",
		Description:      "Installs, updates, runs and watches the game servers (web interface on port 8888 by default).",
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true,
	}, "-data", dataDir)
	if err != nil {
		return err
	}
	defer s.Close()
	// Started again if it ever stops by itself
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 86400)
	if err := s.Start(); err != nil {
		return fmt.Errorf("installed, but did not start: %w", err)
	}
	fmt.Println("Service installed and started: open http://localhost:8888/")
	return nil
}

func uninstallService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("run this as administrator: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("the service %s is not installed", name)
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	if err := s.Delete(); err != nil {
		return err
	}
	fmt.Println("Service removed. The game servers keep running.")
	return nil
}

func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("run this as administrator: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("the service is not installed (szm install)")
	}
	defer s.Close()
	return s.Start()
}

func stopService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("run this as administrator: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("the service is not installed")
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}
