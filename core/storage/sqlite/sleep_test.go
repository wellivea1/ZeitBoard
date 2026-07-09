package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalSleepPersistenceRoundTripAndEffectiveCorrection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "non24.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	start := time.Date(2026, 3, 1, 4, 30, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	recorded := end.Add(5 * time.Minute)
	obs := SleepObservationRecord{
		ObservationID: "obs_sleep_01",
		Kind:          SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        "America/New_York",
		Sleep:         SleepObservationDetails{Classification: SleepClassificationPrincipal},
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionManual,
			EvidenceStatus:    ProvenanceEvidenceUserReported,
			RecordedAt:        recorded,
			SourceRecordID:    "desktop-manual",
		},
	}
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	listed, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed observations = %d", len(listed))
	}
	if listed[0].ObservationID != obs.ObservationID || listed[0].Provenance.SourceRecordID != "desktop-manual" {
		t.Fatalf("round-tripped observation lost contract fields: %#v", listed[0])
	}

	correctedStart := start.Add(30 * time.Minute)
	classification := SleepClassificationPrincipal
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_01",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           recorded.Add(time.Minute),
		Reason:              CorrectionReasonUserEdit,
		Changes: SleepCorrectionChanges{
			StartAt:             &correctedStart,
			SleepClassification: &classification,
		},
	}
	if err := store.AppendSleepCorrection(ctx, correction); err != nil {
		t.Fatal(err)
	}

	raw, err := store.RawSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := store.EffectiveSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := raw[0].Intervals[0].Interval.Start.UTC; !got.Equal(start) {
		t.Fatalf("raw observation was mutated: got %s want %s", got, start)
	}
	if got := effective[0].Intervals[0].Interval.Start.UTC; !got.Equal(correctedStart) {
		t.Fatalf("effective start = %s, want %s", got, correctedStart)
	}
	if len(effective[0].Intervals[0].StartEvidence.CorrectionIDs) == 0 {
		t.Fatal("effective read did not carry correction provenance")
	}
}

func TestLocalSleepCorrectionsAreAppendOnlyAndSuperseded(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 3, 2, 5, 0, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	obs := SleepObservationRecord{
		ObservationID: "obs_sleep_02",
		Kind:          SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        "America/New_York",
		Sleep:         SleepObservationDetails{Classification: SleepClassificationPrincipal},
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionManual,
			EvidenceStatus:    ProvenanceEvidenceUserReported,
			RecordedAt:        end,
		},
	}
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	firstStart := start.Add(15 * time.Minute)
	first := SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_02",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           end.Add(time.Minute),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{StartAt: &firstStart},
	}
	if err := store.AppendSleepCorrection(ctx, first); err != nil {
		t.Fatal(err)
	}
	excluded := true
	second := SleepCorrectionRecord{
		CorrectionID:           "corr_sleep_03",
		TargetObservationID:    obs.ObservationID,
		SupersedesCorrectionID: first.CorrectionID,
		CreatedAt:              end.Add(2 * time.Minute),
		Reason:                 CorrectionReasonUserEdit,
		Changes: SleepCorrectionChanges{
			StartAt:  &firstStart,
			EndAt:    &end,
			Excluded: &excluded,
		},
	}
	if err := store.AppendSleepCorrection(ctx, second); err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListSleepCorrections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(corrections) != 2 {
		t.Fatalf("corrections stored = %d, want append-only history of 2", len(corrections))
	}
	effective, err := store.CorrectedSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !effective[0].Suppressed {
		t.Fatal("superseding suppression correction was not applied")
	}
	if got := effective[0].Intervals[0].Interval.Start.UTC; !got.Equal(firstStart) {
		t.Fatalf("superseding correction failed to retain full effective start: got %s want %s", got, firstStart)
	}
}

