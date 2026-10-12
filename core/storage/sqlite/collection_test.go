package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
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
