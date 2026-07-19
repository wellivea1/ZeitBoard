package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSleepImportPreviewCommitAndRepeatAreAccountedFor(t *testing.T) {
	store := openSleepImportTestStore(t)
	ctx := context.Background()
	observation := importedSleepObservation("obs_import_01", "fitbit-2021-10-28", time.Date(2021, 10, 29, 1, 18, 0, 0, time.UTC))
	input := SleepImportInput{FileName: "owner-history.json", Contents: sleepImportJSON(t, observation)}

	preview, err := store.PreviewSleepImport(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || !preview.CanImport || preview.ReadyRows != 1 || preview.InvalidRows != 0 || preview.DuplicateRows != 0 {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	committed, err := store.ImportSleepObservations(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if committed.DryRun || committed.ImportedRows != 1 || committed.CanImport {
		t.Fatalf("unexpected commit report: %#v", committed)
	}
	listed, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Provenance.AcquisitionMethod != ProvenanceAcquisitionFileImport || listed[0].Provenance.SourceRecordID != "fitbit-2021-10-28" {
		t.Fatalf("imported provenance was not preserved: %#v", listed)
	}

	repeated, err := store.PreviewSleepImport(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.CanImport || repeated.ReadyRows != 0 || repeated.DuplicateRows != 1 || repeated.Rows[0].StatusDetail == "" {
		t.Fatalf("repeat import was not reported as an exact duplicate: %#v", repeated)
	}
}

func TestSleepImportMixedValidityIsAtomicAndReportsTheBadRow(t *testing.T) {
	store := openSleepImportTestStore(t)
	valid := importedSleepObservation("obs_import_02", "fitbit-valid", time.Date(2022, 1, 1, 5, 0, 0, 0, time.UTC))
	invalid := importedSleepObservation("obs_import_03", "", time.Date(2022, 1, 2, 5, 0, 0, 0, time.UTC))
	input := SleepImportInput{FileName: "mixed.json", Contents: sleepImportJSON(t, valid, invalid)}

	report, err := store.ImportSleepObservations(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if report.CanImport || report.ImportedRows != 0 || report.InvalidRows != 1 || report.Rows[1].RowNumber != 2 {
		t.Fatalf("invalid row was not reported atomically: %#v", report)
	}
	if !strings.Contains(strings.Join(report.Rows[1].Errors, " "), "source_record_id") {
		t.Fatalf("missing per-row source id error: %#v", report.Rows[1])
	}
	listed, err := store.ListSleepObservations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("valid rows were partially imported despite an invalid batch: %#v", listed)
	}
}

func TestSleepImportFileErrorBlocksAndAccountsForOtherwiseValidRows(t *testing.T) {
	store := openSleepImportTestStore(t)
	observation := importedSleepObservation("obs_import_schema", "fitbit-schema", time.Date(2022, 1, 1, 5, 0, 0, 0, time.UTC))
	contents := strings.Replace(sleepImportJSON(t, observation), `"schema_version":"v1"`, `"schema_version":"v2"`, 1)
	report, err := store.PreviewSleepImport(context.Background(), SleepImportInput{FileName: "wrong-version.json", Contents: contents})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalRows != 1 || report.ReadyRows != 0 || report.InvalidRows != 1 || report.CanImport {
		t.Fatalf("file-level error did not reconcile row accounting: %#v", report)
	}
	if report.Rows[0].Status != SleepImportStatusInvalid || report.Rows[0].StatusDetail == "" {
		t.Fatalf("otherwise valid row was not explicitly blocked: %#v", report.Rows[0])
	}
}

func TestSleepImportDeduplicatesExactSourceRowsAndRejectsConflicts(t *testing.T) {
	store := openSleepImportTestStore(t)
	start := time.Date(2023, 3, 1, 5, 0, 0, 0, time.UTC)
	first := importedSleepObservation("obs_import_04", "fitbit-shared", start)
	exact := importedSleepObservation("obs_import_05", "fitbit-shared", start)
	report, err := store.PreviewSleepImport(context.Background(), SleepImportInput{
		FileName: "duplicates.json",
		Contents: sleepImportJSON(t, first, exact),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CanImport || report.ReadyRows != 1 || report.DuplicateRows != 1 || report.Rows[1].Status != SleepImportStatusDuplicate {
		t.Fatalf("exact source duplicate was not accounted for: %#v", report)
	}

	conflict := importedSleepObservation("obs_import_06", "fitbit-shared", start.Add(time.Hour))
	report, err = store.PreviewSleepImport(context.Background(), SleepImportInput{
		FileName: "conflict.json",
		Contents: sleepImportJSON(t, first, conflict),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.CanImport || report.InvalidRows != 2 || report.Rows[0].Status != SleepImportStatusInvalid || report.Rows[1].Status != SleepImportStatusInvalid {
		t.Fatalf("conflicting source id did not invalidate both rows: %#v", report)
	}

	changedProvenance := importedSleepObservation("obs_import_07", "fitbit-shared", start)
	changedProvenance.Provenance.RecordedAt = changedProvenance.Provenance.RecordedAt.Add(time.Minute)
	report, err = store.PreviewSleepImport(context.Background(), SleepImportInput{
		FileName: "provenance-conflict.json",
		Contents: sleepImportJSON(t, first, changedProvenance),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.CanImport || report.InvalidRows != 2 {
		t.Fatalf("changed provenance was silently deduplicated: %#v", report)
	}
}

func TestSleepImportCSVReportsPerRowValidationErrors(t *testing.T) {
	store := openSleepImportTestStore(t)
	header := strings.Join(sleepImportCSVColumns, ",")
	valid := "obs_csv_01,sleep_episode,2023-01-01T05:00:00Z,2023-01-01T13:00:00Z,America/New_York,principal,file_import,directly_observed,2023-01-01T13:00:00Z,fitbit-csv-1"
	invalid := "obs_csv_02,sleep_episode,2023-01-02T13:00:00Z,2023-01-02T05:00:00Z,America/New_York,principal,file_import,directly_observed,2023-01-02T13:00:00Z,fitbit-csv-2"
	report, err := store.PreviewSleepImport(context.Background(), SleepImportInput{
		FileName: "owner-history.csv",
		Contents: header + "\n" + valid + "\n" + invalid + "\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalRows != 2 || report.ReadyRows != 1 || report.InvalidRows != 1 || report.Rows[1].RowNumber != 3 {
		t.Fatalf("CSV row accounting is wrong: %#v", report)
	}
	if !strings.Contains(strings.Join(report.Rows[1].Errors, " "), "end") {
		t.Fatalf("CSV row did not expose its range error: %#v", report.Rows[1])
	}
}

func TestSleepImportCSVDoesNotSilentlyTrimTheSourceIdentifier(t *testing.T) {
	store := openSleepImportTestStore(t)
	header := strings.Join(sleepImportCSVColumns, ",")
	row := "obs_csv_whitespace,sleep_episode,2023-01-01T05:00:00Z,2023-01-01T13:00:00Z,America/New_York,principal,file_import,directly_observed,2023-01-01T13:00:00Z, fitbit-csv-whitespace"
	report, err := store.PreviewSleepImport(context.Background(), SleepImportInput{
		FileName: "owner-history.csv",
		Contents: header + "\n" + row + "\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.InvalidRows != 1 || !strings.Contains(strings.Join(report.Rows[0].Errors, " "), "surrounding whitespace") {
		t.Fatalf("CSV source identifier was silently normalized: %#v", report)
	}
}

func TestSleepImportDatabaseConstraintAndHardErasureCoverImportedData(t *testing.T) {
	store := openSleepImportTestStore(t)
	ctx := context.Background()
	first := importedSleepObservation("obs_import_08", "fitbit-unique", time.Date(2023, 6, 1, 5, 0, 0, 0, time.UTC))
	if err := store.AppendSleepObservation(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := importedSleepObservation("obs_import_09", "fitbit-unique", time.Date(2023, 6, 2, 5, 0, 0, 0, time.UTC))
	if err := store.AppendSleepObservation(ctx, second); err == nil {
		t.Fatal("database should reject a repeated imported source_record_id")
	}
	if err := store.DeleteSleepObservation(ctx, first.ObservationID); err != nil {
		t.Fatal(err)
	}
	exported, err := store.ExportSleepData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.ObservationSet.Observations) != 0 {
		t.Fatalf("hard erasure left imported data behind: %#v", exported)
	}
}

func TestSleepImportMigrationToleratesExistingDuplicatesButBlocksNewOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-duplicates.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER trg_local_sleep_import_source_record`); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2023, 6, 1, 5, 0, 0, 0, time.UTC)
	first := importedSleepObservation("obs_import_legacy_1", "fitbit-legacy-duplicate", start)
	second := importedSleepObservation("obs_import_legacy_2", "fitbit-legacy-duplicate", start)
	if err := store.AppendSleepObservation(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSleepObservation(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("migration should not brick a legacy store with repeated source ids: %v", err)
	}
	defer store.Close()
	third := importedSleepObservation("obs_import_legacy_3", "fitbit-legacy-duplicate", start)
	if err := store.AppendSleepObservation(context.Background(), third); err == nil {
		t.Fatal("migration trigger should block a new repeated source id")
	}
	_, err = store.PreviewSleepImport(context.Background(), SleepImportInput{
		FileName: "legacy.json",
		Contents: sleepImportJSON(t, first),
	})
	if err == nil || !strings.Contains(err.Error(), "repeated imported source_record_id") {
		t.Fatalf("importer should refuse an ambiguous legacy dedupe state: %v", err)
	}
}

func openSleepImportTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "sleep-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func importedSleepObservation(observationID, sourceRecordID string, start time.Time) SleepObservationRecord {
	end := start.Add(8 * time.Hour)
	return SleepObservationRecord{
		ObservationID: observationID,
		Kind:          SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        "America/New_York",
		Sleep:         SleepObservationDetails{Classification: SleepClassificationPrincipal},
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionFileImport,
			EvidenceStatus:    ProvenanceEvidenceDirectlyObserved,
			RecordedAt:        end,
			SourceRecordID:    sourceRecordID,
		},
	}
}

func sleepImportJSON(t *testing.T, observations ...SleepObservationRecord) string {
	t.Helper()
	encoded, err := json.Marshal(SleepObservationSet{
		SchemaVersion: "v1",
		GeneratedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Observations:  observations,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
