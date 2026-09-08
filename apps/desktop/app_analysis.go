package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/domain"
	"non24.app/core/estimation"
	"non24.app/core/freshness"
	"non24.app/core/recompute"
	storage "non24.app/core/storage/sqlite"
)

// One worker owns both timer-driven refresh and cold/invalidated foreground reads.
// A result is never used as a substitute for the evidence it was computed from.
func (a *App) localAnalysisWorker() *recompute.Worker {
	a.analysisMu.Lock()
	defer a.analysisMu.Unlock()
	if a.analysisWorker == nil {
		a.analysisWorker = &recompute.Worker{
			Orchestrator: recompute.Orchestrator{Analysis: desktopSleepAnalysis{app: a}, Journal: storage.SleepAnalysisJournal{Store: a.store}},
			Schedule:     recompute.Schedule{Heartbeat: time.Minute}, Now: a.currentTime,
			// Detailed failures may contain SQL or private record identifiers.
			// Readers already surface their typed refusal/error; no health log.
			Logf: func(string, ...any) {},
		}
		if a.closing.Load() {
			a.analysisWorker.Close()
		}
	}
	return a.analysisWorker
}

func (a *App) startLocalAnalysis() {
	if _, err := a.requireStore(); err != nil {
		return
	}
	worker := a.localAnalysisWorker()
	a.analysisMu.Lock()
	defer a.analysisMu.Unlock()
	if a.analysisStop != nil {
		return
	}
	a.analysisStop = make(chan struct{})
	a.analysisDone = worker.Start(a.analysisStop)
}

func (a *App) stopLocalAnalysis() {
	a.analysisMu.Lock()
	stop, done, worker := a.analysisStop, a.analysisDone, a.analysisWorker
	a.analysisStop, a.analysisDone = nil, nil
	a.analysisMu.Unlock()
	if stop != nil {
		close(stop)
	}
	if worker != nil {
		worker.Close()
	}
	if done != nil {
		<-done
	}
}

func (a *App) requestLocalAnalysis(reason recompute.Reason) {
	a.analysisMu.Lock()
	worker := a.analysisWorker
	a.analysisMu.Unlock()
	if worker != nil {
		worker.Request(reason)
	}
	a.emitLocalAnalysisUpdate()
}

func (a *App) emitLocalAnalysisUpdate() {
	a.window.mu.Lock()
	ctx, ready := a.window.ctx, a.window.ready
	a.window.mu.Unlock()
	if ready && !a.closing.Load() {
		runtime.EventsEmit(ctx, "zeitboard:analysis-updated")
	}
}

type desktopSleepAnalysis struct{ app *App }

func (d desktopSleepAnalysis) Prepare(ctx context.Context, now time.Time) (recompute.Prepared, error) {
	store, err := d.app.requireStore()
	if err != nil {
		return recompute.Prepared{}, err
	}
	input, err := store.ReadSleepAnalysisInput(ctx)
	if err != nil {
		return recompute.Prepared{}, err
	}
	value, err := d.app.computeSleepAnalysis(ctx, input, now)
	if err != nil {
		return recompute.Prepared{}, err
	}
	return recompute.Prepared{Inputs: recompute.Fingerprint(value.Inputs), Content: recompute.Fingerprint(value.Content), ValidUntil: value.ValidUntil,
		Apply: func(ctx context.Context, stamp recompute.Stamp) error {
			value.ChangedAt = stamp.At
			if err := store.SaveSleepAnalysis(ctx, value); err != nil {
				return err
			}
			d.app.emitLocalAnalysisUpdate()
			return nil
		}}, nil
}

