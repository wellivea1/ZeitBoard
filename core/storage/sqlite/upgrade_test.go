package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A database made by an earlier build opens in this one with exactly the
// schema a new database gets, and keeps its records. The store upgrades by
// creating what is missing (CREATE ... IF NOT EXISTS) and by explicit drops,
// which is sound only while nothing that exists changes shape; this is the
// test that notices if something does.
//
// testdata/upgrade-from-2026-09-08.sql was written by the store as it was on
// 2026-09-08 (commit 465c26e), the build the completion plan started from,
// with synthetic records made through that store's own methods.
func TestADatabaseFromAnEarlierBuildUpgradesToTheCurrentSchema(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "upgrade-from-2026-09-08.sql"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	earlierPath := filepath.Join(dir, "earlier.db")
	earlier, err := sql.Open("sqlite", earlierPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := earlier.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	if err := earlier.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(earlierPath)
	if err != nil {
		t.Fatalf("an earlier build's database did not open: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	fresh, err := Open(filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	got, want := schemaOf(t, upgraded), schemaOf(t, fresh)
	for name, sql := range want {
		if got[name] != sql {
			t.Errorf("%s upgraded as\n  %s\nbut a new database has\n  %s", name, got[name], sql)
		}
	}
	for name := range got {
		if _, found := want[name]; !found {
			t.Errorf("%s is left over from the earlier build", name)
		}
	}

	// Every record survives, and reads as it did.
	ctx := context.Background()
	reviews, total, err := upgraded.ReadSleepReviewPage(ctx, 0, 10)
	if err != nil || total != 2 {
		t.Fatalf("sleep = %d nights, %v", total, err)
	}
	corrected := reviews[0].Effective.Intervals[0].Interval.Start.UTC
	if want := time.Date(2026, 9, 7, 4, 30, 0, 0, time.UTC); !corrected.Equal(want) {
		t.Fatalf("the corrected night starts at %s, want %s", corrected, want)
	}
	tasks, err := upgraded.ListTasks(ctx)
	if err != nil || len(tasks) != 1 || tasks[0].Title != "Synthetic upgrade task" {
		t.Fatalf("tasks = %+v, %v", tasks, err)
	}
	medications, err := upgraded.ListMedications(ctx)
	if err != nil || len(medications) != 1 {
		t.Fatalf("medications = %+v, %v", medications, err)
	}
	doses, err := upgraded.EffectiveMedicationEvents(ctx)
	if err != nil || len(doses) != 1 || doses[0].Event.Status != MedicationEventSkipped || len(doses[0].Corrections) != 1 {
		t.Fatalf("doses = %+v, %v", doses, err)
	}
	markers, err := upgraded.ListRhythmMarkers(ctx)
	if err != nil || len(markers) != 1 {
		t.Fatalf("markers = %+v, %v", markers, err)
	}
	sources, err := upgraded.ListCalendarSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range sources {
		found = found || source.SourceID == "calendar_source_import_01"
	}
	if !found {
		t.Fatalf("the imported calendar is gone: %+v", sources)
	}

	// And it takes new records like any other.
	next := testSleepObservation("obs_sleep_after_upgrade", time.Date(2026, 9, 8, 4, 0, 0, 0, time.UTC), time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if err := upgraded.AppendSleepObservation(ctx, next); err != nil {
		t.Fatal(err)
	}
}

// schemaOf is every table, index, trigger and view, by kind and name, with
// its SQL's whitespace folded.
func schemaOf(t *testing.T, store *Store) map[string]string {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `SELECT type, name, sql FROM sqlite_master WHERE sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	schema := map[string]string{}
	for rows.Next() {
		var kind, name, text string
		if err := rows.Scan(&kind, &name, &text); err != nil {
			t.Fatal(err)
		}
		schema[kind+" "+name] = strings.Join(strings.Fields(text), " ")
	}
	return schema
}
