package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	syncmodel "non24.app/server/internal/sync"
)

// Payload builders for the medication kinds (ADR-0048). The store does not
// validate payloads (the API does); these are shaped like the real records so
// the append path's routing reads what it would read in production.
func medicationRevision(id string, revision int) syncmodel.PushRecord {
	return syncmodel.PushRecord{
		RecordID: syncmodel.MedicationRevisionID(id, revision), Kind: syncmodel.KindMedication,
		CreatedAt: time.Date(2026, 9, 2, 12, 0, revision, 0, time.UTC),
		Payload: json.RawMessage(fmt.Sprintf(`{"medication_id":%q,"label":"canary-medication-label","active":true,"created_at":"2026-09-01T12:00:00Z","revision":%d,"updated_at":"2026-09-02T12:00:00Z"}`, id, revision)),
	}
}

func medicationDose(eventID, medicationID string) syncmodel.PushRecord {
	return syncmodel.PushRecord{
		RecordID: eventID, Kind: syncmodel.KindMedicationEvent,
		CreatedAt: time.Date(2026, 9, 2, 12, 1, 0, 0, time.UTC),
		Payload: json.RawMessage(fmt.Sprintf(`{"event_id":%q,"medication_id":%q,"dose_at":"2026-09-02T02:00:00Z","zone_id":"UTC","status":"taken","scheduled":true,"provenance":{"acquisition_method":"manual","evidence_status":"user_reported","recorded_at":"2026-09-02T02:01:00Z"}}`, eventID, medicationID)),
	}
}

func medicationFix(correctionID, eventID string) syncmodel.PushRecord {
	return syncmodel.PushRecord{
		RecordID: correctionID, Kind: syncmodel.KindMedicationCorrection,
		CreatedAt: time.Date(2026, 9, 2, 12, 2, 0, 0, time.UTC),
		Payload: json.RawMessage(fmt.Sprintf(`{"correction_id":%q,"target_event_id":%q,"created_at":"2026-09-02T03:00:00Z","reason":"user_edit","changes":{"status":"skipped"}}`, correctionID, eventID)),
	}
}

func openMedicationStore(t *testing.T) (*Store, context.Context, time.Time) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(t.TempDir()+"/server.db", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 9, 2, 13, 0, 0, 0, time.UTC)
	if err := st.RegisterDevice(ctx, "device_synthetic", "synthetic", bytes.Repeat([]byte{1}, 32), now); err != nil {
		t.Fatal(err)
	}
	return st, ctx, now
}

func liveRecordIDs(t *testing.T, st *Store) []string {
	t.Helper()
	rows, err := st.db.Query(`SELECT record_id FROM sync_records WHERE kind != 'tombstone' ORDER BY record_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Deleting a medication deletes all of it — every revision, every recorded
// dose and every correction of one — and nothing later from an offline device
// brings any of it back. Another medication whose id merely begins with the
// same letters is untouched.
func TestErasingAMedicationTakesItsRevisionsDosesAndCorrections(t *testing.T) {
	st, ctx, now := openMedicationStore(t)
	if _, _, err := st.Append(ctx, "device_synthetic", []syncmodel.PushRecord{
		medicationRevision("med_evening", 1),
		medicationRevision("med_evening", 2),
		medicationDose("dose_one", "med_evening"),
		medicationFix("medcor_one", "dose_one"),
		medicationRevision("med_evening_rival", 1),
		medicationDose("dose_rival", "med_evening_rival"),
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := st.EraseSyncRecords(ctx, "device_synthetic", []string{"med_evening_r2"}, now); err != nil {
		t.Fatal(err)
	}
	if got := liveRecordIDs(t, st); fmt.Sprint(got) != "[dose_rival med_evening_rival_r1]" {
		t.Fatalf("after erasing the medication, retained = %v", got)
	}

	// An offline device's later revision, dose or correction is refused.
	_, accepted, err := st.Append(ctx, "device_synthetic", []syncmodel.PushRecord{
		medicationRevision("med_evening", 3),
		medicationDose("dose_late", "med_evening"),
		medicationFix("medcor_late", "dose_one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted != 0 {
		t.Errorf("an erased medication accepted %d later records", accepted)
	}
}

func TestErasingADoseTakesItsCorrections(t *testing.T) {
	st, ctx, now := openMedicationStore(t)
	if _, _, err := st.Append(ctx, "device_synthetic", []syncmodel.PushRecord{
		medicationRevision("med_morning", 1),
		medicationDose("dose_two", "med_morning"),
		medicationFix("medcor_two", "dose_two"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.EraseSyncRecords(ctx, "device_synthetic", []string{"dose_two"}, now); err != nil {
		t.Fatal(err)
	}
	if got := liveRecordIDs(t, st); fmt.Sprint(got) != "[med_morning_r1]" {
		t.Fatalf("after erasing the dose, retained = %v", got)
	}
}
