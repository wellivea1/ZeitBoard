package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
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
