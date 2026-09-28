package main

import (
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
)

func TestTheDoseHistoryIsReadByPageNewestFirst(t *testing.T) {
	app := newTestApp(t)
	fixedNow := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	app.nowFn = func() time.Time { return fixedNow }
	empty, err := app.GetMedicationHistoryPage(MedicationHistoryPageInput{})
	if err != nil || empty.Status != "empty" || empty.Total != 0 || len(empty.Events) != 0 {
		t.Fatalf("no doses = %+v, %v", empty, err)
	}

	morning, err := app.AddMedication(MedicationInput{Label: "Synthetic morning tablet"})
	if err != nil {
		t.Fatal(err)
	}
	evening, err := app.AddMedication(MedicationInput{Label: "Synthetic evening tablet"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, medication := range evening.Medications {
		ids[medication.Label] = medication.MedicationID
	}
	ids["Synthetic morning tablet"] = morning.Medications[0].MedicationID
	// 55 doses an hour apart, alternating, the newest an hour ago.
	for hour := 55; hour >= 1; hour-- {
		label := "Synthetic morning tablet"
		if hour%2 == 0 {
			label = "Synthetic evening tablet"
		}
		if _, err := app.LogMedicationEvent(MedicationEventInput{
			MedicationID: ids[label], ZoneID: "UTC", Status: storage.MedicationEventTaken,
			DoseLocal: fixedNow.Add(-time.Duration(hour) * time.Hour).Format("2006-01-02T15:04"),
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := app.GetMedicationHistoryPage(MedicationHistoryPageInput{})
	if err != nil || first.Total != 55 || len(first.Events) != medicationHistoryPageSize || first.Page != 0 ||
		first.Message != "55 recorded doses." {
		t.Fatalf("first page = %d of %d, page %d, %q, %v", len(first.Events), first.Total, first.Page, first.Message, err)
	}
	if first.Events[0].DoseAt != fixedNow.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("the newest dose is not first: %s", first.Events[0].DoseAt)
	}
	second, err := app.GetMedicationHistoryPage(MedicationHistoryPageInput{Page: 1})
	if err != nil || len(second.Events) != 5 || second.Events[4].DoseAt != fixedNow.Add(-55*time.Hour).Format(time.RFC3339) {
		t.Fatalf("second page = %+v, %v", second.Events, err)
	}
	// The pages meet: the first page's last dose is an hour after the second's first.
	lastOnFirst, _ := time.Parse(time.RFC3339, first.Events[medicationHistoryPageSize-1].DoseAt)
	firstOnSecond, _ := time.Parse(time.RFC3339, second.Events[0].DoseAt)
	if lastOnFirst.Sub(firstOnSecond) != time.Hour {
		t.Fatalf("pages run to %s then from %s", lastOnFirst, firstOnSecond)
	}
	if beyond, err := app.GetMedicationHistoryPage(MedicationHistoryPageInput{Page: 9}); err != nil || beyond.Page != 1 || len(beyond.Events) != 5 {
		t.Fatalf("past the end = page %d with %d doses, %v", beyond.Page, len(beyond.Events), err)
	}

	// Each medication carries its own newest dose, for the quick taps.
	medications, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	for _, medication := range medications.Medications {
		want := fixedNow.Add(-2 * time.Hour) // evening: even hours
		if medication.Label == "Synthetic morning tablet" {
			want = fixedNow.Add(-time.Hour)
		}
		if medication.LastDose == nil || medication.LastDose.DoseAt != want.Format(time.RFC3339) ||
			medication.LastDose.MedicationID != medication.MedicationID {
			t.Errorf("%s last dose = %+v, want %s", medication.Label, medication.LastDose, want)
		}
	}
}

// Moved from the window with the choosing: doses recorded on two devices in
// different zones, where the later dose has the earlier civil clock. The
// newest is the later instant, not the later clock reading.
func TestTheLastDoseIsTheLatestInstant(t *testing.T) {
	app := newTestApp(t)
	app.nowFn = func() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) }
	created, err := app.AddMedication(MedicationInput{Label: "Synthetic tablet"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Medications[0].MedicationID
	for _, dose := range []MedicationEventInput{
		{MedicationID: id, DoseLocal: "2026-09-27T04:41", ZoneID: "UTC", Status: storage.MedicationEventTaken},
		{MedicationID: id, DoseLocal: "2026-09-27T00:46", ZoneID: "America/New_York", Status: storage.MedicationEventSkipped},
	} {
		if _, err := app.LogMedicationEvent(dose); err != nil {
			t.Fatal(err)
		}
	}
	medications, err := app.GetMedications()
	if err != nil {
		t.Fatal(err)
	}
	if last := medications.Medications[0].LastDose; last == nil || last.Status != storage.MedicationEventSkipped ||
		last.DoseAt != "2026-09-27T04:46:00Z" {
		t.Fatalf("last dose = %+v", last)
	}
}
