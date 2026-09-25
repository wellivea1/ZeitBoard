package recompute_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"non24.app/core/recompute"
)

type analysisFunc func(context.Context, time.Time) (recompute.Prepared, error)

func (f analysisFunc) Prepare(ctx context.Context, at time.Time) (recompute.Prepared, error) {
	return f(ctx, at)
}

func awaitWorker(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not finish")
	}
}

func TestWorkerCloseCancelsForegroundAndRejectsFurtherWork(t *testing.T) {
	entered := make(chan struct{})
	orchestrator, _ := newOrchestrator(analysisFunc(func(ctx context.Context, _ time.Time) (recompute.Prepared, error) {
		close(entered)
		<-ctx.Done()
		return recompute.Prepared{}, ctx.Err()
	}))
	worker := &recompute.Worker{Orchestrator: orchestrator}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if err := worker.RunNow(context.Background(), recompute.ReasonEvidence); !errors.Is(err, context.Canceled) {
			t.Errorf("foreground result: %v", err)
		}
	}()
	awaitWorker(t, entered)
	worker.Close()
	awaitWorker(t, finished)
	if err := worker.RunNow(context.Background(), recompute.ReasonEvidence); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed worker accepted work: %v", err)
	}
	awaitWorker(t, worker.Start(make(chan struct{})))
	worker.Close()
}

type observedRecoveryJournal struct {
	memoryJournal
	entered chan struct{}
}

func (j *observedRecoveryJournal) MarkInterrupted(ctx context.Context, at time.Time) (int, error) {
	close(j.entered)
	return j.memoryJournal.MarkInterrupted(ctx, at)
}

func TestWorkerRecoveryCannotInterruptAnActiveForegroundPublish(t *testing.T) {
	applying, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	journal := &observedRecoveryJournal{entered: make(chan struct{})}
	worker := &recompute.Worker{Orchestrator: recompute.Orchestrator{Journal: journal, Analysis: analysisFunc(func(context.Context, time.Time) (recompute.Prepared, error) {
		return recompute.Prepared{Inputs: "synthetic", Content: "result", Apply: func(context.Context, recompute.Stamp) error {
			if calls.Add(1) == 1 {
				close(applying)
				<-release
			}
			return nil
		}}, nil
	})}, Logf: func(string, ...any) {}}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if err := worker.RunNow(context.Background(), recompute.ReasonEvidence); err != nil {
			t.Error(err)
		}
	}()
	awaitWorker(t, applying)
	stop := make(chan struct{})
	done := worker.Start(stop)
	if worker.Start(stop) != done {
		t.Fatal("duplicate Start did not return the active completion signal")
	}
	select {
	case <-journal.entered:
		t.Error("recovery overlapped the active publication")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	awaitWorker(t, finished)
	awaitWorker(t, journal.entered)
	worker.Close()
	awaitWorker(t, done)
	if len(journal.runs) == 0 || journal.runs[0].State != recompute.StateDone {
		t.Fatal("foreground publication was interrupted by recovery")
	}
}

func TestForegroundExpiryReschedulesSleepingWorker(t *testing.T) {
	runs := make(chan struct{}, 8)
	var calls atomic.Int32
	orchestrator, _ := newOrchestrator(analysisFunc(func(_ context.Context, at time.Time) (recompute.Prepared, error) {
		expiry := at.Add(time.Hour)
		if calls.Add(1) == 2 {
			expiry = at.Add(50 * time.Millisecond)
		}
		return recompute.Prepared{Inputs: "synthetic", Content: "result", ValidUntil: expiry, Apply: func(context.Context, recompute.Stamp) error { runs <- struct{}{}; return nil }}, nil
	}))
	worker := &recompute.Worker{Orchestrator: orchestrator}
	defer worker.Close()
	worker.Start(make(chan struct{}))
	awaitWorker(t, runs)
	if err := worker.RunNow(context.Background(), recompute.ReasonEvidence); err != nil {
		t.Fatal(err)
	}
	awaitWorker(t, runs)
	awaitWorker(t, runs)
}
