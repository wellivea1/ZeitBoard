package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"non24.app/core/quicklog"
	storage "non24.app/core/storage/sqlite"
)

// One-tap sleep logging (UI guideline finding 7, automaticity review's
// highest-value usability item).
//
// Recording a night costs a four-field form filled in by someone who has just
// woken up. Two taps replace it when the pair is plausible, and ask a single
// question when it is not. The judgement lives in core/quicklog so a companion
// can answer identically; this file is the store and the wording.

const quickLogLocalLayout = "2006-01-02T15:04"

type QuickLogStateDTO struct {
	// Status is ok or unavailable.
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`

	// Pending describes an unfinished sleep, if one is marked.
	Pending      bool   `json:"pending"`
	PendingLabel string `json:"pendingLabel,omitempty"`
	PendingSince string `json:"pendingSince,omitempty"`

	// PendingStale marks an onset old enough that "now" is no longer evidence
	// of the wake time. The button stops offering one-tap closure.
	PendingStale bool `json:"pendingStale"`
}

// QuickLogResultDTO is what a tap produced. Outcome is quicklog's vocabulary,
// so the screen renders a question rather than inventing one.
type QuickLogResultDTO struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`

	// Recorded is set when a night was appended.
	Recorded bool   `json:"recorded"`
	Entry    string `json:"entry,omitempty"`

	// SuggestedStartLocal and SuggestedEndLocal prefill the question. A
	// suggestion drawn from the estimator carries SuggestionIsPrediction, so
	// the screen can say which — a forecast a reader mistakes for a record is
	// exactly the confusion this project exists to avoid.
	SuggestedStartLocal    string `json:"suggestedStartLocal,omitempty"`
	SuggestedEndLocal      string `json:"suggestedEndLocal,omitempty"`
	SuggestionIsPrediction bool   `json:"suggestionIsPrediction"`

	State QuickLogStateDTO `json:"state"`
}

// ConfirmQuickSleepInput closes an unfinished sleep with times a person chose,
// after the app declined to guess them.
type ConfirmQuickSleepInput struct {
	StartLocal     string `json:"startLocal"`
	EndLocal       string `json:"endLocal"`
	ZoneID         string `json:"zoneId"`
	Classification string `json:"classification"`
}

// GetQuickLogState reports whether a sleep is waiting to be closed.
func (a *App) GetQuickLogState() (QuickLogStateDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return QuickLogStateDTO{
			Status:  "unavailable",
			Message: "Quick logging needs the desktop app service.",
		}, nil
	}
	return a.quickLogState(a.applicationContext(), store), nil
}

func (a *App) quickLogState(ctx context.Context, store *storage.Store) QuickLogStateDTO {
	pending, err := store.PendingSleep(ctx)
	if err != nil || pending == nil {
		return QuickLogStateDTO{Status: "ok"}
	}
	elapsed := a.currentTime().UTC().Sub(pending.StartedAt)
	return QuickLogStateDTO{
		Status:       "ok",
		Pending:      true,
		PendingLabel: "Sleep marked " + pending.StartedAt.Local().Format("Mon 3:04 PM"),
		PendingSince: pending.StartedAt.Local().Format(quickLogLocalLayout),
		PendingStale: elapsed >= quicklog.StaleAfter,
	}
}

// BeginQuickSleep marks "I am going to sleep" now.
func (a *App) BeginQuickSleep() (QuickLogResultDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject",
			Reason:  "Quick logging needs the desktop app service.",
			State:   QuickLogStateDTO{Status: "unavailable"},
		}, nil
	}
	ctx := a.applicationContext()
	existing, err := store.PendingSleep(ctx)
	if err != nil {
		return QuickLogResultDTO{}, err
	}

	now := a.currentTime().UTC()
	zoneID := localZoneID()
	var previous *quicklog.Pending
	if existing != nil {
		previous = &quicklog.Pending{StartedAt: existing.StartedAt, ZoneID: existing.ZoneID}
	}
	decision := quicklog.BeginSleep(now, zoneID, previous)

	if err := store.SetPendingSleep(ctx, storage.PendingSleepRecord{
		StartedAt: decision.Pending.StartedAt,
		ZoneID:    decision.Pending.ZoneID,
		MarkedAt:  now,
	}); err != nil {
		return QuickLogResultDTO{}, err
	}
	return QuickLogResultDTO{
		Outcome: "pending",
		Reason:  decision.Reason,
		State:   a.quickLogState(ctx, store),
	}, nil
}

// DiscardQuickSleep drops an unfinished sleep without recording anything. It
// exists because a marked onset the person no longer believes is worse than no
// onset at all: left alone it would attach itself to the next wake tap.
func (a *App) DiscardQuickSleep() (QuickLogResultDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject",
			Reason:  "Quick logging needs the desktop app service.",
			State:   QuickLogStateDTO{Status: "unavailable"},
		}, nil
	}
	ctx := a.applicationContext()
	if err := store.ClearPendingSleep(ctx); err != nil {
		return QuickLogResultDTO{}, err
	}
	return QuickLogResultDTO{
		Outcome: "discarded",
		Reason:  "The unfinished sleep was discarded. Nothing was recorded.",
		State:   a.quickLogState(ctx, store),
	}, nil
}

