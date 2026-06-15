package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"non24.app/core/domain"
)

func TestStoreKeepsObservationAndCorrectionSeparateAndDeletesAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "non24.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	observation := domain.SourceObservation{
		ID: "observation-1", SourceID: "wearable", ExternalID: "external-1", Kind: "sleep",
		ObservedAt: domain.MustZonedInstant(now, "UTC"), RecordedAt: now,
		Evidence: domain.Evidence{Acquisition: domain.AcquisitionImported, Status: domain.StatusImported},
		Payload:  json.RawMessage(`{"start":"synthetic"}`),
	}
	if err := store.AppendObservation(ctx, observation); err != nil {
		t.Fatal(err)
	}
	corrected := domain.MustZonedInstant(now.Add(time.Hour), "UTC")
	correction := domain.ManualCorrection{ID: "correction-1", TargetID: "sleep-1", Kind: domain.CorrectionSetSleepStart, InstantValue: &corrected, Active: true, CreatedAt: now}
	if err := store.AddCorrection(ctx, correction); err != nil {
		t.Fatal(err)
	}
	bundle, err := store.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Observations) != 1 || len(bundle.Corrections) != 1 {
		t.Fatalf("export counts = observations %d, corrections %d", len(bundle.Observations), len(bundle.Corrections))
	}
	if string(bundle.Observations[0].Payload) != string(observation.Payload) {
		t.Fatal("source observation was changed by correction storage")
	}
	if err := store.DeleteAll(ctx); err != nil {
		t.Fatal(err)
	}
	bundle, err = store.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Observations)+len(bundle.Corrections)+len(bundle.Estimates)+len(bundle.MedicationEvents)+len(bundle.ShareProfiles) != 0 {
		t.Fatalf("data remains after delete: %#v", bundle)
	}
}
