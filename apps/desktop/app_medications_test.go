package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

func TestMedicationLoggingUsesRealSleepContextAndAppendOnlyCorrections(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Now().UTC().Truncate(time.Minute)
	app.nowFn = func() time.Time { return fixedNow }
	seedSleepEntries(t, app, 12)

	initial, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Empty || initial.FixtureMode || len(initial.Medications) != 0 || len(initial.Events) != 0 {
		t.Fatalf("initial medications = %#v", initial)
	}
	created, err := app.AddMedication(MedicationInput{
		Label:         "Private test label",
		Form:          "tablet",
		StrengthLabel: "user-entered strength",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Empty || len(created.Medications) != 1 || created.Medications[0].ScheduleKind != "none" {
		t.Fatalf("created medications = %#v", created)
	}
	medicationID := created.Medications[0].MedicationID
	location := locationOrUTC(defaultZoneID)
	doseLocal := fixedNow.Add(-2 * time.Hour).In(location).Format("2006-01-02T15:04")
	logged, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medicationID,
		DoseLocal:    doseLocal,
		ZoneID:       defaultZoneID,
		Status:       storage.MedicationEventTaken,
		Scheduled:    true,
		Note:         "user-entered note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(logged.Events) != 1 || logged.Events[0].MedicationLabel != "Private test label" || logged.Events[0].Status != storage.MedicationEventTaken {
		t.Fatalf("logged event = %#v", logged.Events)
	}
	if !strings.Contains(logged.Events[0].WakeRelation, "after recorded wake") || logged.Events[0].SleepRelationKind != "predicted" {
		t.Fatalf("rhythm context = %#v", logged.Events[0])
	}
	event := logged.Events[0]
	corrected, err := app.CorrectMedicationEvent(MedicationEventCorrectionInput{
		EventID:   event.EventID,
		DoseLocal: event.DoseLocal,
		ZoneID:    event.ZoneID,
		Status:    storage.MedicationEventSkipped,
		Scheduled: event.Scheduled,
		Note:      "corrected note",
		Excluded:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(corrected.Events) != 1 || corrected.Events[0].Status != storage.MedicationEventSkipped || corrected.Events[0].Note != "corrected note" || !corrected.Events[0].Excluded || corrected.Events[0].CorrectionCount != 1 {
		t.Fatalf("corrected event = %#v", corrected.Events)
	}
	if corrected.Medications[0].EventCount != 1 {
		t.Fatalf("excluded evidence changed the stored event count: %#v", corrected.Medications[0])
	}

	updated, err := app.UpdateMedication(MedicationUpdateInput{
		MedicationID:  medicationID,
		Revision:      corrected.Medications[0].Revision,
		Label:         "Renamed private label",
		Form:          "capsule",
		StrengthLabel: "revised user label",
		Active:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Medications[0].Active || updated.Medications[0].Revision != 2 || updated.Events[0].MedicationLabel != "Renamed private label" {
		t.Fatalf("updated medication = %#v", updated)
	}

	exported, err := app.ExportMedicationData()
	if err != nil {
		t.Fatal(err)
	}
	if exported.MedicationCount != 1 || exported.EventCount != 1 || !strings.HasSuffix(exported.FileName, ".json") {
		t.Fatalf("export = %#v", exported)
	}
	var payload storage.MedicationDataExport
	if err := json.Unmarshal([]byte(exported.JSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.EventSet.Events[0].Status != storage.MedicationEventTaken || len(payload.EventSet.Corrections) != 1 {
		t.Fatal("export did not preserve raw event plus append-only correction")
	}

	if _, err := app.DeleteMedicationEvent(MedicationEventDeleteInput{EventID: event.EventID, Confirmation: "delete"}); err == nil {
		t.Fatal("event erasure accepted the wrong confirmation")
	}
	withoutEvent, err := app.DeleteMedicationEvent(MedicationEventDeleteInput{EventID: event.EventID, Confirmation: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutEvent.Events) != 0 {
		t.Fatalf("hard-deleted event remained visible: %#v", withoutEvent.Events)
	}
	if _, err := app.DeleteMedication(MedicationDeleteInput{MedicationID: medicationID, Confirmation: "DELETE"}); err != nil {
		t.Fatal(err)
	}
	final, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	if !final.Empty || len(final.Medications) != 0 {
		t.Fatalf("hard-deleted medication remained visible: %#v", final)
	}
}

func TestMedicationLoggingRemainsUsableWithoutAnEstimate(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }
	created, err := app.AddMedication(MedicationInput{Label: "Private no-estimate record"})
	if err != nil {
		t.Fatal(err)
	}
	logged, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: created.Medications[0].MedicationID,
		DoseLocal:    "2026-07-22T08:00",
		ZoneID:       "UTC",
		Status:       storage.MedicationEventTaken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if logged.EstimateStatus != "empty" || logged.Events[0].WakeRelation != "No prior recorded wake" || logged.Events[0].SleepRelationKind != "unavailable" {
		t.Fatalf("no-estimate medications = %#v", logged)
	}
	if !strings.Contains(logged.InteractionDisclaimer, "does not check medication interactions") {
		t.Fatalf("interaction disclaimer = %q", logged.InteractionDisclaimer)
	}
}

func TestMedicationLoggingRejectsFutureAndAdviceLikeInputs(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }
	created, err := app.AddMedication(MedicationInput{Label: "Private input validation"})
	if err != nil {
		t.Fatal(err)
	}
	medicationID := created.Medications[0].MedicationID
	if _, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medicationID,
		DoseLocal:    "2026-07-23T12:00",
		ZoneID:       "UTC",
		Status:       storage.MedicationEventTaken,
	}); err == nil {
		t.Fatal("future medication evidence was accepted")
	}
	if _, err := app.LogMedicationEvent(MedicationEventInput{
		MedicationID: medicationID,
		DoseLocal:    "2026-07-22T11:00",
		ZoneID:       "UTC",
		Status:       "recommended",
	}); err == nil {
		t.Fatal("non-evidence medication status was accepted")
	}
}
