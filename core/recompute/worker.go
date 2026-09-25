package recompute

import (
	"context"
	"log"
	"sync"
	"time"
)

// Worker is the background half of the analysis loop: the goroutine, the timer,
// and the mutual exclusion. Every decision it makes about *when* comes from
// Schedule, which has neither, so the policy stays testable without
// waiting for wall-clock time to pass.
//
// It needs no open window, no user, and no LLM. It exists so that the honest
// answer changes when the truth does rather than when somebody happens to look.
type Worker struct {
	Orchestrator Orchestrator
	Schedule     Schedule
	Now          func() time.Time
	Logf         func(format string, args ...any)

	// running serialises runs. A synchronous RunNow and a scheduled run must
	// not overlap: they would race on the last-completed row and could publish
	// two different stamps for the same content.
	running sync.Mutex

	mu      sync.Mutex
	wake    chan struct{}
	started bool
	done    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

// lifetimeLocked is shared by scheduled and foreground work. Call with mu held.
func (w *Worker) lifetimeLocked() context.Context {
	if w.ctx == nil {
		w.ctx, w.cancel = context.WithCancel(context.Background())
	}
	return w.ctx
}

// Close cancels and joins all work, including synchronous readers, before their
// owner closes storage. A closed worker cannot be restarted.
func (w *Worker) Close() {
	w.mu.Lock()
	w.lifetimeLocked()
	w.closed = true
	w.cancel()
	done := w.done
	w.mu.Unlock()
	if done != nil {
		<-done
	}
	w.running.Lock()
	w.running.Unlock()
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *Worker) logf(format string, args ...any) {
	if w.Logf != nil {
		w.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// Request records that something changed and lets the schedule decide when to
// act. It never blocks and never fails: a lost request would be recoverable
// anyway, because the next run derives the need from the inputs rather than
// from having been asked.
func (w *Worker) Request(reason Reason) {
	w.mu.Lock()
	w.Schedule.Request(reason, w.now())
	wake := w.wake
	w.mu.Unlock()

	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

// RunNow recomputes immediately and waits for it. It is for the paths where a
// caller must not return until the projection exists — creating a share link,
// where the recipient would otherwise open a blank page.
func (w *Worker) RunNow(ctx context.Context, reason Reason) error {
	return w.RunAt(ctx, reason, w.now())
}

// RunAt uses the same serialized pipeline for a reader's explicit as-of instant.
func (w *Worker) RunAt(ctx context.Context, reason Reason, now time.Time) error {
	w.mu.Lock()
	lifetime := w.lifetimeLocked()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return context.Canceled
	}
	ctx, cancel := context.WithCancel(ctx)
	stopCancel := context.AfterFunc(lifetime, cancel)
	defer func() { stopCancel(); cancel() }()
	w.running.Lock()
	defer w.running.Unlock()
	if err := lifetime.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	outcome, err := w.Orchestrator.Run(ctx, reason, now)
	w.mu.Lock()
	if err == nil {
		w.Schedule.Succeeded(now)
		w.Schedule.Expires(outcome.ValidUntil)
	} else if ctx.Err() == nil {
		delay := w.Schedule.Failed(now)
		w.logf("analysis: recompute (%s) failed, retrying in %s: %v", reason, delay.Round(time.Second), err)
	}
	wake := w.wake
	w.mu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
	return err
}

// Start runs the loop until stop is closed or Close is called. The returned
// channel joins the background goroutine. Close also joins foreground work and
// must be called before closing shared storage.
func (w *Worker) Start(stop <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})

	w.mu.Lock()
	if w.closed {
		close(done)
		w.mu.Unlock()
		return done
	}
	if w.started {
		done = w.done
		w.mu.Unlock()
		return done
	}
	w.started = true
	w.done = done
	w.wake = make(chan struct{}, 1)
	wake := w.wake
	lifetime := w.lifetimeLocked()
	w.mu.Unlock()

	go func() {
		defer func() { w.mu.Lock(); w.started = false; close(done); w.mu.Unlock() }()

		ctx, cancel := context.WithCancel(lifetime)
		defer cancel()
		go func() {
			select {
			case <-stop:
				cancel()
			case <-ctx.Done():
			}
		}()

		// A run left marked running belongs to a process that died. Record that
		// and then reconcile: the startup pass below decides what still needs
		// doing by comparing fingerprints, not by trusting the journal.
		w.running.Lock()
		interrupted, err := w.Orchestrator.Recover(ctx, w.now())
		w.running.Unlock()
		if err != nil {
			w.logf("analysis: recovering the recompute journal failed: %v", err)
		} else if interrupted > 0 {
			w.logf("analysis: %d recompute run(s) were interrupted by a restart", interrupted)
		}
		w.runOnce(ctx, ReasonStartup)

		for {
			w.mu.Lock()
			next, scheduled := w.Schedule.NextWake(w.now())
			w.mu.Unlock()

			// A fresh timer per iteration rather than Reset on a shared one:
			// the sleep length is decided anew each time and the Reset dance
			// around an already-fired channel is a well-known way to hang.
			var timeout <-chan time.Time
			var timer *time.Timer
			if scheduled {
				delay := next.Sub(w.now())
				if delay < 0 {
					delay = 0
				}
				timer = time.NewTimer(delay)
				timeout = timer.C
			}

			select {
			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return
			case <-stop:
				if timer != nil {
					timer.Stop()
				}
				return
			case <-wake:
			case <-timeout:
			}
			if timer != nil {
				timer.Stop()
			}

			w.mu.Lock()
			claim, due := w.Schedule.Begin(w.now())
			w.mu.Unlock()
			if !due {
				continue
			}
			if claim.Coalesced > 0 {
				w.logf("analysis: recompute (%s) absorbed %d further request(s)", claim.Reason, claim.Coalesced)
			}
			w.runOnce(ctx, claim.Reason)
		}
	}()

	return done
}

// runOnce performs one recomputation and folds the result back into the
// schedule. A failure is not lost work: the schedule keeps it pending and backs
// off, and even a permanently lost request would be recovered by the next run's
// fingerprint comparison.
func (w *Worker) runOnce(ctx context.Context, reason Reason) {
	_ = w.RunNow(ctx, reason)
}
