//go:build windows

package daemon

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"
)

func runPlatform(configPath, serviceName string) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect Windows service context: %w", err)
	}
	if !isService {
		return runWindowsConsole(configPath)
	}
	return svc.Run(serviceName, serviceHandler{configPath: configPath})
}

func runWindowsConsole(configPath string) error {
	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		close(stop)
	}()
	return Serve(configPath, stop)
}

type serviceHandler struct {
	configPath string
}

func (h serviceHandler) Execute(_ []string, changes <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	stop := make(chan struct{})
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- serve(h.configPath, stop, ready)
	}()

	status := svc.Status{
		State:      svc.StartPending,
		WaitHint:   30_000,
		CheckPoint: 1,
	}
	statuses <- status
	readySignal := (<-chan struct{})(ready)

	for {
		select {
		case <-readySignal:
			status = svc.Status{
				State:   svc.Running,
				Accepts: svc.AcceptStop | svc.AcceptShutdown,
			}
			statuses <- status
			readySignal = nil
		case err := <-done:
			if err != nil {
				log.Printf("service stopped with error: %v", err)
				return false, 1
			}
			return false, 0
		case change, ok := <-changes:
			if !ok {
				close(stop)
				if err := <-done; err != nil {
					log.Printf("service shutdown after control channel closed: %v", err)
					return false, 1
				}
				return false, 0
			}
			switch change.Cmd {
			case svc.Interrogate:
				statuses <- status
			case svc.Stop, svc.Shutdown:
				status = svc.Status{
					State:      svc.StopPending,
					WaitHint:   20_000,
					CheckPoint: 1,
				}
				statuses <- status
				close(stop)
				if err := <-done; err != nil {
					log.Printf("service shutdown failed: %v", err)
					return false, 1
				}
				return false, 0
			default:
				log.Printf("ignoring unsupported service control command %d", change.Cmd)
			}
		}
	}
}
