package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

func TestTheSleepLogIsReadByPage(t *testing.T) {
	app := newTestApp(t)
	empty, err := app.GetSleepLogPage(SleepLogPageInput{})
	if err != nil || empty.Status != "empty" || !empty.Empty || len(empty.Entries) != 0 {
		t.Fatalf("an empty log = %+v, %v", empty, err)
	}

	seedSleepEntries(t, app, sleepLogPageSize+10)
	first, err := app.GetSleepLogPage(SleepLogPageInput{})
	if err != nil || first.Total != sleepLogPageSize+10 || len(first.Entries) != sleepLogPageSize || first.Page != 0 {
		t.Fatalf("first page = %d entries of %d, page %d, %v", len(first.Entries), first.Total, first.Page, err)
	}
	if first.Entries[0].StartLabel == "" || first.Message != "60 local sleep entries stored on this device." {
		t.Fatalf("first page = %+v", first.Entries[0])
	}
	second, err := app.GetSleepLogPage(SleepLogPageInput{Page: 1})
	if err != nil || len(second.Entries) != 10 || second.Page != 1 {
		t.Fatalf("second page = %d entries, page %d, %v", len(second.Entries), second.Page, err)
	}
	// Newest first, and the pages meet without a gap or an overlap.
	newestOnFirst, _ := time.Parse(time.RFC3339Nano, first.Entries[0].StartLocal)
	lastOnFirst, _ := time.Parse(time.RFC3339Nano, first.Entries[sleepLogPageSize-1].StartLocal)
	firstOnSecond, _ := time.Parse(time.RFC3339Nano, second.Entries[0].StartLocal)
	if !newestOnFirst.After(lastOnFirst) || lastOnFirst.Sub(firstOnSecond) != 25*time.Hour {
		t.Fatalf("pages run %s .. %s then %s", newestOnFirst, lastOnFirst, firstOnSecond)
	}
	// Past the end, as after deleting the last page's only night: the last page.
	beyond, err := app.GetSleepLogPage(SleepLogPageInput{Page: 9})
	if err != nil || beyond.Page != 1 || len(beyond.Entries) != 10 {
		t.Fatalf("past the end = page %d with %d entries, %v", beyond.Page, len(beyond.Entries), err)
	}
}

func TestAWeekReadsOnlyTheNightsItTouches(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 20)
	now := time.Now().UTC()
	week, err := app.GetSleepEntriesBetween(SleepRangeInput{
		StartAt: now.Add(-7 * 24 * time.Hour).Format(time.RFC3339), EndAt: now.Format(time.RFC3339),
	})
	// Nights 25 hours apart, the last one 12 hours ago: seven touch the week.
	if err != nil || week.Status != "ready" || len(week.Entries) < 6 || len(week.Entries) > 8 ||
		week.Message != fmt.Sprintf("%d nights in these days.", len(week.Entries)) {
		t.Fatalf("a week = %d nights, %v", len(week.Entries), err)
	}
	for _, input := range []SleepRangeInput{
		{StartAt: now.Format(time.RFC3339), EndAt: now.Add(-time.Hour).Format(time.RFC3339)},
		{StartAt: now.Add(-60 * 24 * time.Hour).Format(time.RFC3339), EndAt: now.Format(time.RFC3339)},
		{StartAt: "last week", EndAt: now.Format(time.RFC3339)},
	} {
		if _, err := app.GetSleepEntriesBetween(input); err == nil {
			t.Errorf("range %+v was accepted", input)
		}
	}
}

