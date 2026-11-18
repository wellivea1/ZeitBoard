package main

import (
	"context"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// A night Health Connect later deleted stays in the log, marked and left out,
// and the owner's correction puts it back (ADR-0072). A night recorded away
// says how far from the home clock (ADR-0073).
func TestTheLogShowsARemovedNightAndANightAway(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	store := app.store
	night := func(id string, start time.Time) storage.SleepObservationRecord {
		record := testSyncObservation(id, start)
		record.ZoneID = "America/New_York"
		record.Provenance.AcquisitionMethod = storage.ProvenanceAcquisitionHealthConnect
		record.Provenance.EvidenceStatus = storage.ProvenanceEvidenceDirectlyObserved
		record.Provenance.SourceRecordID = id
		return record
	}
	// September in New York is four hours behind UTC.
	removed := night("hc-synthetic-removed", time.Date(2026, 9, 1, 4, 0, 0, 0, time.UTC))
	away := night("hc-synthetic-away", time.Date(2026, 9, 4, 21, 0, 0, 0, time.UTC))
	away.SourceOffset = &storage.SleepSourceOffset{StartSeconds: 2 * 3600, EndSeconds: 2 * 3600}
	behind := night("hc-synthetic-behind", time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC))
	behind.SourceOffset = &storage.SleepSourceOffset{StartSeconds: -4 * 3600, EndSeconds: -(9*3600 + 30*60)}
	for _, record := range []storage.SleepObservationRecord{removed, away, behind} {
		if _, err := store.InsertSyncedSleepObservation(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	gone := true
	if _, err := store.InsertSyncedSleepCorrection(ctx, storage.SleepCorrectionRecord{
		CorrectionID: "cor-synthetic-removed", TargetObservationID: removed.ObservationID,
		CreatedAt: removed.EndAt.Add(30 * time.Hour), Reason: "source_deleted",
		AcquisitionMethod: storage.ProvenanceAcquisitionHealthConnect,
		Changes:           storage.SleepCorrectionChanges{Excluded: &gone},
	}); err != nil {
		t.Fatal(err)
	}

	entries := func() map[string]SleepEntryDTO {
		t.Helper()
		page, err := app.GetSleepLogPage(SleepLogPageInput{})
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]SleepEntryDTO{}
		for _, entry := range page.Entries {
			byID[entry.ObservationID] = entry
		}
		return byID
	}
	log := entries()
	if entry := log[removed.ObservationID]; !entry.RemovedAtSource || !entry.Suppressed || entry.NeedsReview ||
		len(entry.History) != 1 || entry.History[0].Summary != "Removed at the source" {
		t.Fatalf("the removed night reads %+v", entry)
	}
	if entry := log[away.ObservationID]; entry.RecordedAway != "Recorded 6 hours ahead of home" || entry.RemovedAtSource || entry.Suppressed {
		t.Fatalf("the night away reads %q, removed %v", entry.RecordedAway, entry.RemovedAtSource)
	}
	// A clock that changed during the night is said by the end that differs.
	if entry := log[behind.ObservationID]; entry.RecordedAway != "Recorded 5 hours 30 minutes behind home" {
		t.Fatalf("the night that changed clocks reads %q", entry.RecordedAway)
	}
	if log[removed.ObservationID].RecordedAway != "" {
		t.Fatal("a night at home says it was recorded away")
	}

	// The estimator is given the nights away and not the removed one.
	sessions, err := store.EffectiveSleepSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counted := 0
	for _, session := range sessions {
		if session.IsPrincipalSleep() {
			counted++
		}
	}
	if counted != 2 {
		t.Fatalf("%d nights count, want the two nights away", counted)
	}

	// The owner's correction puts the removed night back, still marked.
	entry := log[removed.ObservationID]
	if _, err := app.CorrectSleepEntry(SleepCorrectionInput{
		ObservationID: entry.ObservationID, ReviewToken: entry.ReviewToken,
		StartLocal: entry.StartLocal, EndLocal: entry.EndLocal, ZoneID: entry.ZoneID,
		Classification: entry.Classification, Excluded: false,
	}); err != nil {
		t.Fatal(err)
	}
	if entry := entries()[removed.ObservationID]; !entry.RemovedAtSource || entry.Suppressed || entry.NeedsReview {
		t.Fatalf("after the owner's correction the night reads removed %v, left out %v, review %v", entry.RemovedAtSource, entry.Suppressed, entry.NeedsReview)
	}
}
