package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"non24.app/core/ingest"
	"non24.app/core/platform/activity"
	storage "non24.app/core/storage/sqlite"
)

type ActivityCollectionInput struct {
	Enabled bool   `json:"enabled"`
	ZoneID  string `json:"zoneId"`
}

type ActivityCollectionDTO struct {
	Enabled     bool   `json:"enabled"`
	Running     bool   `json:"running"`
	Supported   bool   `json:"supported"`
	ZoneID      string `json:"zoneId"`
	RecordCount int    `json:"recordCount"`
	LastError   string `json:"lastError"`
}

type ActivityExportDTO struct {
	FileName    string `json:"fileName"`
	RecordCount int    `json:"recordCount"`
	Saved       bool   `json:"saved"`
}

var saveActivityDataDialog = runtime.SaveFileDialog

func (a *App) activityCollector(zone string) ingest.Collector {
	if a.activitySource != nil {
		return a.activitySource
	}
	return activity.SafeCollector{ZoneID: zone}
}

func (a *App) activityHealthLocked() ingest.ServiceHealth {
	if a.collector == nil {
		return ingest.ServiceHealth{CollectorIDs: []string{}}
	}
	result := a.collector.Health(context.Background())
	if result.LastError != "" {
		result.LastError = "Activity collection stopped because its records could not be saved. Retry collection or check local storage."
	}
	return result
}

func (a *App) startActivityService(ctx context.Context) {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	a.activityStarted = true
	a.activityClosed = false
	a.reconcileActivityLocked(ctx)
}

func (a *App) stopActivityService(ctx context.Context) {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	a.activityStarted = false
	a.activityClosed = true
	if a.collector != nil {
		_ = a.collector.Stop(ctx)
	}
}

func (a *App) reconcileActivityLocked(ctx context.Context) {
	if !a.activityStarted || a.activityClosed {
		return
	}
	store, err := a.requireStore()
	if err != nil {
		if a.collector != nil {
			_ = a.collector.Stop(ctx)
			a.collector = nil
		}
		a.activityError = "Local storage is unavailable; activity collection is stopped."
		return
	}
	pref, err := store.CollectionPreference(ctx, activity.SourceID)
	if err != nil {
		if a.collector != nil {
			_ = a.collector.Stop(ctx)
			a.collector = nil
		}
		a.activityError = "Saved activity consent could not be read; collection is stopped."
		return
	}
	if !pref.Enabled {
		if a.collector != nil {
			_ = a.collector.Stop(ctx)
			a.collector = nil
		}
		return
	}
	capabilities, err := a.activityCollector(pref.ZoneID).Capabilities(ctx)
	if err != nil || (!capabilities.ActiveIdle && !capabilities.SessionState) {
		a.activityError = "Activity collection is unavailable on this platform."
		return
	}
	a.activityError = ""
	if a.collector == nil {
		a.collector = ingest.NewManager(store, a.activityCollector(pref.ZoneID))
	}
	if err := a.collector.Start(ctx); err != nil {
		a.activityError = "Activity collection could not start."
	}
}

func (a *App) GetActivityCollection() (ActivityCollectionDTO, error) {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	return a.activityStatusLocked()
}

func (a *App) activityStatusLocked() (ActivityCollectionDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return ActivityCollectionDTO{}, err
	}
	pref, err := store.CollectionPreference(a.applicationContext(), activity.SourceID)
	if err != nil {
		return ActivityCollectionDTO{}, errors.New("Saved activity consent could not be read; collection is unavailable.")
	}
	if pref.ZoneID == "" {
		pref.ZoneID = localZoneID()
	}
	capabilities, err := a.activityCollector(pref.ZoneID).Capabilities(a.applicationContext())
	if err != nil {
		return ActivityCollectionDTO{}, errors.New("Activity capabilities could not be read.")
	}
	count, err := store.SourceObservationCount(a.applicationContext(), activity.SourceID)
	if err != nil {
		return ActivityCollectionDTO{}, errors.New("Saved activity records could not be read.")
	}
	health := a.activityHealthLocked()
	lastError := a.activityError
	if health.LastError != "" {
		lastError = health.LastError
	}
	return ActivityCollectionDTO{Enabled: pref.Enabled, Running: health.Running,
		Supported: capabilities.ActiveIdle || capabilities.SessionState, ZoneID: pref.ZoneID,
		RecordCount: count, LastError: lastError}, nil
}