func TestLocalSleepExportAndDeleteDistinguishSuppressFromErasure(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 3, 3, 5, 0, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	obs := testSleepObservation("obs_sleep_03", start, end)
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	excluded := true
	suppression := SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_04",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           end.Add(time.Minute),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{Excluded: &excluded},
	}
	if err := store.AppendSleepCorrection(ctx, suppression); err != nil {
		t.Fatal(err)
	}

	exported, err := store.ExportSleepData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if exported.SchemaVersion != "v1" || exported.ObservationSet.SchemaVersion != "v1" || exported.CorrectionSet.SchemaVersion != "v1" {
		t.Fatalf("export did not preserve v1 set shape: %#v", exported)
	}
	if len(exported.ObservationSet.Observations) != 1 || len(exported.CorrectionSet.Corrections) != 1 {
		t.Fatalf("suppression should export raw observation and append-only correction: %#v", exported)
	}
	effective, err := store.CorrectedSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 1 || !effective[0].Suppressed {
		t.Fatalf("suppression should only affect effective reads: %#v", effective)
	}

	if err := store.DeleteSleepObservation(ctx, obs.ObservationID); err != nil {
		t.Fatal(err)
	}
	exported, err = store.ExportSleepData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.ObservationSet.Observations) != 0 || len(exported.CorrectionSet.Corrections) != 0 {
		t.Fatalf("delete should erase observation and correction history: %#v", exported)
	}
	if exported.ObservationSet.Observations == nil || exported.CorrectionSet.Corrections == nil {
		t.Fatal("empty export should keep contract arrays, not null slices")
	}
	effective, err = store.CorrectedSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 0 {
		t.Fatalf("deleted sleep entry still appears in effective reads: %#v", effective)
	}
}

func TestDeleteAllSleepDataErasesLocalSleepTables(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	obs := testSleepObservation("obs_sleep_04", start, end)
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	correctedStart := start.Add(20 * time.Minute)
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_05",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           end.Add(time.Minute),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{StartAt: &correctedStart},
	}
	if err := store.AppendSleepCorrection(ctx, correction); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteAllSleepData(ctx); err != nil {
		t.Fatal(err)
	}
	exported, err := store.ExportSleepData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.ObservationSet.Observations)+len(exported.CorrectionSet.Corrections) != 0 {
		t.Fatalf("local sleep data remains after delete all: %#v", exported)
	}
	if exported.ObservationSet.Observations == nil || exported.CorrectionSet.Corrections == nil {
		t.Fatal("empty delete-all export should keep contract arrays, not null slices")
	}
}

func TestLocalSleepSyncTrackingCursorAndErasure(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 3, 5, 5, 0, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	obs := testSleepObservation("obs_sleep_05", start, end)
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	correctedStart := start.Add(15 * time.Minute)
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_06",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           end.Add(time.Minute),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{StartAt: &correctedStart},
	}
	if err := store.AppendSleepCorrection(ctx, correction); err != nil {
		t.Fatal(err)
	}

	records, err := store.LocalSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].RecordID != obs.ObservationID || records[0].Kind != SleepSyncKindObservation || records[1].RecordID != correction.CorrectionID || records[1].Kind != SleepSyncKindCorrection {
		t.Fatalf("unexpected sync records: %#v", records)
	}
	unpushed, err := store.UnpushedSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpushed) != 2 {
		t.Fatalf("unpushed records = %d, want 2", len(unpushed))
	}
	if err := store.MarkSleepSyncRecordsPushed(ctx, records, end.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	unpushed, err = store.UnpushedSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpushed) != 0 {
		t.Fatalf("pushed records still pending: %#v", unpushed)
	}
	if err := store.SaveSleepSyncCursor(ctx, 42); err != nil {
		t.Fatal(err)
	}
	cursor, err := store.SleepSyncCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != 42 {
		t.Fatalf("cursor = %d, want 42", cursor)
	}

	if err := store.DeleteSleepObservation(ctx, obs.ObservationID); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	unpushed, err = store.UnpushedSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpushed) != 1 || unpushed[0].RecordID != obs.ObservationID {
		t.Fatalf("erasure should remove pushed tracking for sleep IDs, got %#v", unpushed)
	}
}