// CompleteQuickSleep is "I woke up". It records the night when the pair is
// plausible and returns a question when it is not.
func (a *App) CompleteQuickSleep() (QuickLogResultDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject",
			Reason:  "Quick logging needs the desktop app service.",
			State:   QuickLogStateDTO{Status: "unavailable"},
		}, nil
	}
	ctx := a.applicationContext()
	pending, err := store.PendingSleep(ctx)
	if err != nil {
		return QuickLogResultDTO{}, err
	}

	now := a.currentTime().UTC()
	in := quicklog.WakeInput{Now: now}
	if pending != nil {
		in.Pending = &quicklog.Pending{StartedAt: pending.StartedAt, ZoneID: pending.ZoneID}
	} else {
		in.PredictedOnset = a.predictedOnset(ctx, now)
	}
	decision := quicklog.ResolveWake(in)

	result := QuickLogResultDTO{
		Outcome:                string(decision.Outcome),
		Reason:                 decision.Reason,
		SuggestionIsPrediction: decision.SuggestionIsPrediction,
	}
	if !decision.SuggestedStart.IsZero() {
		result.SuggestedStartLocal = decision.SuggestedStart.Local().Format(quickLogLocalLayout)
	}
	result.SuggestedEndLocal = now.Local().Format(quickLogLocalLayout)

	if decision.Outcome != quicklog.OutcomeRecord {
		result.State = a.quickLogState(ctx, store)
		return result, nil
	}

	zoneID := pending.ZoneID
	if strings.TrimSpace(zoneID) == "" {
		zoneID = localZoneID()
	}
	entry, err := a.appendQuickEpisode(decision.Start, decision.End, zoneID, storage.SleepClassificationPrincipal)
	if err != nil {
		result.Outcome = "reject"
		result.Reason = err.Error()
		result.State = a.quickLogState(ctx, store)
		return result, nil
	}
	if err := store.ClearPendingSleep(ctx); err != nil {
		return QuickLogResultDTO{}, err
	}
	result.Recorded = true
	result.Entry = entry
	result.State = a.quickLogState(ctx, store)
	return result, nil
}

// ConfirmQuickSleep records the night a person supplied after the app declined
// to guess it.
func (a *App) ConfirmQuickSleep(input ConfirmQuickSleepInput) (QuickLogResultDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject",
			Reason:  "Quick logging needs the desktop app service.",
			State:   QuickLogStateDTO{Status: "unavailable"},
		}, nil
	}
	ctx := a.applicationContext()

	zoneID := strings.TrimSpace(input.ZoneID)
	if zoneID == "" {
		zoneID = localZoneID()
	}
	classification := storage.SleepClassificationPrincipal
	if input.Classification == storage.SleepClassificationNap {
		classification = storage.SleepClassificationNap
	}

	start, err := parseQuickLocal(input.StartLocal, "sleep start")
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject", Reason: err.Error(), State: a.quickLogState(ctx, store),
		}, nil
	}
	end, err := parseQuickLocal(input.EndLocal, "wake time")
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject", Reason: err.Error(), State: a.quickLogState(ctx, store),
		}, nil
	}

	entry, err := a.appendQuickEpisode(start, end, zoneID, classification)
	if err != nil {
		return QuickLogResultDTO{
			Outcome: "reject", Reason: err.Error(), State: a.quickLogState(ctx, store),
		}, nil
	}
	if err := store.ClearPendingSleep(ctx); err != nil {
		return QuickLogResultDTO{}, err
	}
	return QuickLogResultDTO{
		Outcome:  "record",
		Recorded: true,
		Entry:    entry,
		Reason:   "Recorded.",
		State:    a.quickLogState(ctx, store),
	}, nil
}

// appendQuickEpisode writes the episode through the same append-only path the
// form uses, with provenance that says a person reported it. A tap is still a
// self-report, not an observation by a device, and the estimator weighs the
// difference.
func (a *App) appendQuickEpisode(
	start, end time.Time,
	zoneID string,
	classification string,
) (string, error) {
	interval, err := quicklog.Episode(start, end, zoneID)
	if err != nil {
		return "", errors.New("Those times do not describe a sleep: check that waking comes after falling asleep.")
	}
	entry, err := a.AddSleepEntry(SleepEntryInput{
		StartLocal:     interval.Start.UTC.In(time.Local).Format(quickLogLocalLayout),
		EndLocal:       interval.End.UTC.In(time.Local).Format(quickLogLocalLayout),
		ZoneID:         localZoneID(),
		Classification: classification,
	})
	if err != nil {
		return "", err
	}
	return entry.ObservationID, nil
}

// predictedOnset is the estimator's next predicted sleep start, offered only as
// a labelled prefill. An error or a refusal yields nothing rather than a
// fallback, because a fabricated onset is worse than an empty field.
func (a *App) predictedOnset(ctx context.Context, now time.Time) time.Time {
	state, err := a.localEstimate(ctx, now)
	if err != nil || state.Status != "estimated" {
		return time.Time{}
	}
	if len(state.Estimate.PredictedSleepWindows) == 0 {
		return time.Time{}
	}
	// The most recent predicted onset that is already behind us; a window that
	// has not started yet cannot be when this sleep began.
	var best time.Time
	for _, window := range state.Estimate.PredictedSleepWindows {
		onset := window.Interval.Start.UTC
		if onset.After(now) {
			continue
		}
		if best.IsZero() || onset.After(best) {
			best = onset
		}
	}
	return best
}

func parseQuickLocal(value, field string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.ParseInLocation(quickLogLocalLayout, trimmed, time.Local)
	if err != nil {
		return time.Time{}, errors.New("Choose a " + field + ".")
	}
	if parsed.Format(quickLogLocalLayout) != trimmed {
		return time.Time{}, errors.New("That local time does not exist on that date.")
	}
	return parsed.UTC(), nil
}

func localZoneID() string {
	zone := time.Local.String()
	if zone == "" || zone == "Local" {
		return defaultZoneID
	}
	return zone
}
