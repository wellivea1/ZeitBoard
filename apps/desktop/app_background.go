package main

import (
	"context"
	"sync"
	"time"
)

const desktopBackgroundInterval = time.Minute

// periodicWorker runs a pass on a timer and whenever it is woken, one pass at a
// time. A wake during a pass runs another right after it, so work queued while
// a pass runs is never left for the next tick.
type periodicWorker struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}
}

func (w *periodicWorker) start(parent context.Context, interval time.Duration, pass func(context.Context)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done, wake := make(chan struct{}), make(chan struct{}, 1)
	w.cancel, w.done, w.wake = cancel, done, wake
	go func() {
		defer close(done)
		for {
			if ctx.Err() != nil {
				return
			}
			pass(ctx)
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

func (w *periodicWorker) nudge() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wake != nil {
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}

// stop cancels the worker and waits for its pass to end.
func (w *periodicWorker) stop() {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	w.mu.Lock()
	w.cancel, w.done, w.wake = nil, nil, nil
	w.mu.Unlock()
}

// The service belongs to the app context, never to a screen. Clock timers wake
// after host resume; failed exchanges retry from durable outbox/cursor state.
func (a *App) startDesktopBackground(parent context.Context, interval time.Duration) {
	a.background.start(parent, interval, func(ctx context.Context) {
		a.activityMu.Lock()
		a.reconcileActivityLocked(ctx)
		a.activityMu.Unlock()
		// Foreground sync/configuration already owns the same pipeline. A
		// timer tick must not queue a duplicate exchange behind it.
		if a.backendSyncMu.TryLock() {
			_, _ = a.syncNowLocked(ctx)
			a.backendSyncMu.Unlock()
		}
	})
}

func (a *App) wakeDesktopBackground() {
	a.background.nudge()
}

func (a *App) cancelBackendRun() {
	a.backendRunMu.Lock()
	defer a.backendRunMu.Unlock()
	if a.backendRunCancel != nil {
		a.backendRunCancel()
	}
}

func (a *App) stopDesktopBackground() {
	a.background.stop()
	a.cancelBackendRun()
	// Join any foreground sync before the caller closes clients and storage.
	a.backendSyncMu.Lock()
	a.backendSyncMu.Unlock()
}
