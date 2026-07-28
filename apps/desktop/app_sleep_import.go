package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	storage "non24.app/core/storage/sqlite"
)

var openSleepDataDialog = runtime.OpenFileDialog

type SleepImportInput struct {
	FileName string `json:"fileName"`
	Contents string `json:"contents"`
}

type SleepImportFileInput struct {
	ImportToken string `json:"importToken"`
}

type pendingSleepImportFile struct {
	path   string
	digest [sha256.Size]byte
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
	ImportToken   string              `json:"importToken,omitempty"`
	Canceled      bool                `json:"canceled"`
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
	report, err := store.PreviewSleepImport(a.applicationContext(), storage.SleepImportInput{
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
	report, err := store.ImportSleepObservations(a.applicationContext(), storage.SleepImportInput{
		FileName: input.FileName,
		Contents: input.Contents,
	})
	if err != nil {
		return SleepImportDTO{}, err
	}
	return sleepImportDTO(report), nil
}

// PreviewSleepImportFile keeps the selected file contents on the Go side of
// the Wails bridge. The returned token names one immutable preview selection.
func (a *App) PreviewSleepImportFile() (SleepImportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepImportDTO{}, err
	}
	a.clearPendingSleepImports()
	path, err := openSleepDataDialog(a.applicationContext(), runtime.OpenDialogOptions{
		Title: "Import sleep observations",
		Filters: []runtime.FileFilter{
			{DisplayName: "Sleep observation files (*.json;*.csv)", Pattern: "*.json;*.csv"},
			{DisplayName: "JSON files (*.json)", Pattern: "*.json"},
			{DisplayName: "CSV files (*.csv)", Pattern: "*.csv"},
		},
	})
	if err != nil {
		return SleepImportDTO{}, fmt.Errorf("select sleep import file: %w", err)
	}
	if path == "" {
		return canceledSleepImportDTO(), nil
	}

	selected, digest, err := readSleepImportFile(path)
	if err != nil {
		return SleepImportDTO{}, err
	}
	report, err := store.PreviewSleepImport(a.applicationContext(), selected)
	if err != nil {
		return SleepImportDTO{}, err
	}
	token := newLocalID("sleep_import")
	a.sleepImportMu.Lock()
	a.sleepImportPending = map[string]pendingSleepImportFile{
		token: {path: path, digest: digest},
	}
	a.sleepImportMu.Unlock()

	result := sleepImportDTO(report)
	result.ImportToken = token
	return result, nil
}

// ImportSleepDataFile consumes a preview token and rejects a file that changed
// after preview, so the committed bytes are exactly the bytes the owner saw.
func (a *App) ImportSleepDataFile(input SleepImportFileInput) (SleepImportDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return SleepImportDTO{}, err
	}
	pending, ok := a.consumePendingSleepImport(input.ImportToken)
	if !ok {
		return SleepImportDTO{}, fmt.Errorf("sleep import preview expired; choose the file again")
	}
	selected, digest, err := readSleepImportFile(pending.path)
	if err != nil {
		return SleepImportDTO{}, err
	}
	if digest != pending.digest {
		return SleepImportDTO{}, fmt.Errorf("sleep import file changed after preview; choose it again")
	}
	report, err := store.ImportSleepObservations(a.applicationContext(), selected)
	if err != nil {
		return SleepImportDTO{}, err
	}
	return sleepImportDTO(report), nil
}

func readSleepImportFile(path string) (storage.SleepImportInput, [sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return storage.SleepImportInput{}, [sha256.Size]byte{}, fmt.Errorf("open sleep import: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return storage.SleepImportInput{}, [sha256.Size]byte{}, fmt.Errorf("inspect sleep import: %w", err)
	}
	if !info.Mode().IsRegular() {
		return storage.SleepImportInput{}, [sha256.Size]byte{}, fmt.Errorf("sleep import must be a regular file")
	}
	if info.Size() > storage.MaxSleepImportBytes {
		return storage.SleepImportInput{}, [sha256.Size]byte{}, fmt.Errorf("sleep import exceeds the 8 MiB limit")
	}
	contents, err := io.ReadAll(io.LimitReader(file, storage.MaxSleepImportBytes+1))
	if err != nil {
		return storage.SleepImportInput{}, [sha256.Size]byte{}, fmt.Errorf("read sleep import: %w", err)
	}
	if len(contents) > storage.MaxSleepImportBytes {
		return storage.SleepImportInput{}, [sha256.Size]byte{}, fmt.Errorf("sleep import exceeds the 8 MiB limit")
	}
	return storage.SleepImportInput{
		FileName: filepath.Base(path),
		Contents: string(contents),
	}, sha256.Sum256(contents), nil
}

func (a *App) consumePendingSleepImport(token string) (pendingSleepImportFile, bool) {
	a.sleepImportMu.Lock()
	defer a.sleepImportMu.Unlock()
	pending, ok := a.sleepImportPending[token]
	if ok {
		delete(a.sleepImportPending, token)
	}
	return pending, ok
}

func (a *App) clearPendingSleepImports() {
	a.sleepImportMu.Lock()
	a.sleepImportPending = nil
	a.sleepImportMu.Unlock()
}

func canceledSleepImportDTO() SleepImportDTO {
	return SleepImportDTO{
		FileName: "No file selected",
		Format:   "",
		DryRun:   true,
		Errors:   []string{},
		Rows:     []SleepImportRowDTO{},
		Message:  "No file was selected.",
		Canceled: true,
	}
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
