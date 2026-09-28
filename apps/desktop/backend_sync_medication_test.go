package main

import (
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

// Two computers enrolled on one stateful peer exchange medication definitions,
// doses, dose corrections and context markers through the real sync client and
// stores, converge when both edit a definition while apart, and lose a deleted
// record everywhere (ADR-0048).
func TestMedicationRecordsTravelBetweenComputers(t *testing.T) {
	peer := newRecoveryPeer(t)
	clock := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	tick := func() { clock = clock.Add(time.Minute) }
	a, b := newTestApp(t), newTestApp(t)
	for _, app := range []*App{a, b} {
		app.nowFn = func() time.Time { return clock }
		enrollRecoveryPeer(t, app, peer)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	created, err := a.AddMedication(MedicationInput{Label: "Synthetic evening tablet", Form: "tablet"})
	must(err)
	medicationID := created.Medications[0].MedicationID
	tick()
	_, err = a.LogMedicationEvent(MedicationEventInput{MedicationID: medicationID, DoseLocal: "2026-09-19T21:00", ZoneID: "UTC", Status: storage.MedicationEventTaken, Scheduled: true})
	must(err)
	_, err = a.AddRhythmMarker(RhythmMarkerInput{Kind: storage.RhythmMarkerTravel, StartLocal: "2026-09-18T08:00", EndLocal: "2026-09-19T20:00", ZoneID: "UTC", Note: "Synthetic trip"})
	must(err)
	if status := syncRecoveryPeer(t, a); status.PushedCount != 3 || status.PendingPushCount != 0 {
		t.Fatalf("first computer upload = %+v", status)
	}
	if status := syncRecoveryPeer(t, b); status.PulledCount != 3 || status.PendingPushCount != 0 {
		t.Fatalf("second computer download = %+v", status)
	}
	onB, err := b.GetMedications()
	must(err)
	markersOnB, err := b.GetRhythmMarkers()
	must(err)
	if len(onB.Medications) != 1 || onB.Medications[0].Label != "Synthetic evening tablet" || len(doseHistory(t, b)) != 1 ||
		len(markersOnB.Markers) != 1 || markersOnB.Markers[0].Note != "Synthetic trip" {
		t.Fatalf("second computer holds %+v and %+v", onB, markersOnB)
	}

	// A correction made on one computer changes the other's clinician report.
	tick()
	dose := doseHistory(t, b)[0]
	_, err = b.CorrectMedicationEvent(MedicationEventCorrectionInput{EventID: dose.EventID, DoseLocal: dose.DoseLocal, ZoneID: dose.ZoneID,
		Status: storage.MedicationEventSkipped, Scheduled: dose.Scheduled, Note: dose.Note})
	must(err)
	syncRecoveryPeer(t, b)
	syncRecoveryPeer(t, a)
	report, err := a.GetMedicationClinicianReport(MedicationClinicalReportInput{RangeMode: "custom", FromDate: "2026-09-13", ToDate: "2026-09-20",
		ZoneID: "UTC", DayStartHour: 18, IncludeMedication: true})
	must(err)
	if report.Summary.MedicationEvents != 1 || report.Summary.RecordedSkipped != 1 || report.Summary.RecordedTaken != 0 {
		t.Fatalf("report after a remote correction = %+v", report.Summary)
	}

	// Both edit the definition while apart: one renames it, the other gives
	// it a schedule. Both edits survive on both computers.
	tick()
	_, err = a.UpdateMedication(MedicationUpdateInput{MedicationID: medicationID, Revision: 1, Label: "Synthetic evening tablet, renamed", Form: "tablet", Active: true})
	must(err)
	_, err = b.UpdateMedicationSchedule(MedicationScheduleInput{MedicationID: medicationID, Revision: 1, Kind: storage.MedicationScheduleFixedClock,
		ZoneID: "UTC", CivilTimes: []string{"21:00"}})
	must(err)
	syncRecoveryPeer(t, a)
	syncRecoveryPeer(t, b)
	syncRecoveryPeer(t, a)
	for name, app := range map[string]*App{"first": a, "second": b} {
		medications, err := app.GetMedications()
		must(err)
		got := medications.Medications[0]
		if got.Label != "Synthetic evening tablet, renamed" || got.Schedule == nil || got.Schedule.Kind != storage.MedicationScheduleFixedClock || got.Revision != 3 {
			t.Fatalf("%s computer did not converge: %+v", name, got)
		}
	}

	// Deleting on one computer erases on the other.
	_, err = a.DeleteMedication(MedicationDeleteInput{MedicationID: medicationID, Confirmation: "DELETE"})
	must(err)
	_, err = b.DeleteRhythmMarker(RhythmMarkerDeleteInput{MarkerID: markersOnB.Markers[0].MarkerID, Confirmation: "DELETE"})
	must(err)
	syncRecoveryPeer(t, a)
	syncRecoveryPeer(t, b)
	statusA := syncRecoveryPeer(t, a)
	onA, err := a.GetMedications()
	must(err)
	onB, err = b.GetMedications()
	must(err)
	markersOnA, err := a.GetRhythmMarkers()
	must(err)
	if len(onA.Medications) != 0 || len(onB.Medications) != 0 || len(doseHistory(t, b)) != 0 || len(markersOnA.Markers) != 0 {
		t.Fatalf("deletions did not travel: %+v / %+v / %+v", onA, onB, markersOnA)
	}
	if statusA.PendingPushCount != 0 || statusA.PendingErasureCount != 0 || statusA.WaitingRecordCount != 0 {
		t.Fatalf("work left over: %+v", statusA)
	}
}
