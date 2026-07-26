package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"non24.app/desktop/internal/localagent"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := localagent.RunBridge(ctx, os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
