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