func (a *App) SetActivityCollection(input ActivityCollectionInput) (ActivityCollectionDTO, error) {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	if a.activityClosed || a.closing.Load() {
		return ActivityCollectionDTO{}, errors.New("ZeitBoard is quitting; reopen it to change collection.")
	}
	if _, err := time.LoadLocation(input.ZoneID); err != nil || input.ZoneID == "" || input.ZoneID == "Local" {
		return ActivityCollectionDTO{}, errors.New("Choose a valid IANA activity time zone, such as America/New_York.")
	}
	store, err := a.requireStore()
	if err != nil {
		return ActivityCollectionDTO{}, err
	}
	capabilities, err := a.activityCollector(input.ZoneID).Capabilities(a.applicationContext())
	if input.Enabled && (err != nil || (!capabilities.ActiveIdle && !capabilities.SessionState)) {
		return ActivityCollectionDTO{}, errors.New("Activity collection is unavailable on this platform.")
	}
	// Persist consent before starting. Revocation is saved before joining the
	// current collector; its bounded final shutdown record describes the stop.
	if err := store.SetCollectionPreference(a.applicationContext(), activity.SourceID, storage.CollectionPreference{Enabled: input.Enabled, ZoneID: input.ZoneID}); err != nil {
		return ActivityCollectionDTO{}, errors.New("Activity consent could not be saved. Collection settings were not changed.")
	}
	if a.collector != nil {
		_ = a.collector.Stop(a.applicationContext())
		a.collector = nil
	}
	a.activityError = ""
	a.reconcileActivityLocked(a.applicationContext())
	return a.activityStatusLocked()
}

func (a *App) DeleteActivityData(confirmation string) (ActivityCollectionDTO, error) {
	if confirmation != deleteConfirm {
		return ActivityCollectionDTO{}, errors.New("Type DELETE to erase local activity records.")
	}
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	store, err := a.requireStore()
	if err != nil {
		return ActivityCollectionDTO{}, err
	}
	pref, err := store.CollectionPreference(a.applicationContext(), activity.SourceID)
	if err != nil {
		return ActivityCollectionDTO{}, err
	}
	pref.Enabled = false
	if pref.ZoneID == "" {
		pref.ZoneID = localZoneID()
	}
	if err := store.SetCollectionPreference(a.applicationContext(), activity.SourceID, pref); err != nil {
		return ActivityCollectionDTO{}, errors.New("Collection could not be disabled; activity records were not erased.")
	}
	if a.collector != nil {
		_ = a.collector.Stop(a.applicationContext())
		a.collector = nil
	}
	if err := store.DeleteSourceObservations(a.applicationContext(), activity.SourceID); err != nil {
		return ActivityCollectionDTO{}, errors.New("Collection is off, but activity records could not be erased.")
	}
	a.activityError = ""
	return a.activityStatusLocked()
}

func (a *App) SaveActivityDataExport() (ActivityExportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return ActivityExportDTO{}, err
	}
	path, err := saveActivityDataDialog(a.applicationContext(), runtime.SaveDialogOptions{
		Title: "Export local activity records", DefaultFilename: "zeitboard-activity.json", CanCreateDirectories: true,
		Filters: []runtime.FileFilter{{DisplayName: "JSON files (*.json)", Pattern: "*.json"}},
	})
	if err != nil {
		return ActivityExportDTO{}, errors.New("The activity export destination could not be selected.")
	}
	if path == "" {
		return ActivityExportDTO{}, nil
	}
	var count int
	err = writePrivateStreamAtomic(path, func(out io.Writer) error {
		var err error
		count, err = store.WriteSourceObservations(a.applicationContext(), activity.SourceID, out)
		return err
	})
	if err != nil {
		return ActivityExportDTO{}, errors.New("Activity records could not be exported. The destination file was not replaced.")
	}
	return ActivityExportDTO{FileName: filepath.Base(path), RecordCount: count, Saved: true}, nil
}
