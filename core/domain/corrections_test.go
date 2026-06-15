package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestCorrectionOverridesEffectiveValueWithoutMutatingSource(t *testing.T) {
	zone := "America/New_York"
	start := MustZonedInstant(time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC), zone)
	end := MustZonedInstant(time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC), zone)
	source := []SleepSession{{
		ID: "sleep-1",
		Intervals: []SleepInterval{{
			Interval:      TimeRange{Start: start, End: end},
			StartEvidence: Evidence{Acquisition: AcquisitionImported, Status: StatusImported, ObservationIDs: []ObservationID{"wearable-1"}},
			EndEvidence:   Evidence{Acquisition: AcquisitionImported, Status: StatusImported, ObservationIDs: []ObservationID{"wearable-1"}},
		}},
	}}
	original := cloneSleepSession(source[0])
	correctedStart := MustZonedInstant(start.UTC.Add(45*time.Minute), zone)
	correction := ManualCorrection{
		ID: "correction-1", TargetID: "sleep-1", Kind: CorrectionSetSleepStart,
		InstantValue: &correctedStart, Active: true, CreatedAt: time.Now().UTC(),
	}

	effective, err := ApplySleepCorrections(source, []ManualCorrection{correction})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source[0], original) {
		t.Fatal("source session was mutated")
	}
	got := effective[0].Intervals[0]
	if !got.Interval.Start.UTC.Equal(correctedStart.UTC) {
		t.Fatalf("effective start = %v, want %v", got.Interval.Start.UTC, correctedStart.UTC)
	}
	if got.StartEvidence.Status != StatusUserConfirmed {
		t.Fatalf("status = %q", got.StartEvidence.Status)
	}
	if len(got.StartEvidence.ObservationIDs) != 1 || got.StartEvidence.ObservationIDs[0] != "wearable-1" {
		t.Fatal("source provenance was not preserved")
	}
}
