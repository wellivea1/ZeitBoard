package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/platform/privatefile"
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

// The database holds every recorded sleep. Before this, it was created with
// os.Chmod-style mode bits that Windows ignores, so on a shared machine the
// file inherited whatever the profile directory allowed — measurably including
// SYSTEM and BUILTIN\Administrators.
func TestTheDatabaseFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zeitboard.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.FilePermissionError(); err != nil {
		t.Fatalf("the database permissions were not applied: %v", err)
	}

	// Migration writes, so the write-ahead log and shared-memory file already
	// exist by the time Open returns. They carry the same content as the
	// database and used to be left with whatever the directory allowed, so the
	// assertion covers all three — and nothing below re-applies the
	// restriction, because what is under test is that opening the store was
	// enough.
	if err := store.SetPendingSleep(context.Background(), PendingSleepRecord{
		StartedAt: time.Now().UTC(), ZoneID: "UTC",
	}); err != nil {
		t.Fatal(err)
	}

	found := 0
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if _, statErr := os.Stat(candidate); statErr != nil {
			continue
		}
		access, describeErr := privatefile.Describe(candidate)
		if describeErr != nil {
			t.Fatalf("describe %s: %v", filepath.Base(candidate), describeErr)
		}
		if !access.OwnerOnly {
			t.Errorf("%s is reachable by another account: %s", filepath.Base(candidate), access.Detail)
		}
		if access.Inherited {
			t.Errorf("%s still inherits from its directory: %s", filepath.Base(candidate), access.Detail)
		}
		found++
	}
	if found < 3 {
		t.Errorf("only %d of the three database files were present to check", found)
	}
}

// An in-memory store has no file to restrict, and must not report a failure for
// one that does not exist.
func TestAnInMemoryStoreNeedsNoPermissions(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.FilePermissionError(); err != nil {
		t.Errorf("in-memory store reported a permission failure: %v", err)
	}
}
