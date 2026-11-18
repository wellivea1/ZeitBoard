package main

import (
	"context"
	"errors"
	"path/filepath"

	"non24.app/core/sharedsetting"
	storage "non24.app/core/storage/sqlite"
)

// Choices that travel between the owner's devices (ADR-0074): whether a
// status may show on share links by default, whether the sleep buttons also
// report one, and the clock. A change made here is kept as a record and
// uploaded with the next sync; a newer one made on another device is followed.
// status.json and regional.json hold the values in use, and follow the
// record that stands.

// settingsFollowedAnnouncement tells the window that a choice made on another
// device is now in use here, so the views that show it read it again.
const settingsFollowedAnnouncement = "zeitboard:settings-followed"

// recordSharedSetting keeps a choice the owner made on this computer, dated
// after the one that stands so it is the newest wherever it arrives. A choice
// that is already the one standing is not recorded again.
func (a *App) recordSharedSetting(ctx context.Context, name string, value sharedsetting.Value) error {
	store, err := a.requireStore()
	if err != nil {
		// No store, no other device to tell: the choice still applies here.
		return nil
	}
	current, err := store.CurrentSettings(ctx)
	if err != nil {
		return err
	}
	var standing *sharedsetting.Record
	if record, ok := current[name]; ok {
		if record.Value.Equal(value) {
			return nil
		}
		standing = &record
	}
	record := sharedsetting.Record{
		SettingID: newLocalID("setting"),
		Name:      name,
		Value:     value,
		ChangedAt: sharedsetting.NextChangeAt(a.currentTime(), standing),
	}
	if err := store.AddSettingRecord(ctx, record); err != nil {
		return err
	}
	a.wakeDesktopBackground()
	return nil
}

// followSharedSettings puts in use here the choices that stand, wherever they
// were made. It reports whether anything changed, and tells the window when
// something did.
func (a *App) followSharedSettings(ctx context.Context) (bool, error) {
	store, err := a.requireStore()
	if err != nil {
		return false, nil
	}
	statusChanged, statusErr := a.followStatusSettings(ctx, store)
	clockChanged, clockErr := a.followClockSetting(ctx, store)
	if statusChanged || clockChanged {
		a.announce(settingsFollowedAnnouncement)
	}
	return statusChanged || clockChanged, errors.Join(statusErr, clockErr)
}

// followStatusSettings reads what stands under the lock a save holds while it
// records, so a choice just made here is never undone by an older one.
func (a *App) followStatusSettings(ctx context.Context, store *storage.Store) (bool, error) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	current, err := store.CurrentSettings(ctx)
	if err != nil {
		return false, err
	}
	state, revision := a.loadStatusSettings()
	next := state
	if record, ok := current[sharedsetting.ShareStatusByDefault]; ok {
		next.ShareByDefault, _ = record.Value.Bool()
	}
	if record, ok := current[sharedsetting.FollowSleepTaps]; ok {
		next.FollowSleepTaps, _ = record.Value.Bool()
	}
	if next == state {
		return false, nil
	}
	if a.configDir != "" {
		if err := statusSettingsStore(filepath.Join(a.configDir, statusSettingsFileName)).write(next, revision+1); err != nil {
			return false, err
		}
	} else {
		a.statusSettings, a.statusRevision = next, revision+1
	}
	return true, nil
}

func (a *App) followClockSetting(ctx context.Context, store *storage.Store) (bool, error) {
	a.regionalMu.Lock()
	defer a.regionalMu.Unlock()
	current, err := store.CurrentSettings(ctx)
	if err != nil {
		return false, err
	}
	record, ok := current[sharedsetting.Clock]
	if !ok {
		return false, nil
	}
	preference, _ := record.Value.Text()
	if a.regional.Clock == preference {
		return false, nil
	}
	state := RegionalSettingsDTO{Clock: preference}
	next := a.regionalRevision + 1
	if a.configDir != "" {
		if err := regionalStore(filepath.Join(a.configDir, regionalFileName)).write(state, next); err != nil {
			return false, err
		}
	}
	a.regional = state
	a.regionalRevision = next
	a.regionalErr = ""
	a.applyClockLocked()
	return true, nil
}

// sharedStatusChanges records the status choices that differ from what is in
// use, before the new ones are stored.
func (a *App) sharedStatusChanges(ctx context.Context, from, to StatusSettingsDTO) error {
	if from.ShareByDefault != to.ShareByDefault {
		if err := a.recordSharedSetting(ctx, sharedsetting.ShareStatusByDefault, sharedsetting.On(to.ShareByDefault)); err != nil {
			return err
		}
	}
	if from.FollowSleepTaps != to.FollowSleepTaps {
		return a.recordSharedSetting(ctx, sharedsetting.FollowSleepTaps, sharedsetting.On(to.FollowSleepTaps))
	}
	return nil
}

// pushSettingRecords sends the choices the owner made here (ADR-0074).
func (a *App) pushSettingRecords(ctx context.Context, store *storage.Store, client desktopBackendClient) (int, error) {
	return pushPendingSyncRecords(ctx, client, storage.MaxSettingSyncPageSize,
		store.PendingSettingSyncRecords, store.MarkSettingSyncRecordsPushed,
		func(record storage.SettingSyncRecord) syncPushRecord {
			return syncPushRecord{RecordID: record.RecordID, Kind: storage.SyncKindSetting, CreatedAt: record.CreatedAt.UTC(), Payload: record.Payload}
		})
}
