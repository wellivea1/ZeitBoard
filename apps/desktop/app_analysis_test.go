package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
	"non24.app/core/recompute"
	storage "non24.app/core/storage/sqlite"
)

type countedLocalEstimator struct{ calls atomic.Int32 }

func (e *countedLocalEstimator) Estimate(ctx context.Context, sessions []domain.SleepSession, at time.Time) (domain.PhaseEstimate, error) {
	e.calls.Add(1)
	return (estimation.RobustEstimator{}).Estimate(ctx, sessions, at)
}

func seedAnalysisEvidence(t *testing.T, app *App, now, recorded time.Time) {
	t.Helper()
	for i := 0; i < 8; i++ {
		start := now.Add(-16 * time.Hour).Add(-time.Duration(7-i) * 25 * time.Hour)
		record := testSyncObservation(newLocalID("synthetic_analysis"), start)
		record.Provenance.RecordedAt = recorded
		if err := app.store.AppendSleepObservation(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
}

func waitAnalysis(t *testing.T, app *App, check func(*storage.SleepAnalysis) bool) *storage.SleepAnalysis {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		input, err := app.store.ReadSleepAnalysisInput(context.Background())
		if err == nil && input.Cached != nil && check(input.Cached) {
			return input.Cached
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("background analysis did not publish expected state")
	return nil
}

func TestBackgroundAnalysisPublishesAndExpiresWithNoViews(t *testing.T) {
	a := newTestApp(t)
	base := time.Date(2026, 9, 8, 12, 0, 1, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(base.UnixNano())
	a.nowFn = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	seedAnalysisEvidence(t, a, base, base.Add(-6*time.Hour).Add(500*time.Millisecond))
	a.startLocalAnalysis()
	t.Cleanup(a.stopLocalAnalysis)
	first := waitAnalysis(t, a, func(v *storage.SleepAnalysis) bool {
		return v.Freshness != nil && v.Freshness.State == freshness.StateCurrent
	})
	if !first.ValidUntil.Equal(base.Add(500 * time.Millisecond)) {
		t.Fatalf("expiry ignored policy: %v", first.ValidUntil)
	}
	clock.Store(base.Add(time.Second).UnixNano())
	aged := waitAnalysis(t, a, func(v *storage.SleepAnalysis) bool {
		return v.Freshness != nil && v.Freshness.State == freshness.StateStale
	})
	if !aged.ChangedAt.After(first.ChangedAt) {
		t.Fatal("expiry did not change materialized content stamp")
	}
	// Foreground consumes what was already produced, without another estimator run.
	counter := &countedLocalEstimator{}
	a.stopLocalAnalysis()
	a.analysisEstimator = counter
	if _, err := a.GetOverview(); err != nil {
		t.Fatal(err)
	}
	if counter.calls.Load() != 0 {
		t.Fatal("foreground ignored the background result")
	}
}

func TestAnalysisSurvivesReopenAndUnchangedRefreshKeepsContentTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-analysis.db")
	base := time.Date(2026, 9, 8, 12, 0, 1, 0, time.UTC)
	open := func() *App {
		store, err := storage.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		a := newAppWithStore(store, nil)
		a.nowFn = func() time.Time { return base }
		a.configDir = filepath.Join(t.TempDir(), "config")
		t.Cleanup(func() { a.stopLocalAnalysis(); store.Close() })
		return a
	}
	a := open()
	seedAnalysisEvidence(t, a, base, base.Add(-time.Hour))
	first, err := a.localEstimate(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	a = open()
	counter := &countedLocalEstimator{}
	a.analysisEstimator = counter
	state, err := a.localEstimate(context.Background(), base)
	if err != nil || counter.calls.Load() != 0 || state.Estimate.ID != first.Estimate.ID {
		t.Fatalf("reopen did not consume valid snapshot: %v", err)
	}
	if err := a.localAnalysisWorker().RunAt(context.Background(), recompute.ReasonHeartbeat, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	input, err := a.store.ReadSleepAnalysisInput(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if input.Cached == nil || !input.Cached.ChangedAt.Equal(first.ChangedAt) || !input.Cached.ComputedAt.After(first.ComputedAt) {
		t.Fatal("housekeeping made unchanged evidence look new")
	}
	// A requested earlier minute cannot consume a future cached analysis.
	if _, err := a.localEstimate(context.Background(), base.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if counter.calls.Load() != 2 {
		t.Fatal("clock regression reused future analysis")
	}
}

type blockedLocalEstimator struct{ entered, release chan struct{} }

func (e blockedLocalEstimator) Estimate(ctx context.Context, sessions []domain.SleepSession, at time.Time) (domain.PhaseEstimate, error) {
	close(e.entered)
	<-e.release
	return (estimation.RobustEstimator{}).Estimate(ctx, sessions, at)
}

func TestErasureDuringAnalysisCannotPublishOrRetainItsFingerprint(t *testing.T) {
	a := newTestApp(t)
	now := time.Date(2026, 9, 8, 12, 0, 1, 0, time.UTC)
	seedAnalysisEvidence(t, a, now, now.Add(-time.Hour))
	block := blockedLocalEstimator{entered: make(chan struct{}), release: make(chan struct{})}
	a.analysisEstimator = block
	done := make(chan error, 1)
	go func() { done <- a.localAnalysisWorker().RunAt(context.Background(), recompute.ReasonEvidence, now) }()
	<-block.entered
	if err := a.store.DeleteAllSleepData(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(block.release)
	if err := <-done; !errors.Is(err, storage.ErrStaleAnalysis) {
		t.Fatalf("erased analysis published: %v", err)
	}
	input, err := a.store.ReadSleepAnalysisInput(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if input.Cached != nil || len(input.Snapshot.EffectiveSessions) != 0 {
		t.Fatal("erased evidence survived in analysis")
	}
	if _, found, err := (storage.SleepAnalysisJournal{Store: a.store}).LastCompleted(context.Background()); err != nil || found {
		t.Fatal("erased fingerprint survived in journal")
	}
}

func TestAnalysisInvalidatesOnCorrectionAndNeverServesAnOldForecast(t *testing.T) {
	a := newTestApp(t)
	now := time.Date(2026, 9, 8, 12, 0, 1, 0, time.UTC)
	seedAnalysisEvidence(t, a, now, now.Add(-time.Hour))
	first, err := a.localEstimate(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.store.ListSleepObservations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	excluded := true
	if err := a.store.AppendSleepCorrection(context.Background(), storage.SleepCorrectionRecord{
		CorrectionID: newLocalID("synthetic_correction"), TargetObservationID: rows[7].ObservationID, CreatedAt: now, Reason: storage.CorrectionReasonUserEdit, Changes: storage.SleepCorrectionChanges{Excluded: &excluded},
	}); err != nil {
		t.Fatal(err)
	}
	input, err := a.store.ReadSleepAnalysisInput(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if input.Cached != nil {
		t.Fatal("correction left old derived data")
	}
	second, err := a.localEstimate(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if second.Estimate.ID == first.Estimate.ID {
		t.Fatal("correction returned stale forecast")
	}
}
