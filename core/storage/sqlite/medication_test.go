package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testMedicationRecord(now time.Time) MedicationRecord {
	return MedicationRecord{
		MedicationID:  "med_local_01",
		Label:         "Private medication label",
		Form:          "tablet",
		StrengthLabel: "user-entered strength",
		Active:        true,
		Schedule:      &MedicationSchedule{Kind: MedicationScheduleAsNeeded},
		CreatedAt:     now,
		Revision:      1,
		UpdatedAt:     now,
	}
}

func testMedicationEvent(now time.Time) MedicationEventRecord {
	return MedicationEventRecord{
		EventID:      "dose_local_01",
		MedicationID: "med_local_01",
		DoseAt:       now.Add(-time.Hour),
		ZoneID:       "America/New_York",
		Status:       MedicationEventTaken,
		Scheduled:    false,
		Note:         "Private event note",
		Provenance: SleepObservationProvenance{
			AcquisitionMethod: ProvenanceAcquisitionManual,
			EvidenceStatus:    ProvenanceEvidenceUserReported,
			RecordedAt:        now,
		},
	}
}

func TestMedicationDefinitionsEventsAndCorrectionsPreserveRawEvidence(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	medication := testMedicationRecord(now)
	medication.ClinicianRule = "Clinician instruction entered verbatim by the user"
	if err := store.CreateMedication(ctx, medication); err != nil {
		t.Fatal(err)
	}

	updated := medication
	updated.Label = "Renamed private medication"
	updated.Active = false
	updated.Revision = 2
	updated.UpdatedAt = now.Add(time.Minute)
	if err := store.UpdateMedication(ctx, updated, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMedication(ctx, updated, 1); !errors.Is(err, ErrMedicationRevisionConflict) {
		t.Fatalf("stale definition update error = %v", err)
	}

	event := testMedicationEvent(now)
	if err := store.AppendMedicationEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE local_medication_events SET status = 'skipped' WHERE event_id = ?`, event.EventID); err == nil {
		t.Fatal("database allowed a medication event to be edited in place")
	}
	note := "Corrected private note"
	status := MedicationEventSkipped
	firstCorrection := MedicationEventCorrectionRecord{
		CorrectionID:  "medcorr_local_01",
		TargetEventID: event.EventID,
		CreatedAt:     now.Add(time.Minute),
		Reason:        MedicationCorrectionUserEdit,
		Changes: MedicationEventCorrectionChanges{
			Status: &status,
			Note:   &note,
		},
	}
	if err := store.AppendMedicationEventCorrection(ctx, firstCorrection); err != nil {
		t.Fatal(err)
	}
	invalidZone := firstCorrection
	invalidZone.CorrectionID = "medcorr_invalid_zone"
	invalidZone.SupersedesCorrectionID = firstCorrection.CorrectionID
	invalidZone.CreatedAt = now.Add(2 * time.Minute)
	invalidZone.Changes = MedicationEventCorrectionChanges{ZoneID: stringPointer("Not/A_Real_Zone")}
	if err := store.AppendMedicationEventCorrection(ctx, invalidZone); err == nil {
		t.Fatal("correction with an invalid time zone was accepted")
	}
	stale := firstCorrection
	stale.CorrectionID = "medcorr_local_stale"
	stale.CreatedAt = now.Add(2 * time.Minute)
	if err := store.AppendMedicationEventCorrection(ctx, stale); !errors.Is(err, ErrMedicationCorrectionConflict) {
		t.Fatalf("stale correction error = %v", err)
	}
	excluded := true
	secondCorrection := MedicationEventCorrectionRecord{
		CorrectionID:           "medcorr_local_02",
		TargetEventID:          event.EventID,
		SupersedesCorrectionID: firstCorrection.CorrectionID,
		CreatedAt:              now.Add(2 * time.Minute),
		Reason:                 MedicationCorrectionDuplicate,
		Changes:                MedicationEventCorrectionChanges{Excluded: &excluded},
	}
	if err := store.AppendMedicationEventCorrection(ctx, secondCorrection); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE local_medication_event_corrections SET reason = 'invalid_time' WHERE correction_id = ?`, firstCorrection.CorrectionID); err == nil {
		t.Fatal("database allowed a medication correction to be edited in place")
	}

	effective, err := store.EffectiveMedicationEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 1 || effective[0].Event.Status != MedicationEventSkipped || effective[0].Event.Note != note || !effective[0].Excluded || len(effective[0].Corrections) != 2 {
		t.Fatalf("effective events = %#v", effective)
	}
	exported, err := store.ExportMedicationData(ctx, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.MedicationSet.Medications) != 1 || len(exported.EventSet.Events) != 1 || len(exported.EventSet.Corrections) != 2 {
		t.Fatalf("export = %#v", exported)
	}
	if exported.SchemaVersion != "v2" || exported.MedicationSet.SchemaVersion != "v2" || exported.EventSet.SchemaVersion != "v2" {
		t.Fatalf("schedule-capable export versions = %#v", exported)
	}
	if exported.MedicationSet.Medications[0].ClinicianRule != medication.ClinicianRule {
		t.Fatal("export did not preserve the user-entered clinician rule")
	}
	if exported.EventSet.Events[0].Status != MedicationEventTaken || exported.EventSet.Events[0].Note != event.Note {
		t.Fatal("export rewrote raw medication evidence instead of preserving corrections")
	}
	encoded, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"medications":[`)) || !bytes.Contains(encoded, []byte(`"corrections":[`)) {
		t.Fatal("contract export did not retain array fields")
	}
}

func TestMedicationErasureRemovesPrivateBytesFromDatabaseAndWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "medication-erasure.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	medicationMarker := "medication-erasure-marker-23f68a19"
	eventMarker := "event-erasure-marker-9c8d0412"
	medication := testMedicationRecord(now)
	medication.Label = medicationMarker
	medication.Form = "form-" + medicationMarker
	medication.StrengthLabel = "strength-" + medicationMarker
	if err := store.CreateMedication(ctx, medication); err != nil {
		t.Fatal(err)
	}
	event := testMedicationEvent(now)
	event.Note = "note-" + eventMarker
	if err := store.AppendMedicationEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	note := "correction-" + eventMarker
	if err := store.AppendMedicationEventCorrection(ctx, MedicationEventCorrectionRecord{
		CorrectionID:  "medcorr_erasure_01",
		TargetEventID: event.EventID,
		CreatedAt:     now.Add(time.Minute),
		Reason:        MedicationCorrectionUserEdit,
		Changes:       MedicationEventCorrectionChanges{Note: &note},
	}); err != nil {
		t.Fatal(err)
	}

	readFiles := func() ([]byte, []byte) {
		t.Helper()
		databaseBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		walBytes, err := os.ReadFile(path + "-wal")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return databaseBytes, walBytes
	}
	databaseBytes, walBytes := readFiles()
	if !bytes.Contains(databaseBytes, []byte(eventMarker)) && !bytes.Contains(walBytes, []byte(eventMarker)) {
		t.Fatal("medication marker was not persisted before the erasure check")
	}
	if err := store.DeleteMedicationEvent(ctx, event.EventID); err != nil {
		t.Fatal(err)
	}
	databaseBytes, walBytes = readFiles()
	if bytes.Contains(databaseBytes, []byte(eventMarker)) || bytes.Contains(walBytes, []byte(eventMarker)) {
		t.Fatal("deleted event or correction data remains in the SQLite database or WAL")
	}
	if !bytes.Contains(databaseBytes, []byte(medicationMarker)) && !bytes.Contains(walBytes, []byte(medicationMarker)) {
		t.Fatal("event erasure also removed the medication definition")
	}
	exported, err := store.ExportMedicationData(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.MedicationSet.Medications) != 1 || len(exported.EventSet.Events) != 0 || len(exported.EventSet.Corrections) != 0 {
		t.Fatalf("event erasure left event data or removed its definition: %#v", exported)
	}
	if err := store.DeleteMedication(ctx, medication.MedicationID); err != nil {
		t.Fatal(err)
	}
	databaseBytes, walBytes = readFiles()
	if bytes.Contains(databaseBytes, []byte(medicationMarker)) {
		t.Fatal("deleted medication data remains in the compacted SQLite database")
	}
	if bytes.Contains(walBytes, []byte(medicationMarker)) {
		t.Fatal("deleted medication data remains in the SQLite WAL")
	}
	exported, err = store.ExportMedicationData(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.MedicationSet.Medications) != 0 || len(exported.EventSet.Events) != 0 || len(exported.EventSet.Corrections) != 0 {
		t.Fatalf("erased data remained in export: %#v", exported)
	}
}

func stringPointer(value string) *string { return &value }

func TestMedicationScheduleValidationRequiresExplicitUserAuthoredRules(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	record := testMedicationRecord(now)
	record.Schedule = &MedicationSchedule{Kind: MedicationScheduleFixedClock}
	if err := validateMedicationRecord(record); err == nil {
		t.Fatal("fixed-clock schedule without user-entered civil times was accepted")
	}
	record.Schedule = &MedicationSchedule{Kind: MedicationScheduleAsNeeded, CivilTimes: []string{"22:00"}}
	if err := validateMedicationRecord(record); err == nil {
		t.Fatal("as-needed schedule with an inferred clock time was accepted")
	}
	record.Schedule = &MedicationSchedule{Kind: MedicationScheduleFixedClock, CivilTimes: []string{"22:00", "22:00"}}
	if err := validateMedicationRecord(record); err == nil {
		t.Fatal("duplicate fixed-clock times were accepted")
	}
	record.Schedule = &MedicationSchedule{Kind: MedicationScheduleAsNeeded, ReminderEnabled: true}
	if err := validateMedicationRecord(record); err == nil {
		t.Fatal("as-needed schedule with a timed reminder was accepted")
	}
	record.Schedule = &MedicationSchedule{Kind: MedicationScheduleFixedClock, ZoneID: "Local", CivilTimes: []string{"22:00"}}
	if err := validateMedicationRecord(record); err == nil {
		t.Fatal("machine-local pseudo-zone was accepted")
	}
	record.Schedule = &MedicationSchedule{
		Kind:            MedicationScheduleFixedClock,
		ZoneID:          "America/New_York",
		CivilTimes:      []string{"22:00", "08:00"},
		ReminderEnabled: true,
	}
	record.ClinicianRule = "Use the schedule provided by my clinician"
	if err := validateMedicationRecord(record); err != nil {
		t.Fatalf("valid user-authored schedule was rejected: %v", err)
	}
	normalized := normalizeMedicationRecord(record)
	if normalized.Schedule.CivilTimes[0] != "08:00" || normalized.Schedule.CivilTimes[1] != "22:00" {
		t.Fatalf("schedule times were not normalized: %#v", normalized.Schedule.CivilTimes)
	}
}

func TestMedicationReminderClaimsAreAtMostOnceAndCascadeOnErasure(t *testing.T) {
	store, ctx := openCalendarTestStore(t)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	medication := testMedicationRecord(now)
	if err := store.CreateMedication(ctx, medication); err != nil {
		t.Fatal(err)
	}
	claim := MedicationReminderClaim{
		OccurrenceID: "reminder_occurrence_01",
		MedicationID: medication.MedicationID,
		ScheduledAt:  now,
		ClaimedAt:    now.Add(10 * time.Second),
	}
	claimed, err := store.ClaimMedicationReminder(ctx, claim)
	if err != nil || !claimed {
		t.Fatalf("first claim = %t, %v", claimed, err)
	}
	claimed, err = store.ClaimMedicationReminder(ctx, claim)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = %t, %v", claimed, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE local_medication_reminder_claims
		SET claimed_at = ? WHERE occurrence_id = ?`, formatSQLiteTime(now.Add(time.Minute)), claim.OccurrenceID); err == nil {
		t.Fatal("database allowed a reminder claim to be edited in place")
	}
	reused := claim
	reused.ScheduledAt = now.Add(time.Hour)
	reused.ClaimedAt = reused.ScheduledAt
	if _, err := store.ClaimMedicationReminder(ctx, reused); err == nil {
		t.Fatal("claim identifier was reused for a different occurrence")
	}
	if err := store.DeleteMedication(ctx, medication.MedicationID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_medication_reminder_claims`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("reminder claims remained after medication erasure: %d", count)
	}
	if _, err := store.ClaimMedicationReminder(ctx, claim); !errors.Is(err, ErrMedicationNotFound) {
		t.Fatalf("claim for erased medication error = %v", err)
	}
}
