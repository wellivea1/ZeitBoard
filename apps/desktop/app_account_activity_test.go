package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"non24.app/core/inference"
	storage "non24.app/core/storage/sqlite"
)

// syntheticSleepStart is when synthetic night j begins: each night an hour
// later than the last, as a free-running rhythm drifts.
func syntheticSleepStart(night int) time.Time {
	return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(night) * 25 * time.Hour)
}

// writeSyntheticTakeout writes a Takeout .zip whose searches stop half an hour
// before each of nights 0 to 11 and start again twenty minutes after it ends,
// eight hours later. Every title is a canary that must never come back.
func writeSyntheticTakeout(t *testing.T) string {
	t.Helper()
	type entry struct {
		Header string `json:"header"`
		Title  string `json:"title"`
		Time   string `json:"time"`
	}
	var entries []entry
	add := func(at time.Time) {
		entries = append(entries, entry{Header: "Search", Title: "Searched for canary-terms", Time: at.Format(time.RFC3339)})
	}
	for wake := 0; wake <= 12; wake++ {
		from := syntheticSleepStart(wake-1).Add(8*time.Hour + 20*time.Minute)
		until := syntheticSleepStart(wake).Add(-30 * time.Minute)
		for at := from; at.Before(until); at = at.Add(15 * time.Minute) {
			add(at)
		}
		add(until)
	}
	contents, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "takeout-20260920T000000Z-001.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	writer, err := archive.Create("Takeout/My Activity/Search/MyActivity.json")
	if err == nil {
		_, err = writer.Write(contents)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func chooseActivityExport(t *testing.T, path string) {
	t.Helper()
	previous := openActivityExportDialog
	openActivityExportDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) { return path, nil }
	t.Cleanup(func() { openActivityExportDialog = previous })
}

func recordSyntheticNight(t *testing.T, app *App, night int) {
	t.Helper()
	start := syntheticSleepStart(night)
	if _, err := app.AddSleepEntry(SleepEntryInput{
		StartLocal: start.Format("2006-01-02T15:04"), EndLocal: start.Add(8 * time.Hour).Format("2006-01-02T15:04"),
		ZoneID: "UTC", Classification: storage.SleepClassificationPrincipal,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertNoCanary(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "canary") {
		t.Fatalf("what was searched came back: %s", encoded)
	}
}

// An export is previewed, kept, and compared with the nights recorded while
// it was kept, and nothing that was searched leaves the reader.
func TestAGoogleExportSuggestsNightsBesideRecordedOnes(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	for night := 6; night <= 12; night++ {
		recordSyntheticNight(t, app, night)
	}
	before, err := app.store.SleepPlanningFingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chooseActivityExport(t, writeSyntheticTakeout(t))

	preview, err := app.PreviewActivityExport()
	if err != nil {
		t.Fatal(err)
	}
	assertNoCanary(t, preview)
	if preview.Token == "" || preview.Nights != 12 || preview.Unusable != 0 || preview.Minutes != preview.Entries {
		t.Fatalf("preview %+v", preview)
	}
	if kept, err := app.GetAccountActivity(); err != nil || kept.Imported {
		t.Fatalf("a preview kept something: %+v, %v", kept, err)
	}

	kept, err := app.ImportActivityExport(AccountActivityImportInput{Token: preview.Token})
	if err != nil {
		t.Fatal(err)
	}
	assertNoCanary(t, kept)
	if !kept.Imported || kept.Nights != 12 || kept.Minutes != preview.Minutes || kept.Days < 12 {
		t.Fatalf("kept %+v", kept)
	}
	// Night 12 comes after the activity ends, so only 6 to 11 can be compared.
	agreement := kept.Agreement
	if agreement == nil || agreement.Recorded != 6 || agreement.Matched != 6 {
		t.Fatalf("agreement %+v", agreement)
	}
	if want := "On those nights, suggestions typically began 30 minutes before your recorded sleep and ended 20 minutes after your recorded wake."; agreement.Summary != want {
		t.Errorf("summary %q", agreement.Summary)
	}
	if len(kept.Suggested) < 12 || len(kept.Recorded) < 6 {
		t.Errorf("figure has %d suggested and %d recorded segments", len(kept.Suggested), len(kept.Recorded))
	}

	// The same preview cannot be kept twice.
	if _, err := app.ImportActivityExport(AccountActivityImportInput{Token: preview.Token}); err == nil {
		t.Error("a spent preview was kept again")
	}
	// Suggestions reach no estimate or plan.
	after, err := app.store.SleepPlanningFingerprint(context.Background())
	if err != nil || after != before {
		t.Errorf("keeping activity changed what plans are made from: %v", err)
	}
}

func TestRemovingImportedActivityTakesTheWordAndLeavesNothing(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }
	chooseActivityExport(t, writeSyntheticTakeout(t))
	preview, err := app.PreviewActivityExport()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportActivityExport(AccountActivityImportInput{Token: preview.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RemoveAccountActivity(AccountActivityRemoveInput{Confirmation: "remove"}); err == nil {
		t.Fatal("removed without the word")
	}
	removed, err := app.RemoveAccountActivity(AccountActivityRemoveInput{Confirmation: "REMOVE"})
	if err != nil || removed.Imported || removed.Nights != 0 || len(removed.Suggested) != 0 {
		t.Fatalf("after removal %+v, %v", removed, err)
	}
	minutes, err := app.store.AccountActivityMinutes(context.Background())
	if err != nil || len(minutes) != 0 {
		t.Fatalf("%d minutes left, %v", len(minutes), err)
	}
}

func TestAnExportAsWebPagesSaysHowToExportIt(t *testing.T) {
	app := newTestApp(t)
	path := filepath.Join(t.TempDir(), "MyActivity.html")
	if err := os.WriteFile(path, []byte("<html>canary</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	chooseActivityExport(t, path)
	_, err := app.PreviewActivityExport()
	if err == nil || !strings.Contains(err.Error(), "set My Activity's format to JSON") {
		t.Fatalf("an HTML export read as %v", err)
	}

	chooseActivityExport(t, "")
	canceled, err := app.PreviewActivityExport()
	if err != nil || !canceled.Canceled {
		t.Fatalf("closing the dialog read as %+v, %v", canceled, err)
	}
}

// Nights are cut at midnight where the reader is, through a change of clocks.
func TestNightsAreCutAtMidnight(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	local := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, location) }
	nights := []inference.TruthInterval{
		{StartAt: local(31, 23, 0), EndAt: local(32, 7, 30)}, // into November 1, when clocks go back
		{StartAt: local(30, 2, 15), EndAt: local(30, 9, 0)},
	}
	got := nightSegments(nights, civilDay(local(30, 12, 0), location), location)
	want := [][3]int{{1, 23 * 60, 24 * 60}, {2, 0, 7*60 + 30}, {0, 2*60 + 15, 9 * 60}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("segments %v, want %v", got, want)
	}
}

func TestEdgesAreWordedFromTheRecordedNight(t *testing.T) {
	for bias, want := range map[time.Duration]string{
		-30 * time.Minute:              "30 minutes before your recorded sleep",
		3 * time.Minute:                "at about your recorded sleep",
		time.Hour + 5*time.Minute + 20: "1 hour 5 minutes after your recorded sleep",
	} {
		if got := edgeWords(bias, "your recorded sleep"); got != want {
			t.Errorf("%s: %q, want %q", bias, got, want)
		}
	}
}