func TestDeleteEnqueuesErasuresOnlyForPushedRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 4, 2, 3, 0, 0, 0, time.UTC)
	tp := func(value time.Time) *time.Time { return &value }

	pushed := testSleepObservation("obs_pushed_01", start, start.Add(8*time.Hour))
	unpushed := testSleepObservation("obs_unpushed_01", start.Add(25*time.Hour), start.Add(33*time.Hour))
	if err := store.AppendSleepObservation(ctx, pushed); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSleepObservation(ctx, unpushed); err != nil {
		t.Fatal(err)
	}
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_pushed_01",
		TargetObservationID: pushed.ObservationID,
		CreatedAt:           start.Add(10 * time.Hour),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{EndAt: tp(start.Add(7 * time.Hour))},
	}
	if err := store.AppendSleepCorrection(ctx, correction); err != nil {
		t.Fatal(err)
	}
	// Mark the observation + its correction pushed; leave the other unpushed.
	records, err := store.LocalSleepSyncRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	toMark := make([]SleepSyncRecord, 0, 2)
	for _, record := range records {
		if record.RecordID == pushed.ObservationID || record.RecordID == correction.CorrectionID {
			toMark = append(toMark, record)
		}
	}
	if err := store.MarkSleepSyncRecordsPushed(ctx, toMark, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	// Deleting the pushed observation enqueues erasures for it AND its correction.
	if err := store.DeleteSleepObservation(ctx, pushed.ObservationID); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending erasures = %v, want observation + correction", pending)
	}

	// Deleting the never-pushed observation enqueues nothing new.
	if err := store.DeleteSleepObservation(ctx, unpushed.ObservationID); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("unpushed delete should not enqueue erasure: %v", pending)
	}

	// Clearing removes confirmed entries.
	if err := store.ClearSyncErasures(ctx, pending); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("cleared erasures should be empty, got %v", pending)
	}
}

func TestEraseSyncedSleepRecordAppliesTombstoneIdempotently(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 4, 3, 2, 0, 0, 0, time.UTC)
	tp := func(value time.Time) *time.Time { return &value }

	obs := testSleepObservation("obs_tombstoned_01", start, start.Add(8*time.Hour))
	if err := store.AppendSleepObservation(ctx, obs); err != nil {
		t.Fatal(err)
	}
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_tombstoned_01",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           start.Add(10 * time.Hour),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{EndAt: tp(start.Add(7 * time.Hour))},
	}
	if err := store.AppendSleepCorrection(ctx, correction); err != nil {
		t.Fatal(err)
	}

	applied, err := store.EraseSyncedSleepRecord(ctx, obs.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("tombstone application should report applied")
	}
	observations, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListSleepCorrections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 || len(corrections) != 0 {
		t.Fatalf("tombstone must erase observation and corrections: obs=%d corr=%d", len(observations), len(corrections))
	}
	// Applying a tombstone must NOT re-enqueue an outbound erasure.
	pending, err := store.PendingSyncErasures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("tombstone application enqueued erasures: %v", pending)
	}
	// Idempotent on repeat.
	applied, err = store.EraseSyncedSleepRecord(ctx, obs.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("repeat tombstone application should be a no-op")
	}
}

func TestInsertSyncedSleepRecordsDedupesByContractID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "non24.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	start := time.Date(2026, 3, 6, 5, 0, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	obs := testSleepObservation("obs_sleep_06", start, end)

	inserted, err := store.InsertSyncedSleepObservation(ctx, obs)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first synced observation insert should report inserted")
	}
	inserted, err = store.InsertSyncedSleepObservation(ctx, obs)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("second synced observation insert should dedupe")
	}
	correctedEnd := end.Add(20 * time.Minute)
	correction := SleepCorrectionRecord{
		CorrectionID:        "corr_sleep_07",
		TargetObservationID: obs.ObservationID,
		CreatedAt:           end.Add(time.Minute),
		Reason:              CorrectionReasonUserEdit,
		Changes:             SleepCorrectionChanges{EndAt: &correctedEnd},
	}
	inserted, err = store.InsertSyncedSleepCorrection(ctx, correction)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first synced correction insert should report inserted")
	}
	inserted, err = store.InsertSyncedSleepCorrection(ctx, correction)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("second synced correction insert should dedupe")
	}
	observations, err := store.ListSleepObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := store.ListSleepCorrections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || len(corrections) != 1 {
		t.Fatalf("deduped records = %d observations, %d corrections", len(observations), len(corrections))
	}
}

func testSleepObservation(id string, start, end time.Time) SleepObservationRecord {
	return SleepObservationRecord{
		ObservationID: id,
		Kind:          SleepKindEpisode,
		StartAt:       start,
		EndAt:         end,
		ZoneID:        "America/New_York",
		Sleep:         SleepObservationDetails{Classification: SleepClassificationPrincipal},
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionManual,
			EvidenceStatus:    ProvenanceEvidenceUserReported,
			RecordedAt:        end,
			SourceRecordID:    "desktop-manual",
		},
	}
}
