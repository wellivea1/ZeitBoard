package main

import (
	"context"
	"testing"
	"time"

	storage "non24.app/core/storage/sqlite"
	"non24.app/desktop/platform/autostart"
)

func setupSteps(t *testing.T, app *App) map[string]bool {
	t.Helper()
	guide, err := app.GetSetupGuide()
	if err != nil {
		t.Fatal(err)
	}
	done := make(map[string]bool, len(guide.Steps))
	for _, step := range guide.Steps {
		done[step.ID] = step.Done
	}
	return done
}

// Each step is done by doing it, and by nothing else: the working week a new
// install starts with is a placeholder, and an empty profile has no phone, no
// calendar and no login entry.
func TestEachSetupStepIsDoneByDoingIt(t *testing.T) {
	app := newTestApp(t)
	startup := &fakeAutostart{value: autostart.Registration{Available: true}}
	app.startupControl = startup

	if steps := setupSteps(t, app); len(steps) != 4 || steps[setupStepPhone] || steps[setupStepReaching] ||
		steps[setupStepCalendar] || steps[setupStepStartup] {
		t.Fatalf("a new profile has steps done: %v", steps)
	}

	// A night from the phone.
	nightEnd := time.Now().UTC().Truncate(time.Minute).Add(-6 * time.Hour)
	if err := app.store.AppendSleepObservation(context.Background(), storage.SleepObservationRecord{
		ObservationID: "hc-1b2c3d4e5f60718293a4b5c6",
		Kind:          storage.SleepKindEpisode,
		StartAt:       nightEnd.Add(-8 * time.Hour),
		EndAt:         nightEnd,
		ZoneID:        defaultZoneID,
		Sleep:         storage.SleepObservationDetails{Classification: storage.SleepClassificationPrincipal},
		Provenance: storage.SleepObservationProvenance{
			AcquisitionMethod: storage.ProvenanceAcquisitionHealthConnect,
			EvidenceStatus:    storage.ProvenanceEvidenceDirectlyObserved,
			RecordedAt:        nightEnd,
			SourceRecordID:    "synthetic-provider-record",
		},
	}); err != nil {
		t.Fatal(err)
	}
	// The people to reach, saved as they are.
	saveReaching(t, app, app.currentReachingHours().Contacts...)
	// A calendar.
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ZeitBoard Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:setup-guide@synthetic.example\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:" + start.Format("20060102T150405Z") + "\r\n" +
		"DTEND:" + start.Add(time.Hour).Format("20060102T150405Z") + "\r\n" +
		"SUMMARY:Synthetic appointment\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := app.ImportCalendarFile(CalendarFileInput{FileName: "synthetic.ics", Contents: ics, ZoneID: defaultZoneID}); err != nil {
		t.Fatal(err)
	}
	// Starting with Windows.
	if _, err := app.SetStartupSettings(StartupSettingsInput{Enabled: true, StartHidden: true}); err != nil {
		t.Fatal(err)
	}

	for step, done := range setupSteps(t, app) {
		if !done {
			t.Errorf("%s is not done after doing it", step)
		}
	}
}

// A login entry that points at another installation does not start this one.
func TestALoginEntryForAnotherCopyIsNotDone(t *testing.T) {
	app := newTestApp(t)
	app.startupControl = &fakeAutostart{value: autostart.Registration{Available: true, Registered: true}}
	if setupSteps(t, app)[setupStepStartup] {
		t.Error("an entry for another executable counts as starting with Windows")
	}
}

// Away from Windows there is no login entry to make, so the step is not
// offered rather than left undone for good.
func TestAStepThisComputerCannotTakeIsLeftOut(t *testing.T) {
	app := newTestApp(t)
	app.startupControl = &fakeAutostart{value: autostart.Registration{Available: false}}
	steps := setupSteps(t, app)
	if _, offered := steps[setupStepStartup]; offered || len(steps) != 3 {
		t.Errorf("steps %v", steps)
	}
}
