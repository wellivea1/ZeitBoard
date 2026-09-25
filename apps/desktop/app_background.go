package main

import (
	"context"
	"time"
)

const desktopBackgroundInterval = time.Minute

// The service belongs to the app context, never to a screen. Clock timers wake
// after host resume; failed exchanges retry from durable outbox/cursor state.
func (a *App) startDesktopBackground(parent context.Context, interval time.Duration) {
	a.backgroundMu.Lock()
	defer a.backgroundMu.Unlock()
	if a.backgroundCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done, wake := make(chan struct{}), make(chan struct{}, 1)
	a.backgroundCancel, a.backgroundDone, a.backgroundWake = cancel, done, wake
	go func() {
		defer close(done)
		for {
			if ctx.Err() != nil {
				return
			}
			a.activityMu.Lock()
			a.reconcileActivityLocked(ctx)
			a.activityMu.Unlock()
			// Foreground sync/configuration already owns the same pipeline. A
			// timer tick must not queue a duplicate exchange behind it.
			if a.backendSyncMu.TryLock() {
				_, _ = a.syncNowLocked(ctx)
				a.backendSyncMu.Unlock()
			}
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-wake:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()
}

func (a *App) wakeDesktopBackground() {
	a.backgroundMu.Lock()
	defer a.backgroundMu.Unlock()
	if a.backgroundWake != nil {
		select {
		case a.backgroundWake <- struct{}{}:
		default:
		}
	}
}

func (a *App) cancelBackendRun() {
	a.backendRunMu.Lock()
	defer a.backendRunMu.Unlock()
	if a.backendRunCancel != nil {
		a.backendRunCancel()
	}
}

func (a *App) stopDesktopBackground() {
	a.backgroundMu.Lock()
	cancel, done := a.backgroundCancel, a.backgroundDone
	a.backgroundMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	a.cancelBackendRun()
	// Join any foreground sync before the caller closes clients and storage.
	a.backendSyncMu.Lock()
	a.backendSyncMu.Unlock()
	a.backgroundMu.Lock()
	a.backgroundCancel, a.backgroundDone, a.backgroundWake = nil, nil, nil
	a.backgroundMu.Unlock()
}
