package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestCollectionBatchRollsBackAndExportIsSourceScoped(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	row := domain.SourceObservation{ID: "synthetic-1", SourceID: "desktop-activity", Kind: "activity",
		ObservedAt: domain.MustZonedInstant(time.Now(), "UTC"), RecordedAt: time.Now(), Payload: json.RawMessage(`{"state":"startup"}`)}
	if err := store.Append(ctx, []domain.SourceObservation{row, row}); err == nil {
		t.Fatal("duplicate batch succeeded")
	}
	if n, _ := store.SourceObservationCount(ctx, row.SourceID); n != 0 {
		t.Fatal("partial batch was committed")
	}
	other := row
	other.ID = "synthetic-2"
	other.SourceID = "other"
	if err := store.Append(ctx, []domain.SourceObservation{row, other}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if n, err := store.WriteSourceObservations(ctx, row.SourceID, &out); err != nil || n != 1 {
		t.Fatalf("export: %d %v", n, err)
	}
	if !json.Valid(out.Bytes()) || bytes.Contains(out.Bytes(), []byte("synthetic-2")) {
		t.Fatalf("invalid source scope: %s", out.Bytes())
	}
	if err := store.DeleteSourceObservations(ctx, row.SourceID); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.SourceObservationCount(ctx, "other"); n != 1 {
		t.Fatal("erased another source")
	}
}

// A record the operating system hands over twice is kept once, and reading a
// source back gives its observations in time order with the newest log time.
func TestARecordReadTwiceIsKeptOnce(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	at := time.Date(2026, 10, 11, 6, 12, 0, 0, time.UTC)
	logged := func(id string, when time.Time) domain.SourceObservation {
		return domain.SourceObservation{ID: domain.ObservationID("power-log-" + id), SourceID: "desktop-activity",
			ExternalID: "windows-power-log:" + id, Kind: "activity", ObservedAt: domain.MustZonedInstant(when, "UTC"),
			RecordedAt: at, Evidence: domain.Evidence{Algorithm: "windows-power-log-v1"},
			Payload: json.RawMessage(`{"state":"suspended"}`)}
	}
	first := []domain.SourceObservation{logged("1", at.Add(time.Hour)), logged("2", at)}
	if added, err := store.AppendNew(ctx, first); err != nil || added != 2 {
		t.Fatalf("first read added %d, %v", added, err)
	}
	if added, err := store.AppendNew(ctx, append(first, logged("3", at.Add(2*time.Hour)))); err != nil || added != 1 {
		t.Fatalf("second read added %d, %v", added, err)
	}
	collected := domain.SourceObservation{ID: "activity-synthetic-1", SourceID: "desktop-activity", Kind: "activity",
		ObservedAt: domain.MustZonedInstant(at.Add(3*time.Hour), "UTC"), RecordedAt: at,
		Evidence: domain.Evidence{Algorithm: "activity-v3"}, Payload: json.RawMessage(`{"state":"idle"}`)}
	if err := store.Append(ctx, []domain.SourceObservation{collected}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendNew(ctx, []domain.SourceObservation{collected}); err == nil {
		t.Fatal("an observation without an external ID was kept as if it had one")
	}

	observations, err := store.SourceObservations(ctx, "desktop-activity")
	if err != nil || len(observations) != 4 {
		t.Fatalf("read %d observations, %v", len(observations), err)
	}
	if observations[0].ID != "power-log-2" || observations[3].ID != "activity-synthetic-1" {
		t.Errorf("order %s … %s", observations[0].ID, observations[3].ID)
	}
	fromLog, err := store.SourceObservationSpan(ctx, "desktop-activity", "windows-power-log-v1")
	if err != nil || fromLog.Count != 3 || !fromLog.Oldest.Equal(at) || !fromLog.Latest.Equal(at.Add(2*time.Hour)) {
		t.Errorf("logged span %+v, %v", fromLog, err)
	}
	if all, err := store.SourceObservationSpan(ctx, "desktop-activity", ""); err != nil || all.Count != 4 || !all.Latest.Equal(at.Add(3*time.Hour)) {
		t.Errorf("whole span %+v, %v", all, err)
	}
	if none, err := store.SourceObservationSpan(ctx, "desktop-activity", "nothing-v1"); err != nil || none.Count != 0 || !none.Latest.IsZero() {
		t.Errorf("an algorithm with nothing kept gave %+v, %v", none, err)
	}
}

// Every read of desktop activity's status spans what was read from Windows'
// log, and Data Sources reads it each minute. That span searches its own index
// rather than parse the evidence of every observation the desktop has kept.
func TestOneAlgorithmsSpanReadsItsIndex(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	spanReadsItsIndex(t, store)
}

// A database from the build before the index holds observations and no index
// of their algorithm. Opening it builds one over what is kept.
func TestOpeningBuildsTheIndexOverWhatWasKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "earlier.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 10, 11, 6, 12, 0, 0, time.UTC)
	kept := []domain.SourceObservation{
		{ID: "power-log-1", SourceID: "desktop-activity", ExternalID: "windows-power-log:1", Kind: "activity",
			ObservedAt: domain.MustZonedInstant(at, "UTC"), RecordedAt: at,
			Evidence: domain.Evidence{Algorithm: "windows-power-log-v1"}, Payload: json.RawMessage(`{"state":"suspended"}`)},
		{ID: "activity-synthetic-1", SourceID: "desktop-activity", Kind: "activity",
			ObservedAt: domain.MustZonedInstant(at.Add(time.Hour), "UTC"), RecordedAt: at,
			Evidence: domain.Evidence{Algorithm: "activity-v3"}, Payload: json.RawMessage(`{"state":"idle"}`)},
	}
	if err := store.Append(ctx, kept); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP INDEX idx_source_observations_algorithm`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("a database holding observations did not open: %v", err)
	}
	defer reopened.Close()
	if span, err := reopened.SourceObservationSpan(ctx, "desktop-activity", "windows-power-log-v1"); err != nil || span.Count != 1 || !span.Latest.Equal(at) {
		t.Errorf("span of what was kept before %+v, %v", span, err)
	}
	spanReadsItsIndex(t, reopened)
}

// Evidence SQLite cannot parse, which no build writes but a damaged file could
// hold, leaves the index unbuilt rather than the database unopened.
func TestEvidenceSQLiteCannotParseStillOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "damaged.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, statement := range []string{
		`DROP INDEX idx_source_observations_algorithm`,
		`INSERT INTO source_observations(
			id, source_id, kind, observed_utc, zone_id, recorded_at, evidence_json, payload_json
		) VALUES('damaged-1', 'desktop-activity', 'activity', '2026-10-11T06:12:00Z', 'UTC',
			'2026-10-11T06:12:00Z', CAST('{"algorithm":' AS BLOB), CAST('{}' AS BLOB))`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("one damaged evidence kept the database from opening: %v", err)
	}
	defer reopened.Close()
	if span, err := reopened.SourceObservationSpan(ctx, "desktop-activity", ""); err != nil || span.Count != 1 {
		t.Errorf("whole span %+v, %v", span, err)
	}
}

// spanReadsItsIndex fails the test unless SQLite answers one algorithm's span
// from idx_source_observations_algorithm alone.
func spanReadsItsIndex(t *testing.T, store *Store) {
	t.Helper()
	query, args := spanQuery("desktop-activity", "windows-power-log-v1")
	rows, err := store.db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if len(plan) != 1 || !strings.Contains(plan[0], "COVERING INDEX idx_source_observations_algorithm") {
		t.Fatalf("plan %q", plan)
	}
}
