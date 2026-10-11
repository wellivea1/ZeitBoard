package main

import (
	"context"
	"time"

	"non24.app/core/sources"
	storage "non24.app/core/storage/sqlite"
)

// What a phone found when it last checked Health Connect (ADR-0054), for the
// views that judge the data: Data Sources, Home and Week, and the assistant
// snapshot. The desktop reads no source itself; every status comes from
// reports a phone sent through the owner's server. The window words them, as
// it words every time.

type SourceStatusDTO struct {
	Source          string `json:"source"`
	Outcome         string `json:"outcome"`
	ReportedAt      string `json:"reportedAt"`
	Since           string `json:"since"`
	ReadThrough     string `json:"readThrough,omitempty"`
	BackgroundReads bool   `json:"backgroundReads"`
	Held            int    `json:"held"`
	// NewestRecordEnd is when the newest sleep record from the source ends,
	// as the source recorded it.
	NewestRecordEnd string `json:"newestRecordEnd,omitempty"`
}

// The acquisition method of the sleep records each reporting source sends.
var reportingSourceAcquisition = map[string]string{
	sources.HealthConnect: storage.ProvenanceAcquisitionHealthConnect,
}

// GetSourceStatuses says what each reporting source's newest reports say.
func (a *App) GetSourceStatuses() ([]SourceStatusDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return nil, err
	}
	statuses, err := sourceStatuses(a.applicationContext(), store)
	if err != nil {
		return nil, err
	}
	dtos := make([]SourceStatusDTO, 0, len(statuses))
	for _, status := range statuses {
		dtos = append(dtos, SourceStatusDTO{
			Source: status.Source, Outcome: status.Outcome,
			ReportedAt: rfc3339OrEmpty(status.ReportedAt), Since: rfc3339OrEmpty(status.Since),
			ReadThrough: rfc3339OrEmpty(status.ReadThrough), BackgroundReads: status.BackgroundReads,
			Held: status.Held, NewestRecordEnd: rfc3339OrEmpty(status.newestRecordEnd),
		})
	}
	return dtos, nil
}

// sourceStatus is a source's status and when its newest sleep record ends.
type sourceStatus struct {
	sources.Status
	newestRecordEnd time.Time
}

func sourceStatuses(ctx context.Context, store *storage.Store) ([]sourceStatus, error) {
	received, err := store.SourceReports(ctx)
	if err != nil {
		return nil, err
	}
	var statuses []sourceStatus
	for _, status := range sources.Statuses(received) {
		newest, err := store.NewestSleepEndFrom(ctx, reportingSourceAcquisition[status.Source])
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, sourceStatus{Status: status, newestRecordEnd: newest})
	}
	return statuses, nil
}
