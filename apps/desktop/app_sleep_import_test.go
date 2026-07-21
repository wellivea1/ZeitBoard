package main

import (
	"strings"
	"testing"
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