func (a *App) computeSleepAnalysis(ctx context.Context, input storage.SleepAnalysisInput, now time.Time) (storage.SleepAnalysis, error) {
	asOf := now.UTC().Truncate(time.Minute)
	value := storage.SleepAnalysis{Inputs: input.Fingerprint, Algorithm: estimation.AlgorithmVersion, AsOf: asOf, ComputedAt: now.UTC(), ValidUntil: asOf.Add(time.Minute), Status: "empty", Message: "Add your first sleep entry to start a local estimate."}
	sessions := input.Snapshot.EffectiveSessions
	if len(sessions) > 0 {
		estimator := a.analysisEstimator
		if estimator == nil {
			estimator = estimation.RobustEstimator{}
		}
		estimate, err := estimator.Estimate(ctx, sessions, asOf)
		if err != nil {
			var refusal *estimation.EstimationRefusal
			if !errors.As(err, &refusal) {
				return value, err
			}
			value.Status, value.Message, value.Refusal = "refused", refusal.Message, refusal
		} else {
			estimate.CreatedAt = now.UTC()
			value.Status, value.Message, value.Estimate = "estimated", "", estimate
			state := analysisState(value, sessions)
			if latest, ok := latestPrincipalSession(sessions); ok && len(estimate.PredictedSleepWindows) > 0 {
				inputs := desktopFreshnessInputs(state, latest, estimate.PredictedSleepWindows[0].Interval, now)
				assessment := freshness.Default().Assess(inputs)
				value.Freshness = &assessment
				if expiry := freshness.Default().NextChange(inputs); !expiry.IsZero() && expiry.Before(value.ValidUntil) {
					value.ValidUntil = expiry
				}
			}
		}
	}
	var err error
	value.Content, err = analysisContent(value)
	return value, err
}

func analysisContent(value storage.SleepAnalysis) (string, error) {
	// Housekeeping time and IDs derived from it do not constitute new evidence.
	value.Inputs, value.Content = "", ""
	value.AsOf, value.ComputedAt, value.ChangedAt, value.ValidUntil = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	value.Estimate.ID, value.Estimate.AsOf, value.Estimate.CreatedAt = "", domain.ZonedInstant{}, time.Time{}
	clearIDs := func(windows []domain.AvailabilityWindow) []domain.AvailabilityWindow {
		result := append([]domain.AvailabilityWindow(nil), windows...)
		for i := range result {
			result[i].ID, result[i].EstimateID = "", ""
		}
		return result
	}
	value.Estimate.PredictedSleepWindows = clearIDs(value.Estimate.PredictedSleepWindows)
	value.Estimate.PredictedWakingWindows = clearIDs(value.Estimate.PredictedWakingWindows)
	if value.Freshness != nil {
		value.Freshness = &freshness.Assessment{State: value.Freshness.State, Reason: value.Freshness.Reason}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", errors.New("sleep analysis content could not be encoded")
	}
	return string(recompute.Digest([]string{string(data)})), nil
}

func analysisState(value storage.SleepAnalysis, sessions []domain.SleepSession) localEstimateState {
	return localEstimateState{Status: value.Status, Message: value.Message, Sessions: sessions, Estimate: value.Estimate, Refusal: value.Refusal, ComputedAt: value.ComputedAt, ChangedAt: value.ChangedAt}
}

func usableSleepAnalysis(value *storage.SleepAnalysis, now time.Time) bool {
	if value == nil || value.Algorithm != estimation.AlgorithmVersion || !value.AsOf.Equal(now.UTC().Truncate(time.Minute)) || !now.Before(value.ValidUntil) {
		return false
	}
	content, err := analysisContent(*value)
	if err != nil || value.Content != content {
		return false
	}
	switch value.Status {
	case "empty":
		return value.Refusal == nil
	case "refused":
		return value.Refusal != nil
	case "estimated":
		return value.Refusal == nil && len(value.Estimate.PredictedSleepWindows) > 0 && value.Estimate.AlgorithmVersion == estimation.AlgorithmVersion
	default:
		return false
	}
}

func (a *App) localEstimate(ctx context.Context, now time.Time) (localEstimateState, error) {
	store, err := a.requireStore()
	if err != nil {
		return localEstimateState{Status: "unavailable", Message: err.Error()}, nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		input, err := store.ReadSleepAnalysisInput(ctx)
		if err != nil {
			return localEstimateState{}, err
		}
		if usableSleepAnalysis(input.Cached, now) {
			return analysisState(*input.Cached, input.Snapshot.EffectiveSessions), nil
		}
		if attempt == 2 {
			break
		}
		if a.closing.Load() {
			return localEstimateState{}, errors.New("ZeitBoard is quitting; analysis has stopped")
		}
		if err := a.localAnalysisWorker().RunAt(ctx, recompute.ReasonEvidence, now); err != nil && !errors.Is(err, storage.ErrStaleAnalysis) {
			return localEstimateState{}, err
		}
	}
	return localEstimateState{}, storage.ErrStaleAnalysis
}

func analysisUpdatedLabel(state localEstimateState) string {
	if state.ChangedAt.IsZero() {
		return "Local estimate unavailable"
	}
	return "Estimate content changed " + state.ChangedAt.Local().Format("Jan 2, 3:04 PM")
}
