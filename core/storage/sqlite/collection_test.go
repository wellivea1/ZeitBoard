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
