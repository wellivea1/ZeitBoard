package main

import (
	"context"
	"errors"
	"time"

	"non24.app/core/platform/activity"
	storage "non24.app/core/storage/sqlite"
)

// Windows' own record of this computer's power (ADR-0060). With desktop
// activity on and this second choice made, ZeitBoard reads when the computer
// slept, woke, shut down and started, including the times ZeitBoard was not
// running, and keeps them with the collector's records.

const (
	powerLogInterval = 30 * time.Minute

	// maxPowerLogRead bounds one read. A laptop logs a few thousand entries a
	// year; a longer log is read over several passes.
	maxPowerLogRead = 20_000
)

type PowerLogInput struct {
	Enabled bool `json:"enabled"`
}

// PowerLogDTO is the choice to read the log, and what has been read.
type PowerLogDTO struct {
	Supported  bool   `json:"supported"`
	Enabled    bool   `json:"enabled"`
	Entries    int    `json:"entries"`
	SinceLabel string `json:"sinceLabel"`
	LastError  string `json:"lastError"`
}

func (a *App) powerLog() activity.PowerLog {
	if a.powerLogSource != nil {
		return a.powerLogSource
	}
	return activity.DefaultPowerLog()
}

func (a *App) startPowerLogReader(parent context.Context) {
	a.powerLogReader.start(parent, powerLogInterval, func(ctx context.Context) {
		a.activityMu.Lock()
		defer a.activityMu.Unlock()
		a.readPowerLogLocked(ctx)
	})
}

// SetPowerLogReading saves the choice to read Windows' power log, and reads
// it at once when it is turned on.
func (a *App) SetPowerLogReading(input PowerLogInput) (ActivityCollectionDTO, error) {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	store, err := a.requireStore()
	if err != nil {
		return ActivityCollectionDTO{}, err
	}
	ctx := a.applicationContext()
	collection, err := store.CollectionPreference(ctx, activity.SourceID)
	if err != nil {
		return ActivityCollectionDTO{}, errors.New("Saved activity consent could not be read. Nothing was changed.")
	}
	if input.Enabled && !collection.Enabled {
		return ActivityCollectionDTO{}, errors.New("Turn on desktop activity before reading Windows' power log.")
	}
	if input.Enabled && a.powerLog() == nil {
		return ActivityCollectionDTO{}, errors.New("This computer keeps no power log ZeitBoard can read.")
	}
	zone := collection.ZoneID
	if zone == "" {
		zone = localZoneID()
	}
	if err := store.SetCollectionPreference(ctx, activity.PowerLogSourceID, storage.CollectionPreference{Enabled: input.Enabled, ZoneID: zone}); err != nil {
		return ActivityCollectionDTO{}, errors.New("The choice could not be saved. Nothing was changed.")
	}
	a.powerLogError = ""
	if input.Enabled {
		a.readPowerLogLocked(ctx)
	}
	return a.activityStatusLocked()
}

// readPowerLogLocked keeps the power entries Windows logged since the newest
// one kept, reading as far back as the log goes the first time. It reads only
// while desktop activity and this choice are both on.
func (a *App) readPowerLogLocked(ctx context.Context) {
	log := a.powerLog()
	store, err := a.requireStore()
	if log == nil || err != nil || a.activityClosed {
		return
	}
	collection, err := store.CollectionPreference(ctx, activity.SourceID)
	if err != nil || !collection.Enabled {
		return
	}
	reading, err := store.CollectionPreference(ctx, activity.PowerLogSourceID)
	if err != nil || !reading.Enabled {
		return
	}
	kept, err := store.SourceObservationSpan(ctx, activity.SourceID, activity.PowerLogVersion)
	if err != nil {
		a.powerLogError = "What was read from Windows' power log could not be checked. ZeitBoard tries again in half an hour."
		return
	}
	since := kept.Latest
	if !since.IsZero() {
		// An entry can be written a moment after the time it gives. Reading
		// the last hour again misses none, and keeps none twice.
		since = since.Add(-time.Hour)
	}
	events, err := log.After(since, maxPowerLogRead)
	if err != nil {
		a.powerLogError = "Windows' power log could not be read. ZeitBoard tries again in half an hour."
		return
	}
	observations, err := activity.PowerObservations(events, collection.ZoneID, a.currentTime())
	if err == nil {
		_, err = store.AppendNew(ctx, observations)
	}
	if err != nil {
		a.powerLogError = "What Windows logged could not be saved. ZeitBoard tries again in half an hour."
		return
	}
	a.powerLogError = ""
}

func (a *App) powerLogStatusLocked(ctx context.Context, store *storage.Store) PowerLogDTO {
	status := PowerLogDTO{Supported: a.powerLog() != nil, LastError: a.powerLogError}
	if reading, err := store.CollectionPreference(ctx, activity.PowerLogSourceID); err == nil {
		status.Enabled = reading.Enabled
	}
	if kept, err := store.SourceObservationSpan(ctx, activity.SourceID, activity.PowerLogVersion); err == nil && kept.Count > 0 {
		status.Entries = kept.Count
		status.SinceLabel = kept.Oldest.In(displayLocation()).Format("Jan 2, 2006")
	}
	return status
}