func TestTheLogsSourcesAreCountedWithoutSendingTheLog(t *testing.T) {
	app := newTestApp(t)
	empty, err := app.GetSleepSources()
	if err != nil || empty.Status != "empty" || len(empty.Sources) != 0 || empty.LatestCorrected != nil {
		t.Fatalf("no nights = %+v, %v", empty, err)
	}

	seedSleepEntries(t, app, 5)
	page, err := app.GetSleepLogPage(SleepLogPageInput{})
	if err != nil {
		t.Fatal(err)
	}
	newest, older := page.Entries[0], page.Entries[3]
	if _, err := app.SuppressSleepEntry(SleepSuppressInput{ObservationID: older.ObservationID, ReviewToken: older.ReviewToken}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CorrectSleepEntry(SleepCorrectionInput{
		ObservationID: newest.ObservationID, ReviewToken: newest.ReviewToken, ZoneID: newest.ZoneID,
		StartLocal: newest.StartLocal, EndLocal: newest.EndLocal, Classification: "nap",
	}); err != nil {
		t.Fatal(err)
	}

	sources, err := app.GetSleepSources()
	if err != nil {
		t.Fatal(err)
	}
	if sources.Status != "ready" || sources.Total != 5 || sources.CorrectedCount != 2 || sources.SuppressedCount != 1 ||
		len(sources.Sources) != 1 || sources.Sources[0].Total != 5 || sources.Sources[0].Corrected != 2 || sources.Sources[0].Suppressed != 1 {
		t.Fatalf("sources = %+v", sources)
	}
	// The newest corrected night drives the correction inspector.
	if sources.LatestCorrected == nil || sources.LatestCorrected.ObservationID != newest.ObservationID ||
		!strings.Contains(sources.LatestCorrected.History[0].Summary, "classification nap") {
		t.Fatalf("latest corrected = %+v", sources.LatestCorrected)
	}
}

// Moved from the window with the counting: imported nights observed and
// imported nights reported are different evidence, and are counted apart.
func TestSleepSourcesKeepEachProvenanceApart(t *testing.T) {
	app := newTestApp(t)
	night := func(id, day, evidence string) string {
		return `{"observation_id":"` + id + `","kind":"sleep_episode",` +
			`"start_at":"2023-01-` + day + `T05:00:00Z","end_at":"2023-01-` + day + `T13:00:00Z",` +
			`"zone_id":"America/New_York","sleep":{"classification":"principal"},` +
			`"provenance":{"acquisition_method":"file_import","evidence_status":"` + evidence + `",` +
			`"recorded_at":"2023-01-` + day + `T13:00:00Z","source_record_id":"synthetic-` + id + `"}}`
	}
	if _, err := app.ImportSleepData(SleepImportInput{FileName: "synthetic.json", Contents: `{"schema_version":"v1",` +
		`"generated_at":"2024-01-01T00:00:00Z","observations":[` +
		night("obs_observed", "01", "directly_observed") + `,` + night("obs_reported", "02", "user_reported") + `]}`}); err != nil {
		t.Fatal(err)
	}
	sources, err := app.GetSleepSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(sources.Sources) != 2 || sources.Sources[0].Source != sources.Sources[1].Source ||
		sources.Sources[0].Provenance == sources.Sources[1].Provenance ||
		sources.Sources[0].Total != 1 || sources.Sources[1].Total != 1 {
		t.Fatalf("provenances were counted together: %+v", sources.Sources)
	}
}

// The roadmap's one-click correction undo: taking back an edit adds a
// correction, as every edit does, and restores the night as it was before.
func TestUndoTakesBackANightsLatestEdit(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 3)
	page, err := app.GetSleepLogPage(SleepLogPageInput{})
	if err != nil {
		t.Fatal(err)
	}
	night := page.Entries[0]
	if night.CanUndo {
		t.Fatal("a night without edits offers an undo")
	}
	if _, err := app.UndoSleepCorrection(SleepUndoInput{ObservationID: night.ObservationID, ReviewToken: night.ReviewToken}); err == nil {
		t.Fatal("a night without edits was undone")
	}

	recordedStart, _ := time.Parse(time.RFC3339Nano, night.StartLocal)
	edit := func(entry SleepEntryDTO, start time.Time) SleepEntryDTO {
		t.Helper()
		edited, err := app.CorrectSleepEntry(SleepCorrectionInput{
			ObservationID: entry.ObservationID, ReviewToken: entry.ReviewToken, ZoneID: entry.ZoneID,
			StartLocal: start.Format("2006-01-02T15:04"), EndLocal: entry.EffectiveEndLocal[:16], Classification: "principal",
		})
		if err != nil {
			t.Fatal(err)
		}
		return edited
	}
	first := edit(night, recordedStart.Add(30*time.Minute))
	second := edit(first, recordedStart.Add(45*time.Minute))

	// Undoing the second edit restores the first.
	undone, err := app.UndoSleepCorrection(SleepUndoInput{ObservationID: second.ObservationID, ReviewToken: second.ReviewToken})
	if err != nil {
		t.Fatal(err)
	}
	if undone.EffectiveStartLocal[:16] != recordedStart.Add(30*time.Minute).Format("2006-01-02T15:04") || len(undone.History) != 3 || !undone.CanUndo {
		t.Fatalf("after one undo: starts %s with %d corrections", undone.EffectiveStartLocal, len(undone.History))
	}
	// Undoing that restores the night as recorded, and says so.
	restored, err := app.UndoSleepCorrection(SleepUndoInput{ObservationID: undone.ObservationID, ReviewToken: undone.ReviewToken})
	if err != nil {
		t.Fatal(err)
	}
	if restored.EffectiveStartLocal != restored.StartLocal || restored.Suppressed || len(restored.History) != 4 ||
		restored.History[0].Summary != "Restored the night as recorded" || restored.CanUndo {
		t.Fatalf("after two undos: %+v", restored)
	}
	// A stale form cannot undo.
	if _, err := app.UndoSleepCorrection(SleepUndoInput{ObservationID: undone.ObservationID, ReviewToken: undone.ReviewToken}); err == nil {
		t.Fatal("an undo from a stale review was accepted")
	}
}

