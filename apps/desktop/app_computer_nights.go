package main

import (
	"errors"
	"fmt"
	"time"

	"non24.app/core/inference"
	"non24.app/core/platform/activity"
)

// Nights this computer suggests (ADR-0060): the quiet stretches in its
// activity records, the collector's and those read from Windows' power log,
// beside the recorded nights. Like a Google export's (ADR-0059), they are
// shadow evidence that nothing else reads.

// ComputerNightsDTO is whether this computer's activity has been kept, and
// once it has, the nights it suggests.
type ComputerNightsDTO struct {
	Collected bool             `json:"collected"`
	Figure    *NightsFigureDTO `json:"figure,omitempty"`
}

func (a *App) GetComputerNights() (ComputerNightsDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return ComputerNightsDTO{}, err
	}
	ctx := a.applicationContext()
	a.computerNightsMu.Lock()
	defer a.computerNightsMu.Unlock()
	span, err := store.SourceObservationSpan(ctx, activity.SourceID, "")
	if err != nil {
		return ComputerNightsDTO{}, errors.New("This computer's activity could not be read.")
	}
	if span.Count == 0 {
		return ComputerNightsDTO{}, nil
	}
	key := fmt.Sprintf("%d %s", span.Count, span.Latest.Format(time.RFC3339Nano))
	if a.computerNights.key != key {
		observations, err := store.SourceObservations(ctx, activity.SourceID)
		if err != nil {
			return ComputerNightsDTO{}, errors.New("This computer's activity could not be read.")
		}
		transitions := activity.Transitions(observations)
		if len(transitions) == 0 {
			return ComputerNightsDTO{}, nil
		}
		a.computerNights = cachedNights{
			key: key, count: len(transitions),
			first: transitions[0].At, last: transitions[len(transitions)-1].At,
			nights: suggestNights(inference.FromActivity(transitions)),
		}
	}
	sessions, err := store.EffectiveSleepSessions(ctx)
	if err != nil {
		return ComputerNightsDTO{}, errors.New("Recorded nights could not be read.")
	}
	nights := a.computerNights
	return ComputerNightsDTO{
		Collected: true,
		Figure:    nightsFigure(nights.nights, sessions, nights.first, nights.last, displayLocation()),
	}, nil
}
