package readmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"non24.app/core/medication"
	"non24.app/server/internal/auth"
	"non24.app/server/internal/store"
	syncmodel "non24.app/server/internal/sync"
)

func testDosePayload(id string, at time.Time, status string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"event_id":%q,"medication_id":"med_synthetic","dose_at":%q,"zone_id":"UTC","status":%q,"scheduled":true,"provenance":{"acquisition_method":"manual","evidence_status":"user_reported","recorded_at":%q}}`,
		id, at.Format(time.RFC3339), status, at.Format(time.RFC3339)))
}

func testDoseCorrectionPayload(id, target string, at time.Time, changes string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"correction_id":%q,"target_event_id":%q,"created_at":%q,"reason":"user_edit","changes":%s}`,
		id, target, at.Format(time.RFC3339), changes))
}

// The server reads a dose as the desktop does: a skip corrected to taken is
// taken, and an exclusion stays with the dose (ADR-0058).
func TestEffectiveDosesApplyCorrections(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "zeitboardd.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RegisterDevice(ctx, "dev_test", "desktop", auth.HashToken("token"), time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	night := time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC)
	req := syncmodel.PushRequest{
		SchemaVersion: syncmodel.SchemaVersion,
		Records: []syncmodel.PushRecord{
			testPushRecord("dose_corrected", syncmodel.KindMedicationEvent, testDosePayload("dose_corrected", night, medication.EventSkipped)),
			testPushRecord("dose_excluded", syncmodel.KindMedicationEvent, testDosePayload("dose_excluded", night.Add(24*time.Hour), medication.EventSkipped)),
			testPushRecord("dose_kept", syncmodel.KindMedicationEvent, testDosePayload("dose_kept", night.Add(48*time.Hour), medication.EventSkipped)),
			testPushRecord("fix_taken", syncmodel.KindMedicationCorrection,
				testDoseCorrectionPayload("fix_taken", "dose_corrected", night.Add(time.Hour), `{"status":"taken"}`)),
			testPushRecord("fix_excluded", syncmodel.KindMedicationCorrection,
				testDoseCorrectionPayload("fix_excluded", "dose_excluded", night.Add(25*time.Hour), `{"excluded":true}`)),
		},
	}
	if err := syncmodel.ValidatePushRequest(&req); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Append(ctx, "dev_test", req.Records); err != nil {
		t.Fatal(err)
	}

	doses, err := (MedicationReader{Store: st}).EffectiveDoses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(doses) != 3 {
		t.Fatalf("doses %+v", doses)
	}
	if doses[0].Event.EventID != "dose_corrected" || doses[0].Event.Status != medication.EventTaken {
		t.Errorf("the corrected dose reads %+v", doses[0].Event)
	}
	if !doses[1].Excluded {
		t.Error("the exclusion was lost")
	}
	if doses[2].Event.Status != medication.EventSkipped || doses[2].Excluded {
		t.Errorf("the untouched dose reads %+v", doses[2])
	}
}
