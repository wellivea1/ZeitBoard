package main

import (
	"context"
	"time"

	storage "non24.app/core/storage/sqlite"
)

type SleepImportInput struct {
	FileName string `json:"fileName"`
	Contents string `json:"contents"`
}

type SleepImportDTO struct {
	FileName      string              `json:"fileName"`
	Format        string              `json:"format"`
	DryRun        bool                `json:"dryRun"`
	TotalRows     int                 `json:"totalRows"`
	ReadyRows     int                 `json:"readyRows"`
	DuplicateRows int                 `json:"duplicateRows"`
	InvalidRows   int                 `json:"invalidRows"`
	ImportedRows  int                 `json:"importedRows"`
	CanImport     bool                `json:"canImport"`
	Errors        []string            `json:"errors"`
	Rows          []SleepImportRowDTO `json:"rows"`
	Message       string              `json:"message"`
}

type SleepImportRowDTO struct {
	RowNumber      int      `json:"rowNumber"`
	ObservationID  string   `json:"observationId,omitempty"`
	SourceRecordID string   `json:"sourceRecordId,omitempty"`
	StartLabel     string   `json:"startLabel,omitempty"`
	EndLabel       string   `json:"endLabel,omitempty"`
	ZoneID         string   `json:"zoneId,omitempty"`
	Classification string   `json:"classification,omitempty"`
	Status         string   `json:"status"`
	Errors         []string `json:"errors"`
	StatusDetail   string   `json:"statusDetail,omitempty"`
}

func (a *App) PreviewSleepImport(input SleepImportInput) (SleepImportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepImportDTO{}, err
	}
	report, err := store.PreviewSleepImport(context.Background(), storage.SleepImportInput{
		FileName: input.FileName,
		Contents: input.Contents,
	})
	if err != nil {
		return SleepImportDTO{}, err
	}
	return sleepImportDTO(report), nil
}

func (a *App) ImportSleepData(input SleepImportInput) (SleepImportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepImportDTO{}, err
	}
	report, err := store.ImportSleepObservations(context.Background(), storage.SleepImportInput{
		FileName: input.FileName,
		Contents: input.Contents,
	})
	if err != nil {
		return SleepImportDTO{}, err
	}
	return sleepImportDTO(report), nil
}

func sleepImportDTO(report storage.SleepImportReport) SleepImportDTO {
	rows := make([]SleepImportRowDTO, 0, len(report.Rows))
	for _, row := range report.Rows {
		rows = append(rows, SleepImportRowDTO{
			RowNumber:      row.RowNumber,
			ObservationID:  row.Observation.ObservationID,
			SourceRecordID: row.Observation.Provenance.SourceRecordID,
			StartLabel:     importTimeLabel(row.Observation.StartAt, row.Observation.ZoneID),
			EndLabel:       importTimeLabel(row.Observation.EndAt, row.Observation.ZoneID),
			ZoneID:         row.Observation.ZoneID,
			Classification: row.Observation.Sleep.Classification,
			Status:         row.Status,
			Errors:         nonNilStrings(row.Errors),
			StatusDetail:   row.StatusDetail,
		})
	}
	return SleepImportDTO{
		FileName:      report.FileName,
		Format:        report.Format,
		DryRun:        report.DryRun,
		TotalRows:     report.TotalRows,
		ReadyRows:     report.ReadyRows,
		DuplicateRows: report.DuplicateRows,
		InvalidRows:   report.InvalidRows,
		ImportedRows:  report.ImportedRows,
		CanImport:     report.CanImport,
		Errors:        nonNilStrings(report.Errors),
		Rows:          rows,
		Message:       report.Message,
	}
}

func importTimeLabel(value time.Time, zoneID string) string {
	if value.IsZero() {
		return ""
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		location = time.UTC
	}
	return value.In(location).Format("Jan 2, 2006, 3:04 PM")
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}
