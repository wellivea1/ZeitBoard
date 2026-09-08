package main

import (
	"context"
	"testing"
	"time"

	"non24.app/core/sleepv1"
)

func TestSleepLogReviewsConflictAndRejectsStaleForm(t *testing.T) {
	app := newTestApp(t)
	added, err := app.AddSleepEntry(SleepEntryInput{StartLocal: "2026-03-05T04:30", EndLocal: "2026-03-05T12:30", ZoneID: "UTC", Classification: "principal"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := app.requireStore()
	if err != nil {
		t.Fatal(err)
	}
	first, second := time.Date(2026, 3, 5, 4, 10, 0, 123, time.UTC), time.Date(2026, 3, 5, 4, 20, 0, 456, time.UTC)
	for i, start := range []time.Time{first, second} {
		id := []string{"cor_first", "cor_second"}[i]
		err := store.AppendSleepCorrection(context.Background(), sleepv1.Correction{CorrectionID: id, TargetObservationID: added.ObservationID, CreatedAt: time.Now().UTC(), Reason: "user_edit", Changes: sleepv1.CorrectionChanges{StartAt: &start}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReadSleepSnapshot(context.Background()); err == nil {
		t.Fatal("conflict entered the estimator snapshot")
	}
	listed, err := app.ListSleepEntries()
	if err != nil || len(listed.Entries) != 1 {
		t.Fatal("conflict made the editable log unavailable")
	}
	current := listed.Entries[0]
	if !current.NeedsReview || len(current.ActiveEdits) != 2 {
		t.Fatal("log concealed disputed alternatives")
	}
	input := SleepCorrectionInput{ObservationID: added.ObservationID, ReviewToken: added.ReviewToken, StartLocal: first.Format(time.RFC3339Nano), EndLocal: current.EffectiveEndLocal, ZoneID: "UTC", Classification: "unknown", Excluded: true}
	if _, err := app.CorrectSleepEntry(input); err == nil {
		t.Fatal("stale form overrode unseen edits")
	}
	if _, err := app.SuppressSleepEntry(SleepSuppressInput{ObservationID: current.ObservationID, ReviewToken: current.ReviewToken}); err == nil {
		t.Fatal("suppression bypassed review")
	}
	input.ReviewToken = current.ReviewToken
	resolved, err := app.CorrectSleepEntry(input)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.NeedsReview || !resolved.Suppressed || resolved.EffectiveClassification != "unknown" {
		t.Fatal("resolution lost classification or exclusion")
	}
	review, err := store.SleepReview(context.Background(), added.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if !review.Effective.Intervals[0].Interval.Start.UTC.Equal(first) || len(review.ManualCorrections) != 1 || len(review.ManualCorrections[0].SupersedesCorrectionIDs) != 2 {
		t.Fatal("review lost precision or parent edits")
	}
}

func TestSleepCorrectionRetainsRepeatedHourAndNanoseconds(t *testing.T) {
	app := newTestApp(t)
	entry, err := app.AddSleepEntry(SleepEntryInput{StartLocal: "2026-11-01T01:30:00.123456789-04:00", EndLocal: "2026-11-01T01:30:00.987654321-05:00", ZoneID: "America/New_York", Classification: "principal"})
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := app.CorrectSleepEntry(SleepCorrectionInput{ObservationID: entry.ObservationID, ReviewToken: entry.ReviewToken, StartLocal: entry.EffectiveStartLocal, EndLocal: entry.EffectiveEndLocal, ZoneID: entry.ZoneID, Classification: "nap"})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.EffectiveStartLocal != entry.EffectiveStartLocal || corrected.EffectiveEndLocal != entry.EffectiveEndLocal {
		t.Fatal("editing classification changed exact repeated-hour endpoints")
	}
}