func TestUndoingAnExclusionIncludesTheNightAgain(t *testing.T) {
	app := newTestApp(t)
	seedSleepEntries(t, app, 2)
	page, err := app.GetSleepLogPage(SleepLogPageInput{})
	if err != nil {
		t.Fatal(err)
	}
	excluded, err := app.SuppressSleepEntry(SleepSuppressInput{ObservationID: page.Entries[0].ObservationID, ReviewToken: page.Entries[0].ReviewToken})
	if err != nil || !excluded.Suppressed {
		t.Fatalf("excluded = %+v, %v", excluded.Suppressed, err)
	}
	undone, err := app.UndoSleepCorrection(SleepUndoInput{ObservationID: excluded.ObservationID, ReviewToken: excluded.ReviewToken})
	if err != nil || undone.Suppressed || undone.CanUndo {
		t.Fatalf("undoing the exclusion: %+v, %v", undone, err)
	}
}

// A night as recorded is the source's own latest word: undoing the owner's
// edit keeps a revision the provider made, rather than going back past it.
func TestUndoKeepsTheSourcesOwnRevision(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-36 * time.Hour).Truncate(time.Minute)
	observation := storage.SleepObservationRecord{
		ObservationID: "obs_provider_01",
		Kind:          storage.SleepKindEpisode,
		StartAt:       start,
		EndAt:         start.Add(8 * time.Hour),
		ZoneID:        defaultZoneID,
		Sleep:         storage.SleepObservationDetails{Classification: storage.SleepClassificationPrincipal},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionHealthConnect,
			EvidenceStatus:    storage.ProvenanceEvidenceDirectlyObserved,
			RecordedAt:        start.Add(8 * time.Hour),
			SourceRecordID:    "synthetic-provider-record",
		},
	}
	if err := app.store.AppendSleepObservation(ctx, observation); err != nil {
		t.Fatal(err)
	}
	revised := start.Add(20 * time.Minute)
	if err := app.store.AppendSleepCorrection(ctx, storage.SleepCorrectionRecord{
		CorrectionID: "corr_provider_01", TargetObservationID: observation.ObservationID,
		AcquisitionMethod: storage.ProvenanceAcquisitionHealthConnect, Reason: storage.CorrectionReasonSourceConflict,
		CreatedAt: start.Add(9 * time.Hour), Changes: storage.SleepCorrectionChanges{StartAt: &revised},
	}); err != nil {
		t.Fatal(err)
	}
	night, err := app.sleepEntryByID(observation.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation(defaultZoneID)
	edited, err := app.CorrectSleepEntry(SleepCorrectionInput{
		ObservationID: night.ObservationID, ReviewToken: night.ReviewToken, ZoneID: night.ZoneID,
		StartLocal: start.Add(50 * time.Minute).In(location).Format("2006-01-02T15:04"), EndLocal: night.EffectiveEndLocal[:16], Classification: "principal",
	})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := app.UndoSleepCorrection(SleepUndoInput{ObservationID: edited.ObservationID, ReviewToken: edited.ReviewToken})
	if err != nil {
		t.Fatal(err)
	}
	if restored.EffectiveStartLocal != night.EffectiveStartLocal || restored.CanUndo ||
		restored.History[0].Summary != "Restored the night as recorded" {
		t.Fatalf("undo went past the provider's revision: starts %s, want %s", restored.EffectiveStartLocal, night.EffectiveStartLocal)
	}
}
