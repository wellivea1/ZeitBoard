package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestSleepImportBindingPreviewsThenCommitsContractData(t *testing.T) {
	app := newTestApp(t)
	input := SleepImportInput{
		FileName: "owner-history.json",
		Contents: `{
  "schema_version":"v1",
  "generated_at":"2024-01-01T00:00:00Z",
  "observations":[{
    "observation_id":"obs_owner_001",
    "kind":"sleep_episode",
    "start_at":"2023-01-01T05:00:00Z",
    "end_at":"2023-01-01T13:00:00Z",
    "zone_id":"America/New_York",
    "sleep":{"classification":"principal"},
    "provenance":{
      "acquisition_method":"file_import",
      "evidence_status":"directly_observed",
      "recorded_at":"2023-01-01T13:00:00Z",
      "source_record_id":"fitbit-owner-001"
    }
  }]
}`,
	}

	preview, err := app.PreviewSleepImport(input)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || !preview.CanImport || preview.ReadyRows != 1 || len(preview.Rows) != 1 {
		t.Fatalf("unexpected binding preview: %#v", preview)
	}
	if preview.Rows[0].StartLabel == "" || preview.Rows[0].SourceRecordID != "fitbit-owner-001" {
		t.Fatalf("preview omitted row provenance: %#v", preview.Rows[0])
	}

	committed, err := app.ImportSleepData(input)
	if err != nil {
		t.Fatal(err)
	}
	if committed.ImportedRows != 1 || committed.Rows[0].Status != "imported" {
		t.Fatalf("unexpected binding commit: %#v", committed)
	}
	entries, err := app.ListSleepEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Entries) != 1 || entries.Entries[0].SourceLabel != "Imported sleep" || !strings.Contains(entries.Entries[0].ProvenanceLabel, "file import") {
		t.Fatalf("imported entry did not reach the local read model: %#v", entries)
	}
}

func TestSleepImportBindingReturnsRowErrorsWithoutWriting(t *testing.T) {
	app := newTestApp(t)
	header := "observation_id,kind,start_at,end_at,zone_id,sleep_classification,acquisition_method,evidence_status,recorded_at,source_record_id"
	badRow := "obs_owner_002,sleep_episode,2023-01-02T13:00:00Z,2023-01-02T05:00:00Z,America/New_York,principal,file_import,user_reported,2023-01-02T13:00:00Z,transcribed-owner-002"
	report, err := app.ImportSleepData(SleepImportInput{FileName: "transcription.csv", Contents: header + "\n" + badRow + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	if report.InvalidRows != 1 || report.ImportedRows != 0 || len(report.Rows[0].Errors) == 0 {
		t.Fatalf("binding hid validation errors: %#v", report)
	}
	entries, err := app.ListSleepEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Entries) != 0 {
		t.Fatalf("invalid binding import wrote data: %#v", entries)
	}
}

func TestNativeSleepImportUsesOneUsePreviewToken(t *testing.T) {
	app := newTestApp(t)
	path := filepath.Join(t.TempDir(), "owner-history.csv")
	contents := strings.Join([]string{
		"observation_id,kind,start_at,end_at,zone_id,sleep_classification,acquisition_method,evidence_status,recorded_at,source_record_id",
		"obs_native_001,sleep_episode,2023-01-01T05:00:00Z,2023-01-01T13:00:00Z,America/New_York,principal,file_import,directly_observed,2023-01-01T13:00:00Z,native-owner-001",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	previousDialog := openSleepDataDialog
	openSleepDataDialog = func(_ context.Context, options runtime.OpenDialogOptions) (string, error) {
		if options.Title == "" || len(options.Filters) != 3 {
			t.Fatalf("unexpected open dialog options: %#v", options)
		}
		return path, nil
	}
	t.Cleanup(func() { openSleepDataDialog = previousDialog })

	preview, err := app.PreviewSleepImportFile()
	if err != nil {
		t.Fatal(err)
	}
	if preview.ImportToken == "" || preview.Canceled || !preview.CanImport || preview.ReadyRows != 1 {
		t.Fatalf("unexpected native preview: %#v", preview)
	}
	committed, err := app.ImportSleepDataFile(SleepImportFileInput{ImportToken: preview.ImportToken})
	if err != nil {
		t.Fatal(err)
	}
	if committed.ImportedRows != 1 || committed.ImportToken != "" {
		t.Fatalf("unexpected native commit: %#v", committed)
	}
	if _, err := app.ImportSleepDataFile(SleepImportFileInput{ImportToken: preview.ImportToken}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected consumed token rejection, got %v", err)
	}
}

func TestNativeSleepImportRejectsFileChangedAfterPreview(t *testing.T) {
	app := newTestApp(t)
	path := filepath.Join(t.TempDir(), "owner-history.csv")
	original := strings.Join([]string{
		"observation_id,kind,start_at,end_at,zone_id,sleep_classification,acquisition_method,evidence_status,recorded_at,source_record_id",
		"obs_native_002,sleep_episode,2023-01-01T05:00:00Z,2023-01-01T13:00:00Z,America/New_York,principal,file_import,directly_observed,2023-01-01T13:00:00Z,native-owner-002",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	previousDialog := openSleepDataDialog
	openSleepDataDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		return path, nil
	}
	t.Cleanup(func() { openSleepDataDialog = previousDialog })

	preview, err := app.PreviewSleepImportFile()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original+"# changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportSleepDataFile(SleepImportFileInput{ImportToken: preview.ImportToken}); err == nil || !strings.Contains(err.Error(), "changed after preview") {
		t.Fatalf("expected changed-file rejection, got %v", err)
	}
	if _, err := app.ImportSleepDataFile(SleepImportFileInput{ImportToken: preview.ImportToken}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("changed-file token should be consumed, got %v", err)
	}
}

func TestNativeSleepImportCancelDoesNotCreateToken(t *testing.T) {
	app := newTestApp(t)
	previousDialog := openSleepDataDialog
	openSleepDataDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		return "", nil
	}
	app.sleepImportMu.Lock()
	app.sleepImportPending = map[string]pendingSleepImportFile{"stale-token": {}}
	app.sleepImportMu.Unlock()
	t.Cleanup(func() { openSleepDataDialog = previousDialog })

	report, err := app.PreviewSleepImportFile()
	if err != nil {
		t.Fatal(err)
	}
	if !report.Canceled || report.ImportToken != "" || report.CanImport || report.Rows == nil || report.Errors == nil {
		t.Fatalf("unexpected cancel response: %#v", report)
	}
	if _, ok := app.consumePendingSleepImport("stale-token"); ok {
		t.Fatal("canceling a new selection left the previous import token valid")
	}
}
