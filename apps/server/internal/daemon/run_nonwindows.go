//go:build !windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"
)

func runPlatform(configPath, _ string) error {
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
